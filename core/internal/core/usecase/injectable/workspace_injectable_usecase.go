package injectable

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// CreateWorkspaceInjectableCommand represents the command to create a workspace injectable.
type CreateWorkspaceInjectableCommand struct {
	WorkspaceID  string
	Key          string
	Label        string
	Description  string
	DefaultValue string
	Metadata     map[string]any
}

// UpdateWorkspaceInjectableCommand represents the command to update a workspace injectable.
type UpdateWorkspaceInjectableCommand struct {
	ID           string
	WorkspaceID  string
	Key          *string
	Label        *string
	Description  *string
	DefaultValue *string
	Metadata     map[string]any
}

// WorkspaceInjectableUseCase defines the input port for workspace injectable operations.
type WorkspaceInjectableUseCase interface {
	// CreateInjectable creates a new TEXT type injectable for the workspace.
	CreateInjectable(ctx context.Context, cmd CreateWorkspaceInjectableCommand) (*entity.InjectableDefinition, error)

	// GetInjectable retrieves an injectable by ID (must belong to workspace).
	GetInjectable(ctx context.Context, id, workspaceID string) (*entity.InjectableDefinition, error)

	// ListInjectables lists all injectables owned by the workspace (excluding deleted).
	ListInjectables(ctx context.Context, workspaceID string) ([]*entity.InjectableDefinition, error)

	// UpdateInjectable updates a workspace-owned injectable.
	UpdateInjectable(ctx context.Context, cmd UpdateWorkspaceInjectableCommand) (*entity.InjectableDefinition, error)

	// DeleteInjectable soft-deletes an injectable (sets is_deleted=true).
	DeleteInjectable(ctx context.Context, id, workspaceID string) error

	// ActivateInjectable sets is_active=true for an injectable.
	ActivateInjectable(ctx context.Context, id, workspaceID string) (*entity.InjectableDefinition, error)

	// DeactivateInjectable sets is_active=false for an injectable.
	DeactivateInjectable(ctx context.Context, id, workspaceID string) (*entity.InjectableDefinition, error)
}
