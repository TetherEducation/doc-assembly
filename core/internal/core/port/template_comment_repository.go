package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// TemplateCommentRepository stores schools' comments on contract templates.
//
// There is deliberately no Update or Delete: comments are append-only, like
// approvals.
type TemplateCommentRepository interface {
	// Create inserts a comment and fills in its ID, CreatedAt and VersionNumber.
	Create(ctx context.Context, comment *entity.TemplateComment) error

	// ListForTemplate returns one audience's comments on a template, oldest first.
	ListForTemplate(ctx context.Context, templateID, audience string) ([]*entity.TemplateComment, error)
}
