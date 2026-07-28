package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// AuthorizeRole creates a middleware that checks if the user has at least the required role.
// This middleware must be applied after WorkspaceContext.
func AuthorizeRole(requiredRole entity.WorkspaceRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip for OPTIONS requests
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		// Get user's role from context
		userRole, ok := GetWorkspaceRole(c)
		if !ok {
			slog.WarnContext(c.Request.Context(), "authorization failed: no workspace role in context",
				slog.String("operation_id", GetOperationID(c)),
			)
			abortWithError(c, http.StatusForbidden, entity.ErrMissingWorkspaceID)
			return
		}

		// Check if user has sufficient permissions
		if !userRole.HasPermission(requiredRole) {
			slog.WarnContext(c.Request.Context(), "authorization failed: insufficient permissions",
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

// RequireOwner is a convenience middleware that requires OWNER role.
func RequireOwner() gin.HandlerFunc {
	return AuthorizeRole(entity.WorkspaceRoleOwner)
}

// RequireAdmin is a convenience middleware that requires at least ADMIN role.
func RequireAdmin() gin.HandlerFunc {
	return AuthorizeRole(entity.WorkspaceRoleAdmin)
}

// RequireEditor is a convenience middleware that requires at least EDITOR role.
func RequireEditor() gin.HandlerFunc {
	return AuthorizeRole(entity.WorkspaceRoleEditor)
}

// RequireOperator is a convenience middleware that requires at least OPERATOR role.
func RequireOperator() gin.HandlerFunc {
	return AuthorizeRole(entity.WorkspaceRoleOperator)
}

// RequireViewer is a convenience middleware that requires at least VIEWER role.
func RequireViewer() gin.HandlerFunc {
	return AuthorizeRole(entity.WorkspaceRoleViewer)
}

// RequireWorkspaceAccess creates a middleware that ensures the user has access to the workspace.
// This is a simpler check than AuthorizeRole - it just verifies the user is a member.
func RequireWorkspaceAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip for OPTIONS requests
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		// Check if workspace ID is in context (set by WorkspaceContext)
		if _, ok := GetWorkspaceID(c); !ok {
			abortWithError(c, http.StatusBadRequest, entity.ErrMissingWorkspaceID)
			return
		}

		// If we have a workspace ID, WorkspaceContext already validated access
		c.Next()
	}
}
