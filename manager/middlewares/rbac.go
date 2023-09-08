package middlewares

import (
	"app/base/utils"
	"github.com/gin-gonic/gin"
)

func RBAC() gin.HandlerFunc {
	enableRBACCHeck := utils.GetBoolEnvOrDefault("ENABLE_RBAC", true)
	if !enableRBACCHeck {
		return func(c *gin.Context) {}
	}

	return func(c *gin.Context) {
		return // Unclear if we would still want to do anything here in the future with authz devolved to the handlers.
	}
}
