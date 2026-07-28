package middleware

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

const (
	// WorkspaceIDHeader is the header name for the workspace ID.
	WorkspaceIDHeader = "X-Workspace-ID"
	// internalUserIDKey is the context key for the internal user ID (from DB).
	internalUserIDKey = "internal_user_id"
	// workspaceIDKey is the context key for the current workspace ID.
	workspaceIDKey = "workspace_id"
	// workspaceRoleKey is the context key for the user's role in the current workspace.
	workspaceRoleKey = "workspace_role"
)

// IdentityContext creates a middleware that syncs the user from IdP and loads workspace context.
// It requires MultiOIDCAuth middleware to be applied before this.
// If bootstrapEnabled is true and no users exist in the database, the first user to login
// will be automatically created as SUPERADMIN.
func IdentityContext(pool *pgxpool.Pool, bootstrapEnabled bool, userRepo port.UserRepository, workspaceMemberRepo port.WorkspaceMemberRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		ctx := c.Request.Context()
		email, _ := GetUserEmail(c)

		user, err := userRepo.FindByEmail(ctx, email)
		if err == nil {
			if !user.IsLinkedToIdP() {
				activateInvitedUser(ctx, c, user, userRepo, workspaceMemberRepo)
			}
			c.Set(internalUserIDKey, user.ID)
			c.Next()
			return
		}

		if !errors.Is(err, entity.ErrUserNotFound) {
			abortWithError(c, http.StatusInternalServerError, err)
			return
		}

		// User not found - try bootstrap
		userID, ok := handleUserNotFound(c, pool, bootstrapEnabled)
		if !ok {
			return
		}

		c.Set(internalUserIDKey, userID)
		c.Next()
	}
}

// handleUserNotFound attempts bootstrap when user is not found in the database.
// Returns (userID, true) on success, ("", false) on failure (response already sent).
func handleUserNotFound(c *gin.Context, pool *pgxpool.Pool, bootstrapEnabled bool) (string, bool) {
	ctx := c.Request.Context()
	email, hasEmail := GetUserEmail(c)
	externalID, _ := GetUserID(c)
	fullName, _ := GetUserName(c)

	slog.DebugContext(ctx, "bootstrap attempt",
		slog.Bool("bootstrap_enabled", bootstrapEnabled),
		slog.Bool("has_email", hasEmail),
		slog.String("email", email),
		slog.String("external_id", externalID),
		slog.String("operation_id", GetOperationID(c)),
	)

	if !bootstrapEnabled {
		slog.WarnContext(ctx, "bootstrap disabled, rejecting user",
			slog.String("email", email),
			slog.String("operation_id", GetOperationID(c)),
		)
		abortWithError(c, http.StatusForbidden, entity.ErrUserNotFound)
		return "", false
	}

	if !hasEmail || email == "" {
		slog.ErrorContext(ctx, "bootstrap failed: JWT missing email claim",
			slog.String("external_id", externalID),
			slog.String("operation_id", GetOperationID(c)),
		)
		abortWithError(c, http.StatusUnauthorized, errors.New("missing email claim in token"))
		return "", false
	}

	userID, bootstrapped, err := tryBootstrapFirstUser(ctx, pool, email, fullName, externalID)
	if err != nil {
		slog.ErrorContext(ctx, "bootstrap failed",
			slog.String("error", err.Error()),
			slog.String("operation_id", GetOperationID(c)),
		)
		abortWithError(c, http.StatusInternalServerError, errors.New("bootstrap failed"))
		return "", false
	}

	if !bootstrapped {
		abortWithError(c, http.StatusForbidden, entity.ErrUserNotFound)
		return "", false
	}

	slog.WarnContext(ctx, "BOOTSTRAP: first user created as SUPERADMIN",
		slog.String("email", email),
		slog.String("user_id", userID),
		slog.String("event", "system_bootstrap"),
		slog.String("operation_id", GetOperationID(c)),
	)

	return userID, true
}

