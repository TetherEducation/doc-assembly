package template

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	templateuc "github.com/rendis/doc-assembly/core/internal/core/usecase/template"
)

// NewTemplateService creates a new template service.
func NewTemplateService(
	templateRepo port.TemplateRepository,
	versionRepo port.TemplateVersionRepository,
	tagRepo port.TemplateTagRepository,
	workspaceRepo port.WorkspaceRepository,
	processResolver port.ProcessResolver,
) templateuc.TemplateUseCase {
	return &TemplateService{
		templateRepo:    templateRepo,
		versionRepo:     versionRepo,
		tagRepo:         tagRepo,
		workspaceRepo:   workspaceRepo,
		processResolver: processResolver,
	}
}

// TemplateService implements template business logic.
type TemplateService struct {
	templateRepo    port.TemplateRepository
	versionRepo     port.TemplateVersionRepository
	tagRepo         port.TemplateTagRepository
	workspaceRepo   port.WorkspaceRepository
	processResolver port.ProcessResolver
}

// CreateTemplate creates a new template with an initial draft version.
//
//nolint:funlen
func (s *TemplateService) CreateTemplate(ctx context.Context, cmd templateuc.CreateTemplateCommand) (*entity.Template, *entity.TemplateVersion, error) {
	if err := s.validateTemplateWorkspace(ctx, cmd.WorkspaceID); err != nil {
		return nil, nil, err
	}

	// Check for duplicate title
	exists, err := s.templateRepo.ExistsByTitle(ctx, cmd.WorkspaceID, cmd.Title)
	if err != nil {
		return nil, nil, fmt.Errorf("checking template existence: %w", err)
	}
	if exists {
		return nil, nil, entity.ErrTemplateAlreadyExists
	}

	process := cmd.Process
	if process == "" {
		process = entity.DefaultProcess
	}
	processType := entity.ProcessType(cmd.ProcessType)
	if !processType.IsValid() {
		processType = entity.DefaultProcessType
	}

	template := &entity.Template{
		ID:              uuid.NewString(),
		WorkspaceID:     cmd.WorkspaceID,
		FolderID:        cmd.FolderID,
		Title:           cmd.Title,
		IsPublicLibrary: cmd.IsPublicLibrary,
		Process:         process,
		ProcessType:     processType,
		CreatedAt:       time.Now().UTC(),
	}

	if err := template.Validate(); err != nil {
		return nil, nil, fmt.Errorf("validating template: %w", err)
	}

	id, err := s.templateRepo.Create(ctx, template)
	if err != nil {
		return nil, nil, fmt.Errorf("creating template: %w", err)
	}
	template.ID = id

	// Create initial draft version
	var createdBy *string
	if cmd.CreatedBy != "" {
		createdBy = &cmd.CreatedBy
	}
	version := entity.NewTemplateVersion(template.ID, 1, "Initial Version", createdBy)
	version.ID = uuid.NewString()
	version.ContentStructure = cmd.ContentStructure

	versionID, err := s.versionRepo.Create(ctx, version)
	if err != nil {
		// Rollback template creation on failure
		_ = s.templateRepo.Delete(ctx, template.ID)
		return nil, nil, fmt.Errorf("creating initial version: %w", err)
	}
	version.ID = versionID

	slog.InfoContext(ctx, "template created with initial version",
		slog.String("template_id", template.ID),
		slog.String("version_id", version.ID),
		slog.String("title", template.Title),
		slog.String("workspace_id", template.WorkspaceID),
	)

	return template, version, nil
}

func (s *TemplateService) validateTemplateWorkspace(ctx context.Context, workspaceID string) error {
	if s.workspaceRepo == nil {
		return nil
	}
	workspace, err := s.workspaceRepo.FindByID(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("finding template workspace: %w", err)
	}
	if workspace.Type == entity.WorkspaceTypeSystem {
		return entity.ErrInvalidWorkspaceType
	}
	return nil
}

