//go:build integration

package templatecommentrepo_test

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	templatecommentrepo "github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/database/postgres/template_comment_repo"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

type commentFixture struct {
	templateID string
	v1, v2     string
}

func setupCommentFixture(t *testing.T, pool *pgxpool.Pool, code string) commentFixture {
	t.Helper()
	tenantID := testhelper.CreateTestTenant(t, pool, "Comment Tenant "+code, code)
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, tenantID) })
	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Comment Workspace "+code,
		entity.WorkspaceTypeClient)
	t.Cleanup(func() { testhelper.CleanupWorkspace(t, pool, workspaceID) })
	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Comment Template "+code, nil)
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(),
			"DELETE FROM content.template_comments WHERE template_id = $1", templateID)
		testhelper.CleanupTemplate(t, pool, templateID)
	})
	v1 := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "v1", entity.VersionStatusDraft)
	v2 := testhelper.CreateTestTemplateVersion(t, pool, templateID, 2, "v2", entity.VersionStatusDraft)
	return commentFixture{templateID: templateID, v1: v1, v2: v2}
}

func newComment(t *testing.T, templateID, versionID, audience, body string) *entity.TemplateComment {
	t.Helper()
	c, err := entity.NewTemplateComment(entity.NewTemplateCommentInput{
		TemplateID: templateID, TemplateVersionID: versionID, Audience: audience,
		Body: body, AuthorEmail: "d@school.cl", AuthorName: "D", AuthorCampus: "CAMPUS",
	})
	require.NoError(t, err)
	return c
}

func TestTemplateCommentRepo_CreateFillsIDTimeAndVersionNumber(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	repo := templatecommentrepo.New(pool)
	fx := setupCommentFixture(t, pool, "CMTCRT")

	c := newComment(t, fx.templateID, fx.v2, "AUD", "hola")
	require.NoError(t, repo.Create(context.Background(), c))

	assert.NotEmpty(t, c.ID)
	assert.False(t, c.CreatedAt.IsZero())
	assert.Equal(t, 2, c.VersionNumber)
}

func TestTemplateCommentRepo_ListScopesByAudienceOldestFirst(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	repo := templatecommentrepo.New(pool)
	ctx := context.Background()
	fx := setupCommentFixture(t, pool, "CMTLST")

	first := newComment(t, fx.templateID, fx.v1, "NETWORK", "primero")
	require.NoError(t, repo.Create(ctx, first))
	other := newComment(t, fx.templateID, fx.v1, "OTHER", "otra audiencia")
	require.NoError(t, repo.Create(ctx, other))
	second := newComment(t, fx.templateID, fx.v2, "NETWORK", "segundo")
	require.NoError(t, repo.Create(ctx, second))

	comments, err := repo.ListForTemplate(ctx, fx.templateID, "NETWORK")
	require.NoError(t, err)
	require.Len(t, comments, 2)
	assert.Equal(t, first.ID, comments[0].ID)
	assert.Equal(t, 1, comments[0].VersionNumber)
	assert.Equal(t, second.ID, comments[1].ID)
	assert.Equal(t, 2, comments[1].VersionNumber)
	assert.Equal(t, "segundo", comments[1].Body)
	require.NotNil(t, comments[1].AuthorName)
	assert.Equal(t, "D", *comments[1].AuthorName)

	empty, err := repo.ListForTemplate(ctx, fx.templateID, "NOBODY")
	require.NoError(t, err)
	assert.NotNil(t, empty)
	assert.Empty(t, empty)
}

// The check constraint backs the entity: a row that skipped validation is refused.
func TestTemplateCommentRepo_CheckConstraintRejectsInvalidBodies(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ctx := context.Background()
	fx := setupCommentFixture(t, pool, "CMTCHK")

	insert := func(audience, body string) error {
		_, err := pool.Exec(ctx, `
			INSERT INTO content.template_comments
				(template_id, template_version_id, audience, body, author_email, author_campus)
			VALUES ($1, $2, $3, $4, 'e', 'c')`,
			fx.templateID, fx.v1, audience, body)
		return err
	}

	assert.Error(t, insert("A", "   "), "blank body must be rejected")
	assert.Error(t, insert("A", strings.Repeat("a", entity.TemplateCommentMaxLength+1)), "over-long body must be rejected")
	assert.Error(t, insert(" ", "ok"), "blank audience must be rejected")
	assert.NoError(t, insert("A", strings.Repeat("ñ", entity.TemplateCommentMaxLength)), "4000 characters is allowed")
}

func TestTemplateCommentRepo_DeletingTemplateCascades(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	repo := templatecommentrepo.New(pool)
	ctx := context.Background()
	fx := setupCommentFixture(t, pool, "CMTCAS")

	require.NoError(t, repo.Create(ctx, newComment(t, fx.templateID, fx.v1, "A", "x")))

	_, err := pool.Exec(ctx, "DELETE FROM content.template_versions WHERE template_id = $1", fx.templateID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx, "DELETE FROM content.templates WHERE id = $1", fx.templateID)
	require.NoError(t, err)

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		"SELECT count(*) FROM content.template_comments WHERE template_id = $1", fx.templateID).Scan(&count))
	assert.Zero(t, count)
}
