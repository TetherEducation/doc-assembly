package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

const (
	// TenantIDHeader is the header name for the tenant ID.
	TenantIDHeader = "X-Tenant-ID"
	// tenantIDKey is the context key for the current tenant ID.
	tenantIDKey = "tenant_id"
	// tenantRoleKey is the context key for the user's role in the current tenant.
	tenantRoleKey = "tenant_role"
)

// TenantContext creates a middleware that requires and loads the user's role for a specific tenant.
// The tenant ID must come from the X-Tenant-ID header.
// This middleware should only be applied to routes that require tenant context.
// Users with system roles (SUPERADMIN) get automatic access as TENANT_OWNER.
func TenantContext(tenantMemberRepo port.TenantMemberRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip for OPTIONS requests
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		// Get tenant ID from header (required)
		tenantID := c.GetHeader(TenantIDHeader)
		if tenantID == "" {
			abortWithError(c, http.StatusBadRequest, entity.ErrMissingTenantID)
			return
		}

		// Validate UUID format
		if _, err := uuid.Parse(tenantID); err != nil {
			abortWithError(c, http.StatusBadRequest, entity.ErrInvalidTenantID)
			return
		}

		// Get internal user ID
		internalUserID, ok := GetInternalUserID(c)
		if !ok {
			abortWithError(c, http.StatusUnauthorized, entity.ErrUnauthorized)
			return
		}

		// Check if user has system role (SUPERADMIN can access any tenant)
		if sysRole, hasSysRole := GetSystemRole(c); hasSysRole {
			if sysRole.HasPermission(entity.SystemRoleSuperAdmin) {
				// SUPERADMIN gets full tenant access
				c.Set(tenantIDKey, tenantID)
				c.Set(tenantRoleKey, entity.TenantRoleOwner)
				slog.DebugContext(c.Request.Context(), "superadmin tenant access granted",
					slog.String("user_id", internalUserID),
					slog.String("tenant_id", tenantID),
					slog.String("operation_id", GetOperationID(c)),
				)
				c.Next()
				return
			}
		}

		// Load user's role in this tenant
		member, err := tenantMemberRepo.FindActiveByUserAndTenant(c.Request.Context(), internalUserID, tenantID)
		if err != nil {
			slog.WarnContext(c.Request.Context(), "tenant access denied",
				slog.String("error", err.Error()),
				slog.String("user_id", internalUserID),
				slog.String("tenant_id", tenantID),
				slog.String("operation_id", GetOperationID(c)),
			)
			abortWithError(c, http.StatusForbidden, entity.ErrTenantAccessDenied)
			return
		}

		// Store tenant context
		c.Set(tenantIDKey, tenantID)
		c.Set(tenantRoleKey, member.Role)

		c.Next()
	}
}

// GetTenantID retrieves the current tenant ID from the Gin context.
func GetTenantID(c *gin.Context) (string, bool) {
	if val, exists := c.Get(tenantIDKey); exists {
		if tenantID, ok := val.(string); ok && tenantID != "" {
			return tenantID, true
		}
	}
	return "", false
}

// GetTenantIDFromHeader retrieves the tenant ID directly from the X-Tenant-ID header.
// Use this when you need to check the header without requiring full TenantContext middleware.
// Returns false if the header is missing or contains an invalid UUID.
func GetTenantIDFromHeader(c *gin.Context) (string, bool) {
	tenantID := c.GetHeader(TenantIDHeader)
	if tenantID == "" {
		return "", false
	}
	if _, err := uuid.Parse(tenantID); err != nil {
		return "", false
	}
	return tenantID, true
}

// GetTenantRole retrieves the user's role in the current tenant.
func GetTenantRole(c *gin.Context) (entity.TenantRole, bool) {
	if val, exists := c.Get(tenantRoleKey); exists {
		if role, ok := val.(entity.TenantRole); ok {
			return role, true
		}
	}
	return "", false
}

// AuthorizeTenantRole creates a middleware that checks if the user has at least the required tenant role.
// This middleware must be applied after TenantContext.
func AuthorizeTenantRole(requiredRole entity.TenantRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip for OPTIONS requests
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		// Get user's tenant role from context
		userRole, ok := GetTenantRole(c)
		if !ok {
			slog.WarnContext(c.Request.Context(), "authorization failed: no tenant role in context",
				slog.String("operation_id", GetOperationID(c)),
			)
			abortWithError(c, http.StatusForbidden, entity.ErrMissingTenantID)
			return
		}

		// Check if user has sufficient permissions
		if !userRole.HasPermission(requiredRole) {
			slog.WarnContext(c.Request.Context(), "authorization failed: insufficient tenant permissions",
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

// RequireTenantOwner is a convenience middleware that requires TENANT_OWNER role.
func RequireTenantOwner() gin.HandlerFunc {
	return AuthorizeTenantRole(entity.TenantRoleOwner)
}

// RequireTenantAdmin is a convenience middleware that requires at least TENANT_ADMIN role.
func RequireTenantAdmin() gin.HandlerFunc {
	return AuthorizeTenantRole(entity.TenantRoleAdmin)
}

// RequireTenantAccess creates a middleware that ensures the user has access to the tenant.
// This is a simpler check - it just verifies the user is a member.
func RequireTenantAccess() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip for OPTIONS requests
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		// Check if tenant ID is in context (set by TenantContext)
		if _, ok := GetTenantID(c); !ok {
			abortWithError(c, http.StatusBadRequest, entity.ErrMissingTenantID)
			return
		}

		// If we have a tenant ID, TenantContext already validated access
		c.Next()
	}
}
