package template

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

type commentRepoFake struct {
	created    []*entity.TemplateComment
	listed     []*entity.TemplateComment
	listedTpl  string
	listedAud  string
	listCalled bool
}

func (f *commentRepoFake) Create(_ context.Context, c *entity.TemplateComment) error {
	c.ID = "comment-1"
	c.VersionNumber = 3
	c.CreatedAt = time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)
	f.created = append(f.created, c)
	return nil
}

func (f *commentRepoFake) ListForTemplate(
	_ context.Context, templateID, audience string,
) ([]*entity.TemplateComment, error) {
	f.listCalled = true
	f.listedTpl = templateID
	f.listedAud = audience
	return f.listed, nil
}

type commentVersionStub struct {
	templateID    string
	workspaceCode string
	err           error
}

func (s *commentVersionStub) FindByIDWithDetailsAndTemplateWorkspace(
	_ context.Context, _ string,
) (*port.TemplateVersionContext, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &port.TemplateVersionContext{
		Template:  &entity.Template{ID: s.templateID},
		Workspace: &entity.Workspace{Code: s.workspaceCode},
	}, nil
}

type commentTemplateStub struct {
	workspaceCode string
	err           error
}

func (s *commentTemplateStub) FindTemplateWorkspaceByTemplateID(
	_ context.Context, templateID string,
) (*port.TemplateWorkspace, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &port.TemplateWorkspace{
		Template:  &entity.Template{ID: templateID},
		Workspace: &entity.Workspace{Code: s.workspaceCode},
	}, nil
}

func newCommentService(repo *commentRepoFake) *TemplateCommentService {
	return &TemplateCommentService{
		commentRepo:  repo,
		versionRepo:  &commentVersionStub{templateID: "tpl-from-version", workspaceCode: testWorkspace},
		templateRepo: &commentTemplateStub{workspaceCode: testWorkspace},
	}
}

func validAddCommentCommand() templateuc.AddCommentCommand {
	return templateuc.AddCommentCommand{
		TemplateVersionID: "ver-1",
		WorkspaceCode:     testWorkspace,
		Audience:          testWorkspace,
		Body:              "  Falta la cláusula de retiro.  ",
		Email:             "director@ayelen.cl",
		Name:              "Directora",
		Campus:            "AYELEN",
	}
}

func TestAddCommentDerivesTemplateFromVersion(t *testing.T) {
	t.Parallel()

	repo := &commentRepoFake{}
	svc := newCommentService(repo)

	comment, err := svc.AddComment(context.Background(), validAddCommentCommand())
	require.NoError(t, err)
	require.Len(t, repo.created, 1)
	assert.Equal(t, "tpl-from-version", repo.created[0].TemplateID)
	assert.Equal(t, "ver-1", repo.created[0].TemplateVersionID)
	assert.Equal(t, "Falta la cláusula de retiro.", comment.Body)
	assert.Equal(t, 3, comment.VersionNumber)
}

func TestAddCommentWorkspaceClaimIsCaseInsensitive(t *testing.T) {
	t.Parallel()

	repo := &commentRepoFake{}
	svc := newCommentService(repo)
	cmd := validAddCommentCommand()
	cmd.WorkspaceCode = "ayelen"

	_, err := svc.AddComment(context.Background(), cmd)
	require.NoError(t, err)
}

func TestAddCommentRefusesEmptyWorkspaceClaim(t *testing.T) {
	t.Parallel()

	repo := &commentRepoFake{}
	svc := newCommentService(repo)
	cmd := validAddCommentCommand()
	cmd.WorkspaceCode = "  "

	_, err := svc.AddComment(context.Background(), cmd)
	require.ErrorIs(t, err, ErrWorkspaceScopeRequired)
	assert.Empty(t, repo.created)
}

