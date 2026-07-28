package catalog

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// CreateDocumentTypeCommand represents the command to create a document type.
type CreateDocumentTypeCommand struct {
	TenantID    string
	Code        string
	Name        entity.I18nText
	Description entity.I18nText
}

// UpdateDocumentTypeCommand represents the command to update a document type.
type UpdateDocumentTypeCommand struct {
	ID          string
	TenantID    string // Required to verify ownership (cannot modify global types)
	Name        entity.I18nText
	Description entity.I18nText
}

// DeleteDocumentTypeCommand represents the command to delete a document type.
type DeleteDocumentTypeCommand struct {
	ID            string
	TenantID      string  // Required to verify ownership (cannot delete global types)
	Force         bool    // If true, delete even if templates are assigned (sets them to NULL)
	ReplaceWithID *string // If set, replace document_type_id in templates with this type before deleting
}

// DeleteDocumentTypeResult represents the result of attempting to delete a document type.
type DeleteDocumentTypeResult struct {
	Deleted    bool                               // True if deletion was performed
	Templates  []*entity.DocumentTypeTemplateInfo // Templates using this type (if not deleted)
	CanReplace bool                               // True if replacement is possible
}

// DocumentTypeUseCase defines the input port for document type operations.
type DocumentTypeUseCase interface {
	// CreateDocumentType creates a new document type.
	CreateDocumentType(ctx context.Context, cmd CreateDocumentTypeCommand) (*entity.DocumentType, error)

	// GetDocumentType retrieves a document type by ID.
	GetDocumentType(ctx context.Context, id string) (*entity.DocumentType, error)

	// GetDocumentTypeByCode retrieves a document type by code within a tenant.
	GetDocumentTypeByCode(ctx context.Context, tenantID, code string) (*entity.DocumentType, error)

	// ListDocumentTypes lists all document types for a tenant with pagination.
	ListDocumentTypes(ctx context.Context, tenantID string, filters port.DocumentTypeFilters) ([]*entity.DocumentType, int64, error)

	// ListDocumentTypesWithCount lists document types with template usage count.
	ListDocumentTypesWithCount(ctx context.Context, tenantID string, filters port.DocumentTypeFilters) ([]*entity.DocumentTypeListItem, int64, error)

	// UpdateDocumentType updates a document type's details (name and description only).
	UpdateDocumentType(ctx context.Context, cmd UpdateDocumentTypeCommand) (*entity.DocumentType, error)

	// DeleteDocumentType attempts to delete a document type.
	// If templates are assigned and Force is false, returns templates list without deleting.
	// If ReplaceWithID is set, replaces the type in all templates before deleting.
	DeleteDocumentType(ctx context.Context, cmd DeleteDocumentTypeCommand) (*DeleteDocumentTypeResult, error)
}
