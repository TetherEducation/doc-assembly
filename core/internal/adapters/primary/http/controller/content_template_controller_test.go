//go:build integration

package controller_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/testing/testhelper"
)

// =============================================================================
// Template List Tests
// =============================================================================

// TestContentTemplateController_ListTemplates tests the GET /content/templates endpoint.
func TestContentTemplateController_ListTemplates(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup: tenant + workspace + users
	tenantID := testhelper.CreateTestTenant(t, pool, "Template List Tenant", "TLST01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Template List Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	owner := testhelper.CreateTestUser(t, pool, "owner-tpl-list@test.com", "Owner User", nil)
	defer testhelper.CleanupUser(t, pool, owner.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

	viewer := testhelper.CreateTestUser(t, pool, "viewer-tpl-list@test.com", "Viewer User", nil)
	defer testhelper.CleanupUser(t, pool, viewer.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

	// Create templates
	template1 := testhelper.CreateTestTemplate(t, pool, workspaceID, "Contract Template", nil)
	defer testhelper.CleanupTemplate(t, pool, template1)

	template2 := testhelper.CreateTestTemplate(t, pool, workspaceID, "Invoice Template", nil)
	defer testhelper.CleanupTemplate(t, pool, template2)

	t.Run("success with VIEWER", func(t *testing.T) {
		resp, body := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/content/templates")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp dto.ListTemplatesResponse
		err := json.Unmarshal(body, &listResp)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, listResp.Total, 2)
	})

	t.Run("success with OWNER", func(t *testing.T) {
		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/content/templates")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp dto.ListTemplatesResponse
		err := json.Unmarshal(body, &listResp)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, listResp.Total, 2)
	})

	t.Run("success with empty list", func(t *testing.T) {
		emptyWsID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Empty Template Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, emptyWsID)
		testhelper.CreateTestWorkspaceMember(t, pool, emptyWsID, owner.ID, entity.WorkspaceRoleOwner, nil)

		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(emptyWsID).
			GET("/api/v1/content/templates")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp dto.ListTemplatesResponse
		err := json.Unmarshal(body, &listResp)
		require.NoError(t, err)

		assert.Equal(t, 0, listResp.Total)
	})

	t.Run("success with search filter", func(t *testing.T) {
		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/content/templates?search=Contract")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp dto.ListTemplatesResponse
		err := json.Unmarshal(body, &listResp)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, listResp.Total, 1)
	})

	t.Run("success with folder filter", func(t *testing.T) {
		folderID := testhelper.CreateTestFolder(t, pool, workspaceID, "Templates Folder", nil)
		defer testhelper.CleanupFolder(t, pool, folderID)

		templateInFolder := testhelper.CreateTestTemplate(t, pool, workspaceID, "Template In Folder", &folderID)
		defer testhelper.CleanupTemplate(t, pool, templateInFolder)

		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET(fmt.Sprintf("/api/v1/content/templates?folderId=%s", folderID))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp dto.ListTemplatesResponse
		err := json.Unmarshal(body, &listResp)
		require.NoError(t, err)

		assert.Equal(t, 1, listResp.Total)
	})

	t.Run("success with folder filter excludes child folders", func(t *testing.T) {
		// Create parent folder
		parentFolderID := testhelper.CreateTestFolder(t, pool, workspaceID, "Parent Folder", nil)
		defer testhelper.CleanupFolder(t, pool, parentFolderID)

		// Create child folder under parent
		childFolderID := testhelper.CreateTestFolder(t, pool, workspaceID, "Child Folder", &parentFolderID)
		defer testhelper.CleanupFolder(t, pool, childFolderID)

		// Create template in parent folder
		templateInParent := testhelper.CreateTestTemplate(t, pool, workspaceID, "Template In Parent", &parentFolderID)
		defer testhelper.CleanupTemplate(t, pool, templateInParent)

		// Create template in child folder
		templateInChild := testhelper.CreateTestTemplate(t, pool, workspaceID, "Template In Child", &childFolderID)
		defer testhelper.CleanupTemplate(t, pool, templateInChild)

		// Query templates by parent folder ID
		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET(fmt.Sprintf("/api/v1/content/templates?folderId=%s", parentFolderID))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp dto.ListTemplatesResponse
		err := json.Unmarshal(body, &listResp)
		require.NoError(t, err)

		// Should only return 1 template (the one in parent folder, NOT the one in child folder)
		assert.Equal(t, 1, listResp.Total, "Should only return templates directly in parent folder, not child folders")

		// Verify it's the parent template
		if assert.Len(t, listResp.Items, 1) {
			assert.Equal(t, "Template In Parent", listResp.Items[0].Title)
			assert.Equal(t, parentFolderID, *listResp.Items[0].FolderID)
		}
	})

	t.Run("success with hasPublishedVersion filter", func(t *testing.T) {
		// Create template with published version
		templateWithPublished := testhelper.CreateTestTemplate(t, pool, workspaceID, "Published Template", nil)
		defer testhelper.CleanupTemplate(t, pool, templateWithPublished)

		testhelper.CreateTestTemplateVersion(t, pool, templateWithPublished, 1, "v1.0", entity.VersionStatusPublished)

		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/content/templates?hasPublishedVersion=true")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp dto.ListTemplatesResponse
		err := json.Unmarshal(body, &listResp)
		require.NoError(t, err)

		// At least our published template should be in the results
		assert.GreaterOrEqual(t, listResp.Total, 1)
	})

	t.Run("success returns versionCount and publishedVersionNumber", func(t *testing.T) {
		// Create template with multiple versions including published
		templateWithVersions := testhelper.CreateTestTemplate(t, pool, workspaceID, "Template With Versions", nil)
		defer testhelper.CleanupTemplate(t, pool, templateWithVersions)

		// Create 3 versions: archived (should NOT be counted), draft, and published (v2)
		testhelper.CreateTestTemplateVersion(t, pool, templateWithVersions, 1, "v1.0 Archived", entity.VersionStatusArchived)
		testhelper.CreateTestTemplateVersion(t, pool, templateWithVersions, 2, "v2.0 Published", entity.VersionStatusPublished)
		testhelper.CreateTestTemplateVersion(t, pool, templateWithVersions, 3, "v3.0 Draft", entity.VersionStatusDraft)

		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/content/templates?search=Template+With+Versions")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp dto.ListTemplatesResponse
		err := json.Unmarshal(body, &listResp)
		require.NoError(t, err)

		require.Equal(t, 1, listResp.Total)
		require.Len(t, listResp.Items, 1)

		item := listResp.Items[0]
		assert.Equal(t, templateWithVersions, item.ID)
		assert.True(t, item.HasPublishedVersion)
		// VersionCount should be 2 (v2 published + v3 draft, excluding v1 archived)
		assert.Equal(t, 2, item.VersionCount)
		// PublishedVersionNumber should be 2 (v2 is published)
		require.NotNil(t, item.PublishedVersionNumber)
		assert.Equal(t, 2, *item.PublishedVersionNumber)
	})

	t.Run("success returns nil publishedVersionNumber when no published version", func(t *testing.T) {
		// Create template with only draft version
		templateDraftOnly := testhelper.CreateTestTemplate(t, pool, workspaceID, "Template Draft Only", nil)
		defer testhelper.CleanupTemplate(t, pool, templateDraftOnly)

		testhelper.CreateTestTemplateVersion(t, pool, templateDraftOnly, 1, "v1.0 Draft", entity.VersionStatusDraft)

		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/content/templates?search=Template+Draft+Only")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp dto.ListTemplatesResponse
		err := json.Unmarshal(body, &listResp)
		require.NoError(t, err)

		require.Equal(t, 1, listResp.Total)
		require.Len(t, listResp.Items, 1)

		item := listResp.Items[0]
		assert.Equal(t, templateDraftOnly, item.ID)
		assert.False(t, item.HasPublishedVersion)
		assert.Equal(t, 1, item.VersionCount)
		assert.Nil(t, item.PublishedVersionNumber)
	})

	t.Run("forbidden without workspace membership", func(t *testing.T) {
		nonMember := testhelper.CreateTestUser(t, pool, "nonmember-tpl@test.com", "Non Member", nil)
		defer testhelper.CleanupUser(t, pool, nonMember.ID)

		resp, _ := client.
			WithAuth(nonMember.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/content/templates")

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("bad request without workspace header", func(t *testing.T) {
		resp, _ := client.
			WithAuth(owner.BearerHeader).
			GET("/api/v1/content/templates")

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("unauthorized without token", func(t *testing.T) {
		resp, _ := client.
			WithWorkspaceID(workspaceID).
			GET("/api/v1/content/templates")

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("bad request when sandbox mode is requested for system workspace", func(t *testing.T) {
		var systemWorkspaceID string
		err := pool.QueryRow(
			context.Background(),
			`SELECT id FROM tenancy.workspaces WHERE tenant_id = $1 AND type = 'SYSTEM' AND is_sandbox = FALSE LIMIT 1`,
			tenantID,
		).Scan(&systemWorkspaceID)
		require.NoError(t, err)

		testhelper.CreateTestWorkspaceMember(t, pool, systemWorkspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

		req, err := http.NewRequest(http.MethodGet, ts.URL()+"/api/v1/content/templates", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", owner.BearerHeader)
		req.Header.Set("X-Workspace-ID", systemWorkspaceID)
		req.Header.Set("X-Sandbox-Mode", "true")

		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)

		var errorResp map[string]string
		err = json.Unmarshal(body, &errorResp)
		require.NoError(t, err)
		assert.Equal(t, entity.ErrSandboxNotSupported.Error(), errorResp["error"])
	})
}

// =============================================================================
// Template Create Tests
// =============================================================================

// TestContentTemplateController_CreateTemplate tests the POST /content/templates endpoint.
func TestContentTemplateController_CreateTemplate(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success with EDITOR", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Create Template Tenant", "CTST01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Create Template Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-ct@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates", map[string]interface{}{
				"title":           "New Contract Template",
				"isPublicLibrary": false,
			})

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResp dto.TemplateCreateResponse
		err := json.Unmarshal(body, &createResp)
		require.NoError(t, err)

		assert.Equal(t, "New Contract Template", createResp.Template.Title)
		assert.NotEmpty(t, createResp.Template.ID)
		assert.NotNil(t, createResp.InitialVersion)
		assert.Equal(t, "DRAFT", createResp.InitialVersion.Status)
		assert.Equal(t, 1, createResp.InitialVersion.VersionNumber)
		defer testhelper.CleanupTemplate(t, pool, createResp.Template.ID)
	})

	t.Run("success with ADMIN", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Create Template Tenant 2", "CTST02")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		admin := testhelper.CreateTestUser(t, pool, "admin-ct@test.com", "Admin User", nil)
		defer testhelper.CleanupUser(t, pool, admin.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

		resp, body := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates", map[string]interface{}{
				"title": "Admin Template",
			})

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResp dto.TemplateCreateResponse
		err := json.Unmarshal(body, &createResp)
		require.NoError(t, err)
		defer testhelper.CleanupTemplate(t, pool, createResp.Template.ID)
	})

	t.Run("success with OWNER", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Create Template Tenant 3", "CTST03")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		owner := testhelper.CreateTestUser(t, pool, "owner-ct@test.com", "Owner User", nil)
		defer testhelper.CleanupUser(t, pool, owner.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates", map[string]interface{}{
				"title": "Owner Template",
			})

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResp dto.TemplateCreateResponse
		err := json.Unmarshal(body, &createResp)
		require.NoError(t, err)
		defer testhelper.CleanupTemplate(t, pool, createResp.Template.ID)
	})

	t.Run("bad request for system workspace", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Create Template System Tenant", "CTSYS")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		var systemWorkspaceID string
		err := pool.QueryRow(
			context.Background(),
			`SELECT id FROM tenancy.workspaces WHERE tenant_id = $1 AND type = 'SYSTEM' AND is_sandbox = FALSE LIMIT 1`,
			tenantID,
		).Scan(&systemWorkspaceID)
		require.NoError(t, err)

		owner := testhelper.CreateTestUser(t, pool, "owner-system-template@test.com", "Owner User", nil)
		defer testhelper.CleanupUser(t, pool, owner.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, systemWorkspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

		resp, _ := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(systemWorkspaceID).
			POST("/api/v1/content/templates", map[string]interface{}{
				"title": "System Template",
			})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("success with folder ID", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Create Template Tenant 4", "CTST04")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-ct2@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		folderID := testhelper.CreateTestFolder(t, pool, workspaceID, "Templates Folder", nil)
		defer testhelper.CleanupFolder(t, pool, folderID)

		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates", map[string]interface{}{
				"title":    "Template In Folder",
				"folderId": folderID,
			})

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResp dto.TemplateCreateResponse
		err := json.Unmarshal(body, &createResp)
		require.NoError(t, err)

		assert.Equal(t, &folderID, createResp.Template.FolderID)
		defer testhelper.CleanupTemplate(t, pool, createResp.Template.ID)
	})

	t.Run("success with isPublicLibrary true", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Create Template Tenant 5", "CTST05")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-ct3@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates", map[string]interface{}{
				"title":           "Public Library Template",
				"isPublicLibrary": true,
			})

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResp dto.TemplateCreateResponse
		err := json.Unmarshal(body, &createResp)
		require.NoError(t, err)

		assert.True(t, createResp.Template.IsPublicLibrary)
		defer testhelper.CleanupTemplate(t, pool, createResp.Template.ID)
	})

	t.Run("forbidden with VIEWER", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Create Template Tenant 6", "CTST06")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		viewer := testhelper.CreateTestUser(t, pool, "viewer-ct@test.com", "Viewer User", nil)
		defer testhelper.CleanupUser(t, pool, viewer.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

		resp, _ := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates", map[string]interface{}{
				"title": "Viewer Template",
			})

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("validation error - empty title", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Create Template Tenant 7", "CTST07")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-ct4@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates", map[string]interface{}{
				"title": "",
			})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("response contains template and initial version", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Create Template Tenant 8", "CTST08")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-ct5@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates", map[string]interface{}{
				"title": "Full Response Template",
			})

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResp dto.TemplateCreateResponse
		err := json.Unmarshal(body, &createResp)
		require.NoError(t, err)

		// Verify template fields
		assert.NotEmpty(t, createResp.Template.ID)
		assert.Equal(t, workspaceID, createResp.Template.WorkspaceID)
		assert.Equal(t, "Full Response Template", createResp.Template.Title)
		assert.NotZero(t, createResp.Template.CreatedAt)

		// Verify initial version fields
		assert.NotNil(t, createResp.InitialVersion)
		assert.NotEmpty(t, createResp.InitialVersion.ID)
		assert.Equal(t, createResp.Template.ID, createResp.InitialVersion.TemplateID)
		assert.Equal(t, 1, createResp.InitialVersion.VersionNumber)
		assert.Equal(t, "DRAFT", createResp.InitialVersion.Status)

		defer testhelper.CleanupTemplate(t, pool, createResp.Template.ID)
	})
}

