package organization

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// CreateWorkspaceCommand represents the command to create a workspace.
type CreateWorkspaceCommand struct {
	TenantID  *string
	Name      string
	Code      string
	Type      entity.WorkspaceType
	CreatedBy string
}

// UpdateWorkspaceCommand represents the command to update a workspace.
type UpdateWorkspaceCommand struct {
	ID   string
	Name *string // nil = don't change
	Code *string // nil = don't change
}

// UpdateWorkspaceStatusCommand represents the command to update a workspace's status.
type UpdateWorkspaceStatusCommand struct {
	ID     string
	Status entity.WorkspaceStatus
}

// WorkspaceUseCase defines the input port for workspace operations.
type WorkspaceUseCase interface {
	// CreateWorkspace creates a new workspace.
	CreateWorkspace(ctx context.Context, cmd CreateWorkspaceCommand) (*entity.Workspace, error)

	// GetWorkspace retrieves a workspace by ID.
	GetWorkspace(ctx context.Context, id string) (*entity.Workspace, error)

	// ListUserWorkspaces lists all workspaces a user has access to.
	ListUserWorkspaces(ctx context.Context, userID string) ([]*entity.WorkspaceWithRole, error)

	// ListWorkspacesPaginated lists workspaces for a tenant with pagination and optional search.
	// When filters.Query is provided, orders by similarity. Otherwise, orders by access history.
	ListWorkspacesPaginated(ctx context.Context, tenantID, userID string, filters port.WorkspaceFilters) ([]*entity.Workspace, int64, error)

	// UpdateWorkspace updates a workspace's details.
	UpdateWorkspace(ctx context.Context, cmd UpdateWorkspaceCommand) (*entity.Workspace, error)

	// ArchiveWorkspace archives a workspace (soft delete).
	ArchiveWorkspace(ctx context.Context, id string) error

	// ActivateWorkspace activates a workspace.
	ActivateWorkspace(ctx context.Context, id string) error

	// UpdateWorkspaceStatus updates a workspace's status (ACTIVE, SUSPENDED, ARCHIVED).
	UpdateWorkspaceStatus(ctx context.Context, cmd UpdateWorkspaceStatusCommand) (*entity.Workspace, error)

	// GetSystemWorkspace retrieves the system workspace for a tenant.
	GetSystemWorkspace(ctx context.Context, tenantID *string) (*entity.Workspace, error)
}
