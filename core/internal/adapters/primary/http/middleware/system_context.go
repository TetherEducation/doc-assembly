package middleware

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

const (
	// systemRoleKey is the context key for the user's system role.
	systemRoleKey = "system_role"
)

// SystemRoleContext creates a middleware that loads the user's system role if they have one.
// This middleware is optional - it does not fail if the user has no system role.
// It should be applied after IdentityContext.
func SystemRoleContext(systemRoleRepo port.SystemRoleRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip for OPTIONS requests
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		// Get internal user ID
		internalUserID, ok := GetInternalUserID(c)
		if !ok {
			// No internal user ID, continue without system role
			c.Next()
			return
		}

		// Try to load system role (may not exist for most users)
		assignment, err := systemRoleRepo.FindByUserID(c.Request.Context(), internalUserID)
		if err == nil {
			c.Set(systemRoleKey, assignment.Role)
			slog.DebugContext(c.Request.Context(), "loaded system role",
				slog.String("user_id", internalUserID),
				slog.String("role", string(assignment.Role)),
				slog.String("operation_id", GetOperationID(c)),
			)
		} else if !errors.Is(err, entity.ErrSystemRoleNotFound) {
			slog.WarnContext(c.Request.Context(), "failed to load system role",
				slog.String("user_id", internalUserID),
				slog.String("error", err.Error()),
				slog.String("operation_id", GetOperationID(c)),
			)
		}
		// If no system role found, continue anyway

		c.Next()
	}
}

// GetSystemRole retrieves the user's system role from the Gin context.
func GetSystemRole(c *gin.Context) (entity.SystemRole, bool) {
	if val, exists := c.Get(systemRoleKey); exists {
		if role, ok := val.(entity.SystemRole); ok {
			return role, true
		}
	}
	return "", false
}

// HasSystemRole checks if the user has any system role.
func HasSystemRole(c *gin.Context) bool {
	_, ok := GetSystemRole(c)
	return ok
}

// AuthorizeSystemRole creates a middleware that checks if the user has at least the required system role.
// This middleware must be applied after SystemRoleContext.
func AuthorizeSystemRole(requiredRole entity.SystemRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip for OPTIONS requests
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		// Get user's system role from context
		userRole, ok := GetSystemRole(c)
		if !ok {
			slog.WarnContext(c.Request.Context(), "authorization failed: no system role",
				slog.String("required_role", string(requiredRole)),
				slog.String("operation_id", GetOperationID(c)),
			)
			abortWithError(c, http.StatusForbidden, entity.ErrInsufficientRole)
			return
		}

		// Check if user has sufficient permissions
		if !userRole.HasPermission(requiredRole) {
			slog.WarnContext(c.Request.Context(), "authorization failed: insufficient system permissions",
				slog.String("user_role", string(userRole)),
				slog.String("required_role", string(requiredRole)),
				slog.String("operation_id", GetOperationID(c)),
			)
			abortWithError(c, http.StatusForbidden, entity.ErrInsufficientRole)
			return
		}

		c.Next()
	}
}

// RequireSuperAdmin is a convenience middleware that requires SUPERADMIN role.
func RequireSuperAdmin() gin.HandlerFunc {
	return AuthorizeSystemRole(entity.SystemRoleSuperAdmin)
}

// RequirePlatformAdmin is a convenience middleware that requires at least PLATFORM_ADMIN role.
func RequirePlatformAdmin() gin.HandlerFunc {
	return AuthorizeSystemRole(entity.SystemRolePlatformAdmin)
}
