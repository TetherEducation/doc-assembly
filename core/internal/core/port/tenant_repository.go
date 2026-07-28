package port

import (
	"context"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// TenantFilters defines filters for paginated tenant listing.
type TenantFilters struct {
	Limit  int
	Offset int
	UserID string // Used for access-based sorting
	Query  string // Optional search filter for name/code
}

// TenantRepository defines the interface for tenant data access.
type TenantRepository interface {
	// Create creates a new tenant.
	Create(ctx context.Context, tenant *entity.Tenant) (string, error)

	// FindByID finds a tenant by ID.
	FindByID(ctx context.Context, id string) (*entity.Tenant, error)

	// FindByCode finds a tenant by code.
	FindByCode(ctx context.Context, code string) (*entity.Tenant, error)

	// FindAll lists all tenants.
	FindAll(ctx context.Context) ([]*entity.Tenant, error)

	// FindAllPaginated lists tenants with pagination and returns total count.
	FindAllPaginated(ctx context.Context, filters TenantFilters) ([]*entity.Tenant, int64, error)

	// SearchByNameOrCode searches tenants by name or code similarity using trigram.
	SearchByNameOrCode(ctx context.Context, query string, limit int) ([]*entity.Tenant, error)

	// Update updates a tenant.
	Update(ctx context.Context, tenant *entity.Tenant) error

	// UpdateStatus updates a tenant's status.
	UpdateStatus(ctx context.Context, id string, status entity.TenantStatus, updatedAt *time.Time) error

	// Delete deletes a tenant.
	Delete(ctx context.Context, id string) error

	// ExistsByCode checks if a tenant with the given code exists.
	ExistsByCode(ctx context.Context, code string) (bool, error)

	// FindSystemTenant finds the system tenant (is_system = true).
	FindSystemTenant(ctx context.Context) (*entity.Tenant, error)
}
