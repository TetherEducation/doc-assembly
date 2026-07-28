//go:build integration

package controller_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	tenantrepo "github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/database/postgres/tenant_repo"
	workspacerepo "github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/database/postgres/workspace_repo"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

// =============================================================================
// Workspace Operations Tests
// =============================================================================

// TestWorkspaceController_GetWorkspace tests the GET /workspace endpoint.
func TestWorkspaceController_GetWorkspace(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup: tenant + workspace + users
	tenantID := testhelper.CreateTestTenant(t, pool, "Test Tenant", "GWST01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Test Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	owner := testhelper.CreateTestUser(t, pool, "owner-ws@test.com", "Owner User", nil)
	defer testhelper.CleanupUser(t, pool, owner.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

	tenantOwner := testhelper.CreateTestUser(t, pool, "tenant-owner-ws@test.com", "Tenant Owner", nil)
	defer testhelper.CleanupUser(t, pool, tenantOwner.ID)
	testhelper.CreateTestTenantMember(t, pool, tenantID, tenantOwner.ID, entity.TenantRoleOwner, nil)

	tenantAdmin := testhelper.CreateTestUser(t, pool, "tenant-admin-ws@test.com", "Tenant Admin", nil)
	defer testhelper.CleanupUser(t, pool, tenantAdmin.ID)
	testhelper.CreateTestTenantMember(t, pool, tenantID, tenantAdmin.ID, entity.TenantRoleAdmin, nil)

	superAdminRole := entity.SystemRoleSuperAdmin
	superAdmin := testhelper.CreateTestUser(t, pool, "superadmin-get-ws@test.com", "Super Admin", &superAdminRole)
	defer testhelper.CleanupUser(t, pool, superAdmin.ID)

	viewer := testhelper.CreateTestUser(t, pool, "viewer-ws@test.com", "Viewer User", nil)
	defer testhelper.CleanupUser(t, pool, viewer.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

	t.Run("success with VIEWER", func(t *testing.T) {
		resp, body := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/workspace")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var wsResp dto.WorkspaceResponse
		err := json.Unmarshal(body, &wsResp)
		require.NoError(t, err)

		assert.Equal(t, workspaceID, wsResp.ID)
		assert.Equal(t, "Test Workspace", wsResp.Name)
		assert.Equal(t, "VIEWER", wsResp.Role)
	})

	t.Run("success with OWNER", func(t *testing.T) {
		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/workspace")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var wsResp dto.WorkspaceResponse
		err := json.Unmarshal(body, &wsResp)
		require.NoError(t, err)

		assert.Equal(t, workspaceID, wsResp.ID)
		assert.Equal(t, "OWNER", wsResp.Role)
	})

	t.Run("tenant owner inherits OWNER workspace access without membership", func(t *testing.T) {
		resp, body := client.
			WithAuth(tenantOwner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/workspace")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var wsResp dto.WorkspaceResponse
		err := json.Unmarshal(body, &wsResp)
		require.NoError(t, err)

		assert.Equal(t, workspaceID, wsResp.ID)
		assert.Equal(t, "OWNER", wsResp.Role)
	})

	t.Run("superadmin inherits OWNER workspace access without membership", func(t *testing.T) {
		resp, body := client.
			WithAuth(superAdmin.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/workspace")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var wsResp dto.WorkspaceResponse
		err := json.Unmarshal(body, &wsResp)
		require.NoError(t, err)

		assert.Equal(t, workspaceID, wsResp.ID)
		assert.Equal(t, "OWNER", wsResp.Role)
	})

	t.Run("tenant admin does not inherit workspace access", func(t *testing.T) {
		resp, _ := client.
			WithAuth(tenantAdmin.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/workspace")

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("forbidden without membership", func(t *testing.T) {
		nonMember := testhelper.CreateTestUser(t, pool, "nonmember-ws@test.com", "Non Member", nil)
		defer testhelper.CleanupUser(t, pool, nonMember.ID)

		resp, _ := client.
			WithAuth(nonMember.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/workspace")

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("bad request without workspace header", func(t *testing.T) {
		resp, _ := client.
			WithAuth(owner.BearerHeader).
			GET("/api/v1/workspace")

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// TestWorkspaceController_UpdateWorkspace tests the PUT /workspace endpoint.
func TestWorkspaceController_UpdateWorkspace(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success with ADMIN", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Tenant", "UWST01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Original Name", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		admin := testhelper.CreateTestUser(t, pool, "admin-upd@test.com", "Admin User", nil)
		defer testhelper.CleanupUser(t, pool, admin.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

		resp, body := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT("/api/v1/workspace", map[string]interface{}{
				"name": "Updated Name",
			})

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var wsResp dto.WorkspaceResponse
		err := json.Unmarshal(body, &wsResp)
		require.NoError(t, err)

		assert.Equal(t, "Updated Name", wsResp.Name)
	})

	t.Run("forbidden with EDITOR", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Tenant 2", "UWST02")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-upd@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT("/api/v1/workspace", map[string]interface{}{
				"name": "New Name",
			})

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("forbidden with VIEWER", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Tenant 3", "UWST03")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		viewer := testhelper.CreateTestUser(t, pool, "viewer-upd@test.com", "Viewer User", nil)
		defer testhelper.CleanupUser(t, pool, viewer.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

		resp, _ := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT("/api/v1/workspace", map[string]interface{}{
				"name": "New Name",
			})

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}

// TestWorkspaceController_ArchiveWorkspace tests the DELETE /workspace endpoint.
func TestWorkspaceController_ArchiveWorkspace(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success with OWNER", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Archive Tenant", "AWST01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "To Archive", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		owner := testhelper.CreateTestUser(t, pool, "owner-arch@test.com", "Owner User", nil)
		defer testhelper.CleanupUser(t, pool, owner.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

		resp, _ := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE("/api/v1/workspace")

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("forbidden with ADMIN", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Archive Tenant 2", "AWST02")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		admin := testhelper.CreateTestUser(t, pool, "admin-arch@test.com", "Admin User", nil)
		defer testhelper.CleanupUser(t, pool, admin.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE("/api/v1/workspace")

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}

// =============================================================================
// Workspace Member Tests
// =============================================================================

// TestWorkspaceController_ListMembers tests the GET /workspace/members endpoint.
func TestWorkspaceController_ListMembers(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Members Tenant", "LMST01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Members Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	owner := testhelper.CreateTestUser(t, pool, "owner-mem@test.com", "Owner User", nil)
	defer testhelper.CleanupUser(t, pool, owner.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

	member := testhelper.CreateTestUser(t, pool, "member-mem@test.com", "Member User", nil)
	defer testhelper.CleanupUser(t, pool, member.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, member.ID, entity.WorkspaceRoleEditor, nil)

	t.Run("success with VIEWER", func(t *testing.T) {
		viewer := testhelper.CreateTestUser(t, pool, "viewer-mem@test.com", "Viewer User", nil)
		defer testhelper.CleanupUser(t, pool, viewer.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

		resp, body := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/workspace/members")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp dto.ListResponse[dto.MemberResponse]
		err := json.Unmarshal(body, &listResp)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, listResp.Count, 3)
	})
}

// TestWorkspaceController_InviteMember tests the POST /workspace/members endpoint.
func TestWorkspaceController_InviteMember(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success with ADMIN", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Invite Tenant", "IMST01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Invite Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		admin := testhelper.CreateTestUser(t, pool, "admin-inv@test.com", "Admin User", nil)
		defer testhelper.CleanupUser(t, pool, admin.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

		invitee := testhelper.CreateTestUser(t, pool, "invitee@test.com", "Invitee User", nil)
		defer testhelper.CleanupUser(t, pool, invitee.ID)

		resp, body := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/workspace/members", map[string]interface{}{
				"email": invitee.Email,
				"role":  "EDITOR",
			})

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var memberResp dto.MemberResponse
		err := json.Unmarshal(body, &memberResp)
		require.NoError(t, err)

		assert.Equal(t, "EDITOR", memberResp.Role)
	})

	t.Run("forbidden with EDITOR", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Invite Tenant 2", "IMST02")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-inv@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		invitee := testhelper.CreateTestUser(t, pool, "invitee2@test.com", "Invitee User", nil)
		defer testhelper.CleanupUser(t, pool, invitee.ID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/workspace/members", map[string]interface{}{
				"email": invitee.Email,
				"role":  "VIEWER",
			})

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("conflict when member already exists", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Invite Tenant 3", "IMST03")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		admin := testhelper.CreateTestUser(t, pool, "admin-inv3@test.com", "Admin User", nil)
		defer testhelper.CleanupUser(t, pool, admin.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

		existingMember := testhelper.CreateTestUser(t, pool, "existing@test.com", "Existing User", nil)
		defer testhelper.CleanupUser(t, pool, existingMember.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, existingMember.ID, entity.WorkspaceRoleViewer, nil)

		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/workspace/members", map[string]interface{}{
				"email": existingMember.Email,
				"role":  "EDITOR",
			})

		assert.Equal(t, http.StatusConflict, resp.StatusCode)
	})
}

// TestWorkspaceController_GetMember tests the GET /workspace/members/:memberId endpoint.
func TestWorkspaceController_GetMember(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Get Member Tenant", "GMST01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Get Member Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	owner := testhelper.CreateTestUser(t, pool, "owner-gm@test.com", "Owner User", nil)
	defer testhelper.CleanupUser(t, pool, owner.ID)
	memberID := testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

	t.Run("success", func(t *testing.T) {
		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET(fmt.Sprintf("/api/v1/workspace/members/%s", memberID))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var memberResp dto.MemberResponse
		err := json.Unmarshal(body, &memberResp)
		require.NoError(t, err)

		assert.Equal(t, memberID, memberResp.ID)
	})

	t.Run("not found", func(t *testing.T) {
		resp, _ := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/workspace/members/00000000-0000-0000-0000-000000000000")

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// TestWorkspaceController_UpdateMemberRole tests the PUT /workspace/members/:memberId endpoint.
func TestWorkspaceController_UpdateMemberRole(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success with OWNER", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Role Tenant", "UMST01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Update Role Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		owner := testhelper.CreateTestUser(t, pool, "owner-ur@test.com", "Owner User", nil)
		defer testhelper.CleanupUser(t, pool, owner.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

		member := testhelper.CreateTestUser(t, pool, "member-ur@test.com", "Member User", nil)
		defer testhelper.CleanupUser(t, pool, member.ID)
		memberID := testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, member.ID, entity.WorkspaceRoleViewer, nil)

		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT(fmt.Sprintf("/api/v1/workspace/members/%s", memberID), map[string]interface{}{
				"role": "ADMIN",
			})

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var memberResp dto.MemberResponse
		err := json.Unmarshal(body, &memberResp)
		require.NoError(t, err)

		assert.Equal(t, "ADMIN", memberResp.Role)
	})

	t.Run("forbidden with ADMIN", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Role Tenant 2", "UMST02")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		admin := testhelper.CreateTestUser(t, pool, "admin-ur@test.com", "Admin User", nil)
		defer testhelper.CleanupUser(t, pool, admin.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

		member := testhelper.CreateTestUser(t, pool, "member-ur2@test.com", "Member User", nil)
		defer testhelper.CleanupUser(t, pool, member.ID)
		memberID := testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, member.ID, entity.WorkspaceRoleViewer, nil)

		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT(fmt.Sprintf("/api/v1/workspace/members/%s", memberID), map[string]interface{}{
				"role": "EDITOR",
			})

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}

// TestWorkspaceController_RemoveMember tests the DELETE /workspace/members/:memberId endpoint.
func TestWorkspaceController_RemoveMember(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success with ADMIN", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Remove Member Tenant", "RMST01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Remove Member Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		admin := testhelper.CreateTestUser(t, pool, "admin-rm@test.com", "Admin User", nil)
		defer testhelper.CleanupUser(t, pool, admin.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

		memberToRemove := testhelper.CreateTestUser(t, pool, "toremove@test.com", "To Remove", nil)
		defer testhelper.CleanupUser(t, pool, memberToRemove.ID)
		memberID := testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, memberToRemove.ID, entity.WorkspaceRoleViewer, nil)

		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE(fmt.Sprintf("/api/v1/workspace/members/%s", memberID))

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("cannot remove OWNER", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Remove Owner Tenant", "RMST02")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		owner := testhelper.CreateTestUser(t, pool, "owner-rm@test.com", "Owner User", nil)
		defer testhelper.CleanupUser(t, pool, owner.ID)
		ownerMemberID := testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

		admin := testhelper.CreateTestUser(t, pool, "admin-rm2@test.com", "Admin User", nil)
		defer testhelper.CleanupUser(t, pool, admin.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE(fmt.Sprintf("/api/v1/workspace/members/%s", ownerMemberID))

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("superadmin can remove legacy OWNER member", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Remove Owner Tenant 2", "RMST03")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		owner := testhelper.CreateTestUser(t, pool, "owner-rm3@test.com", "Owner User", nil)
		defer testhelper.CleanupUser(t, pool, owner.ID)
		ownerMemberID := testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

		superAdminRole := entity.SystemRoleSuperAdmin
		superAdmin := testhelper.CreateTestUser(t, pool, "superadmin-rm@test.com", "Super Admin", &superAdminRole)
		defer testhelper.CleanupUser(t, pool, superAdmin.ID)

		resp, _ := client.
			WithAuth(superAdmin.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE(fmt.Sprintf("/api/v1/workspace/members/%s", ownerMemberID))

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("tenant owner can remove legacy OWNER member in tenant system workspace", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Remove Owner Tenant 4", "RMST04")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		systemWorkspace, err := workspacerepo.New(pool).FindSystemByTenant(context.Background(), &tenantID)
		require.NoError(t, err)

		owner := testhelper.CreateTestUser(t, pool, "owner-rm4@test.com", "Owner User", nil)
		defer testhelper.CleanupUser(t, pool, owner.ID)
		ownerMemberID := testhelper.CreateTestWorkspaceMember(t, pool, systemWorkspace.ID, owner.ID, entity.WorkspaceRoleOwner, nil)

		tenantOwner := testhelper.CreateTestUser(t, pool, "tenant-owner-rm@test.com", "Tenant Owner", nil)
		defer testhelper.CleanupUser(t, pool, tenantOwner.ID)
		testhelper.CreateTestTenantMember(t, pool, tenantID, tenantOwner.ID, entity.TenantRoleOwner, nil)

		resp, _ := client.
			WithAuth(tenantOwner.BearerHeader).
			WithWorkspaceID(systemWorkspace.ID).
			DELETE(fmt.Sprintf("/api/v1/workspace/members/%s", ownerMemberID))

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("global system blocks removing last direct member", func(t *testing.T) {
		systemTenant, err := tenantrepo.New(pool).FindSystemTenant(context.Background())
		require.NoError(t, err)

		globalSystemWorkspace, err := workspacerepo.New(pool).FindSystemByTenant(context.Background(), &systemTenant.ID)
		require.NoError(t, err)

		superAdminRole := entity.SystemRoleSuperAdmin
		superAdmin := testhelper.CreateTestUser(t, pool, "superadmin-global-rm@test.com", "Super Admin", &superAdminRole)
		defer testhelper.CleanupUser(t, pool, superAdmin.ID)

		platformAdminRole := entity.SystemRolePlatformAdmin
		lastDirectMember := testhelper.CreateTestUser(t, pool, "platformadmin-last@test.com", "Platform Admin Last", &platformAdminRole)
		defer testhelper.CleanupUser(t, pool, lastDirectMember.ID)

		var memberID string
		err = pool.QueryRow(context.Background(), `
			SELECT id
			FROM identity.workspace_members
			WHERE workspace_id = $1 AND user_id = $2
		`, globalSystemWorkspace.ID, lastDirectMember.ID).Scan(&memberID)
		require.NoError(t, err)

		resp, body := client.
			WithAuth(superAdmin.BearerHeader).
			WithWorkspaceID(globalSystemWorkspace.ID).
			DELETE(fmt.Sprintf("/api/v1/workspace/members/%s", memberID))

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		assert.Contains(t, string(body), entity.ErrCannotRemoveLastSystemMember.Error())
	})

	t.Run("global system allows removing member when another direct member remains", func(t *testing.T) {
		systemTenant, err := tenantrepo.New(pool).FindSystemTenant(context.Background())
		require.NoError(t, err)

		globalSystemWorkspace, err := workspacerepo.New(pool).FindSystemByTenant(context.Background(), &systemTenant.ID)
		require.NoError(t, err)

		superAdminRole := entity.SystemRoleSuperAdmin
		superAdmin := testhelper.CreateTestUser(t, pool, "superadmin-global-rm2@test.com", "Super Admin", &superAdminRole)
		defer testhelper.CleanupUser(t, pool, superAdmin.ID)

		platformAdminRole := entity.SystemRolePlatformAdmin
		memberOne := testhelper.CreateTestUser(t, pool, "platformadmin-one@test.com", "Platform Admin One", &platformAdminRole)
		defer testhelper.CleanupUser(t, pool, memberOne.ID)
		memberTwo := testhelper.CreateTestUser(t, pool, "platformadmin-two@test.com", "Platform Admin Two", &platformAdminRole)
		defer testhelper.CleanupUser(t, pool, memberTwo.ID)

		var memberID string
		err = pool.QueryRow(context.Background(), `
			SELECT id
			FROM identity.workspace_members
			WHERE workspace_id = $1 AND user_id = $2
		`, globalSystemWorkspace.ID, memberOne.ID).Scan(&memberID)
		require.NoError(t, err)

		resp, _ := client.
			WithAuth(superAdmin.BearerHeader).
			WithWorkspaceID(globalSystemWorkspace.ID).
			DELETE(fmt.Sprintf("/api/v1/workspace/members/%s", memberID))

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("tenant admin cannot remove last direct member from tenant system workspace without inherited access", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Remove Owner Tenant 5", "RMST05")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		systemWorkspace, err := workspacerepo.New(pool).FindSystemByTenant(context.Background(), &tenantID)
		require.NoError(t, err)

		lastDirectMember := testhelper.CreateTestUser(t, pool, "system-last-direct@test.com", "System Last Direct", nil)
		defer testhelper.CleanupUser(t, pool, lastDirectMember.ID)
		memberID := testhelper.CreateTestWorkspaceMember(t, pool, systemWorkspace.ID, lastDirectMember.ID, entity.WorkspaceRoleViewer, nil)

		tenantAdmin := testhelper.CreateTestUser(t, pool, "tenant-admin-rm@test.com", "Tenant Admin", nil)
		defer testhelper.CleanupUser(t, pool, tenantAdmin.ID)
		testhelper.CreateTestTenantMember(t, pool, tenantID, tenantAdmin.ID, entity.TenantRoleAdmin, nil)

		resp, _ := client.
			WithAuth(tenantAdmin.BearerHeader).
			WithWorkspaceID(systemWorkspace.ID).
			DELETE(fmt.Sprintf("/api/v1/workspace/members/%s", memberID))

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}

// =============================================================================
// Folder Tests
// =============================================================================

// TestWorkspaceController_ListFolders tests the GET /workspace/folders endpoint.
func TestWorkspaceController_ListFolders(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Folders Tenant", "LFST01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Folders Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	owner := testhelper.CreateTestUser(t, pool, "owner-fld@test.com", "Owner User", nil)
	defer testhelper.CleanupUser(t, pool, owner.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

	// Create folders
	folder1 := testhelper.CreateTestFolder(t, pool, workspaceID, "Folder 1", nil)
	defer testhelper.CleanupFolder(t, pool, folder1)

	folder2 := testhelper.CreateTestFolder(t, pool, workspaceID, "Folder 2", nil)
	defer testhelper.CleanupFolder(t, pool, folder2)

	t.Run("success with folders", func(t *testing.T) {
		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/workspace/folders")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp dto.ListResponse[dto.FolderResponse]
		err := json.Unmarshal(body, &listResp)
		require.NoError(t, err)

		assert.Equal(t, 2, listResp.Count)
	})

	t.Run("success empty workspace", func(t *testing.T) {
		emptyWsID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Empty Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, emptyWsID)
		testhelper.CreateTestWorkspaceMember(t, pool, emptyWsID, owner.ID, entity.WorkspaceRoleOwner, nil)

		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(emptyWsID).
			GET("/api/v1/workspace/folders")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp dto.ListResponse[dto.FolderResponse]
		err := json.Unmarshal(body, &listResp)
		require.NoError(t, err)

		assert.Equal(t, 0, listResp.Count)
	})
}

// TestWorkspaceController_GetFolderTree tests the GET /workspace/folders/tree endpoint.
func TestWorkspaceController_GetFolderTree(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Tree Tenant", "FTST01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Tree Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	owner := testhelper.CreateTestUser(t, pool, "owner-tree@test.com", "Owner User", nil)
	defer testhelper.CleanupUser(t, pool, owner.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

	// Create hierarchical folders
	parentFolder := testhelper.CreateTestFolder(t, pool, workspaceID, "Parent", nil)
	defer testhelper.CleanupFolder(t, pool, parentFolder)

	childFolder := testhelper.CreateTestFolder(t, pool, workspaceID, "Child", &parentFolder)
	defer testhelper.CleanupFolder(t, pool, childFolder)

	t.Run("success with tree structure", func(t *testing.T) {
		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/workspace/folders/tree")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var treeResp []dto.FolderTreeResponse
		err := json.Unmarshal(body, &treeResp)
		require.NoError(t, err)

		// Root level should have the parent folder
		assert.Len(t, treeResp, 1)
		assert.Equal(t, "Parent", treeResp[0].Name)
		assert.Len(t, treeResp[0].Children, 1)
		assert.Equal(t, "Child", treeResp[0].Children[0].Name)
	})
}

// TestWorkspaceController_CreateFolder tests the POST /workspace/folders endpoint.
func TestWorkspaceController_CreateFolder(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success with EDITOR", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Create Folder Tenant", "CFST01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Create Folder Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-cf@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/workspace/folders", map[string]interface{}{
				"name": "New Folder",
			})

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var folderResp dto.FolderResponse
		err := json.Unmarshal(body, &folderResp)
		require.NoError(t, err)

		assert.Equal(t, "New Folder", folderResp.Name)
		defer testhelper.CleanupFolder(t, pool, folderResp.ID)
	})

	t.Run("forbidden with VIEWER", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Create Folder Tenant 2", "CFST02")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		viewer := testhelper.CreateTestUser(t, pool, "viewer-cf@test.com", "Viewer User", nil)
		defer testhelper.CleanupUser(t, pool, viewer.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

		resp, _ := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/workspace/folders", map[string]interface{}{
				"name": "New Folder",
			})

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("validation error with empty name", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Create Folder Tenant 3", "CFST03")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-cf2@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/workspace/folders", map[string]interface{}{
				"name": "",
			})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// TestWorkspaceController_GetFolder tests the GET /workspace/folders/:folderId endpoint.
func TestWorkspaceController_GetFolder(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Get Folder Tenant", "GFST01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Get Folder Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	owner := testhelper.CreateTestUser(t, pool, "owner-gf@test.com", "Owner User", nil)
	defer testhelper.CleanupUser(t, pool, owner.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

	folderID := testhelper.CreateTestFolder(t, pool, workspaceID, "Test Folder", nil)
	defer testhelper.CleanupFolder(t, pool, folderID)

	t.Run("success", func(t *testing.T) {
		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET(fmt.Sprintf("/api/v1/workspace/folders/%s", folderID))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var folderResp dto.FolderResponse
		err := json.Unmarshal(body, &folderResp)
		require.NoError(t, err)

		assert.Equal(t, folderID, folderResp.ID)
		assert.Equal(t, "Test Folder", folderResp.Name)
	})

	t.Run("not found", func(t *testing.T) {
		resp, _ := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/workspace/folders/00000000-0000-0000-0000-000000000000")

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// TestWorkspaceController_UpdateFolder tests the PUT /workspace/folders/:folderId endpoint.
func TestWorkspaceController_UpdateFolder(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success with EDITOR", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Folder Tenant", "UFST01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Update Folder Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-uf@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		folderID := testhelper.CreateTestFolder(t, pool, workspaceID, "Original Name", nil)
		defer testhelper.CleanupFolder(t, pool, folderID)

		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT(fmt.Sprintf("/api/v1/workspace/folders/%s", folderID), map[string]interface{}{
				"name": "Updated Name",
			})

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var folderResp dto.FolderResponse
		err := json.Unmarshal(body, &folderResp)
		require.NoError(t, err)

		assert.Equal(t, "Updated Name", folderResp.Name)
	})
}

// TestWorkspaceController_DeleteFolder tests the DELETE /workspace/folders/:folderId endpoint.
func TestWorkspaceController_DeleteFolder(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success with ADMIN", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Delete Folder Tenant", "DFST01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Delete Folder Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		admin := testhelper.CreateTestUser(t, pool, "admin-df@test.com", "Admin User", nil)
		defer testhelper.CleanupUser(t, pool, admin.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

		folderID := testhelper.CreateTestFolder(t, pool, workspaceID, "To Delete", nil)
		// No defer cleanup - we're deleting it

		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE(fmt.Sprintf("/api/v1/workspace/folders/%s", folderID))

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("forbidden with EDITOR", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Delete Folder Tenant 2", "DFST02")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-df@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		folderID := testhelper.CreateTestFolder(t, pool, workspaceID, "Folder", nil)
		defer testhelper.CleanupFolder(t, pool, folderID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE(fmt.Sprintf("/api/v1/workspace/folders/%s", folderID))

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("cannot delete folder with children", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Delete Folder Tenant 3", "DFST03")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		admin := testhelper.CreateTestUser(t, pool, "admin-df2@test.com", "Admin User", nil)
		defer testhelper.CleanupUser(t, pool, admin.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

		parentFolder := testhelper.CreateTestFolder(t, pool, workspaceID, "Parent", nil)
		defer testhelper.CleanupFolder(t, pool, parentFolder)

		childFolder := testhelper.CreateTestFolder(t, pool, workspaceID, "Child", &parentFolder)
		defer testhelper.CleanupFolder(t, pool, childFolder)

		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE(fmt.Sprintf("/api/v1/workspace/folders/%s", parentFolder))

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// =============================================================================
// Tag Tests
// =============================================================================

// TestWorkspaceController_ListTags tests the GET /workspace/tags endpoint.
func TestWorkspaceController_ListTags(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Tags Tenant", "LTST01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Tags Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	owner := testhelper.CreateTestUser(t, pool, "owner-tag@test.com", "Owner User", nil)
	defer testhelper.CleanupUser(t, pool, owner.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

	// Create tags
	tag1 := testhelper.CreateTestTag(t, pool, workspaceID, "Tag 1", "#FF0000")
	defer testhelper.CleanupTag(t, pool, tag1)

	tag2 := testhelper.CreateTestTag(t, pool, workspaceID, "Tag 2", "#00FF00")
	defer testhelper.CleanupTag(t, pool, tag2)

	t.Run("success with tags", func(t *testing.T) {
		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/workspace/tags")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp dto.ListResponse[dto.TagWithCountResponse]
		err := json.Unmarshal(body, &listResp)
		require.NoError(t, err)

		assert.Equal(t, 2, listResp.Count)
	})
}

// TestWorkspaceController_CreateTag tests the POST /workspace/tags endpoint.
func TestWorkspaceController_CreateTag(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success with EDITOR", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Create Tag Tenant", "CTST01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Create Tag Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-ct@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/workspace/tags", map[string]interface{}{
				"name":  "New Tag",
				"color": "#3B82F6",
			})

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var tagResp dto.TagResponse
		err := json.Unmarshal(body, &tagResp)
		require.NoError(t, err)

		assert.Equal(t, "new_tag", tagResp.Name)
		assert.Equal(t, "#3B82F6", tagResp.Color)
		defer testhelper.CleanupTag(t, pool, tagResp.ID)
	})

	t.Run("forbidden with VIEWER", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Create Tag Tenant 2", "CTST02")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		viewer := testhelper.CreateTestUser(t, pool, "viewer-ct@test.com", "Viewer User", nil)
		defer testhelper.CleanupUser(t, pool, viewer.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

		resp, _ := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/workspace/tags", map[string]interface{}{
				"name":  "New Tag",
				"color": "#FF0000",
			})

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("validation error with invalid color", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Create Tag Tenant 3", "CTST03")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-ct2@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/workspace/tags", map[string]interface{}{
				"name":  "Tag",
				"color": "invalid",
			})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// TestWorkspaceController_GetTag tests the GET /workspace/tags/:tagId endpoint.
func TestWorkspaceController_GetTag(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Get Tag Tenant", "GTST01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Get Tag Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	owner := testhelper.CreateTestUser(t, pool, "owner-gt@test.com", "Owner User", nil)
	defer testhelper.CleanupUser(t, pool, owner.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

	tagID := testhelper.CreateTestTag(t, pool, workspaceID, "Test Tag", "#FF0000")
	defer testhelper.CleanupTag(t, pool, tagID)

	t.Run("success", func(t *testing.T) {
		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET(fmt.Sprintf("/api/v1/workspace/tags/%s", tagID))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var tagResp dto.TagResponse
		err := json.Unmarshal(body, &tagResp)
		require.NoError(t, err)

		assert.Equal(t, tagID, tagResp.ID)
		assert.Equal(t, "Test Tag", tagResp.Name)
		assert.Equal(t, "#FF0000", tagResp.Color)
	})

	t.Run("not found", func(t *testing.T) {
		resp, _ := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/workspace/tags/00000000-0000-0000-0000-000000000000")

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// TestWorkspaceController_UpdateTag tests the PUT /workspace/tags/:tagId endpoint.
func TestWorkspaceController_UpdateTag(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success with EDITOR", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Tag Tenant", "UTST01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Update Tag Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-ut@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		tagID := testhelper.CreateTestTag(t, pool, workspaceID, "Original", "#FF0000")
		defer testhelper.CleanupTag(t, pool, tagID)

		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT(fmt.Sprintf("/api/v1/workspace/tags/%s", tagID), map[string]interface{}{
				"name":  "Updated",
				"color": "#00FF00",
			})

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var tagResp dto.TagResponse
		err := json.Unmarshal(body, &tagResp)
		require.NoError(t, err)

		assert.Equal(t, "updated", tagResp.Name)
		assert.Equal(t, "#00FF00", tagResp.Color)
	})
}

// TestWorkspaceController_DeleteTag tests the DELETE /workspace/tags/:tagId endpoint.
func TestWorkspaceController_DeleteTag(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success with ADMIN", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Delete Tag Tenant", "DTST01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Delete Tag Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		admin := testhelper.CreateTestUser(t, pool, "admin-dt@test.com", "Admin User", nil)
		defer testhelper.CleanupUser(t, pool, admin.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

		tagID := testhelper.CreateTestTag(t, pool, workspaceID, "To Delete", "#FF0000")
		// No defer cleanup - we're deleting it

		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE(fmt.Sprintf("/api/v1/workspace/tags/%s", tagID))

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("forbidden with EDITOR", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Delete Tag Tenant 2", "DTST02")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-dt@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		tagID := testhelper.CreateTestTag(t, pool, workspaceID, "Tag", "#FF0000")
		defer testhelper.CleanupTag(t, pool, tagID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE(fmt.Sprintf("/api/v1/workspace/tags/%s", tagID))

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("not found", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Delete Tag Tenant 3", "DTST03")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		admin := testhelper.CreateTestUser(t, pool, "admin-dt3@test.com", "Admin User", nil)
		defer testhelper.CleanupUser(t, pool, admin.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE("/api/v1/workspace/tags/00000000-0000-0000-0000-000000000000")

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// =============================================================================
// MoveFolder Tests
// =============================================================================

// TestWorkspaceController_MoveFolder tests the PATCH /workspace/folders/:folderId/move endpoint.
func TestWorkspaceController_MoveFolder(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success move to root", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Move Folder Tenant", "MFST01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Move Folder Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-mf@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		// Create parent and child folder
		parentFolder := testhelper.CreateTestFolder(t, pool, workspaceID, "Parent", nil)
		defer testhelper.CleanupFolder(t, pool, parentFolder)

		childFolder := testhelper.CreateTestFolder(t, pool, workspaceID, "Child", &parentFolder)
		defer testhelper.CleanupFolder(t, pool, childFolder)

		// Move child to root (nil parent)
		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			PATCH(fmt.Sprintf("/api/v1/workspace/folders/%s/move", childFolder), map[string]interface{}{
				"newParentId": nil,
			})

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var folderResp dto.FolderResponse
		err := json.Unmarshal(body, &folderResp)
		require.NoError(t, err)

		assert.Equal(t, childFolder, folderResp.ID)
		assert.Nil(t, folderResp.ParentID)
	})

	t.Run("success move to another parent", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Move Folder Tenant 2", "MFST02")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-mf2@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		// Create two root folders
		folder1 := testhelper.CreateTestFolder(t, pool, workspaceID, "Folder 1", nil)
		defer testhelper.CleanupFolder(t, pool, folder1)

		folder2 := testhelper.CreateTestFolder(t, pool, workspaceID, "Folder 2", nil)
		defer testhelper.CleanupFolder(t, pool, folder2)

		// Move folder2 under folder1
		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			PATCH(fmt.Sprintf("/api/v1/workspace/folders/%s/move", folder2), map[string]interface{}{
				"newParentId": folder1,
			})

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var folderResp dto.FolderResponse
		err := json.Unmarshal(body, &folderResp)
		require.NoError(t, err)

		assert.Equal(t, folder2, folderResp.ID)
		require.NotNil(t, folderResp.ParentID)
		assert.Equal(t, folder1, *folderResp.ParentID)
	})

	t.Run("forbidden with VIEWER", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Move Folder Tenant 3", "MFST03")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		viewer := testhelper.CreateTestUser(t, pool, "viewer-mf@test.com", "Viewer User", nil)
		defer testhelper.CleanupUser(t, pool, viewer.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

		folder := testhelper.CreateTestFolder(t, pool, workspaceID, "Folder", nil)
		defer testhelper.CleanupFolder(t, pool, folder)

		resp, _ := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			PATCH(fmt.Sprintf("/api/v1/workspace/folders/%s/move", folder), map[string]interface{}{
				"newParentId": nil,
			})

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("not found", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Move Folder Tenant 4", "MFST04")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-mf3@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			PATCH("/api/v1/workspace/folders/00000000-0000-0000-0000-000000000000/move", map[string]interface{}{
				"newParentId": nil,
			})

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// =============================================================================
// Additional Validation Tests
// =============================================================================

// TestWorkspaceController_UpdateWorkspace_Validation tests validation errors.
func TestWorkspaceController_UpdateWorkspace_Validation(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("validation error with empty name", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update WS Val Tenant", "UWVT01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		admin := testhelper.CreateTestUser(t, pool, "admin-uwv@test.com", "Admin User", nil)
		defer testhelper.CleanupUser(t, pool, admin.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT("/api/v1/workspace", map[string]interface{}{
				"name": "",
			})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// TestWorkspaceController_UpdateFolder_Validation tests validation errors.
func TestWorkspaceController_UpdateFolder_Validation(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("validation error with empty name", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Folder Val Tenant", "UFVT01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-ufv@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		folderID := testhelper.CreateTestFolder(t, pool, workspaceID, "Folder", nil)
		defer testhelper.CleanupFolder(t, pool, folderID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT(fmt.Sprintf("/api/v1/workspace/folders/%s", folderID), map[string]interface{}{
				"name": "",
			})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// TestWorkspaceController_UpdateTag_Validation tests validation errors.
func TestWorkspaceController_UpdateTag_Validation(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("validation error with empty name", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Tag Val Tenant", "UTVT01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-utv@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		tagID := testhelper.CreateTestTag(t, pool, workspaceID, "Tag", "#FF0000")
		defer testhelper.CleanupTag(t, pool, tagID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT(fmt.Sprintf("/api/v1/workspace/tags/%s", tagID), map[string]interface{}{
				"name":  "",
				"color": "#FF0000",
			})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("validation error with invalid color", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Tag Val Tenant 2", "UTVT02")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-utv2@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		tagID := testhelper.CreateTestTag(t, pool, workspaceID, "Tag", "#FF0000")
		defer testhelper.CleanupTag(t, pool, tagID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT(fmt.Sprintf("/api/v1/workspace/tags/%s", tagID), map[string]interface{}{
				"name":  "Updated",
				"color": "invalid",
			})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// TestWorkspaceController_UpdateMemberRole_Validation tests validation errors.
func TestWorkspaceController_UpdateMemberRole_Validation(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("validation error with invalid role", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Role Val Tenant", "URVT01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		owner := testhelper.CreateTestUser(t, pool, "owner-urv@test.com", "Owner User", nil)
		defer testhelper.CleanupUser(t, pool, owner.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

		member := testhelper.CreateTestUser(t, pool, "member-urv@test.com", "Member User", nil)
		defer testhelper.CleanupUser(t, pool, member.ID)
		memberID := testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, member.ID, entity.WorkspaceRoleViewer, nil)

		resp, _ := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT(fmt.Sprintf("/api/v1/workspace/members/%s", memberID), map[string]interface{}{
				"role": "INVALID_ROLE",
			})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("not found member", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Role Val Tenant 2", "URVT02")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		owner := testhelper.CreateTestUser(t, pool, "owner-urv2@test.com", "Owner User", nil)
		defer testhelper.CleanupUser(t, pool, owner.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

		resp, _ := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT("/api/v1/workspace/members/00000000-0000-0000-0000-000000000000", map[string]interface{}{
				"role": "ADMIN",
			})

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// =============================================================================
// Additional Coverage Tests
// =============================================================================

// TestWorkspaceController_CreateTag_Duplicate tests creating a tag with duplicate name.
func TestWorkspaceController_CreateTag_Duplicate(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Dup Tag Tenant", "DTGT01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Dup Tag Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	editor := testhelper.CreateTestUser(t, pool, "editor-dtg@test.com", "Editor User", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	// Create first tag
	existingTag := testhelper.CreateTestTag(t, pool, workspaceID, "duplicate_tag", "#FF0000")
	defer testhelper.CleanupTag(t, pool, existingTag)

	// Try to create tag with same name
	resp, _ := client.
		WithAuth(editor.BearerHeader).
		WithWorkspaceID(workspaceID).
		POST("/api/v1/workspace/tags", map[string]interface{}{
			"name":  "Duplicate Tag",
			"color": "#00FF00",
		})

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}

// TestWorkspaceController_MoveFolder_CircularReference tests moving a folder to its own descendant.
func TestWorkspaceController_MoveFolder_CircularReference(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Circular Ref Tenant", "CRFT01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Circular Ref Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	editor := testhelper.CreateTestUser(t, pool, "editor-crf@test.com", "Editor User", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	// Create parent -> child -> grandchild hierarchy
	parentFolder := testhelper.CreateTestFolder(t, pool, workspaceID, "Parent", nil)
	defer testhelper.CleanupFolder(t, pool, parentFolder)

	childFolder := testhelper.CreateTestFolder(t, pool, workspaceID, "Child", &parentFolder)
	defer testhelper.CleanupFolder(t, pool, childFolder)

	grandchildFolder := testhelper.CreateTestFolder(t, pool, workspaceID, "Grandchild", &childFolder)
	defer testhelper.CleanupFolder(t, pool, grandchildFolder)

	// Try to move parent under grandchild (circular reference)
	resp, _ := client.
		WithAuth(editor.BearerHeader).
		WithWorkspaceID(workspaceID).
		PATCH(fmt.Sprintf("/api/v1/workspace/folders/%s/move", parentFolder), map[string]interface{}{
			"newParentId": grandchildFolder,
		})

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestWorkspaceController_DeleteFolder_WithTemplates tests deleting a folder that contains templates.
func TestWorkspaceController_DeleteFolder_WithTemplates(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Del Folder Tmpl Tenant", "DFTT01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Del Folder Tmpl Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	admin := testhelper.CreateTestUser(t, pool, "admin-dft@test.com", "Admin User", nil)
	defer testhelper.CleanupUser(t, pool, admin.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

	// Create folder with a template inside
	folderID := testhelper.CreateTestFolder(t, pool, workspaceID, "Folder With Template", nil)
	defer testhelper.CleanupFolder(t, pool, folderID)

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Template in Folder", &folderID)
	defer testhelper.CleanupTemplate(t, pool, templateID)

	// Try to delete folder with template
	resp, _ := client.
		WithAuth(admin.BearerHeader).
		WithWorkspaceID(workspaceID).
		DELETE(fmt.Sprintf("/api/v1/workspace/folders/%s", folderID))

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestWorkspaceController_DeleteTag_InUse tests deleting a tag that is used by templates.
func TestWorkspaceController_DeleteTag_InUse(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Del Tag Use Tenant", "DTUT01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Del Tag Use Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	admin := testhelper.CreateTestUser(t, pool, "admin-dtu@test.com", "Admin User", nil)
	defer testhelper.CleanupUser(t, pool, admin.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

	// Create tag
	tagID := testhelper.CreateTestTag(t, pool, workspaceID, "Used Tag", "#FF0000")
	defer testhelper.CleanupTag(t, pool, tagID)

	// Create template and link to tag
	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Tagged Template", nil)
	defer testhelper.CleanupTemplate(t, pool, templateID)
	testhelper.CreateTestTemplateTag(t, pool, templateID, tagID)

	// Try to delete tag that is in use
	resp, _ := client.
		WithAuth(admin.BearerHeader).
		WithWorkspaceID(workspaceID).
		DELETE(fmt.Sprintf("/api/v1/workspace/tags/%s", tagID))

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

// TestWorkspaceController_CreateFolder_Nested tests creating a nested folder hierarchy.
func TestWorkspaceController_CreateFolder_Nested(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Nested Folder Tenant", "NFST01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Nested Folder Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	editor := testhelper.CreateTestUser(t, pool, "editor-nf@test.com", "Editor User", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	// Create parent folder
	parentFolder := testhelper.CreateTestFolder(t, pool, workspaceID, "Parent Folder", nil)
	defer testhelper.CleanupFolder(t, pool, parentFolder)

	// Create child folder via API
	resp, body := client.
		WithAuth(editor.BearerHeader).
		WithWorkspaceID(workspaceID).
		POST("/api/v1/workspace/folders", map[string]interface{}{
			"name":     "Child Folder",
			"parentId": parentFolder,
		})

	assert.Equal(t, http.StatusCreated, resp.StatusCode)

	var folderResp dto.FolderResponse
	err := json.Unmarshal(body, &folderResp)
	require.NoError(t, err)

	assert.Equal(t, "Child Folder", folderResp.Name)
	require.NotNil(t, folderResp.ParentID)
	assert.Equal(t, parentFolder, *folderResp.ParentID)
	defer testhelper.CleanupFolder(t, pool, folderResp.ID)
}

// TestWorkspaceController_UpdateFolder_DuplicateName tests updating folder with duplicate name at same level.
func TestWorkspaceController_UpdateFolder_DuplicateName(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Dup Folder Tenant", "DPFT01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Dup Folder Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	editor := testhelper.CreateTestUser(t, pool, "editor-dpf@test.com", "Editor User", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	// Create two folders at root level
	folder1 := testhelper.CreateTestFolder(t, pool, workspaceID, "Folder One", nil)
	defer testhelper.CleanupFolder(t, pool, folder1)

	folder2 := testhelper.CreateTestFolder(t, pool, workspaceID, "Folder Two", nil)
	defer testhelper.CleanupFolder(t, pool, folder2)

	// Try to rename folder2 to folder1's name
	resp, _ := client.
		WithAuth(editor.BearerHeader).
		WithWorkspaceID(workspaceID).
		PUT(fmt.Sprintf("/api/v1/workspace/folders/%s", folder2), map[string]interface{}{
			"name": "Folder One",
		})

	assert.Equal(t, http.StatusConflict, resp.StatusCode)
}
