package catalog

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	cataloguc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/catalog"
)

// NewDocumentTypeService creates a new document type service.
func NewDocumentTypeService(
	docTypeRepo port.DocumentTypeRepository,
	templateRepo port.TemplateRepository,
) cataloguc.DocumentTypeUseCase {
	return &DocumentTypeService{
		docTypeRepo:  docTypeRepo,
		templateRepo: templateRepo,
	}
}

// DocumentTypeService implements document type business logic.
type DocumentTypeService struct {
	docTypeRepo  port.DocumentTypeRepository
	templateRepo port.TemplateRepository
}

// CreateDocumentType creates a new document type.
func (s *DocumentTypeService) CreateDocumentType(ctx context.Context, cmd cataloguc.CreateDocumentTypeCommand) (*entity.DocumentType, error) {
	// Check for duplicate code
	exists, err := s.docTypeRepo.ExistsByCode(ctx, cmd.TenantID, cmd.Code)
	if err != nil {
		return nil, fmt.Errorf("checking document type existence: %w", err)
	}
	if exists {
		return nil, entity.ErrDocumentTypeCodeExists
	}

	docType := &entity.DocumentType{
		ID:          uuid.NewString(),
		TenantID:    cmd.TenantID,
		Code:        cmd.Code,
		Name:        cmd.Name,
		Description: cmd.Description,
		CreatedAt:   time.Now().UTC(),
	}

	if err := docType.Validate(); err != nil {
		return nil, fmt.Errorf("validating document type: %w", err)
	}

	id, err := s.docTypeRepo.Create(ctx, docType)
	if err != nil {
		return nil, fmt.Errorf("creating document type: %w", err)
	}
	docType.ID = id

	slog.InfoContext(ctx, "document type created",
		slog.String("document_type_id", docType.ID),
		slog.String("code", docType.Code),
		slog.String("tenant_id", docType.TenantID),
	)

	return docType, nil
}

// GetDocumentType retrieves a document type by ID.
func (s *DocumentTypeService) GetDocumentType(ctx context.Context, id string) (*entity.DocumentType, error) {
	docType, err := s.docTypeRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding document type %s: %w", id, err)
	}
	return docType, nil
}

// GetDocumentTypeByCode retrieves a document type by code within a tenant.
// For non-SYS tenants, also checks global (SYS tenant) types.
func (s *DocumentTypeService) GetDocumentTypeByCode(ctx context.Context, tenantID, code string) (*entity.DocumentType, error) {
	// Check if this is the SYS tenant
	isSys, err := s.docTypeRepo.IsSysTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("checking tenant type: %w", err)
	}

	// SYS tenant only sees its own types
	if isSys {
		docType, err := s.docTypeRepo.FindByCode(ctx, tenantID, code)
		if err != nil {
			return nil, fmt.Errorf("finding document type by code %s: %w", code, err)
		}
		return docType, nil
	}

	// Non-SYS tenants see their types + global types (with priority for own)
	docType, err := s.docTypeRepo.FindByCodeWithGlobalFallback(ctx, tenantID, code)
	if err != nil {
		return nil, fmt.Errorf("finding document type by code %s: %w", code, err)
	}
	return docType, nil
}

// ListDocumentTypes lists all document types for a tenant with pagination.
func (s *DocumentTypeService) ListDocumentTypes(ctx context.Context, tenantID string, filters port.DocumentTypeFilters) ([]*entity.DocumentType, int64, error) {
	docTypes, total, err := s.docTypeRepo.FindByTenant(ctx, tenantID, filters)
	if err != nil {
		return nil, 0, fmt.Errorf("listing document types: %w", err)
	}
	return docTypes, total, nil
}

// ListDocumentTypesWithCount lists document types with template usage count.
// For non-SYS tenants, includes global (SYS tenant) types with priority for own types.
func (s *DocumentTypeService) ListDocumentTypesWithCount(ctx context.Context, tenantID string, filters port.DocumentTypeFilters) ([]*entity.DocumentTypeListItem, int64, error) {
	// Check if this is the SYS tenant
	isSys, err := s.docTypeRepo.IsSysTenant(ctx, tenantID)
	if err != nil {
		return nil, 0, fmt.Errorf("checking tenant type: %w", err)
	}

	// SYS tenant only sees its own types
	if isSys {
		docTypes, total, err := s.docTypeRepo.FindByTenantWithTemplateCount(ctx, tenantID, filters)
		if err != nil {
			return nil, 0, fmt.Errorf("listing document types with count: %w", err)
		}
		return docTypes, total, nil
	}

	// Non-SYS tenants see their types + global types (with priority for own)
	docTypes, total, err := s.docTypeRepo.FindByTenantWithTemplateCountAndGlobal(ctx, tenantID, filters)
	if err != nil {
		return nil, 0, fmt.Errorf("listing document types with count and global: %w", err)
	}
	return docTypes, total, nil
}

