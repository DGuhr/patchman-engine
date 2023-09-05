package middlewares

import (
	"app/base"
	"app/base/api"
	"app/base/rbac"
	"app/base/utils"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

var (
	rbacURL      = ""
	debugRequest = os.Getenv("LOG_LEVEL") == "trace"
	httpClient   = &http.Client{}
)

const xRHIdentity = "x-rh-identity"
const KeyInventoryGroups = "inventoryGroups"

var allPerms = "patch:*:*"
var readPerms = map[string]bool{allPerms: true, "patch:*:read": true}
var writePerms = map[string]bool{allPerms: true, "patch:*:write": true}
var inventoryReadPerms = map[string]bool{
	"inventory:*:*":        true,
	"inventory:*:read":     true,
	"inventory:hosts:*":    true,
	"inventory:hosts:read": true,
}

// handlerName to permissions mapping
var granularPerms = map[string]struct {
	Permission string
	Read       bool
	Write      bool
}{
	"CreateBaselineHandler":        {"patch:template:write", false, true},
	"BaselineUpdateHandler":        {"patch:template:write", false, true},
	"BaselineDeleteHandler":        {"patch:template:write", false, true},
	"BaselineSystemsRemoveHandler": {"patch:template:write", false, true},
	"SystemDeleteHandler":          {"patch:system:write", false, true},
}

// Make RBAC client on demand, with specified identity
func makeClient(identity string) *api.Client {
	client := api.Client{
		HTTPClient:     httpClient,
		Debug:          debugRequest,
		DefaultHeaders: map[string]string{xRHIdentity: identity},
	}
	if rbacURL == "" {
		rbacURL = utils.FailIfEmpty(utils.Cfg.RbacAddress, "RBAC_ADDRESS") + base.RBACApiPrefix +
			"/access/?application=patch,inventory" //what permissions on patch AND inventory does that user have?
	}
	return &client
}

// evaluate whether a user has the necessary permissions to perform a specific action based on the obtained RBAC permissions
func checkPermissions(access *rbac.AccessPagination, handlerName, method string) bool {
	grantedPatch := false
	grantedInventory := false

	//loop through obtained permissions
	for _, a := range access.Data {

		//when both are true, return true directly in subsequent loop calls.
		if grantedPatch && grantedInventory {
			return true
		}

		//check if the obtained permission is contained in the permanent Inventory permissions
		//set grantedInventory to true if it is
		if !grantedInventory {
			grantedInventory = inventoryReadPerms[a.Permission]
		}

		if !grantedPatch {
			// if the called handler is one of the ones where granular permissions apply
			if p, has := granularPerms[handlerName]; has {
				// check against the underlying coarse-grained permission in the struct from granularperms
				if a.Permission == p.Permission {
					// the required coarse-grained permission is present, e.g. patch:template:write
					grantedPatch = true
					continue
				}

				// if the granular permission does not include write permissions in the struct
				//Note: looking at granularPerms this should never be true unless I oversee sth *shrugs*
				// and the evaluated permission is either patch:*:* or patch:*:read set true
				if p.Read && !p.Write && readPerms[a.Permission] {
					// required permission is read permission
					// check whether we have either patch:*:read or patch:*:*
					grantedPatch = true
					continue
				}

				//if a granular permission exists and the evaluated permission is a write permission
				//set true.
				// write perms: "inventory:*:*", "inventory:*:read,"inventory:hosts:*", "inventory:hosts:read"
				if p.Write && !p.Read && writePerms[a.Permission] {
					// required permission is write permission
					// check whether we have either patch:*:write or patch:*:*
					grantedPatch = true
					continue
				}
				// we need both read and write permissions - patch:*:*
				grantedPatch = (a.Permission == allPerms)
			} else {
				// not granular
				// require read permissions for GET and POST //Note: Why post? but should be fine.
				// require write permissions for PUT and DELETE
				switch method {
				case "GET", "POST":
					grantedPatch = readPerms[a.Permission]
				case "PUT", "DELETE":
					grantedPatch = writePerms[a.Permission]
				}
			}
		}
	}
	return grantedPatch && grantedInventory
}

func isAccessGranted(c *gin.Context) bool {
	client := makeClient(c.GetHeader("x-rh-identity"))
	access := rbac.AccessPagination{}
	res, err := client.Request(&base.Context, http.MethodGet, rbacURL, nil, &access)
	if res != nil && res.Body != nil {
		defer res.Body.Close()
	}

	if err != nil {
		utils.LogError("err", err.Error(), "Call to RBAC svc failed")
		status := http.StatusInternalServerError
		if res != nil {
			status = res.StatusCode
		}
		serviceErrorCnt.WithLabelValues("rbac", strconv.Itoa(status)).Inc()
		return false
	}
	nameSplitted := strings.Split(c.HandlerName(), ".")
	handlerName := nameSplitted[len(nameSplitted)-1]

	//check permissions, PDP
	granted := checkPermissions(&access, handlerName, c.Request.Method)

	//when permission is generally granted, find the inventoryGroups that these permissions are granted for
	//a.k.a. the "registered inventory for the account of this person"
	// and set them into the web frameworks context.
	if granted {
		c.Set(KeyInventoryGroups, findInventoryGroups(&access))
	}
	return granted
}

func findInventoryGroups(access *rbac.AccessPagination) map[string]string {
	res := make(map[string]string)

	if len(access.Data) == 0 {
		return res
	}

	groups := []string{}
	for _, a := range access.Data {
		//re-check readpermission
		if !inventoryReadPerms[a.Permission] {
			continue
		}

		if len(a.ResourceDefinitions) == 0 {
			// access to all groups
			return nil
		}

		for _, rd := range a.ResourceDefinitions {
			if rd.AttributeFilter.Key != "group.id" {
				continue
			}
			for _, v := range rd.AttributeFilter.Value {
				if v == nil {
					res[rbac.KeyUngrouped] = "[]"
					continue
				}
				group, err := utils.ParseInventoryGroup(v, nil)
				if err != nil {
					// couldn't marshal inventory group to json
					continue
				}
				groups = append(groups, group)
			}
		}
	}

	if len(groups) > 0 {
		res[rbac.KeyGrouped] = fmt.Sprintf("{%s}", strings.Join(groups, ","))
	}
	return res
}

func RBAC() gin.HandlerFunc {
	enableRBACCHeck := utils.GetBoolEnvOrDefault("ENABLE_RBAC", true)
	if !enableRBACCHeck {
		return func(c *gin.Context) {}
	}

	return func(c *gin.Context) {
		if isAccessGranted(c) {
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized,
			utils.ErrorResponse{Error: "You don't have access to this application"})
	}
}
