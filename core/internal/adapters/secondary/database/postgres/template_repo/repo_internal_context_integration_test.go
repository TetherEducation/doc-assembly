//go:build integration

package templaterepo_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	templaterepo "github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/database/postgres/template_repo"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

func workspaceCode(t *testing.T, pool *pgxpool.Pool, workspaceID string) string {
	t.Helper()
	var code string
	err := pool.QueryRow(context.Background(),
		`SELECT code FROM tenancy.workspaces WHERE id = $1`, workspaceID).Scan(&code)
	require.NoError(t, err, "failed to read workspace code")
	return code
}

func setTemplateProcess(t *testing.T, pool *pgxpool.Pool, templateID, process string) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`UPDATE content.templates SET process = $2 WHERE id = $1`, templateID, process)
	require.NoError(t, err, "failed to set template process")
}

// A template whose stored process differs only in case still names the same process, so it
// must remain reachable. Comparing content.templates.process raw meant a template saved as
// lowercase 'default' matched neither the requested process nor the 'DEFAULT' branch: it
// was unreachable by every request, and its workspace silently fell through to the DEFAULT
// baseline. One production template was in exactly that state.
func TestFindInternalTemplateContext_ResolvesTemplateWithLowercaseProcess(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	repo := templaterepo.New(pool)
	ctx := context.Background()

	tenantID := testhelper.CreateTestTenant(t, pool, "Process Tenant", "PRCTEN")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, tenantID) })

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Process Workspace",
		entity.WorkspaceTypeClient)
	t.Cleanup(func() { testhelper.CleanupWorkspace(t, pool, workspaceID) })

	docTypeID := testhelper.CreateTestDocumentType(t, pool, tenantID,
		"ENROLLMENT_CONFIRMATION", "Comprobante de Matricula")
	t.Cleanup(func() { testhelper.CleanupDocumentType(t, pool, docTypeID) })

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Campus Comprobante", nil)
	t.Cleanup(func() { testhelper.CleanupTemplate(t, pool, templateID) })
	testhelper.SetTemplateDocumentType(t, pool, templateID, docTypeID)

	versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "v1.0",
		entity.VersionStatusPublished)
	t.Cleanup(func() { testhelper.CleanupTemplateVersion(t, pool, versionID) })

	code := workspaceCode(t, pool, workspaceID)
	published := true

	for _, stored := range []string{"default", "DEFAULT", "  default  "} {
		t.Run("stored as "+stored, func(t *testing.T) {
			setTemplateProcess(t, pool, templateID, stored)

			resolved, err := repo.FindInternalTemplateContext(ctx, port.InternalTemplateContextQuery{
				TenantCode:             "PRCTEN",
				RequestedWorkspaceCode: code,
				WorkspaceCodes:         []string{code},
				DocumentType:           "ENROLLMENT_CONFIRMATION",
				// A process the template is not keyed to, so only the 'DEFAULT' branch
				// can match it.
				Process:   "SAE_APP",
				Published: &published,
			})
			require.NoError(t, err, "template stored with process %q should still resolve", stored)
			require.NotNil(t, resolved)
			require.NotNil(t, resolved.Workspace)
			require.Equal(t, code, resolved.Workspace.Code,
				"should resolve to the campus workspace, not fall through")
			require.NotNil(t, resolved.Version)
			require.Equal(t, versionID, resolved.Version.ID)
		})
	}
}

// An exact process match must still outrank the DEFAULT baseline within a workspace, and
// case must not change that ranking.
func TestFindInternalTemplateContext_ExactProcessOutranksDefaultRegardlessOfCase(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	repo := templaterepo.New(pool)
	ctx := context.Background()

	tenantID := testhelper.CreateTestTenant(t, pool, "Rank Tenant", "RNKTEN")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, tenantID) })

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Rank Workspace",
		entity.WorkspaceTypeClient)
	t.Cleanup(func() { testhelper.CleanupWorkspace(t, pool, workspaceID) })

	docTypeID := testhelper.CreateTestDocumentType(t, pool, tenantID,
		"ENROLLMENT_CONFIRMATION", "Comprobante de Matricula")
	t.Cleanup(func() { testhelper.CleanupDocumentType(t, pool, docTypeID) })

	baseline := testhelper.CreateTestTemplate(t, pool, workspaceID, "Baseline", nil)
	t.Cleanup(func() { testhelper.CleanupTemplate(t, pool, baseline) })
	testhelper.SetTemplateDocumentType(t, pool, baseline, docTypeID)
	setTemplateProcess(t, pool, baseline, "default")
	baselineVersion := testhelper.CreateTestTemplateVersion(t, pool, baseline, 1, "v1.0",
		entity.VersionStatusPublished)
	t.Cleanup(func() { testhelper.CleanupTemplateVersion(t, pool, baselineVersion) })

	override := testhelper.CreateTestTemplate(t, pool, workspaceID, "SAE Override", nil)
	t.Cleanup(func() { testhelper.CleanupTemplate(t, pool, override) })
	testhelper.SetTemplateDocumentType(t, pool, override, docTypeID)
	setTemplateProcess(t, pool, override, "sae_app")
	overrideVersion := testhelper.CreateTestTemplateVersion(t, pool, override, 1, "v1.0",
		entity.VersionStatusPublished)
	t.Cleanup(func() { testhelper.CleanupTemplateVersion(t, pool, overrideVersion) })

	code := workspaceCode(t, pool, workspaceID)
	published := true

	resolved, err := repo.FindInternalTemplateContext(ctx, port.InternalTemplateContextQuery{
		TenantCode:             "RNKTEN",
		RequestedWorkspaceCode: code,
		WorkspaceCodes:         []string{code},
		DocumentType:           "ENROLLMENT_CONFIRMATION",
		Process:                "SAE_APP",
		Published:              &published,
	})
	require.NoError(t, err)
	require.NotNil(t, resolved)
	require.NotNil(t, resolved.Version)
	require.Equal(t, overrideVersion, resolved.Version.ID,
		"the process-specific template should win over the DEFAULT baseline")
}
