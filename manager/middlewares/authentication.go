package middlewares

import (
	"app/base/database"
	"app/base/models"
	"app/base/utils"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/pkg/errors"
	"github.com/redhatinsights/identity"

	"github.com/gin-gonic/gin"
)

const KeyAccount = "account"
const UIReferer = "console.redhat.com"
const APISource = "API"
const UISource = "UI"
const KeyUser = "user"

var AccountIDCache = struct {
	Values map[string]int
	Lock   sync.Mutex
}{Values: map[string]int{}, Lock: sync.Mutex{}}

type ginContext gin.Context

// Stores or updates the account data, returning the account id
func GetOrCreateAccount(orgID string) (int, error) {
	rhAccount := models.RhAccount{
		OrgID: &orgID,
	}
	if rhAccount.OrgID == nil || *rhAccount.OrgID == "" {
		// missing OrgID in msg from Inventory
		return 0, errors.New("missing org_id")
	}

	// Find account by OrgID
	err := database.Db.Where("org_id = ?", *rhAccount.OrgID).Find(&rhAccount).Error
	if err != nil {
		utils.LogWarn("err", err, "org_id", *rhAccount.OrgID, "Error in finding account")
	}
	if rhAccount.ID != 0 {
		return rhAccount.ID, nil
	}

	// create new rhAccount with OrgID
	err = database.OnConflictUpdate(database.Db, "org_id", "org_id").Select("org_id").Create(&rhAccount).Error
	if err != nil {
		utils.LogWarn("err", err, "org_id", *rhAccount.OrgID, "Error creating account")
	}
	return rhAccount.ID, err
}

func findAccount(c *gin.Context, orgID string) bool {
	AccountIDCache.Lock.Lock()
	defer AccountIDCache.Lock.Unlock()

	if id, has := AccountIDCache.Values[orgID]; has {
		c.Set(KeyAccount, id)
	} else {
		// create new account if it does not exist
		accID, err := GetOrCreateAccount(orgID)
		if err != nil {
			return false
		}
		AccountIDCache.Values[orgID] = accID
		c.Set(KeyAccount, accID)
	}
	return true
}

func PublicAuthenticator() gin.HandlerFunc {
	return func(c *gin.Context) {
		xrhid := (*ginContext)(c).GetXRHID()
		if xrhid == nil {
			// aborted by GetXRHID
			return
		}
		if findAccount(c, xrhid.Identity.OrgID) {
			c.Set(KeyUser, fmt.Sprintf("%s %s", xrhid.Identity.User.FirstName, xrhid.Identity.User.LastName))
			c.Next()
		}
	}
}

// Check referer type and identify caller source
func CheckReferer() gin.HandlerFunc {
	return func(c *gin.Context) {
		ref := c.GetHeader("Referer")
		account := strconv.Itoa(c.GetInt(KeyAccount))

		if strings.Contains(ref, UIReferer) {
			callerSourceCnt.WithLabelValues(UISource, account).Inc()
		} else {
			callerSourceCnt.WithLabelValues(APISource, account).Inc()
		}
	}
}

func TurnpikeAuthenticator() gin.HandlerFunc {
	return func(c *gin.Context) {
		xrhid := (*ginContext)(c).GetXRHID()
		if xrhid == nil {
			// aborted by GetXRHID
			return
		}
		// Turnpike endpoints only support associate
		if strings.ToLower(xrhid.Identity.Type) != "associate" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, utils.ErrorResponse{Error: "Invalid x-rh-identity header"})
			return
		}
	}
}

func MockAuthenticator(account int) gin.HandlerFunc {
	return func(c *gin.Context) {
		utils.LogWarn("account_id", account, "using mocking account id")
		c.Set(KeyAccount, account)
		c.Next()
	}
}

// Get identity header from gin.Context
func (c *ginContext) GetXRHID() *identity.XRHID {
	identStr := (*gin.Context)(c).GetHeader("x-rh-identity")
	if identStr == "" {
		(*gin.Context)(c).AbortWithStatusJSON(
			http.StatusUnauthorized, utils.ErrorResponse{Error: "Missing x-rh-identity header"})
		return nil
	}
	utils.LogTrace("ident", identStr, "Identity retrieved")

	xrhid, err := utils.ParseXRHID(identStr)
	if err != nil {
		(*gin.Context)(c).AbortWithStatusJSON(
			http.StatusUnauthorized, utils.ErrorResponse{Error: "Invalid x-rh-identity header"})
		return nil
	}
	return xrhid
}
