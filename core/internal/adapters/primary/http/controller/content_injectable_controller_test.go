//go:build integration

package controller_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

// =============================================================================
// Injectable List Tests
// =============================================================================

// TestContentInjectableController_ListInjectables tests the GET /content/injectables endpoint.
func TestContentInjectableController_ListInjectables(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup: tenant + workspace + users
	tenantID := testhelper.CreateTestTenant(t, pool, "Injectable List Tenant", "ILST01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Injectable List Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	owner := testhelper.CreateTestUser(t, pool, "owner-inj-list@test.com", "Owner User", nil)
	defer testhelper.CleanupUser(t, pool, owner.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

	viewer := testhelper.CreateTestUser(t, pool, "viewer-inj-list@test.com", "Viewer User", nil)
	defer testhelper.CleanupUser(t, pool, viewer.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

	// Create injectables
	injectable1 := testhelper.CreateTestInjectable(t, pool, &workspaceID, "customer_name", "Customer Name", entity.InjectableDataTypeText)
	defer testhelper.CleanupInjectable(t, pool, injectable1)

	injectable2 := testhelper.CreateTestInjectable(t, pool, &workspaceID, "total_amount", "Total Amount", entity.InjectableDataTypeNumber)
	defer testhelper.CleanupInjectable(t, pool, injectable2)

	t.Run("success with VIEWER", func(t *testing.T) {
		resp, body := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/content/injectables")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp dto.ListInjectablesResponse
		err := json.Unmarshal(body, &listResp)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, listResp.Total, 2)
	})

	t.Run("success with OWNER", func(t *testing.T) {
		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/content/injectables")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp dto.ListInjectablesResponse
		err := json.Unmarshal(body, &listResp)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, listResp.Total, 2)
	})

	t.Run("success with empty list", func(t *testing.T) {
		// Create a new workspace with no injectables
		emptyWsID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Empty Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, emptyWsID)
		testhelper.CreateTestWorkspaceMember(t, pool, emptyWsID, owner.ID, entity.WorkspaceRoleOwner, nil)

		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(emptyWsID).
			GET("/api/v1/content/injectables")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp dto.ListInjectablesResponse
		err := json.Unmarshal(body, &listResp)
		require.NoError(t, err)

		// May have global injectables but workspace-specific should be 0
		assert.NotNil(t, listResp.Items)
	})

	t.Run("forbidden without workspace membership", func(t *testing.T) {
		nonMember := testhelper.CreateTestUser(t, pool, "nonmember-inj@test.com", "Non Member", nil)
		defer testhelper.CleanupUser(t, pool, nonMember.ID)

		resp, _ := client.
			WithAuth(nonMember.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/content/injectables")

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("bad request without workspace header", func(t *testing.T) {
		resp, _ := client.
			WithAuth(owner.BearerHeader).
			GET("/api/v1/content/injectables")

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("unauthorized without token", func(t *testing.T) {
		resp, _ := client.
			WithWorkspaceID(workspaceID).
			GET("/api/v1/content/injectables")

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// =============================================================================
// Injectable Get Tests
// =============================================================================

// TestContentInjectableController_GetInjectable tests the GET /content/injectables/:injectableId endpoint.
func TestContentInjectableController_GetInjectable(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Get Injectable Tenant", "GIST01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Get Injectable Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	owner := testhelper.CreateTestUser(t, pool, "owner-gi@test.com", "Owner User", nil)
	defer testhelper.CleanupUser(t, pool, owner.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

	editor := testhelper.CreateTestUser(t, pool, "editor-gi@test.com", "Editor User", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	viewer := testhelper.CreateTestUser(t, pool, "viewer-gi@test.com", "Viewer User", nil)
	defer testhelper.CleanupUser(t, pool, viewer.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

	injectableID := testhelper.CreateTestInjectable(t, pool, &workspaceID, "get_test_key", "Get Test Injectable", entity.InjectableDataTypeText)
	defer testhelper.CleanupInjectable(t, pool, injectableID)

	t.Run("success with VIEWER", func(t *testing.T) {
		resp, body := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET(fmt.Sprintf("/api/v1/content/injectables/%s", injectableID))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var injResp dto.InjectableResponse
		err := json.Unmarshal(body, &injResp)
		require.NoError(t, err)

		assert.Equal(t, injectableID, injResp.ID)
		assert.Equal(t, "get_test_key", injResp.Key)
		assert.Equal(t, "Get Test Injectable", injResp.Label["_"])
	})

	t.Run("success with EDITOR", func(t *testing.T) {
		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET(fmt.Sprintf("/api/v1/content/injectables/%s", injectableID))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var injResp dto.InjectableResponse
		err := json.Unmarshal(body, &injResp)
		require.NoError(t, err)

		assert.Equal(t, injectableID, injResp.ID)
	})

	t.Run("success with OWNER", func(t *testing.T) {
		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET(fmt.Sprintf("/api/v1/content/injectables/%s", injectableID))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var injResp dto.InjectableResponse
		err := json.Unmarshal(body, &injResp)
		require.NoError(t, err)

		assert.Equal(t, injectableID, injResp.ID)
	})

	t.Run("not found - non-existent ID", func(t *testing.T) {
		resp, _ := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/content/injectables/00000000-0000-0000-0000-000000000000")

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("unauthorized without token", func(t *testing.T) {
		resp, _ := client.
			WithWorkspaceID(workspaceID).
			GET(fmt.Sprintf("/api/v1/content/injectables/%s", injectableID))

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}
