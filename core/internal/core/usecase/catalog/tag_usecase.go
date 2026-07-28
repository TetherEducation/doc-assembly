package catalog

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// CreateTagCommand represents the command to create a tag.
type CreateTagCommand struct {
	WorkspaceID string
	Name        string
	Color       string
	CreatedBy   string
}

// UpdateTagCommand represents the command to update a tag.
type UpdateTagCommand struct {
	ID    string
	Name  string
	Color string
}

// TagUseCase defines the input port for tag operations.
type TagUseCase interface {
	// CreateTag creates a new tag.
	CreateTag(ctx context.Context, cmd CreateTagCommand) (*entity.Tag, error)

	// GetTag retrieves a tag by ID.
	GetTag(ctx context.Context, id string) (*entity.Tag, error)

	// ListTags lists all tags in a workspace.
	ListTags(ctx context.Context, workspaceID string) ([]*entity.Tag, error)

	// ListTagsWithCount lists all tags with their template counts.
	ListTagsWithCount(ctx context.Context, workspaceID string) ([]*entity.TagWithCount, error)

	// UpdateTag updates a tag's details.
	UpdateTag(ctx context.Context, cmd UpdateTagCommand) (*entity.Tag, error)

	// DeleteTag deletes a tag.
	// Returns error if tag is attached to any templates.
	DeleteTag(ctx context.Context, id string) error

	// FindTagByName finds a tag by name within a workspace.
	FindTagByName(ctx context.Context, workspaceID, name string) (*entity.Tag, error)
}
