package port

import (
	"context"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
)

// InjectableRepository defines the interface for injectable definition data access.
// Note: Injectables are read-only - they are managed via database migrations/seeds.
type InjectableRepository interface {
	// FindByID finds an injectable definition by ID.
	FindByID(ctx context.Context, id string) (*entity.InjectableDefinition, error)

	// FindByWorkspace lists all injectable definitions for a workspace (including global).
	FindByWorkspace(ctx context.Context, workspaceID string) ([]*entity.InjectableDefinition, error)

	// FindByWorkspaceAndKeys lists injectable definitions for a workspace restricted to keys.
	FindByWorkspaceAndKeys(ctx context.Context, workspaceID string, keys []string) ([]*entity.InjectableDefinition, error)

	// FindGenerationAvailableByWorkspaceAndKeys lists DB injectables and active system keys in one read model query.
	FindGenerationAvailableByWorkspaceAndKeys(ctx context.Context, workspaceID string, keys []string) ([]*entity.InjectableDefinition, []string, error)

	// FindGlobal lists all global injectable definitions.
	FindGlobal(ctx context.Context) ([]*entity.InjectableDefinition, error)

	// FindByKey finds an injectable by key within a workspace or globally.
	FindByKey(ctx context.Context, workspaceID *string, key string) (*entity.InjectableDefinition, error)

	// ExistsByKey checks if an injectable with the given key exists.
	ExistsByKey(ctx context.Context, workspaceID *string, key string) (bool, error)

	// ExistsByKeyExcluding checks excluding a specific injectable ID.
	ExistsByKeyExcluding(ctx context.Context, workspaceID *string, key, excludeID string) (bool, error)

	// IsInUse checks if an injectable is used by any template versions.
	IsInUse(ctx context.Context, id string) (bool, error)

	// GetVersionCount returns the number of template versions using this injectable.
	GetVersionCount(ctx context.Context, id string) (int, error)

	// ExistsByKeysForWorkspace returns a set of keys that are accessible to the workspace.
	// It checks both workspace-specific and global injectables.
	ExistsByKeysForWorkspace(ctx context.Context, workspaceID string, keys []string) (map[string]bool, error)
}
