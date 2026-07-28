package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// ProcessFilters contains optional filters for process queries.
type ProcessFilters struct {
	Search string
	Limit  int
	Offset int
}

// ProcessRepository defines the interface for process data access.
type ProcessRepository interface {
	// Create creates a new process.
	Create(ctx context.Context, process *entity.Process) (string, error)

	// FindByID finds a process by ID.
	FindByID(ctx context.Context, id string) (*entity.Process, error)

	// FindByCode finds a process by code within a tenant.
	FindByCode(ctx context.Context, tenantID, code string) (*entity.Process, error)

	// FindByTenant lists all processes for a tenant with pagination.
	FindByTenant(ctx context.Context, tenantID string, filters ProcessFilters) ([]*entity.Process, int64, error)

	// FindByTenantWithTemplateCount lists processes with template usage count.
	FindByTenantWithTemplateCount(ctx context.Context, tenantID string, filters ProcessFilters) ([]*entity.ProcessListItem, int64, error)

	// Update updates a process (name and description only).
	Update(ctx context.Context, process *entity.Process) error

	// Delete deletes a process.
	Delete(ctx context.Context, id string) error

	// ExistsByCode checks if a process with the given code exists in the tenant.
	ExistsByCode(ctx context.Context, tenantID, code string) (bool, error)

	// ExistsByCodeExcluding checks excluding a specific process ID.
	ExistsByCodeExcluding(ctx context.Context, tenantID, code, excludeID string) (bool, error)

	// CountTemplatesByProcess returns the number of templates using this process code.
	// Scoped to tenant via workspace join since process is a VARCHAR column on templates.
	CountTemplatesByProcess(ctx context.Context, tenantID, processCode string) (int, error)

	// FindTemplatesByProcess returns templates assigned to this process code.
	// Scoped to tenant via workspace join since process is a VARCHAR column on templates.
	FindTemplatesByProcess(ctx context.Context, tenantID, processCode string) ([]*entity.ProcessTemplateInfo, error)

	// IsSysTenant checks if the given tenant is the system tenant.
	IsSysTenant(ctx context.Context, tenantID string) (bool, error)

	// FindByTenantWithGlobalFallback lists processes including global (SYS tenant) processes.
	// Tenant's own processes take priority over global processes with the same code.
	FindByTenantWithGlobalFallback(ctx context.Context, tenantID string, filters ProcessFilters) ([]*entity.Process, int64, error)

	// FindByTenantWithTemplateCountAndGlobal lists processes with template count, including global processes.
	FindByTenantWithTemplateCountAndGlobal(ctx context.Context, tenantID string, filters ProcessFilters) ([]*entity.ProcessListItem, int64, error)

	// FindByCodeWithGlobalFallback finds a process by code, checking tenant first then SYS tenant.
	FindByCodeWithGlobalFallback(ctx context.Context, tenantID, code string) (*entity.Process, error)
}
