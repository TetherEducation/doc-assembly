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

// A retired workspace must stop serving. Its templates are not deleted when it is
// archived and its campus code is still first in WorkspaceCodes, so without a status
// filter it stays a priority-1 candidate and captures resolution for that campus --
// outranking the network and DEFAULT fallbacks that were meant to take over. Production
// had 1060800002 archived while holding three PUBLISHED versions left roleless by the
// 2026-08-14 publish bypass: every request for those document types at that campus
// resolved into the retired workspace and 422'd on recipient validation, with healthy
// templates sitting one priority behind it.
func TestFindInternalTemplateContext_ArchivedWorkspaceDoesNotCaptureResolution(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	repo := templaterepo.New(pool)
	ctx := context.Background()

	tenantID := testhelper.CreateTestTenant(t, pool, "Archived Tenant", "ARCTEN")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, tenantID) })

	docTypeID := testhelper.CreateTestDocumentType(t, pool, tenantID,
		"ENROLLMENT_CONFIRMATION", "Comprobante de Matricula")
	t.Cleanup(func() { testhelper.CleanupDocumentType(t, pool, docTypeID) })

	// The campus workspace, first in priority order, about to be retired.
	campusID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Retired Campus",
		entity.WorkspaceTypeClient)
	t.Cleanup(func() { testhelper.CleanupWorkspace(t, pool, campusID) })
	campusTemplate := testhelper.CreateTestTemplate(t, pool, campusID, "Campus Comprobante", nil)
	t.Cleanup(func() { testhelper.CleanupTemplate(t, pool, campusTemplate) })
	testhelper.SetTemplateDocumentType(t, pool, campusTemplate, docTypeID)
	campusVersion := testhelper.CreateTestTemplateVersion(t, pool, campusTemplate, 1, "v1.0",
		entity.VersionStatusPublished)
	t.Cleanup(func() { testhelper.CleanupTemplateVersion(t, pool, campusVersion) })

	// The fallback that should take over once the campus workspace is retired.
	fallbackID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Network Fallback",
		entity.WorkspaceTypeClient)
	t.Cleanup(func() { testhelper.CleanupWorkspace(t, pool, fallbackID) })
	fallbackTemplate := testhelper.CreateTestTemplate(t, pool, fallbackID, "Network Comprobante", nil)
	t.Cleanup(func() { testhelper.CleanupTemplate(t, pool, fallbackTemplate) })
	testhelper.SetTemplateDocumentType(t, pool, fallbackTemplate, docTypeID)
	fallbackVersion := testhelper.CreateTestTemplateVersion(t, pool, fallbackTemplate, 1, "v1.0",
		entity.VersionStatusPublished)
	t.Cleanup(func() { testhelper.CleanupTemplateVersion(t, pool, fallbackVersion) })

	campusCode := workspaceCode(t, pool, campusID)
	fallbackCode := workspaceCode(t, pool, fallbackID)
	published := true

	query := port.InternalTemplateContextQuery{
		TenantCode:             "ARCTEN",
		RequestedWorkspaceCode: campusCode,
		WorkspaceCodes:         []string{campusCode, fallbackCode},
		DocumentType:           "ENROLLMENT_CONFIRMATION",
		Published:              &published,
	}

	// Control: while the campus workspace is ACTIVE it must win on priority, so a pass
	// below cannot come from the campus template being unreachable for some other reason.
	resolved, err := repo.FindInternalTemplateContext(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	require.NotNil(t, resolved.Version)
	require.Equal(t, campusVersion, resolved.Version.ID,
		"an active campus workspace should win on priority")

	testhelper.UpdateWorkspaceStatus(t, pool, campusID, entity.WorkspaceStatusArchived)

	resolved, err = repo.FindInternalTemplateContext(ctx, query)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	require.NotNil(t, resolved.Workspace)
	require.Equal(t, fallbackCode, resolved.Workspace.Code,
		"an archived workspace must not capture resolution for its campus")
	require.NotNil(t, resolved.Version)
	require.Equal(t, fallbackVersion, resolved.Version.ID,
		"resolution should fall through to the next workspace in priority order")
}

// SUSPENDED is a temporary state, not a retirement, so it must keep serving. Pinned so
// the archived filter above is not later widened to w.status = 'ACTIVE' without someone
// deciding that deliberately.
func TestFindInternalTemplateContext_SuspendedWorkspaceStillResolves(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	repo := templaterepo.New(pool)
	ctx := context.Background()

	tenantID := testhelper.CreateTestTenant(t, pool, "Suspended Tenant", "SUSTEN")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, tenantID) })

	docTypeID := testhelper.CreateTestDocumentType(t, pool, tenantID,
		"ENROLLMENT_CONFIRMATION", "Comprobante de Matricula")
	t.Cleanup(func() { testhelper.CleanupDocumentType(t, pool, docTypeID) })

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Suspended Campus",
		entity.WorkspaceTypeClient)
	t.Cleanup(func() { testhelper.CleanupWorkspace(t, pool, workspaceID) })
	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Campus Comprobante", nil)
	t.Cleanup(func() { testhelper.CleanupTemplate(t, pool, templateID) })
	testhelper.SetTemplateDocumentType(t, pool, templateID, docTypeID)
	versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "v1.0",
		entity.VersionStatusPublished)
	t.Cleanup(func() { testhelper.CleanupTemplateVersion(t, pool, versionID) })

	testhelper.UpdateWorkspaceStatus(t, pool, workspaceID, entity.WorkspaceStatusSuspended)

	code := workspaceCode(t, pool, workspaceID)
	published := true

	resolved, err := repo.FindInternalTemplateContext(ctx, port.InternalTemplateContextQuery{
		TenantCode:             "SUSTEN",
		RequestedWorkspaceCode: code,
		WorkspaceCodes:         []string{code},
		DocumentType:           "ENROLLMENT_CONFIRMATION",
		Published:              &published,
	})
	require.NoError(t, err)
	require.NotNil(t, resolved)
	require.NotNil(t, resolved.Version)
	require.Equal(t, versionID, resolved.Version.ID,
		"a suspended workspace should still serve its own template")
}
