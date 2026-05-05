package injectable

import (
	"context"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

// ListInjectablesRequest contains parameters for listing injectables.
type ListInjectablesRequest struct {
	WorkspaceID string // Workspace ID to list injectables for
	Environment entity.Environment
	Codes       []string // Optional injectable keys to restrict the result for generation/validation
}

// ListInjectablesResult contains the list of injectables and groups.
type ListInjectablesResult struct {
	Injectables []*entity.InjectableDefinition
	Groups      []port.GroupConfig
}

// CompleteGenerationInjectablesRequest contains preloaded DB/system injectable data
// that can be hydrated without re-querying the database during generation.
type CompleteGenerationInjectablesRequest struct {
	WorkspaceID      string
	Environment      entity.Environment
	Codes            []string
	DBInjectables    []*entity.InjectableDefinition
	ActiveSystemKeys []string
}

// InjectableUseCase defines the input port for injectable definition operations.
// Note: Injectables are read-only - they are managed via database migrations/seeds.
type InjectableUseCase interface {
	// GetInjectable retrieves an injectable definition by ID.
	GetInjectable(ctx context.Context, id string) (*entity.InjectableDefinition, error)

	// ListInjectables lists all injectable definitions for a workspace (including global, system, and provider).
	ListInjectables(ctx context.Context, req *ListInjectablesRequest) (*ListInjectablesResult, error)

	// CompleteGenerationInjectables hydrates preloaded DB/system injectable data with registry/provider definitions.
	CompleteGenerationInjectables(ctx context.Context, req *CompleteGenerationInjectablesRequest) (*ListInjectablesResult, error)
}
