package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// TemplateTagRepository defines the interface for template-tag relationship data access.
type TemplateTagRepository interface {
	// AddTag adds a tag to a template.
	AddTag(ctx context.Context, templateID, tagID string) error

	// RemoveTag removes a tag from a template.
	RemoveTag(ctx context.Context, templateID, tagID string) error

	// FindTagsByTemplate lists all tags for a template.
	FindTagsByTemplate(ctx context.Context, templateID string) ([]*entity.Tag, error)

	// FindTemplatesByTag lists all templates with a specific tag.
	FindTemplatesByTag(ctx context.Context, tagID string) ([]string, error)

	// Exists checks if a tag is already linked to a template.
	Exists(ctx context.Context, templateID, tagID string) (bool, error)

	// DeleteByTemplate removes all tag associations for a template.
	DeleteByTemplate(ctx context.Context, templateID string) error
}