// tryBootstrapFirstUser atomically creates the first user as SUPERADMIN if the database has no users.
// Returns (userID, true, nil) if bootstrap succeeded, ("", false, nil) if users already exist.
func tryBootstrapFirstUser(ctx context.Context, pool *pgxpool.Pool, email, fullName, externalID string) (string, bool, error) {
	var userID string

	// Atomic insert: only succeeds if no users exist
	err := pool.QueryRow(ctx, `
		INSERT INTO identity.users (email, external_identity_id, full_name, status)
		SELECT $1, $2, $3, 'ACTIVE'
		WHERE NOT EXISTS (SELECT 1 FROM identity.users LIMIT 1)
		RETURNING id
	`, email, externalID, fullName).Scan(&userID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.InfoContext(ctx, "bootstrap skipped: users already exist in database")
			return "", false, nil
		}
		return "", false, fmt.Errorf("inserting bootstrap user: %w", err)
	}

	// Assign SUPERADMIN role
	_, err = pool.Exec(ctx, `
		INSERT INTO identity.system_roles (user_id, role)
		VALUES ($1, 'SUPERADMIN')
	`, userID)
	if err != nil {
		return "", false, fmt.Errorf("assigning superadmin role: %w", err)
	}

	return userID, true, nil
}

// activateInvitedUser transitions a shadow user to ACTIVE on first OIDC login.
// Links user to IdP, sets status to ACTIVE, and activates all pending workspace memberships.
// Errors are non-fatal: logged as warnings but do not block user access.
func activateInvitedUser(
	ctx context.Context,
	c *gin.Context,
	user *entity.User,
	userRepo port.UserRepository,
	workspaceMemberRepo port.WorkspaceMemberRepository,
) {
	externalID, hasID := GetUserID(c)
	if !hasID || externalID == "" {
		slog.WarnContext(ctx, "cannot activate invited user: missing external ID in token",
			slog.String("user_id", user.ID),
			slog.String("email", user.Email),
		)
		return
	}

	// Link user to IdP and set status to ACTIVE
	if err := userRepo.LinkToIdP(ctx, user.ID, externalID); err != nil {
		slog.WarnContext(ctx, "failed to link user to IdP",
			slog.String("user_id", user.ID),
			slog.String("error", err.Error()),
		)
		return
	}

	user.Activate(externalID)
	if err := userRepo.Update(ctx, user); err != nil {
		slog.WarnContext(ctx, "failed to update user status to ACTIVE",
			slog.String("user_id", user.ID),
			slog.String("error", err.Error()),
		)
		return
	}

	// Activate all pending workspace memberships
	memberships, err := workspaceMemberRepo.FindByUser(ctx, user.ID)
	if err != nil {
		slog.WarnContext(ctx, "failed to find workspace memberships for activation",
			slog.String("user_id", user.ID),
			slog.String("error", err.Error()),
		)
		return
	}

	for _, m := range memberships {
		if m.MembershipStatus != entity.MembershipStatusPending {
			continue
		}
		if err := workspaceMemberRepo.Activate(ctx, m.ID); err != nil {
			slog.WarnContext(ctx, "failed to activate workspace membership",
				slog.String("user_id", user.ID),
				slog.String("membership_id", m.ID),
				slog.String("error", err.Error()),
			)
		}
	}

	slog.InfoContext(ctx, "invited user activated on first login",
		slog.String("user_id", user.ID),
		slog.String("email", user.Email),
	)
}

// WorkspaceContext creates a middleware that requires and loads the user's role for a specific workspace.
// The workspace ID must come from the X-Workspace-ID header.
// This middleware should only be applied to routes that require workspace context.
// Users with system roles (SUPERADMIN) get automatic access as OWNER.
// Users with tenant roles (TENANT_OWNER) get automatic access as OWNER for workspaces in their tenant.
func WorkspaceContext(
	workspaceRepo port.WorkspaceRepository,
	workspaceMemberRepo port.WorkspaceMemberRepository,
	tenantMemberRepo port.TenantMemberRepository,
) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		workspaceID := c.GetHeader(WorkspaceIDHeader)
		if workspaceID == "" {
			abortWithError(c, http.StatusBadRequest, entity.ErrMissingWorkspaceID)
			return
		}

		internalUserID, ok := GetInternalUserID(c)
		if !ok {
			abortWithError(c, http.StatusUnauthorized, entity.ErrUnauthorized)
			return
		}

		// Check superadmin access
		if checkSuperAdminAccess(c, workspaceID, internalUserID) {
			c.Next()
			return
		}

		// Check tenant owner access
		workspace, _ := workspaceRepo.FindByID(c.Request.Context(), workspaceID)
		if checkTenantOwnerAccess(c, workspace, workspaceID, internalUserID, tenantMemberRepo) {
			c.Next()
			return
		}

		// Check workspace member access
		member := checkWorkspaceMemberAccess(c, workspaceID, internalUserID, workspaceMemberRepo)
		if member == nil {
			abortWithError(c, http.StatusForbidden, entity.ErrWorkspaceAccessDenied)
			return
		}

		c.Set(workspaceIDKey, workspaceID)
		c.Set(workspaceRoleKey, member.Role)
		c.Next()
	}
}

