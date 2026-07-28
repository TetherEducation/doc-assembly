package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// TagRepository defines the interface for tag data access.
type TagRepository interface {
	// Create creates a new tag.
	Create(ctx context.Context, tag *entity.Tag) (string, error)

	// FindByID finds a tag by ID.
	FindByID(ctx context.Context, id string) (*entity.Tag, error)

	// FindByWorkspace lists all tags in a workspace.
	FindByWorkspace(ctx context.Context, workspaceID string) ([]*entity.Tag, error)

	// FindByWorkspaceWithCount lists all tags with template counts.
	FindByWorkspaceWithCount(ctx context.Context, workspaceID string) ([]*entity.TagWithCount, error)

	// FindByName finds a tag by name within a workspace.
	FindByName(ctx context.Context, workspaceID, name string) (*entity.Tag, error)

	// Update updates a tag.
	Update(ctx context.Context, tag *entity.Tag) error

	// Delete deletes a tag.
	Delete(ctx context.Context, id string) error

	// ExistsByName checks if a tag with the given name exists in the workspace.
	ExistsByName(ctx context.Context, workspaceID, name string) (bool, error)

	// ExistsByNameExcluding checks excluding a specific tag ID.
	ExistsByNameExcluding(ctx context.Context, workspaceID, name, excludeID string) (bool, error)

	// IsInUse checks if a tag is attached to any templates.
	IsInUse(ctx context.Context, id string) (bool, error)

	// GetTemplateCount returns the number of templates using this tag.
	GetTemplateCount(ctx context.Context, id string) (int, error)
}
