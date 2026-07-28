package template

import (
	"context"
	"encoding/json"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// CreateTemplateCommand represents the command to create a template.
type CreateTemplateCommand struct {
	WorkspaceID      string
	FolderID         *string
	Title            string
	ContentStructure json.RawMessage
	IsPublicLibrary  bool
	Process          string // defaults to "default" if empty
	ProcessType      string // defaults to "CANONICAL_NAME" if empty
	CreatedBy        string
}

// UpdateTemplateCommand represents the command to update a template.
// All fields except ID are optional to support partial updates.
type UpdateTemplateCommand struct {
	ID              string
	Title           *string
	FolderID        *string
	IsPublicLibrary *bool
}

// CloneTemplateCommand represents the command to clone a template.
type CloneTemplateCommand struct {
	SourceTemplateID string
	VersionID        string
	NewTitle         string
	TargetFolderID   *string
	ClonedBy         string
}

// SetProcessFieldsCommand represents the command to set process fields on a template.
type SetProcessFieldsCommand struct {
	TemplateID  string
	WorkspaceID string
	Process     string
	ProcessType string
}

// AssignDocumentTypeCommand represents the command to assign/unassign a document type.
type AssignDocumentTypeCommand struct {
	TemplateID     string
	WorkspaceID    string  // For conflict checking
	DocumentTypeID *string // nil to unassign
	Force          bool    // true = reassign even if type is used by another template
}

// AssignDocumentTypeResult represents the result of assigning a document type.
type AssignDocumentTypeResult struct {
	Template *entity.Template
	Conflict *TemplateConflictInfo // Non-nil if there's a conflict and Force=false
}

// TemplateConflictInfo represents info about a conflicting template.
type TemplateConflictInfo struct {
	ID    string
	Title string
}

// TemplateUseCase defines the input port for template operations.
type TemplateUseCase interface {
	// CreateTemplate creates a new template with an initial draft version.
	CreateTemplate(ctx context.Context, cmd CreateTemplateCommand) (*entity.Template, *entity.TemplateVersion, error)

	// GetTemplate retrieves a template by ID.
	GetTemplate(ctx context.Context, id string) (*entity.Template, error)

	// GetTemplateWithDetails retrieves a template with published version, tags, and folder.
	GetTemplateWithDetails(ctx context.Context, id string) (*entity.TemplateWithDetails, error)

	// GetTemplateWithAllVersions retrieves a template with all its versions.
	GetTemplateWithAllVersions(ctx context.Context, id string) (*entity.TemplateWithAllVersions, error)

	// ListTemplates lists all templates in a workspace with optional filters.
	ListTemplates(ctx context.Context, workspaceID string, filters port.TemplateFilters) ([]*entity.TemplateListItem, error)

	// ListTemplatesByFolder lists all templates in a folder.
	ListTemplatesByFolder(ctx context.Context, folderID string) ([]*entity.TemplateListItem, error)

	// ListPublicLibrary lists all public library templates.
	ListPublicLibrary(ctx context.Context, workspaceID string) ([]*entity.TemplateListItem, error)

	// UpdateTemplate updates a template's metadata.
	UpdateTemplate(ctx context.Context, cmd UpdateTemplateCommand) (*entity.Template, error)

	// CloneTemplate creates a copy of an existing template from its published version.
	CloneTemplate(ctx context.Context, cmd CloneTemplateCommand) (*entity.Template, *entity.TemplateVersion, error)

	// DeleteTemplate deletes a template and all its versions.
	DeleteTemplate(ctx context.Context, id string) error

	// AddTag adds a tag to a template.
	AddTag(ctx context.Context, templateID, tagID string) error

	// RemoveTag removes a tag from a template.
	RemoveTag(ctx context.Context, templateID, tagID string) error

	// SetProcessFields sets the process and processType on a template.
	SetProcessFields(ctx context.Context, cmd SetProcessFieldsCommand) (*entity.Template, error)

	// AssignDocumentType assigns or unassigns a document type to a template.
	// If the type is already assigned to another template in the workspace and Force=false,
	// returns conflict info without making changes.
	AssignDocumentType(ctx context.Context, cmd AssignDocumentTypeCommand) (*AssignDocumentTypeResult, error)

	// FindByDocumentTypeCode finds templates by document type code across a tenant.
	FindByDocumentTypeCode(ctx context.Context, tenantID, code string) ([]*entity.TemplateListItem, error)
}
