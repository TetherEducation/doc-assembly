package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

const (
	AutomationKeyHeader = "X-Automation-Key" //nolint:gosec // This is a header name, not a credential

	automationKeyIDCtxKey          = "automationKeyID"
	automationKeyPrefixCtxKey      = "automationKeyPrefix"
	automationAllowedTenantsCtxKey = "automationAllowedTenants"
)

// AutomationKeyAuth validates the X-Automation-Key header against the database.
// On success, injects automationKeyID, automationKeyPrefix, and automationAllowedTenants into the Gin context.
func AutomationKeyAuth(keyRepo port.AutomationAPIKeyRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		rawKey := c.GetHeader(AutomationKeyHeader)
		if rawKey == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing X-Automation-Key header"})
			return
		}

		keyHash := hashAPIKey(rawKey)

		// Look up in database
		key, err := keyRepo.FindByHash(c.Request.Context(), keyHash)
		if err != nil || key == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or revoked API key"})
			return
		}

		// Defence-in-depth: also check active status explicitly
		if !key.IsActive || key.IsRevoked() {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or revoked API key"})
			return
		}

		// Ensure this is an automation key (not an internal key)
		if key.KeyType != entity.KeyTypeAutomation {
			abortWithError(c, http.StatusUnauthorized, entity.ErrInvalidAPIKey)
			return
		}

		// Inject into context
		c.Set(automationKeyIDCtxKey, key.ID)
		c.Set(automationKeyPrefixCtxKey, key.KeyPrefix)
		c.Set(automationAllowedTenantsCtxKey, key.AllowedTenants)

		c.Next()
	}
}

// GetAutomationKeyID returns the automation API key ID from the Gin context.
func GetAutomationKeyID(c *gin.Context) (string, bool) {
	v, ok := c.Get(automationKeyIDCtxKey)
	if !ok {
		return "", false
	}
	id, ok := v.(string)
	return id, ok
}

// GetAutomationKeyPrefix returns the automation API key prefix from the Gin context.
func GetAutomationKeyPrefix(c *gin.Context) (string, bool) {
	v, ok := c.Get(automationKeyPrefixCtxKey)
	if !ok {
		return "", false
	}
	prefix, ok := v.(string)
	return prefix, ok
}

// GetAutomationAllowedTenants returns the allowed tenants slice from the Gin context.
// Returns nil if the key has global access (all tenants allowed).
func GetAutomationAllowedTenants(c *gin.Context) []string {
	v, ok := c.Get(automationAllowedTenantsCtxKey)
	if !ok {
		return nil
	}
	tenants, _ := v.([]string)
	return tenants
}
