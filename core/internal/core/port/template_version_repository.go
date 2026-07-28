package port

import (
	"context"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// TemplateVersionFilters contains optional filters for version queries.
type TemplateVersionFilters struct {
	Status *entity.VersionStatus
	Limit  int
	Offset int
}

// TemplateVersionContext contains a version with the template/workspace context needed by generation.
type TemplateVersionContext struct {
	Version   *entity.TemplateVersionWithDetails
	Template  *entity.Template
	Workspace *entity.Workspace
}

// TemplateVersionRepository defines the interface for template version data access.
type TemplateVersionRepository interface {
	// Create creates a new template version.
	Create(ctx context.Context, version *entity.TemplateVersion) (string, error)

	// FindByID finds a template version by ID.
	FindByID(ctx context.Context, id string) (*entity.TemplateVersion, error)

	// FindByIDWithDetails finds a template version by ID with all related data (injectables, signer roles).
	FindByIDWithDetails(ctx context.Context, id string) (*entity.TemplateVersionWithDetails, error)

	// FindByIDWithDetailsAndTemplateWorkspace finds a template version with details plus template/workspace context.
	FindByIDWithDetailsAndTemplateWorkspace(ctx context.Context, id string) (*TemplateVersionContext, error)

	// FindByTemplateID lists all versions for a template.
	FindByTemplateID(ctx context.Context, templateID string) ([]*entity.TemplateVersion, error)

	// FindByTemplateIDWithDetails lists all versions for a template with full details.
	FindByTemplateIDWithDetails(ctx context.Context, templateID string) ([]*entity.TemplateVersionWithDetails, error)

	// FindPublishedByTemplateID finds the currently published version for a template.
	FindPublishedByTemplateID(ctx context.Context, templateID string) (*entity.TemplateVersion, error)

	// FindPublishedByTemplateIDWithDetails finds the published version with all details.
	FindPublishedByTemplateIDWithDetails(ctx context.Context, templateID string) (*entity.TemplateVersionWithDetails, error)

	// FindScheduledToPublish finds all versions scheduled to publish before the given time.
	FindScheduledToPublish(ctx context.Context, before time.Time) ([]*entity.TemplateVersion, error)

	// FindScheduledToArchive finds all published versions scheduled to archive before the given time.
	FindScheduledToArchive(ctx context.Context, before time.Time) ([]*entity.TemplateVersion, error)

	// Update updates a template version.
	Update(ctx context.Context, version *entity.TemplateVersion) error

	// UpdateStatus updates a version's status with optional user tracking.
	UpdateStatus(ctx context.Context, id string, status entity.VersionStatus, userID *string) error

	// Delete deletes a template version.
	Delete(ctx context.Context, id string) error

	// ExistsByVersionNumber checks if a version number already exists for the template.
	ExistsByVersionNumber(ctx context.Context, templateID string, versionNumber int) (bool, error)

	// ExistsByName checks if a version name already exists for the template.
	ExistsByName(ctx context.Context, templateID, name string) (bool, error)

	// ExistsByNameExcluding checks if a version name exists excluding a specific version ID.
	ExistsByNameExcluding(ctx context.Context, templateID, name, excludeID string) (bool, error)

	// GetNextVersionNumber returns the next available version number for a template.
	GetNextVersionNumber(ctx context.Context, templateID string) (int, error)

	// HasScheduledVersion checks if the template has a version with SCHEDULED status.
	HasScheduledVersion(ctx context.Context, templateID string) (bool, error)

	// ExistsScheduledAtTime checks if another version is scheduled at the exact time for the template.
	ExistsScheduledAtTime(ctx context.Context, templateID string, scheduledAt time.Time, excludeVersionID *string) (bool, error)

	// CountByTemplateID returns the number of versions for a template.
	CountByTemplateID(ctx context.Context, templateID string) (int, error)
}