func TestAddCommentRefusesWorkspaceMismatch(t *testing.T) {
	t.Parallel()

	repo := &commentRepoFake{}
	svc := newCommentService(repo)
	cmd := validAddCommentCommand()
	cmd.WorkspaceCode = "OTHER_SCHOOL"

	_, err := svc.AddComment(context.Background(), cmd)
	require.ErrorIs(t, err, ErrWorkspaceMismatch)
	assert.Empty(t, repo.created)
}

func TestAddCommentPropagatesMissingVersion(t *testing.T) {
	t.Parallel()

	repo := &commentRepoFake{}
	svc := newCommentService(repo)
	svc.versionRepo = &commentVersionStub{err: entity.ErrVersionNotFound}

	_, err := svc.AddComment(context.Background(), validAddCommentCommand())
	require.ErrorIs(t, err, entity.ErrVersionNotFound)
}

func TestAddCommentRejectsInvalidBodyWithoutWriting(t *testing.T) {
	t.Parallel()

	repo := &commentRepoFake{}
	svc := newCommentService(repo)
	cmd := validAddCommentCommand()
	cmd.Body = "   "

	_, err := svc.AddComment(context.Background(), cmd)
	require.ErrorIs(t, err, entity.ErrTemplateCommentInvalid)
	assert.Empty(t, repo.created)
}

func TestListCommentsScopesByTemplateAndAudience(t *testing.T) {
	t.Parallel()

	repo := &commentRepoFake{listed: []*entity.TemplateComment{{ID: "a"}, {ID: "b"}}}
	svc := newCommentService(repo)

	comments, err := svc.ListComments(context.Background(), templateuc.ListCommentsQuery{
		TemplateID: "tpl-1", WorkspaceCode: testWorkspace, Audience: " CAMPUS_X ",
	})
	require.NoError(t, err)
	assert.Len(t, comments, 2)
	assert.Equal(t, "tpl-1", repo.listedTpl)
	assert.Equal(t, "CAMPUS_X", repo.listedAud)
}

func TestListCommentsRefusesWorkspaceMismatch(t *testing.T) {
	t.Parallel()

	repo := &commentRepoFake{}
	svc := newCommentService(repo)

	_, err := svc.ListComments(context.Background(), templateuc.ListCommentsQuery{
		TemplateID: "tpl-1", WorkspaceCode: "OTHER_SCHOOL", Audience: testWorkspace,
	})
	require.ErrorIs(t, err, ErrWorkspaceMismatch)
	assert.False(t, repo.listCalled)
}

func TestListCommentsRefusesEmptyWorkspaceClaim(t *testing.T) {
	t.Parallel()

	repo := &commentRepoFake{}
	svc := newCommentService(repo)

	_, err := svc.ListComments(context.Background(), templateuc.ListCommentsQuery{
		TemplateID: "tpl-1", Audience: testWorkspace,
	})
	require.ErrorIs(t, err, ErrWorkspaceScopeRequired)
	assert.False(t, repo.listCalled)
}

func TestListCommentsRequiresAudience(t *testing.T) {
	t.Parallel()

	repo := &commentRepoFake{}
	svc := newCommentService(repo)

	_, err := svc.ListComments(context.Background(), templateuc.ListCommentsQuery{
		TemplateID: "tpl-1", WorkspaceCode: testWorkspace, Audience: "  ",
	})
	require.ErrorIs(t, err, entity.ErrTemplateCommentInvalid)
	assert.False(t, repo.listCalled)
}

func TestListCommentsPropagatesMissingTemplate(t *testing.T) {
	t.Parallel()

	repo := &commentRepoFake{}
	svc := newCommentService(repo)
	svc.templateRepo = &commentTemplateStub{err: entity.ErrTemplateNotFound}

	_, err := svc.ListComments(context.Background(), templateuc.ListCommentsQuery{
		TemplateID: "tpl-1", WorkspaceCode: testWorkspace, Audience: testWorkspace,
	})
	require.ErrorIs(t, err, entity.ErrTemplateNotFound)
}