// checkSuperAdminAccess checks if user has superadmin access and grants OWNER role.
// Returns true if access was granted and processing should stop.
func checkSuperAdminAccess(c *gin.Context, workspaceID, internalUserID string) bool {
	sysRole, hasSysRole := GetSystemRole(c)
	if !hasSysRole || !sysRole.HasPermission(entity.SystemRoleSuperAdmin) {
		return false
	}

	c.Set(workspaceIDKey, workspaceID)
	c.Set(workspaceRoleKey, entity.WorkspaceRoleOwner)
	slog.DebugContext(c.Request.Context(), "superadmin workspace access granted",
		slog.String("user_id", internalUserID),
		slog.String("workspace_id", workspaceID),
		slog.String("operation_id", GetOperationID(c)),
	)
	return true
}

// checkTenantOwnerAccess checks if user is tenant owner for the workspace's tenant.
// Returns true if access was granted and processing should stop.
func checkTenantOwnerAccess(
	c *gin.Context,
	workspace *entity.Workspace,
	workspaceID, internalUserID string,
	tenantMemberRepo port.TenantMemberRepository,
) bool {
	if workspace == nil || workspace.TenantID == nil || *workspace.TenantID == "" {
		return false
	}

	tenantMember, err := tenantMemberRepo.FindActiveByUserAndTenant(
		c.Request.Context(), internalUserID, *workspace.TenantID,
	)
	if err != nil || !tenantMember.Role.HasPermission(entity.TenantRoleOwner) {
		return false
	}

	c.Set(workspaceIDKey, workspaceID)
	c.Set(workspaceRoleKey, entity.WorkspaceRoleOwner)
	slog.DebugContext(c.Request.Context(), "tenant owner workspace access granted",
		slog.String("user_id", internalUserID),
		slog.String("workspace_id", workspaceID),
		slog.String("tenant_id", *workspace.TenantID),
		slog.String("operation_id", GetOperationID(c)),
	)
	return true
}

// checkWorkspaceMemberAccess verifies user has direct workspace membership.
// Returns the member if found, nil otherwise.
func checkWorkspaceMemberAccess(
	c *gin.Context,
	workspaceID, internalUserID string,
	workspaceMemberRepo port.WorkspaceMemberRepository,
) *entity.WorkspaceMember {
	member, err := workspaceMemberRepo.FindActiveByUserAndWorkspace(
		c.Request.Context(), internalUserID, workspaceID,
	)
	if err != nil {
		slog.WarnContext(c.Request.Context(), "workspace access denied",
			slog.String("error", err.Error()),
			slog.String("user_id", internalUserID),
			slog.String("workspace_id", workspaceID),
			slog.String("operation_id", GetOperationID(c)),
		)
		return nil
	}
	return member
}

// GetInternalUserID retrieves the internal user ID from the Gin context.
func GetInternalUserID(c *gin.Context) (string, bool) {
	if val, exists := c.Get(internalUserIDKey); exists {
		if userID, ok := val.(string); ok && userID != "" {
			return userID, true
		}
	}
	return "", false
}

// GetWorkspaceID retrieves the current workspace ID from the Gin context.
func GetWorkspaceID(c *gin.Context) (string, bool) {
	if val, exists := c.Get(workspaceIDKey); exists {
		if wsID, ok := val.(string); ok && wsID != "" {
			return wsID, true
		}
	}
	return "", false
}

// GetWorkspaceIDFromHeader retrieves the workspace ID directly from the X-Workspace-ID header.
// Use this when you need to check the header without requiring full WorkspaceContext middleware.
func GetWorkspaceIDFromHeader(c *gin.Context) (string, bool) {
	workspaceID := c.GetHeader(WorkspaceIDHeader)
	return workspaceID, workspaceID != ""
}

// GetWorkspaceRole retrieves the user's role in the current workspace.
func GetWorkspaceRole(c *gin.Context) (entity.WorkspaceRole, bool) {
	if val, exists := c.Get(workspaceRoleKey); exists {
		if role, ok := val.(entity.WorkspaceRole); ok {
			return role, true
		}
	}
	return "", false
}
