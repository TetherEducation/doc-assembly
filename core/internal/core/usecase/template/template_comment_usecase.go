package template

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// AddCommentCommand adds a comment to the template a version belongs to.
//
// There is no template ID: it is derived from the version server-side, so a caller
// cannot attach a comment to one template while naming another's version.
type AddCommentCommand struct {
	TemplateVersionID string

	// WorkspaceCode the caller claims the version belongs to. Checked, not trusted.
	WorkspaceCode string

	Audience string
	Body     string
	Email    string
	Name     string
	Campus   string
}

// ListCommentsQuery lists one audience's thread on a template.
type ListCommentsQuery struct {
	TemplateID string

	// WorkspaceCode the caller claims the template belongs to. Checked, not trusted.
	WorkspaceCode string

	Audience string
}

// TemplateCommentUseCase is the CRM-only comment thread on a template.
type TemplateCommentUseCase interface {
	// AddComment appends a comment, tagged with the version it was written against.
	AddComment(ctx context.Context, cmd AddCommentCommand) (*entity.TemplateComment, error)

	// ListComments returns one audience's comments on a template, oldest first.
	ListComments(ctx context.Context, query ListCommentsQuery) ([]*entity.TemplateComment, error)
}
