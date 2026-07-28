package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

const (
	// DummyUserID is the fixed external identity ID for dummy auth mode.
	DummyUserID = "00000000-0000-0000-0000-000000000001"
	// DummyUserEmail is the fixed email for dummy auth mode.
	DummyUserEmail = "admin@docengine.local"
	// DummyUserName is the fixed name for dummy auth mode.
	DummyUserName = "Doc Engine Admin"
)

// DummyAuth creates a middleware that bypasses JWT validation and injects
// a fixed superadmin identity. Used when no auth config is provided (dev mode).
func DummyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		// Set JWT-level claims (normally set by JWTAuth)
		c.Set(userIDKey, DummyUserID)
		c.Set(userEmailKey, DummyUserEmail)
		c.Set(userNameKey, DummyUserName)

		c.Next()
	}
}

// DummyIdentityAndRoles creates a middleware that sets the internal user ID
// and grants SUPERADMIN system role. Used in dummy auth mode to bypass
// IdentityContext and SystemRoleContext middlewares.
func DummyIdentityAndRoles(internalUserID string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		c.Set(internalUserIDKey, internalUserID)
		c.Set(systemRoleKey, entity.SystemRoleSuperAdmin)

		c.Next()
	}
}