// UpdateDocumentType updates a document type's details (name and description only).
// Global types (from SYS tenant) cannot be modified by other tenants.
func (s *DocumentTypeService) UpdateDocumentType(ctx context.Context, cmd cataloguc.UpdateDocumentTypeCommand) (*entity.DocumentType, error) {
	docType, err := s.docTypeRepo.FindByID(ctx, cmd.ID)
	if err != nil {
		return nil, fmt.Errorf("finding document type: %w", err)
	}

	// Check ownership: cannot modify global types
	if docType.TenantID != cmd.TenantID {
		return nil, entity.ErrCannotModifyGlobalType
	}

	docType.Name = cmd.Name
	docType.Description = cmd.Description
	now := time.Now().UTC()
	docType.UpdatedAt = &now

	if err := docType.Validate(); err != nil {
		return nil, fmt.Errorf("validating document type: %w", err)
	}

	if err := s.docTypeRepo.Update(ctx, docType); err != nil {
		return nil, fmt.Errorf("updating document type: %w", err)
	}

	slog.InfoContext(ctx, "document type updated",
		slog.String("document_type_id", docType.ID),
		slog.String("code", docType.Code),
	)

	return docType, nil
}

// DeleteDocumentType attempts to delete a document type.
// Global types (from SYS tenant) cannot be deleted by other tenants.
func (s *DocumentTypeService) DeleteDocumentType(ctx context.Context, cmd cataloguc.DeleteDocumentTypeCommand) (*cataloguc.DeleteDocumentTypeResult, error) {
	docType, err := s.docTypeRepo.FindByID(ctx, cmd.ID)
	if err != nil {
		return nil, fmt.Errorf("finding document type: %w", err)
	}

	// Check ownership: cannot delete global types
	if docType.TenantID != cmd.TenantID {
		return nil, entity.ErrCannotModifyGlobalType
	}

	templates, err := s.docTypeRepo.FindTemplatesByType(ctx, cmd.ID)
	if err != nil {
		return nil, fmt.Errorf("finding templates by type: %w", err)
	}

	// If templates exist and no action specified, return info without deleting
	if len(templates) > 0 && !cmd.Force && cmd.ReplaceWithID == nil {
		return &cataloguc.DeleteDocumentTypeResult{
			Deleted:    false,
			Templates:  templates,
			CanReplace: true,
		}, nil
	}

	// Handle template updates before deletion
	if err := s.handleTemplatesBeforeDelete(ctx, templates, cmd); err != nil {
		return nil, err
	}

	if err := s.docTypeRepo.Delete(ctx, cmd.ID); err != nil {
		return nil, fmt.Errorf("deleting document type: %w", err)
	}

	slog.InfoContext(ctx, "document type deleted",
		slog.String("document_type_id", docType.ID),
		slog.String("code", docType.Code),
		slog.Int("templates_affected", len(templates)),
	)

	return &cataloguc.DeleteDocumentTypeResult{Deleted: true, Templates: templates}, nil
}

// handleTemplatesBeforeDelete updates templates before deleting the document type.
func (s *DocumentTypeService) handleTemplatesBeforeDelete(ctx context.Context, templates []*entity.DocumentTypeTemplateInfo, cmd cataloguc.DeleteDocumentTypeCommand) error {
	if len(templates) == 0 {
		return nil
	}

	// Replace with another type
	if cmd.ReplaceWithID != nil {
		if _, err := s.docTypeRepo.FindByID(ctx, *cmd.ReplaceWithID); err != nil {
			return fmt.Errorf("replacement document type not found: %w", err)
		}
		return s.updateTemplatesDocumentType(ctx, templates, cmd.ReplaceWithID)
	}

	// Force delete: clear type from all templates
	if cmd.Force {
		return s.updateTemplatesDocumentType(ctx, templates, nil)
	}

	return nil
}

// updateTemplatesDocumentType updates document type for multiple templates.
func (s *DocumentTypeService) updateTemplatesDocumentType(ctx context.Context, templates []*entity.DocumentTypeTemplateInfo, newTypeID *string) error {
	for _, tmpl := range templates {
		if err := s.templateRepo.UpdateDocumentType(ctx, tmpl.ID, newTypeID); err != nil {
			return fmt.Errorf("updating document type for template %s: %w", tmpl.ID, err)
		}
	}
	return nil
}