// GetTemplate retrieves a template by ID.
func (s *TemplateService) GetTemplate(ctx context.Context, id string) (*entity.Template, error) {
	template, err := s.templateRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding template %s: %w", id, err)
	}
	return template, nil
}

// GetTemplateWithDetails retrieves a template with published version, tags, and folder.
func (s *TemplateService) GetTemplateWithDetails(ctx context.Context, id string) (*entity.TemplateWithDetails, error) {
	details, err := s.templateRepo.FindByIDWithDetails(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding template details %s: %w", id, err)
	}
	return details, nil
}

// GetTemplateWithAllVersions retrieves a template with all its versions.
func (s *TemplateService) GetTemplateWithAllVersions(ctx context.Context, id string) (*entity.TemplateWithAllVersions, error) {
	details, err := s.templateRepo.FindByIDWithAllVersions(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding template with versions %s: %w", id, err)
	}
	return details, nil
}

// ListTemplates lists all templates in a workspace with optional filters.
func (s *TemplateService) ListTemplates(ctx context.Context, workspaceID string, filters port.TemplateFilters) ([]*entity.TemplateListItem, error) {
	templates, err := s.templateRepo.FindByWorkspace(ctx, workspaceID, filters)
	if err != nil {
		return nil, fmt.Errorf("listing templates: %w", err)
	}
	return templates, nil
}

// ListTemplatesByFolder lists all templates in a folder.
func (s *TemplateService) ListTemplatesByFolder(ctx context.Context, folderID string) ([]*entity.TemplateListItem, error) {
	templates, err := s.templateRepo.FindByFolder(ctx, folderID)
	if err != nil {
		return nil, fmt.Errorf("listing templates by folder: %w", err)
	}
	return templates, nil
}

// ListPublicLibrary lists all public library templates.
func (s *TemplateService) ListPublicLibrary(ctx context.Context, workspaceID string) ([]*entity.TemplateListItem, error) {
	templates, err := s.templateRepo.FindPublicLibrary(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("listing public library: %w", err)
	}
	return templates, nil
}

// UpdateTemplate updates a template's metadata.
// Supports partial updates - only non-nil fields are updated.
func (s *TemplateService) UpdateTemplate(ctx context.Context, cmd templateuc.UpdateTemplateCommand) (*entity.Template, error) {
	template, err := s.templateRepo.FindByID(ctx, cmd.ID)
	if err != nil {
		return nil, fmt.Errorf("finding template: %w", err)
	}

	// Check for duplicate title if changed
	if cmd.Title != nil && template.Title != *cmd.Title {
		exists, err := s.templateRepo.ExistsByTitleExcluding(ctx, template.WorkspaceID, *cmd.Title, template.ID)
		if err != nil {
			return nil, fmt.Errorf("checking template title: %w", err)
		}
		if exists {
			return nil, entity.ErrTemplateAlreadyExists
		}
		template.Title = *cmd.Title
	}

	if cmd.FolderID != nil {
		if *cmd.FolderID == "root" {
			template.FolderID = nil
		} else {
			template.FolderID = cmd.FolderID
		}
	}

	if cmd.IsPublicLibrary != nil {
		template.IsPublicLibrary = *cmd.IsPublicLibrary
	}

	now := time.Now().UTC()
	template.UpdatedAt = &now

	if err := template.Validate(); err != nil {
		return nil, fmt.Errorf("validating template: %w", err)
	}

	if err := s.templateRepo.Update(ctx, template); err != nil {
		return nil, fmt.Errorf("updating template: %w", err)
	}

	slog.InfoContext(ctx, "template updated",
		slog.String("template_id", template.ID),
		slog.String("title", template.Title),
	)

	return template, nil
}

