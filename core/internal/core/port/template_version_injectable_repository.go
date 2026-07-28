package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// TemplateVersionInjectableRepository defines the interface for template version injectable configuration data access.
type TemplateVersionInjectableRepository interface {
	// Create creates a new template version injectable configuration.
	Create(ctx context.Context, injectable *entity.TemplateVersionInjectable) (string, error)

	// FindByID finds a template version injectable by ID.
	FindByID(ctx context.Context, id string) (*entity.TemplateVersionInjectable, error)

	// FindByVersionID lists all injectables for a template version with their definitions.
	FindByVersionID(ctx context.Context, versionID string) ([]*entity.VersionInjectableWithDefinition, error)

	// Update updates a template version injectable configuration.
	Update(ctx context.Context, injectable *entity.TemplateVersionInjectable) error

	// Delete deletes a template version injectable configuration.
	Delete(ctx context.Context, id string) error

	// DeleteByVersionID deletes all injectable configurations for a template version.
	DeleteByVersionID(ctx context.Context, versionID string) error

	// Exists checks if an injectable definition is already linked to a version.
	Exists(ctx context.Context, versionID, injectableDefID string) (bool, error)

	// ExistsSystemKey checks if a system injectable key is already linked to a version.
	ExistsSystemKey(ctx context.Context, versionID, systemKey string) (bool, error)

	// CopyFromVersion copies all injectable configurations from one version to another.
	CopyFromVersion(ctx context.Context, sourceVersionID, targetVersionID string) error
}
