package organization

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// CreateTenantCommand represents the command to create a tenant.
type CreateTenantCommand struct {
	Code        string
	Name        string
	Description string
}

// UpdateTenantCommand represents the command to update a tenant.
type UpdateTenantCommand struct {
	ID          string
	Name        string
	Description string
	Settings    map[string]any
}

// UpdateTenantStatusCommand represents the command to update a tenant's status.
type UpdateTenantStatusCommand struct {
	ID     string
	Status entity.TenantStatus
}

// TenantUseCase defines the input port for tenant operations.
type TenantUseCase interface {
	// CreateTenant creates a new tenant with its system workspace.
	CreateTenant(ctx context.Context, cmd CreateTenantCommand) (*entity.Tenant, error)

	// GetTenant retrieves a tenant by ID.
	GetTenant(ctx context.Context, id string) (*entity.Tenant, error)

	// GetTenantByCode retrieves a tenant by its code.
	GetTenantByCode(ctx context.Context, code string) (*entity.Tenant, error)

	// SearchTenants searches tenants by name or code similarity.
	SearchTenants(ctx context.Context, query string) ([]*entity.Tenant, error)

	// ListTenantsPaginated lists tenants with pagination.
	ListTenantsPaginated(ctx context.Context, filters port.TenantFilters) ([]*entity.Tenant, int64, error)

	// ListTenantWorkspaces lists workspaces for a tenant with optional search (system admin use).
	ListTenantWorkspaces(ctx context.Context, tenantID string, filters port.WorkspaceFilters) ([]*entity.Workspace, int64, error)

	// ListUserTenants lists all tenants a user belongs to with their roles.
	ListUserTenants(ctx context.Context, userID string) ([]*entity.TenantWithRole, error)

	// ListUserTenantsPaginated lists tenants a user belongs to with pagination and optional search.
	// When filters.Query is provided, orders by similarity. Otherwise, orders by access history.
	ListUserTenantsPaginated(ctx context.Context, userID string, filters port.TenantMemberFilters) ([]*entity.TenantWithRole, int64, error)

	// UpdateTenant updates a tenant's details.
	UpdateTenant(ctx context.Context, cmd UpdateTenantCommand) (*entity.Tenant, error)

	// UpdateTenantStatus updates a tenant's status (ACTIVE, SUSPENDED, ARCHIVED).
	UpdateTenantStatus(ctx context.Context, cmd UpdateTenantStatusCommand) (*entity.Tenant, error)

	// DeleteTenant deletes a tenant and all its data.
	DeleteTenant(ctx context.Context, id string) error
}
