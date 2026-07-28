package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// WorkspaceInjectableRepository defines the interface for workspace-owned injectable management.
type WorkspaceInjectableRepository interface {
	// Create creates a new workspace-owned injectable.
	Create(ctx context.Context, injectable *entity.InjectableDefinition) (string, error)

	// Update updates a workspace-owned injectable.
	Update(ctx context.Context, injectable *entity.InjectableDefinition) error

	// SoftDelete marks an injectable as deleted (is_deleted=true).
	SoftDelete(ctx context.Context, id, workspaceID string) error

	// SetActive sets the is_active flag for an injectable.
	SetActive(ctx context.Context, id, workspaceID string, isActive bool) error

	// FindByID finds an injectable by ID, ensuring it belongs to the workspace and is not deleted.
	FindByID(ctx context.Context, id, workspaceID string) (*entity.InjectableDefinition, error)

	// FindByWorkspaceOwned lists injectables owned by a workspace (excluding deleted).
	FindByWorkspaceOwned(ctx context.Context, workspaceID string) ([]*entity.InjectableDefinition, error)

	// ExistsByKey checks if an injectable with the given key exists for the workspace.
	ExistsByKey(ctx context.Context, workspaceID, key string) (bool, error)

	// ExistsByKeyExcluding checks if an injectable with the given key exists, excluding a specific ID.
	ExistsByKeyExcluding(ctx context.Context, workspaceID, key, excludeID string) (bool, error)
}
