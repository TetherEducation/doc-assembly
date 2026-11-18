//go:build integration

package controller_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

// TestGalleryController_AssetTenantIsTheWorkspaces covers the tenant a gallery
// asset is stored under: the tenant of the workspace it is uploaded to,
// whatever X-Tenant-ID says, including nothing. The gallery routes run in
// workspace context, which does not check that header.
func TestGalleryController_AssetTenantIsTheWorkspaces(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)

	tenantID := testhelper.CreateTestTenant(t, pool, "Gallery Tenant Own", "GALT03")
	defer testhelper.CleanupTenant(t, pool, tenantID)
	otherTenantID := testhelper.CreateTestTenant(t, pool, "Gallery Tenant Other", "GALT04")
	defer testhelper.CleanupTenant(t, pool, otherTenantID)
	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Gallery Workspace", entity.WorkspaceTypeClient)

	editor := testhelper.CreateTestUser(t, pool, "gallery-tenant@test.com", "Gallery Editor", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	headers := map[string]string{
		"the workspace's tenant": tenantID,
		"another tenant":         otherTenantID,
		"no tenant":              "",
	}
	for name, tenantHeader := range headers {
		t.Run("X-Tenant-ID names "+name, func(t *testing.T) {
			resp, body := uploadGalleryAsset(t, ts.URL(), editor.BearerHeader, tenantHeader, workspaceID,
				name+".png", []byte("image uploaded with "+name), "image/png")
			require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
			uploaded := testhelper.ParseJSON[dto.GalleryAssetResponse](t, body)

			var storedTenantID string
			err := pool.QueryRow(context.Background(),
				`SELECT tenant_id FROM content.gallery_assets WHERE id = $1`, uploaded.ID).Scan(&storedTenantID)
			require.NoError(t, err)
			assert.Equal(t, tenantID, storedTenantID, "the asset is not stored under its workspace's tenant")
		})
	}
}
