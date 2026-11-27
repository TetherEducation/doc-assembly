package template

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

// commentVersionReader is the slice of the template version repository the comment
// service needs: which template and workspace a version belongs to.
type commentVersionReader interface {
	FindByIDWithDetailsAndTemplateWorkspace(ctx context.Context, id string) (*port.TemplateVersionContext, error)
}

// commentTemplateReader is the slice of the template repository the comment service
// needs: which workspace a template belongs to.
type commentTemplateReader interface {
	FindTemplateWorkspaceByTemplateID(ctx context.Context, templateID string) (*port.TemplateWorkspace, error)
}

// TemplateCommentService runs the CRM-only comment thread on templates.
//
// Comments are not part of content_structure, so nothing here touches content or
// approvals: commenting never changes a checksum or makes an approval stale.
type TemplateCommentService struct {
	commentRepo  port.TemplateCommentRepository
	versionRepo  commentVersionReader
	templateRepo commentTemplateReader
}

// NewTemplateCommentService creates the template comment service.
func NewTemplateCommentService(
	commentRepo port.TemplateCommentRepository,
	versionRepo port.TemplateVersionRepository,
	templateRepo port.TemplateRepository,
) templateuc.TemplateCommentUseCase {
	return &TemplateCommentService{
		commentRepo:  commentRepo,
		versionRepo:  versionRepo,
		templateRepo: templateRepo,
	}
}

// AddComment appends a comment to the template the version belongs to.
//
// The template is derived from the version, and the version's workspace must be
// the one the caller claims, with the same semantics as approvals.
func (s *TemplateCommentService) AddComment(
	ctx context.Context,
	cmd templateuc.AddCommentCommand,
) (*entity.TemplateComment, error) {
	claimed := strings.TrimSpace(cmd.WorkspaceCode)
	if claimed == "" {
		return nil, ErrWorkspaceScopeRequired
	}

	versionCtx, err := s.versionRepo.FindByIDWithDetailsAndTemplateWorkspace(ctx, cmd.TemplateVersionID)
	if err != nil {
		return nil, fmt.Errorf("finding template version context: %w", err)
	}
	var workspace *entity.Workspace
	if versionCtx != nil {
		workspace = versionCtx.Workspace
	}
	if err := checkClaimedWorkspace(claimed, workspace, "template version "+cmd.TemplateVersionID); err != nil {
		return nil, err
	}
	if versionCtx.Template == nil {
		return nil, fmt.Errorf("template version %s has no template: %w", cmd.TemplateVersionID, entity.ErrTemplateNotFound)
	}

	comment, err := entity.NewTemplateComment(entity.NewTemplateCommentInput{
		TemplateID:        versionCtx.Template.ID,
		TemplateVersionID: cmd.TemplateVersionID,
		Audience:          cmd.Audience,
		Body:              cmd.Body,
		AuthorEmail:       cmd.Email,
		AuthorName:        cmd.Name,
		AuthorCampus:      cmd.Campus,
	})
	if err != nil {
		return nil, err
	}
	if err := s.commentRepo.Create(ctx, comment); err != nil {
		return nil, err
	}

	slog.InfoContext(ctx, "template comment added",
		slog.String("template_id", comment.TemplateID),
		slog.String("version_id", comment.TemplateVersionID),
		slog.String("comment_id", comment.ID),
		slog.String("audience", comment.Audience),
		slog.String("campus", comment.AuthorCampus),
	)
	return comment, nil
}

// ListComments returns one audience's comments on a template, oldest first.
func (s *TemplateCommentService) ListComments(
	ctx context.Context,
	query templateuc.ListCommentsQuery,
) ([]*entity.TemplateComment, error) {
	claimed := strings.TrimSpace(query.WorkspaceCode)
	if claimed == "" {
		return nil, ErrWorkspaceScopeRequired
	}
	audience := strings.TrimSpace(query.Audience)
	if audience == "" {
		return nil, fmt.Errorf("%w: audience is required", entity.ErrTemplateCommentInvalid)
	}

	templateWs, err := s.templateRepo.FindTemplateWorkspaceByTemplateID(ctx, query.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("finding template workspace: %w", err)
	}
	var workspace *entity.Workspace
	if templateWs != nil {
		workspace = templateWs.Workspace
	}
	if err := checkClaimedWorkspace(claimed, workspace, "template "+query.TemplateID); err != nil {
		return nil, err
	}

	return s.commentRepo.ListForTemplate(ctx, query.TemplateID, audience)
}
