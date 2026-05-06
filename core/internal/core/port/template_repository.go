package port

import (
	"context"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
)

// TemplateFilters contains optional filters for template queries.
type TemplateFilters struct {
	FolderID            *string
	RootOnly            bool  // Filter for root folder only (folder_id IS NULL)
	HasPublishedVersion *bool // Filter by whether template has a published version
	TagIDs              []string
	DocumentTypeID      *string // Filter by document type ID
	DocumentTypeCode    string  // Filter by document type code
	Process             *string // Filter by process
	Search              string
	Limit               int
	Offset              int
}

// InternalTemplateBaseQuery contains the base identifiers needed before running template resolution.
type InternalTemplateBaseQuery struct {
	TenantCode    string
	WorkspaceCode string
	DocumentType  string
}

// InternalTemplateBaseContext is a lightweight read model for the base internal-create context.
type InternalTemplateBaseContext struct {
	Tenant       *entity.Tenant
	Workspace    *entity.Workspace
	DocumentType *entity.DocumentType
}

// InternalTemplateContextQuery contains the inputs needed to resolve the full
// internal-create template context in one read model query.
type InternalTemplateContextQuery struct {
	TenantCode             string
	RequestedWorkspaceCode string
	WorkspaceCodes         []string
	DocumentType           string
	Process                string
	Tags                   []string
	Published              *bool
	Environment            entity.Environment
}

// InternalTemplateContext contains the full internal-create template context.
type InternalTemplateContext struct {
	Tenant             *entity.Tenant
	RequestedWorkspace *entity.Workspace
	DocumentType       *entity.DocumentType
	Template           *entity.Template
	Workspace          *entity.Workspace
	Version            *entity.TemplateVersionWithDetails
	DBInjectables      []*entity.InjectableDefinition
	ActiveSystemKeys   []string
}

// TemplateWorkspace contains a template and its owning workspace loaded as one read model.
type TemplateWorkspace struct {
	Template  *entity.Template
	Workspace *entity.Workspace
}

// TemplateResolutionQuery contains the inputs needed to resolve template-version candidates in one read model query.
type TemplateResolutionQuery struct {
	TenantCode     string
	WorkspaceCodes []string
	DocumentType   string
	Process        string
	Tags           []string
	Published      *bool
}

// TemplateResolutionCandidate is a lightweight read model for template-version resolution.
type TemplateResolutionCandidate struct {
	TenantID         string
	TenantCode       string
	WorkspaceID      string
	WorkspaceCode    string
	DocumentTypeID   string
	DocumentTypeCode string
	TemplateID       string
	VersionID        string
	Tags             []string
	Priority         int
}

// TemplateRepository defines the interface for template data access.
type TemplateRepository interface {
	// Create creates a new template.
	Create(ctx context.Context, template *entity.Template) (string, error)

	// FindByID finds a template by ID.
	FindByID(ctx context.Context, id string) (*entity.Template, error)

	// FindByIDWithDetails finds a template by ID with published version, tags, and folder.
	FindByIDWithDetails(ctx context.Context, id string) (*entity.TemplateWithDetails, error)

	// FindByIDWithAllVersions finds a template by ID with all versions.
	FindByIDWithAllVersions(ctx context.Context, id string) (*entity.TemplateWithAllVersions, error)

	// FindByWorkspace lists all templates in a workspace.
	FindByWorkspace(ctx context.Context, workspaceID string, filters TemplateFilters) ([]*entity.TemplateListItem, error)

	// FindByFolder lists all templates in a folder.
	FindByFolder(ctx context.Context, folderID string) ([]*entity.TemplateListItem, error)

	// FindPublicLibrary lists all public library templates.
	FindPublicLibrary(ctx context.Context, workspaceID string) ([]*entity.TemplateListItem, error)

	// Update updates a template.
	Update(ctx context.Context, template *entity.Template) error

	// Delete deletes a template.
	Delete(ctx context.Context, id string) error

	// ExistsByTitle checks if a template with the given title exists in the workspace.
	ExistsByTitle(ctx context.Context, workspaceID, title string) (bool, error)

	// ExistsByTitleExcluding checks excluding a specific template ID.
	ExistsByTitleExcluding(ctx context.Context, workspaceID, title, excludeID string) (bool, error)

	// CountByFolder returns the number of templates in a folder.
	CountByFolder(ctx context.Context, folderID string) (int, error)

	// FindInternalTemplateBaseContext resolves tenant, optional workspace, and document type in one query.
	FindInternalTemplateBaseContext(ctx context.Context, query InternalTemplateBaseQuery) (*InternalTemplateBaseContext, error)

	// FindInternalTemplateContext resolves tenant, optional requested workspace, document type,
	// best template candidate, version details, template, and resolved workspace in one query.
	FindInternalTemplateContext(ctx context.Context, query InternalTemplateContextQuery) (*InternalTemplateContext, error)

	// FindTemplateWorkspaceByTemplateID loads a template and its owning workspace in one query.
	FindTemplateWorkspaceByTemplateID(ctx context.Context, templateID string) (*TemplateWorkspace, error)

	// FindResolutionCandidates finds lightweight template-version candidates for internal template resolution.
	FindResolutionCandidates(ctx context.Context, query TemplateResolutionQuery) ([]TemplateResolutionCandidate, error)

	// FindByDocumentType finds the template assigned to a document type and process in a workspace.
	// Returns nil if no template is assigned to this type+process in the workspace.
	FindByDocumentType(ctx context.Context, workspaceID, documentTypeID, process string) (*entity.Template, error)

	// FindByDocumentTypeCode finds templates by document type code across a tenant.
	FindByDocumentTypeCode(ctx context.Context, tenantID, documentTypeCode string) ([]*entity.TemplateListItem, error)

	// UpdateDocumentType updates the document type assignment for a template.
	UpdateDocumentType(ctx context.Context, templateID string, documentTypeID *string) error

	// UpdateProcessFields updates the process and processType of a template.
	UpdateProcessFields(ctx context.Context, templateID string, process string, processType entity.ProcessType) error
}
