package middleware

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

const (
	// SandboxModeHeader is the header name for enabling sandbox mode.
	SandboxModeHeader = "X-Sandbox-Mode"
	// sandboxModeKey is the context key for sandbox mode flag.
	sandboxModeKey = "sandbox_mode"
	// parentWorkspaceIDKey is the context key for the parent workspace ID when in sandbox mode.
	parentWorkspaceIDKey = "parent_workspace_id"
	// environmentKey is the context key for the resolved environment.
	environmentKey = "environment"
)

// SandboxContext creates a middleware that resolves sandbox workspace when X-Sandbox-Mode header is set.
// This middleware must be applied after WorkspaceContext.
// When the header is set to "true", it looks up the sandbox workspace associated with the parent
// workspace ID and replaces the workspace ID in context with the sandbox ID.
func SandboxContext(workspaceRepo port.WorkspaceRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions || c.GetHeader(SandboxModeHeader) != "true" {
			c.Set(environmentKey, entity.EnvironmentProd)
			c.Next()
			return
		}

		parentWorkspaceID, ok := GetWorkspaceID(c)
		if !ok {
			c.Next()
			return
		}

		sandbox, err := resolveSandbox(c, workspaceRepo, parentWorkspaceID)
		if err != nil {
			return
		}

		c.Set(parentWorkspaceIDKey, parentWorkspaceID)
		c.Set(workspaceIDKey, sandbox.ID)
		c.Set(sandboxModeKey, true)
		c.Set(environmentKey, entity.EnvironmentDev)

		slog.DebugContext(c.Request.Context(), "sandbox mode enabled",
			slog.String("parent_workspace_id", parentWorkspaceID),
			slog.String("sandbox_workspace_id", sandbox.ID),
			slog.String("operation_id", GetOperationID(c)),
		)

		c.Next()
	}
}

// AutomationSandboxContext creates a middleware that resolves sandbox workspace for automation routes.
// Unlike SandboxContext, it reads the workspace ID from the :workspaceId URL path parameter
// instead of the X-Workspace-ID header, and does not require user identity context.
// When X-Sandbox-Mode header is "true", it resolves or auto-creates the sandbox workspace
// and replaces the workspace ID in context.
func AutomationSandboxContext(workspaceRepo port.WorkspaceRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		workspaceID := c.Param("workspaceId")
		if workspaceID != "" {
			c.Set(workspaceIDKey, workspaceID)
		}

		if c.Request.Method == http.MethodOptions || c.GetHeader(SandboxModeHeader) != "true" {
			c.Set(environmentKey, entity.EnvironmentProd)
			c.Next()
			return
		}

		if workspaceID == "" {
			abortWithError(c, http.StatusBadRequest, entity.ErrMissingWorkspaceID)
			return
		}

		sandbox, err := resolveSandbox(c, workspaceRepo, workspaceID)
		if err != nil {
			return
		}

		c.Set(parentWorkspaceIDKey, workspaceID)
		c.Set(workspaceIDKey, sandbox.ID)
		c.Set(sandboxModeKey, true)
		c.Set(environmentKey, entity.EnvironmentDev)

		slog.DebugContext(c.Request.Context(), "automation sandbox mode enabled",
			slog.String("parent_workspace_id", workspaceID),
			slog.String("sandbox_workspace_id", sandbox.ID),
			slog.String("operation_id", GetOperationID(c)),
		)

		c.Next()
	}
}

// resolveSandbox finds the sandbox workspace for the parent, auto-creating it if missing and eligible.
// On error, it aborts the gin context.
func resolveSandbox(c *gin.Context, workspaceRepo port.WorkspaceRepository, parentWorkspaceID string) (*entity.Workspace, error) {
	sandbox, err := workspaceRepo.FindSandboxByParentID(c.Request.Context(), parentWorkspaceID)
	if err == nil {
		return sandbox, nil
	}

	if !errors.Is(err, entity.ErrSandboxNotFound) {
		slog.ErrorContext(c.Request.Context(), "failed to find sandbox workspace",
			slog.String("error", err.Error()),
			slog.String("parent_workspace_id", parentWorkspaceID),
			slog.String("operation_id", GetOperationID(c)),
		)
		abortWithError(c, http.StatusInternalServerError, err)
		return nil, err
	}

	return findOrCreateSandbox(c, workspaceRepo, parentWorkspaceID)
}

