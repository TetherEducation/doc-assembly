package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// DocumentTypeFilters contains optional filters for document type queries.
type DocumentTypeFilters struct {
	Search string
	Limit  int
	Offset int
}

// DocumentTypeRepository defines the interface for document type data access.
type DocumentTypeRepository interface {
	// Create creates a new document type.
	Create(ctx context.Context, docType *entity.DocumentType) (string, error)

	// FindByID finds a document type by ID.
	FindByID(ctx context.Context, id string) (*entity.DocumentType, error)

	// FindByCode finds a document type by code within a tenant.
	FindByCode(ctx context.Context, tenantID, code string) (*entity.DocumentType, error)

	// FindByTenant lists all document types for a tenant with pagination.
	FindByTenant(ctx context.Context, tenantID string, filters DocumentTypeFilters) ([]*entity.DocumentType, int64, error)

	// FindByTenantWithTemplateCount lists document types with template usage count.
	FindByTenantWithTemplateCount(ctx context.Context, tenantID string, filters DocumentTypeFilters) ([]*entity.DocumentTypeListItem, int64, error)

	// Update updates a document type (name and description only, code is immutable).
	Update(ctx context.Context, docType *entity.DocumentType) error

	// Delete deletes a document type.
	Delete(ctx context.Context, id string) error

	// ExistsByCode checks if a document type with the given code exists in the tenant.
	ExistsByCode(ctx context.Context, tenantID, code string) (bool, error)

	// ExistsByCodeExcluding checks excluding a specific document type ID.
	ExistsByCodeExcluding(ctx context.Context, tenantID, code, excludeID string) (bool, error)

	// CountTemplatesByType returns the number of templates using this document type.
	CountTemplatesByType(ctx context.Context, documentTypeID string) (int, error)

	// FindTemplatesByType returns templates assigned to this document type.
	FindTemplatesByType(ctx context.Context, documentTypeID string) ([]*entity.DocumentTypeTemplateInfo, error)

	// IsSysTenant checks if the given tenant is the system tenant.
	IsSysTenant(ctx context.Context, tenantID string) (bool, error)

	// FindByTenantWithGlobalFallback lists document types including global (SYS tenant) types.
	// Tenant's own types take priority over global types with the same code.
	FindByTenantWithGlobalFallback(ctx context.Context, tenantID string, filters DocumentTypeFilters) ([]*entity.DocumentType, int64, error)

	// FindByTenantWithTemplateCountAndGlobal lists document types with template count, including global types.
	FindByTenantWithTemplateCountAndGlobal(ctx context.Context, tenantID string, filters DocumentTypeFilters) ([]*entity.DocumentTypeListItem, int64, error)

	// FindByCodeWithGlobalFallback finds a document type by code, checking tenant first then SYS tenant.
	FindByCodeWithGlobalFallback(ctx context.Context, tenantID, code string) (*entity.DocumentType, error)
}
