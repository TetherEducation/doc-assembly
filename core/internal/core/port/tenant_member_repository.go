package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// TenantMemberFilters defines filters for paginated tenant member queries.
type TenantMemberFilters struct {
	Limit  int
	Offset int
	Query  string // Optional search filter for name/code
}

// TenantMemberRepository defines the interface for tenant membership data access.
type TenantMemberRepository interface {
	// Create creates a new tenant membership.
	Create(ctx context.Context, member *entity.TenantMember) (string, error)

	// FindByID finds a tenant membership by ID.
	FindByID(ctx context.Context, id string) (*entity.TenantMember, error)

	// FindByUserAndTenant finds a membership for a specific user and tenant.
	FindByUserAndTenant(ctx context.Context, userID, tenantID string) (*entity.TenantMember, error)

	// FindByTenant lists all members of a tenant.
	FindByTenant(ctx context.Context, tenantID string) ([]*entity.TenantMemberWithUser, error)

	// FindByUser lists all tenant memberships for a user.
	FindByUser(ctx context.Context, userID string) ([]*entity.TenantMember, error)

	// FindTenantsWithRoleByUser lists all tenants a user belongs to with their roles.
	FindTenantsWithRoleByUser(ctx context.Context, userID string) ([]*entity.TenantWithRole, error)

	// FindActiveByUserAndTenant finds an active membership.
	FindActiveByUserAndTenant(ctx context.Context, userID, tenantID string) (*entity.TenantMember, error)

	// Delete removes a tenant membership.
	Delete(ctx context.Context, id string) error

	// UpdateRole updates a member's tenant role.
	UpdateRole(ctx context.Context, id string, role entity.TenantRole) error

	// CountByRole counts members with a specific role in a tenant.
	CountByRole(ctx context.Context, tenantID string, role entity.TenantRole) (int, error)

	// FindTenantsWithRoleByUserAndIDs lists tenants by specific IDs that the user belongs to with their roles.
	// Returns tenants in the same order as the provided IDs.
	FindTenantsWithRoleByUserAndIDs(ctx context.Context, userID string, tenantIDs []string) ([]*entity.TenantWithRole, error)

	// FindTenantsWithRoleByUserPaginated lists tenants a user belongs to with pagination and optional search.
	// When filters.Query is provided, orders by similarity. Otherwise, orders by access history.
	FindTenantsWithRoleByUserPaginated(ctx context.Context, userID string, filters TenantMemberFilters) ([]*entity.TenantWithRole, int64, error)
}