// findOrCreateSandbox loads the parent workspace, checks eligibility, and creates a sandbox if possible.
// On error, it aborts the gin context and returns nil + error.
//
//nolint:funlen // Controller-style middleware branch handling is explicit for readability.
func findOrCreateSandbox(c *gin.Context, workspaceRepo port.WorkspaceRepository, parentWorkspaceID string) (*entity.Workspace, error) {
	ctx := c.Request.Context()
	opID := GetOperationID(c)

	parent, err := workspaceRepo.FindByID(ctx, parentWorkspaceID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to load parent workspace for sandbox resolution",
			slog.String("error", err.Error()),
			slog.String("parent_workspace_id", parentWorkspaceID),
			slog.String("operation_id", opID),
		)
		abortWithError(c, http.StatusInternalServerError, err)
		return nil, err
	}

	if !parent.CanHaveSandbox() {
		slog.WarnContext(ctx, "sandbox mode requested but workspace does not support sandbox",
			slog.String("parent_workspace_id", parentWorkspaceID),
			slog.String("workspace_type", string(parent.Type)),
			slog.String("operation_id", opID),
		)
		abortWithError(c, http.StatusBadRequest, entity.ErrSandboxNotSupported)
		return nil, entity.ErrSandboxNotSupported
	}

	// Parent is eligible — auto-create sandbox
	sandbox := &entity.Workspace{
		ID:          uuid.NewString(),
		TenantID:    parent.TenantID,
		Name:        fmt.Sprintf("%s (SANDBOX)", parent.Name),
		Code:        "SBX_" + parent.Code,
		Type:        parent.Type,
		Status:      parent.Status,
		IsSandbox:   true,
		SandboxOfID: &parentWorkspaceID,
		CreatedAt:   time.Now().UTC(),
	}

	id, err := workspaceRepo.CreateSandbox(ctx, sandbox)
	if err != nil {
		if errors.Is(err, entity.ErrSandboxNotSupported) {
			slog.WarnContext(ctx, "sandbox workspace creation rejected",
				slog.String("parent_workspace_id", parentWorkspaceID),
				slog.String("operation_id", opID),
			)
			abortWithError(c, http.StatusBadRequest, entity.ErrSandboxNotSupported)
			return nil, entity.ErrSandboxNotSupported
		}
		slog.ErrorContext(ctx, "failed to auto-create sandbox workspace",
			slog.String("error", err.Error()),
			slog.String("parent_workspace_id", parentWorkspaceID),
			slog.String("operation_id", opID),
		)
		abortWithError(c, http.StatusInternalServerError, err)
		return nil, err
	}
	sandbox.ID = id

	slog.InfoContext(ctx, "sandbox workspace auto-created",
		slog.String("parent_workspace_id", parentWorkspaceID),
		slog.String("sandbox_workspace_id", sandbox.ID),
		slog.String("operation_id", opID),
	)

	return sandbox, nil
}

// IsSandboxMode returns true if the request is operating in sandbox mode.
func IsSandboxMode(c *gin.Context) bool {
	if val, exists := c.Get(sandboxModeKey); exists {
		if mode, ok := val.(bool); ok {
			return mode
		}
	}
	return false
}

// GetParentWorkspaceID returns the parent workspace ID when in sandbox mode.
// Returns empty string and false if not in sandbox mode or if parent ID is not set.
func GetParentWorkspaceID(c *gin.Context) (string, bool) {
	if val, exists := c.Get(parentWorkspaceIDKey); exists {
		if id, ok := val.(string); ok && id != "" {
			return id, true
		}
	}
	return "", false
}

// GetEnvironment returns the environment derived from the request context.
func GetEnvironment(c *gin.Context) entity.Environment {
	if val, exists := c.Get(environmentKey); exists {
		if env, ok := val.(entity.Environment); ok {
			return env
		}
	}
	if IsSandboxMode(c) {
		return entity.EnvironmentDev
	}
	return entity.EnvironmentProd
}