// CloneTemplate creates a copy of an existing template from a specific version.
func (s *TemplateService) CloneTemplate(ctx context.Context, cmd templateuc.CloneTemplateCommand) (*entity.Template, *entity.TemplateVersion, error) {
	source, sourceVersion, err := s.validateCloneSource(ctx, cmd.SourceTemplateID, cmd.VersionID)
	if err != nil {
		return nil, nil, err
	}

	newTemplate, err := s.createClonedTemplate(ctx, source, cmd.NewTitle, cmd.TargetFolderID)
	if err != nil {
		return nil, nil, err
	}

	version := entity.NewTemplateVersion(newTemplate.ID, 1, "Initial Version", &cmd.ClonedBy)
	version.ID = uuid.NewString()
	version.ContentStructure = sourceVersion.ContentStructure

	versionID, err := s.versionRepo.Create(ctx, version)
	if err != nil {
		_ = s.templateRepo.Delete(ctx, newTemplate.ID)
		return nil, nil, fmt.Errorf("creating cloned version: %w", err)
	}
	version.ID = versionID

	s.cloneTags(ctx, newTemplate.ID, source.Tags)

	slog.InfoContext(ctx, "template cloned",
		slog.String("source_id", cmd.SourceTemplateID),
		slog.String("source_version_id", cmd.VersionID),
		slog.String("new_id", newTemplate.ID),
		slog.String("version_id", version.ID),
		slog.String("title", newTemplate.Title),
	)

	return newTemplate, version, nil
}

func (s *TemplateService) validateCloneSource(ctx context.Context, templateID, versionID string) (*entity.TemplateWithDetails, *entity.TemplateVersion, error) {
	sourceVersion, err := s.versionRepo.FindByID(ctx, versionID)
	if err != nil {
		return nil, nil, fmt.Errorf("finding source version: %w", err)
	}
	if sourceVersion.TemplateID != templateID {
		return nil, nil, entity.ErrVersionDoesNotBelongToTemplate
	}

	source, err := s.templateRepo.FindByIDWithDetails(ctx, templateID)
	if err != nil {
		return nil, nil, fmt.Errorf("finding source template: %w", err)
	}
	if err := s.validateTemplateWorkspace(ctx, source.WorkspaceID); err != nil {
		return nil, nil, err
	}

	return source, sourceVersion, nil
}

func (s *TemplateService) createClonedTemplate(ctx context.Context, source *entity.TemplateWithDetails, newTitle string, targetFolderID *string) (*entity.Template, error) {
	exists, err := s.templateRepo.ExistsByTitle(ctx, source.WorkspaceID, newTitle)
	if err != nil {
		return nil, fmt.Errorf("checking template title: %w", err)
	}
	if exists {
		return nil, entity.ErrTemplateAlreadyExists
	}

	newTemplate := &entity.Template{
		ID:              uuid.NewString(),
		WorkspaceID:     source.WorkspaceID,
		FolderID:        targetFolderID,
		Title:           newTitle,
		IsPublicLibrary: false,
		Process:         entity.DefaultProcess,
		ProcessType:     entity.DefaultProcessType,
		CreatedAt:       time.Now().UTC(),
	}

	id, err := s.templateRepo.Create(ctx, newTemplate)
	if err != nil {
		return nil, fmt.Errorf("creating cloned template: %w", err)
	}
	newTemplate.ID = id

	return newTemplate, nil
}

func (s *TemplateService) cloneTags(ctx context.Context, newTemplateID string, tags []*entity.Tag) {
	for _, tag := range tags {
		if err := s.tagRepo.AddTag(ctx, newTemplateID, tag.ID); err != nil {
			slog.WarnContext(ctx, "failed to clone tag",
				slog.String("template_id", newTemplateID),
				slog.Any("error", err),
			)
		}
	}
}