// =============================================================================
// Template Get Tests
// =============================================================================

// TestContentTemplateController_GetTemplate tests the GET /content/templates/:templateId endpoint.
func TestContentTemplateController_GetTemplate(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Get Template Tenant", "GTST01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Get Template Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	owner := testhelper.CreateTestUser(t, pool, "owner-gt@test.com", "Owner User", nil)
	defer testhelper.CleanupUser(t, pool, owner.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

	viewer := testhelper.CreateTestUser(t, pool, "viewer-gt@test.com", "Viewer User", nil)
	defer testhelper.CleanupUser(t, pool, viewer.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Get Test Template", nil)
	defer testhelper.CleanupTemplate(t, pool, templateID)

	t.Run("success with VIEWER", func(t *testing.T) {
		resp, body := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET(fmt.Sprintf("/api/v1/content/templates/%s", templateID))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var tplResp dto.TemplateWithDetailsResponse
		err := json.Unmarshal(body, &tplResp)
		require.NoError(t, err)

		assert.Equal(t, templateID, tplResp.ID)
		assert.Equal(t, "Get Test Template", tplResp.Title)
	})

	t.Run("success with EDITOR", func(t *testing.T) {
		editor := testhelper.CreateTestUser(t, pool, "editor-gt@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET(fmt.Sprintf("/api/v1/content/templates/%s", templateID))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var tplResp dto.TemplateWithDetailsResponse
		err := json.Unmarshal(body, &tplResp)
		require.NoError(t, err)

		assert.Equal(t, templateID, tplResp.ID)
	})

	t.Run("success returns published version details", func(t *testing.T) {
		// Create template with published version
		templateWithPub := testhelper.CreateTestTemplate(t, pool, workspaceID, "Template With Published", nil)
		defer testhelper.CleanupTemplate(t, pool, templateWithPub)

		testhelper.CreateTestTemplateVersion(t, pool, templateWithPub, 1, "v1.0 Published", entity.VersionStatusPublished)

		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET(fmt.Sprintf("/api/v1/content/templates/%s", templateWithPub))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var tplResp dto.TemplateWithDetailsResponse
		err := json.Unmarshal(body, &tplResp)
		require.NoError(t, err)

		assert.NotNil(t, tplResp.PublishedVersion)
		assert.Equal(t, "PUBLISHED", tplResp.PublishedVersion.Status)
	})

	t.Run("success returns tags and folder", func(t *testing.T) {
		// Create folder and tag
		folderID := testhelper.CreateTestFolder(t, pool, workspaceID, "Get Template Folder", nil)
		defer testhelper.CleanupFolder(t, pool, folderID)

		templateWithFolder := testhelper.CreateTestTemplate(t, pool, workspaceID, "Template With Folder", &folderID)
		defer testhelper.CleanupTemplate(t, pool, templateWithFolder)

		resp, body := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET(fmt.Sprintf("/api/v1/content/templates/%s", templateWithFolder))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var tplResp dto.TemplateWithDetailsResponse
		err := json.Unmarshal(body, &tplResp)
		require.NoError(t, err)

		assert.NotNil(t, tplResp.Folder)
		assert.Equal(t, folderID, tplResp.Folder.ID)
	})

	t.Run("not found - non-existent ID", func(t *testing.T) {
		resp, _ := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/content/templates/00000000-0000-0000-0000-000000000000")

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("unauthorized without token", func(t *testing.T) {
		resp, _ := client.
			WithWorkspaceID(workspaceID).
			GET(fmt.Sprintf("/api/v1/content/templates/%s", templateID))

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// =============================================================================
// Template Get With All Versions Tests
// =============================================================================

// TestContentTemplateController_GetTemplateWithAllVersions tests the GET /content/templates/:templateId/all-versions endpoint.
func TestContentTemplateController_GetTemplateWithAllVersions(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "All Versions Tenant", "AVST01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "All Versions Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	viewer := testhelper.CreateTestUser(t, pool, "viewer-av@test.com", "Viewer User", nil)
	defer testhelper.CleanupUser(t, pool, viewer.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

	t.Run("success with VIEWER", func(t *testing.T) {
		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "All Versions Template", nil)
		defer testhelper.CleanupTemplate(t, pool, templateID)

		// Create multiple versions
		testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "v1.0", entity.VersionStatusArchived)
		testhelper.CreateTestTemplateVersion(t, pool, templateID, 2, "v2.0", entity.VersionStatusPublished)
		testhelper.CreateTestTemplateVersion(t, pool, templateID, 3, "v3.0 Draft", entity.VersionStatusDraft)

		resp, body := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET(fmt.Sprintf("/api/v1/content/templates/%s/all-versions", templateID))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var tplResp dto.TemplateWithAllVersionsResponse
		err := json.Unmarshal(body, &tplResp)
		require.NoError(t, err)

		assert.Equal(t, templateID, tplResp.ID)
		assert.GreaterOrEqual(t, len(tplResp.Versions), 3)
	})

	t.Run("success returns all versions", func(t *testing.T) {
		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Multi Version Template", nil)
		defer testhelper.CleanupTemplate(t, pool, templateID)

		testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "v1.0", entity.VersionStatusDraft)
		testhelper.CreateTestTemplateVersion(t, pool, templateID, 2, "v2.0", entity.VersionStatusDraft)

		resp, body := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET(fmt.Sprintf("/api/v1/content/templates/%s/all-versions", templateID))

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var tplResp dto.TemplateWithAllVersionsResponse
		err := json.Unmarshal(body, &tplResp)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, len(tplResp.Versions), 2)
	})

	t.Run("not found - non-existent ID", func(t *testing.T) {
		resp, _ := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/content/templates/00000000-0000-0000-0000-000000000000/all-versions")

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// =============================================================================
// Template Update Tests
// =============================================================================

// TestContentTemplateController_UpdateTemplate tests the PUT /content/templates/:templateId endpoint.
func TestContentTemplateController_UpdateTemplate(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success with EDITOR", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Template Tenant", "UTST01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Update Template Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-ut@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Original Title", nil)
		defer testhelper.CleanupTemplate(t, pool, templateID)

		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT(fmt.Sprintf("/api/v1/content/templates/%s", templateID), map[string]interface{}{
				"title": "Updated Title",
			})

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var tplResp dto.TemplateResponse
		err := json.Unmarshal(body, &tplResp)
		require.NoError(t, err)

		assert.Equal(t, "Updated Title", tplResp.Title)
	})

	t.Run("success with ADMIN", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Template Tenant 2", "UTST02")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		admin := testhelper.CreateTestUser(t, pool, "admin-ut@test.com", "Admin User", nil)
		defer testhelper.CleanupUser(t, pool, admin.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Admin Original", nil)
		defer testhelper.CleanupTemplate(t, pool, templateID)

		resp, body := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT(fmt.Sprintf("/api/v1/content/templates/%s", templateID), map[string]interface{}{
				"title": "Admin Updated",
			})

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var tplResp dto.TemplateResponse
		err := json.Unmarshal(body, &tplResp)
		require.NoError(t, err)

		assert.Equal(t, "Admin Updated", tplResp.Title)
	})

	t.Run("success with OWNER", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Template Tenant 3", "UTST03")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		owner := testhelper.CreateTestUser(t, pool, "owner-ut@test.com", "Owner User", nil)
		defer testhelper.CleanupUser(t, pool, owner.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Owner Original", nil)
		defer testhelper.CleanupTemplate(t, pool, templateID)

		resp, _ := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT(fmt.Sprintf("/api/v1/content/templates/%s", templateID), map[string]interface{}{
				"title": "Owner Updated",
			})

		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("success update folder", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Template Tenant 4", "UTST04")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-ut2@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Move Template", nil)
		defer testhelper.CleanupTemplate(t, pool, templateID)

		folderID := testhelper.CreateTestFolder(t, pool, workspaceID, "Target Folder", nil)
		defer testhelper.CleanupFolder(t, pool, folderID)

		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT(fmt.Sprintf("/api/v1/content/templates/%s", templateID), map[string]interface{}{
				"title":    "Move Template",
				"folderId": folderID,
			})

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var tplResp dto.TemplateResponse
		err := json.Unmarshal(body, &tplResp)
		require.NoError(t, err)

		assert.Equal(t, &folderID, tplResp.FolderID)
	})

	t.Run("success update isPublicLibrary", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Template Tenant 5", "UTST05")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-ut3@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Public Template", nil)
		defer testhelper.CleanupTemplate(t, pool, templateID)

		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT(fmt.Sprintf("/api/v1/content/templates/%s", templateID), map[string]interface{}{
				"title":           "Public Template",
				"isPublicLibrary": true,
			})

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var tplResp dto.TemplateResponse
		err := json.Unmarshal(body, &tplResp)
		require.NoError(t, err)

		assert.True(t, tplResp.IsPublicLibrary)
	})

	t.Run("forbidden with VIEWER", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Template Tenant 6", "UTST06")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		viewer := testhelper.CreateTestUser(t, pool, "viewer-ut@test.com", "Viewer User", nil)
		defer testhelper.CleanupUser(t, pool, viewer.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Viewer Template", nil)
		defer testhelper.CleanupTemplate(t, pool, templateID)

		resp, _ := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT(fmt.Sprintf("/api/v1/content/templates/%s", templateID), map[string]interface{}{
				"title": "Viewer Updated",
			})

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("not found - non-existent ID", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Template Tenant 7", "UTST07")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-ut4@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT("/api/v1/content/templates/00000000-0000-0000-0000-000000000000", map[string]interface{}{
				"title": "Not Found",
			})

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("validation error - empty title", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Update Template Tenant 8", "UTST08")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-ut5@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Original", nil)
		defer testhelper.CleanupTemplate(t, pool, templateID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT(fmt.Sprintf("/api/v1/content/templates/%s", templateID), map[string]interface{}{
				"title": "",
			})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// =============================================================================
// Template Delete Tests
// =============================================================================

// TestContentTemplateController_DeleteTemplate tests the DELETE /content/templates/:templateId endpoint.
func TestContentTemplateController_DeleteTemplate(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success with ADMIN", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Delete Template Tenant", "DTST01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Delete Template Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		admin := testhelper.CreateTestUser(t, pool, "admin-dt@test.com", "Admin User", nil)
		defer testhelper.CleanupUser(t, pool, admin.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "To Delete", nil)
		// No defer cleanup - we're deleting it

		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE(fmt.Sprintf("/api/v1/content/templates/%s", templateID))

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("success with OWNER", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Delete Template Tenant 2", "DTST02")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		owner := testhelper.CreateTestUser(t, pool, "owner-dt@test.com", "Owner User", nil)
		defer testhelper.CleanupUser(t, pool, owner.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Owner Delete", nil)
		// No defer cleanup - we're deleting it

		resp, _ := client.
			WithAuth(owner.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE(fmt.Sprintf("/api/v1/content/templates/%s", templateID))

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("forbidden with EDITOR", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Delete Template Tenant 3", "DTST03")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-dt@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Editor Delete", nil)
		defer testhelper.CleanupTemplate(t, pool, templateID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE(fmt.Sprintf("/api/v1/content/templates/%s", templateID))

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("forbidden with VIEWER", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Delete Template Tenant 4", "DTST04")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		viewer := testhelper.CreateTestUser(t, pool, "viewer-dt@test.com", "Viewer User", nil)
		defer testhelper.CleanupUser(t, pool, viewer.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Viewer Delete", nil)
		defer testhelper.CleanupTemplate(t, pool, templateID)

		resp, _ := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE(fmt.Sprintf("/api/v1/content/templates/%s", templateID))

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("not found - non-existent ID", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Delete Template Tenant 5", "DTST05")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		admin := testhelper.CreateTestUser(t, pool, "admin-dt2@test.com", "Admin User", nil)
		defer testhelper.CleanupUser(t, pool, admin.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE("/api/v1/content/templates/00000000-0000-0000-0000-000000000000")

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// =============================================================================
// Template Clone Tests
// =============================================================================

// TestContentTemplateController_CloneTemplate tests the POST /content/templates/:templateId/clone endpoint.
func TestContentTemplateController_CloneTemplate(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success with EDITOR", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Clone Template Tenant", "CLST01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Clone Template Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-cl@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		// Create source template with published version
		sourceTemplateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Source Template", nil)
		defer testhelper.CleanupTemplate(t, pool, sourceTemplateID)
		versionID := testhelper.CreateTestTemplateVersion(t, pool, sourceTemplateID, 1, "v1.0", entity.VersionStatusPublished)

		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST(fmt.Sprintf("/api/v1/content/templates/%s/clone", sourceTemplateID), map[string]interface{}{
				"newTitle":  "Cloned Template",
				"versionId": versionID,
			})

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResp dto.TemplateCreateResponse
		err := json.Unmarshal(body, &createResp)
		require.NoError(t, err)

		assert.Equal(t, "Cloned Template", createResp.Template.Title)
		assert.NotEqual(t, sourceTemplateID, createResp.Template.ID)
		defer testhelper.CleanupTemplate(t, pool, createResp.Template.ID)
	})

	t.Run("success with ADMIN", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Clone Template Tenant 2", "CLST02")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		admin := testhelper.CreateTestUser(t, pool, "admin-cl@test.com", "Admin User", nil)
		defer testhelper.CleanupUser(t, pool, admin.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

		sourceTemplateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Admin Source", nil)
		defer testhelper.CleanupTemplate(t, pool, sourceTemplateID)
		versionID := testhelper.CreateTestTemplateVersion(t, pool, sourceTemplateID, 1, "v1.0", entity.VersionStatusPublished)

		resp, body := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST(fmt.Sprintf("/api/v1/content/templates/%s/clone", sourceTemplateID), map[string]interface{}{
				"newTitle":  "Admin Cloned",
				"versionId": versionID,
			})

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResp dto.TemplateCreateResponse
		err := json.Unmarshal(body, &createResp)
		require.NoError(t, err)
		defer testhelper.CleanupTemplate(t, pool, createResp.Template.ID)
	})

	t.Run("bad request for system workspace source", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Clone Template System Tenant", "CLSYS")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		var systemWorkspaceID string
		err := pool.QueryRow(
			context.Background(),
			`SELECT id FROM tenancy.workspaces WHERE tenant_id = $1 AND type = 'SYSTEM' AND is_sandbox = FALSE LIMIT 1`,
			tenantID,
		).Scan(&systemWorkspaceID)
		require.NoError(t, err)

		editor := testhelper.CreateTestUser(t, pool, "editor-cl-system@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, systemWorkspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		sourceTemplateID := testhelper.CreateTestTemplate(t, pool, systemWorkspaceID, "System Source Template", nil)
		defer testhelper.CleanupTemplate(t, pool, sourceTemplateID)
		versionID := testhelper.CreateTestTemplateVersion(t, pool, sourceTemplateID, 1, "v1.0", entity.VersionStatusPublished)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(systemWorkspaceID).
			POST(fmt.Sprintf("/api/v1/content/templates/%s/clone", sourceTemplateID), map[string]interface{}{
				"newTitle":  "System Clone",
				"versionId": versionID,
			})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("success clones to different folder", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Clone Template Tenant 3", "CLST03")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-cl2@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		sourceTemplateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Source Template", nil)
		defer testhelper.CleanupTemplate(t, pool, sourceTemplateID)
		versionID := testhelper.CreateTestTemplateVersion(t, pool, sourceTemplateID, 1, "v1.0", entity.VersionStatusPublished)

		targetFolderID := testhelper.CreateTestFolder(t, pool, workspaceID, "Target Folder", nil)
		defer testhelper.CleanupFolder(t, pool, targetFolderID)

		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST(fmt.Sprintf("/api/v1/content/templates/%s/clone", sourceTemplateID), map[string]interface{}{
				"newTitle":       "Cloned To Folder",
				"targetFolderId": targetFolderID,
				"versionId":      versionID,
			})

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResp dto.TemplateCreateResponse
		err := json.Unmarshal(body, &createResp)
		require.NoError(t, err)

		assert.Equal(t, &targetFolderID, createResp.Template.FolderID)
		defer testhelper.CleanupTemplate(t, pool, createResp.Template.ID)
	})

	t.Run("forbidden with VIEWER", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Clone Template Tenant 4", "CLST04")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		viewer := testhelper.CreateTestUser(t, pool, "viewer-cl@test.com", "Viewer User", nil)
		defer testhelper.CleanupUser(t, pool, viewer.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

		sourceTemplateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Source Template", nil)
		defer testhelper.CleanupTemplate(t, pool, sourceTemplateID)
		versionID := testhelper.CreateTestTemplateVersion(t, pool, sourceTemplateID, 1, "v1.0", entity.VersionStatusPublished)

		resp, _ := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST(fmt.Sprintf("/api/v1/content/templates/%s/clone", sourceTemplateID), map[string]interface{}{
				"newTitle":  "Viewer Clone",
				"versionId": versionID,
			})

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("not found - source template not found", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Clone Template Tenant 6", "CLST06")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-cl4@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/00000000-0000-0000-0000-000000000000/clone", map[string]interface{}{
				"newTitle":  "Clone Not Found",
				"versionId": "00000000-0000-0000-0000-000000000000",
			})

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("validation error - empty title", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Clone Template Tenant 7", "CLST07")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-cl5@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		sourceTemplateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Source Template", nil)
		defer testhelper.CleanupTemplate(t, pool, sourceTemplateID)
		versionID := testhelper.CreateTestTemplateVersion(t, pool, sourceTemplateID, 1, "v1.0", entity.VersionStatusPublished)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST(fmt.Sprintf("/api/v1/content/templates/%s/clone", sourceTemplateID), map[string]interface{}{
				"newTitle":  "",
				"versionId": versionID,
			})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("success clones draft version", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Clone Template Tenant Draft", "CLSTDR")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-cldraft@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		// Create template with DRAFT version (no published)
		sourceTemplateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Source Template", nil)
		defer testhelper.CleanupTemplate(t, pool, sourceTemplateID)
		draftVersionID := testhelper.CreateTestTemplateVersion(t, pool, sourceTemplateID, 1, "v1.0 Draft", entity.VersionStatusDraft)

		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST(fmt.Sprintf("/api/v1/content/templates/%s/clone", sourceTemplateID), map[string]interface{}{
				"newTitle":  "Cloned From Draft",
				"versionId": draftVersionID,
			})

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResp dto.TemplateCreateResponse
		err := json.Unmarshal(body, &createResp)
		require.NoError(t, err)

		assert.Equal(t, "Cloned From Draft", createResp.Template.Title)
		assert.NotEqual(t, sourceTemplateID, createResp.Template.ID)
		defer testhelper.CleanupTemplate(t, pool, createResp.Template.ID)
	})

	t.Run("success clones archived version", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Clone Template Tenant Archived", "CLSTAR")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-clarch@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		sourceTemplateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Source Template", nil)
		defer testhelper.CleanupTemplate(t, pool, sourceTemplateID)
		archivedVersionID := testhelper.CreateTestTemplateVersion(t, pool, sourceTemplateID, 1, "v1.0 Archived", entity.VersionStatusArchived)

		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST(fmt.Sprintf("/api/v1/content/templates/%s/clone", sourceTemplateID), map[string]interface{}{
				"newTitle":  "Cloned From Archived",
				"versionId": archivedVersionID,
			})

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var createResp dto.TemplateCreateResponse
		err := json.Unmarshal(body, &createResp)
		require.NoError(t, err)
		defer testhelper.CleanupTemplate(t, pool, createResp.Template.ID)
	})

	t.Run("bad request - versionId belongs to different template", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Clone Template Tenant Wrong", "CLSTWRG")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-clwrong@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		// Create TWO templates
		template1ID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Template 1", nil)
		defer testhelper.CleanupTemplate(t, pool, template1ID)

		template2ID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Template 2", nil)
		defer testhelper.CleanupTemplate(t, pool, template2ID)
		version2ID := testhelper.CreateTestTemplateVersion(t, pool, template2ID, 1, "v1.0", entity.VersionStatusPublished)

		// Try to clone template1 using version from template2
		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST(fmt.Sprintf("/api/v1/content/templates/%s/clone", template1ID), map[string]interface{}{
				"newTitle":  "Invalid Clone",
				"versionId": version2ID,
			})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("not found - versionId does not exist", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Clone Template Tenant NoVer", "CLSTNV")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-clnover@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		sourceTemplateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Source Template", nil)
		defer testhelper.CleanupTemplate(t, pool, sourceTemplateID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST(fmt.Sprintf("/api/v1/content/templates/%s/clone", sourceTemplateID), map[string]interface{}{
				"newTitle":  "Clone Non-Existent Version",
				"versionId": "00000000-0000-0000-0000-000000000000",
			})

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})

	t.Run("validation error - missing versionId", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Clone Template Tenant NoID", "CLSTNID")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-clnoid@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		sourceTemplateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Source Template", nil)
		defer testhelper.CleanupTemplate(t, pool, sourceTemplateID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST(fmt.Sprintf("/api/v1/content/templates/%s/clone", sourceTemplateID), map[string]interface{}{
				"newTitle": "Clone Without Version",
			})

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// =============================================================================
// Template Tags Tests
// =============================================================================

// TestContentTemplateController_AddTemplateTags tests the POST /content/templates/:templateId/tags endpoint.
func TestContentTemplateController_AddTemplateTags(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success with EDITOR", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Add Tags Tenant", "ATST01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Add Tags Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-at@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Template With Tags", nil)
		defer testhelper.CleanupTemplate(t, pool, templateID)

		tagID := testhelper.CreateTestTag(t, pool, workspaceID, "Contract", "#FF0000")
		defer testhelper.CleanupTag(t, pool, tagID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST(fmt.Sprintf("/api/v1/content/templates/%s/tags", templateID), map[string]interface{}{
				"tagIds": []string{tagID},
			})

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("success add multiple tags", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Add Tags Tenant 2", "ATST02")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-at2@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Multi Tags Template", nil)
		defer testhelper.CleanupTemplate(t, pool, templateID)

		tag1 := testhelper.CreateTestTag(t, pool, workspaceID, "Tag1", "#FF0000")
		defer testhelper.CleanupTag(t, pool, tag1)

		tag2 := testhelper.CreateTestTag(t, pool, workspaceID, "Tag2", "#00FF00")
		defer testhelper.CleanupTag(t, pool, tag2)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST(fmt.Sprintf("/api/v1/content/templates/%s/tags", templateID), map[string]interface{}{
				"tagIds": []string{tag1, tag2},
			})

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("forbidden with VIEWER", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Add Tags Tenant 3", "ATST03")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		viewer := testhelper.CreateTestUser(t, pool, "viewer-at@test.com", "Viewer User", nil)
		defer testhelper.CleanupUser(t, pool, viewer.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Viewer Tags", nil)
		defer testhelper.CleanupTemplate(t, pool, templateID)

		tagID := testhelper.CreateTestTag(t, pool, workspaceID, "ViewerTag", "#FF0000")
		defer testhelper.CleanupTag(t, pool, tagID)

		resp, _ := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST(fmt.Sprintf("/api/v1/content/templates/%s/tags", templateID), map[string]interface{}{
				"tagIds": []string{tagID},
			})

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("error - template not found", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Add Tags Tenant 4", "ATST04")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-at3@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		tagID := testhelper.CreateTestTag(t, pool, workspaceID, "OrphanTag", "#FF0000")
		defer testhelper.CleanupTag(t, pool, tagID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/00000000-0000-0000-0000-000000000000/tags", map[string]interface{}{
				"tagIds": []string{tagID},
			})

		// API returns 500 when template not found during tag add (FK constraint)
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("error - tag not found", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Add Tags Tenant 5", "ATST05")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-at4@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Template", nil)
		defer testhelper.CleanupTemplate(t, pool, templateID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST(fmt.Sprintf("/api/v1/content/templates/%s/tags", templateID), map[string]interface{}{
				"tagIds": []string{"00000000-0000-0000-0000-000000000000"},
			})

		// API returns 500 when tag not found during add (FK constraint)
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})
}

// TestContentTemplateController_RemoveTemplateTag tests the DELETE /content/templates/:templateId/tags/:tagId endpoint.
func TestContentTemplateController_RemoveTemplateTag(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success with EDITOR", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Remove Tag Tenant", "RTST01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Remove Tag Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-rt@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Template With Tag", nil)
		defer testhelper.CleanupTemplate(t, pool, templateID)

		tagID := testhelper.CreateTestTag(t, pool, workspaceID, "ToRemove", "#FF0000")
		defer testhelper.CleanupTag(t, pool, tagID)

		// First add the tag
		addResp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST(fmt.Sprintf("/api/v1/content/templates/%s/tags", templateID), map[string]interface{}{
				"tagIds": []string{tagID},
			})
		assert.Equal(t, http.StatusNoContent, addResp.StatusCode)

		// Then remove it
		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE(fmt.Sprintf("/api/v1/content/templates/%s/tags/%s", templateID, tagID))

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("forbidden with VIEWER", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Remove Tag Tenant 2", "RTST02")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-rt2@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		viewer := testhelper.CreateTestUser(t, pool, "viewer-rt@test.com", "Viewer User", nil)
		defer testhelper.CleanupUser(t, pool, viewer.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Template", nil)
		defer testhelper.CleanupTemplate(t, pool, templateID)

		tagID := testhelper.CreateTestTag(t, pool, workspaceID, "Tag", "#FF0000")
		defer testhelper.CleanupTag(t, pool, tagID)

		// Add tag as editor
		client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST(fmt.Sprintf("/api/v1/content/templates/%s/tags", templateID), map[string]interface{}{
				"tagIds": []string{tagID},
			})

		// Try to remove as viewer
		resp, _ := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE(fmt.Sprintf("/api/v1/content/templates/%s/tags/%s", templateID, tagID))

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("idempotent - template not found returns success", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Remove Tag Tenant 3", "RTST03")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-rt3@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		tagID := testhelper.CreateTestTag(t, pool, workspaceID, "Tag", "#FF0000")
		defer testhelper.CleanupTag(t, pool, tagID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE(fmt.Sprintf("/api/v1/content/templates/00000000-0000-0000-0000-000000000000/tags/%s", tagID))

		// RemoveTag is idempotent - returns success even if nothing to remove
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("idempotent - tag not associated returns success", func(t *testing.T) {
		tenantID := testhelper.CreateTestTenant(t, pool, "Remove Tag Tenant 4", "RTST04")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace", entity.WorkspaceTypeClient)
		defer testhelper.CleanupWorkspace(t, pool, workspaceID)

		editor := testhelper.CreateTestUser(t, pool, "editor-rt4@test.com", "Editor User", nil)
		defer testhelper.CleanupUser(t, pool, editor.ID)
		testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

		templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Template", nil)
		defer testhelper.CleanupTemplate(t, pool, templateID)

		// Tag exists but is not associated with template
		tagID := testhelper.CreateTestTag(t, pool, workspaceID, "Unassociated", "#FF0000")
		defer testhelper.CleanupTag(t, pool, tagID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE(fmt.Sprintf("/api/v1/content/templates/%s/tags/%s", templateID, tagID))

		// RemoveTag is idempotent - returns success even if tag wasn't associated
		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})
}

// =============================================================================
// Additional Coverage Tests
// =============================================================================

// TestContentTemplateController_CloneTemplate_PreservesTags tests that cloning preserves tags.
func TestContentTemplateController_CloneTemplate_PreservesTags(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Clone Tags Tenant", "CTST09")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Clone Tags Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	editor := testhelper.CreateTestUser(t, pool, "editor-clt@test.com", "Editor User", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	// Create source template with published version
	sourceTemplateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Source With Tags", nil)
	defer testhelper.CleanupTemplate(t, pool, sourceTemplateID)
	versionID := testhelper.CreateTestTemplateVersion(t, pool, sourceTemplateID, 1, "v1.0", entity.VersionStatusPublished)

	// Add tags to source template
	tag1 := testhelper.CreateTestTag(t, pool, workspaceID, "Contract", "#FF0000")
	defer testhelper.CleanupTag(t, pool, tag1)
	tag2 := testhelper.CreateTestTag(t, pool, workspaceID, "Legal", "#00FF00")
	defer testhelper.CleanupTag(t, pool, tag2)
	testhelper.CreateTestTemplateTag(t, pool, sourceTemplateID, tag1)
	testhelper.CreateTestTemplateTag(t, pool, sourceTemplateID, tag2)

	// Clone the template
	cloneResp, cloneBody := client.
		WithAuth(editor.BearerHeader).
		WithWorkspaceID(workspaceID).
		POST(fmt.Sprintf("/api/v1/content/templates/%s/clone", sourceTemplateID), map[string]interface{}{
			"newTitle":  "Cloned With Tags",
			"versionId": versionID,
		})

	assert.Equal(t, http.StatusCreated, cloneResp.StatusCode)

	var createResp dto.TemplateCreateResponse
	err := json.Unmarshal(cloneBody, &createResp)
	require.NoError(t, err)
	defer testhelper.CleanupTemplate(t, pool, createResp.Template.ID)

	// Get the cloned template details to verify tags
	getResp, getBody := client.
		WithAuth(editor.BearerHeader).
		WithWorkspaceID(workspaceID).
		GET(fmt.Sprintf("/api/v1/content/templates/%s", createResp.Template.ID))

	assert.Equal(t, http.StatusOK, getResp.StatusCode)

	var tplResp dto.TemplateWithDetailsResponse
	err = json.Unmarshal(getBody, &tplResp)
	require.NoError(t, err)

	// Verify tags were preserved
	assert.GreaterOrEqual(t, len(tplResp.Tags), 2, "cloned template should have at least 2 tags")
}
