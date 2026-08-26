//go:build integration

package templaterepo_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

// contentDeclaringSignerRoles is a minimal portable document that declares one signer
// role, matching the shape every production template uses.
const contentDeclaringSignerRoles = `{
  "version":"1.1.0",
  "variableIds":[],
  "signerRoles":[{"id":"role-001","label":"Apoderado/a","order":1,
    "name":{"type":"text","value":"x"},"email":{"type":"text","value":"x@test.com"}}],
  "content":{"type":"doc","content":[]}
}`

// contentWithoutSignerRoles is a document that needs no signature at all.
const contentWithoutSignerRoles = `{
  "version":"1.1.0","variableIds":[],"signerRoles":[],
  "content":{"type":"doc","content":[]}
}`

// insertVersion writes a template version directly, bypassing the service — the same
// thing the 2026-08-14 promotion did.
func insertVersion(t *testing.T, pool *pgxpool.Pool, templateID, content string,
	status entity.VersionStatus, versionNumber int) (string, error) {
	t.Helper()
	versionID := uuid.NewString()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO content.template_versions
			(id, template_id, version_number, name, content_structure, status)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)`,
		versionID, templateID, versionNumber, "guard-test", content, status)
	return versionID, err
}

func addSignerRole(t *testing.T, pool *pgxpool.Pool, versionID string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO content.template_version_signer_roles
			(template_version_id, role_name, anchor_string, signer_order)
		VALUES ($1, 'Apoderado/a', '__sig_apoderadoa__', 1)`, versionID)
	require.NoError(t, err)
}

func guardTestTemplate(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()
	tenantID := testhelper.CreateTestTenant(t, pool, "Guard Tenant", "GRDTEN")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, tenantID) })
	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Guard Workspace",
		entity.WorkspaceTypeClient)
	t.Cleanup(func() { testhelper.CleanupWorkspace(t, pool, workspaceID) })
	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Guard Template", nil)
	t.Cleanup(func() { testhelper.CleanupTemplate(t, pool, templateID) })
	return templateID
}

// The incident in one test: a version that declares signer roles must not reach
// PUBLISHED without the derived rows. DocumentGenerator builds roleByAnchor from those
// rows, so publishing without them makes every document creation fail recipient
// validation with a 422 -- which is exactly what happened for eleven days.
func TestSignerRoleGuard_RejectsDirectPublishWithoutDerivedRoles(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	templateID := guardTestTemplate(t, pool)

	_, err := insertVersion(t, pool, templateID, contentDeclaringSignerRoles,
		entity.VersionStatusPublished, 1)
	require.Error(t, err, "publishing without derived signer roles must be rejected")
	require.Contains(t, err.Error(), "signer role")
}

// The legitimate path: PublishVersion calls replaceSignerRoles before flipping status,
// so by the time the row becomes PUBLISHED the derived rows exist.
func TestSignerRoleGuard_AllowsPublishWhenRolesWereDerivedFirst(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	templateID := guardTestTemplate(t, pool)

	versionID, err := insertVersion(t, pool, templateID, contentDeclaringSignerRoles,
		entity.VersionStatusDraft, 1)
	require.NoError(t, err)
	t.Cleanup(func() { testhelper.CleanupTemplateVersion(t, pool, versionID) })

	addSignerRole(t, pool, versionID)

	_, err = pool.Exec(context.Background(),
		`UPDATE content.template_versions SET status = 'PUBLISHED' WHERE id = $1`, versionID)
	require.NoError(t, err, "publishing after deriving roles must succeed")
}

// A document that needs no signature must stay publishable.
func TestSignerRoleGuard_AllowsPublishWhenNoRolesAreDeclared(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	templateID := guardTestTemplate(t, pool)

	versionID, err := insertVersion(t, pool, templateID, contentWithoutSignerRoles,
		entity.VersionStatusPublished, 1)
	require.NoError(t, err, "a document declaring no signer roles must publish")
	t.Cleanup(func() { testhelper.CleanupTemplateVersion(t, pool, versionID) })
}

// The 20 versions already stuck in this state must remain editable so they can be
// repaired or archived; the guard only fires on the transition INTO published.
func TestSignerRoleGuard_DoesNotBlockRepairOfAlreadyPublishedRows(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	templateID := guardTestTemplate(t, pool)
	ctx := context.Background()

	// Reproduce the broken production state, which predates the trigger.
	versionID, err := insertVersion(t, pool, templateID, contentWithoutSignerRoles,
		entity.VersionStatusPublished, 1)
	require.NoError(t, err)
	t.Cleanup(func() { testhelper.CleanupTemplateVersion(t, pool, versionID) })
	_, err = pool.Exec(ctx, `UPDATE content.template_versions
		SET content_structure = $2::jsonb WHERE id = $1`,
		versionID, contentDeclaringSignerRoles)
	require.NoError(t, err, "a published row with no roles must still be writable")

	// Archiving it (the repair path) must not be blocked either.
	_, err = pool.Exec(ctx,
		`UPDATE content.template_versions SET status = 'ARCHIVED' WHERE id = $1`, versionID)
	require.NoError(t, err, "archiving a broken published version must be allowed")
}