// DeleteTemplate deletes a template and all its versions.
func (s *TemplateService) DeleteTemplate(ctx context.Context, id string) error {
	// Delete tag associations
	if err := s.tagRepo.DeleteByTemplate(ctx, id); err != nil {
		slog.WarnContext(ctx, "failed to delete template tags", slog.Any("error", err))
	}

	// Template deletion will cascade to versions (FK constraint)
	if err := s.templateRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("deleting template: %w", err)
	}

	slog.InfoContext(ctx, "template deleted", slog.String("template_id", id))
	return nil
}

// AddTag adds a tag to a template.
func (s *TemplateService) AddTag(ctx context.Context, templateID, tagID string) error {
	// Check if already linked
	exists, err := s.tagRepo.Exists(ctx, templateID, tagID)
	if err != nil {
		return fmt.Errorf("checking tag link: %w", err)
	}
	if exists {
		return nil // Already linked, no error
	}

	if err := s.tagRepo.AddTag(ctx, templateID, tagID); err != nil {
		return fmt.Errorf("adding tag to template: %w", err)
	}

	slog.InfoContext(ctx, "tag added to template",
		slog.String("template_id", templateID),
		slog.String("tag_id", tagID),
	)

	return nil
}

// RemoveTag removes a tag from a template.
func (s *TemplateService) RemoveTag(ctx context.Context, templateID, tagID string) error {
	if err := s.tagRepo.RemoveTag(ctx, templateID, tagID); err != nil {
		return fmt.Errorf("removing tag from template: %w", err)
	}

	slog.InfoContext(ctx, "tag removed from template",
		slog.String("template_id", templateID),
		slog.String("tag_id", tagID),
	)

	return nil
}

// AssignDocumentType assigns or unassigns a document type to a template.
func (s *TemplateService) AssignDocumentType(ctx context.Context, cmd templateuc.AssignDocumentTypeCommand) (*templateuc.AssignDocumentTypeResult, error) {
	// Get the template
	template, err := s.templateRepo.FindByID(ctx, cmd.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("finding template: %w", err)
	}

	// If unassigning (nil), just clear and return
	if cmd.DocumentTypeID == nil {
		if err := s.templateRepo.UpdateDocumentType(ctx, cmd.TemplateID, nil); err != nil {
			return nil, fmt.Errorf("clearing document type: %w", err)
		}
		template.DocumentTypeID = nil
		now := time.Now().UTC()
		template.UpdatedAt = &now

		slog.InfoContext(ctx, "document type unassigned from template",
			slog.String("template_id", template.ID),
		)

		return &templateuc.AssignDocumentTypeResult{Template: template}, nil
	}

	// Check if another template in the same workspace has this type+process
	existingTemplate, err := s.templateRepo.FindByDocumentType(ctx, cmd.WorkspaceID, *cmd.DocumentTypeID, template.Process)
	if err != nil {
		return nil, fmt.Errorf("checking existing document type assignment: %w", err)
	}

	// If there's a conflict with a different template
	if existingTemplate != nil && existingTemplate.ID != cmd.TemplateID {
		if !cmd.Force {
			// Return conflict info without making changes
			return &templateuc.AssignDocumentTypeResult{
				Template: nil,
				Conflict: &templateuc.TemplateConflictInfo{
					ID:    existingTemplate.ID,
					Title: existingTemplate.Title,
				},
			}, nil
		}

		// Force mode: clear the type from the existing template first
		if err := s.templateRepo.UpdateDocumentType(ctx, existingTemplate.ID, nil); err != nil {
			return nil, fmt.Errorf("clearing document type from existing template: %w", err)
		}

		slog.InfoContext(ctx, "document type forcefully reassigned",
			slog.String("from_template_id", existingTemplate.ID),
			slog.String("to_template_id", cmd.TemplateID),
		)
	}

	// Assign the document type to this template
	if err := s.templateRepo.UpdateDocumentType(ctx, cmd.TemplateID, cmd.DocumentTypeID); err != nil {
		return nil, fmt.Errorf("assigning document type: %w", err)
	}

	template.DocumentTypeID = cmd.DocumentTypeID
	now := time.Now().UTC()
	template.UpdatedAt = &now

	slog.InfoContext(ctx, "document type assigned to template",
		slog.String("template_id", template.ID),
		slog.String("document_type_id", *cmd.DocumentTypeID),
	)

	return &templateuc.AssignDocumentTypeResult{Template: template}, nil
}

// SetProcessFields sets the process and processType on a template.
//
//nolint:funlen,gocognit,gocyclo,nestif
func (s *TemplateService) SetProcessFields(ctx context.Context, cmd templateuc.SetProcessFieldsCommand) (*entity.Template, error) {
	processType := entity.ProcessType(cmd.ProcessType)
	if !processType.IsValid() {
		return nil, entity.ErrInvalidProcessType
	}

	if cmd.Process == "" {
		return nil, entity.ErrRequiredField
	}

	// Validate via resolver if configured
	if s.processResolver != nil {
		// Derive tenantID from workspace — get template first
		template, err := s.templateRepo.FindByID(ctx, cmd.TemplateID)
		if err != nil {
			return nil, fmt.Errorf("finding template: %w", err)
		}

		if err := s.processResolver.ValidateProcess(ctx, cmd.WorkspaceID, cmd.Process, cmd.ProcessType); err != nil {
			return nil, fmt.Errorf("validating process: %w", err)
		}

		// Check uniqueness: if template has a document type, ensure no conflict
		if template.DocumentTypeID != nil {
			existing, err := s.templateRepo.FindByDocumentType(ctx, template.WorkspaceID, *template.DocumentTypeID, cmd.Process)
			if err != nil {
				return nil, fmt.Errorf("checking process uniqueness: %w", err)
			}
			if existing != nil && existing.ID != cmd.TemplateID {
				return nil, entity.ErrProcessSlotConflict
			}
		}

		if err := s.templateRepo.UpdateProcessFields(ctx, cmd.TemplateID, cmd.Process, processType); err != nil {
			return nil, fmt.Errorf("updating process fields: %w", err)
		}

		template.Process = cmd.Process
		template.ProcessType = processType
		now := time.Now().UTC()
		template.UpdatedAt = &now

		slog.InfoContext(ctx, "template process fields updated",
			slog.String("template_id", template.ID),
			slog.String("process", cmd.Process),
			slog.String("process_type", cmd.ProcessType),
		)

		return template, nil
	}

	// No resolver — still validate and update
	template, err := s.templateRepo.FindByID(ctx, cmd.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("finding template: %w", err)
	}

	// Check uniqueness
	if template.DocumentTypeID != nil {
		existing, err := s.templateRepo.FindByDocumentType(ctx, template.WorkspaceID, *template.DocumentTypeID, cmd.Process)
		if err != nil {
			return nil, fmt.Errorf("checking process uniqueness: %w", err)
		}
		if existing != nil && existing.ID != cmd.TemplateID {
			return nil, entity.ErrProcessSlotConflict
		}
	}

	if err := s.templateRepo.UpdateProcessFields(ctx, cmd.TemplateID, cmd.Process, processType); err != nil {
		return nil, fmt.Errorf("updating process fields: %w", err)
	}

	template.Process = cmd.Process
	template.ProcessType = processType
	now := time.Now().UTC()
	template.UpdatedAt = &now

	slog.InfoContext(ctx, "template process fields updated",
		slog.String("template_id", template.ID),
		slog.String("process", cmd.Process),
		slog.String("process_type", cmd.ProcessType),
	)

	return template, nil
}

// FindByDocumentTypeCode finds templates by document type code across a tenant.
func (s *TemplateService) FindByDocumentTypeCode(ctx context.Context, tenantID, code string) ([]*entity.TemplateListItem, error) {
	templates, err := s.templateRepo.FindByDocumentTypeCode(ctx, tenantID, code)
	if err != nil {
		return nil, fmt.Errorf("finding templates by document type code: %w", err)
	}
	return templates, nil
}
