//go:build integration

package controller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

// --- Version CRUD Tests ---

func TestTemplateVersionController_ListVersions(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup: tenant + workspace + users + template
	tenantID := testhelper.CreateTestTenant(t, pool, "Test Tenant", "TVLS01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Test Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	viewer := testhelper.CreateTestUser(t, pool, "viewer-list@test.com", "Viewer", nil)
	defer testhelper.CleanupUser(t, pool, viewer.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Test Template", nil)
	defer testhelper.CleanupTemplate(t, pool, templateID)

	versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "v1.0", entity.VersionStatusDraft)
	defer testhelper.CleanupTemplateVersion(t, pool, versionID)

	t.Run("success with VIEWER", func(t *testing.T) {
		resp, body := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/content/templates/" + templateID + "/versions")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var listResp dto.ListTemplateVersionsResponse
		err := json.Unmarshal(body, &listResp)
		require.NoError(t, err)
		assert.Equal(t, 1, listResp.Total)
		assert.Len(t, listResp.Items, 1)
		assert.Equal(t, "v1.0", listResp.Items[0].Name)
	})

	t.Run("missing workspace header", func(t *testing.T) {
		resp, _ := client.
			WithAuth(viewer.BearerHeader).
			GET("/api/v1/content/templates/" + templateID + "/versions")

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestTemplateVersionController_CreateVersion(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup
	tenantID := testhelper.CreateTestTenant(t, pool, "Test Tenant", "TVCR01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Test Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	editor := testhelper.CreateTestUser(t, pool, "editor-create@test.com", "Editor", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	viewer := testhelper.CreateTestUser(t, pool, "viewer-create@test.com", "Viewer", nil)
	defer testhelper.CleanupUser(t, pool, viewer.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Test Template", nil)
	defer testhelper.CleanupTemplate(t, pool, templateID)

	t.Run("success with EDITOR", func(t *testing.T) {
		desc := "First version"
		req := dto.CreateVersionRequest{
			Name:        "v1.0",
			Description: &desc,
		}
		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions", req)

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var versionResp dto.TemplateVersionResponse
		err := json.Unmarshal(body, &versionResp)
		require.NoError(t, err)
		assert.Equal(t, "v1.0", versionResp.Name)
		assert.Equal(t, "DRAFT", versionResp.Status)
		assert.Equal(t, 1, versionResp.VersionNumber)
	})

	t.Run("forbidden with VIEWER", func(t *testing.T) {
		req := dto.CreateVersionRequest{Name: "v2.0"}
		resp, _ := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions", req)

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("validation error - empty name", func(t *testing.T) {
		req := dto.CreateVersionRequest{Name: ""}
		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions", req)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestTemplateVersionController_CreateVersionFromExisting(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup
	tenantID := testhelper.CreateTestTenant(t, pool, "Test Tenant", "TVFE01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Test Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	editor := testhelper.CreateTestUser(t, pool, "editor-fromex@test.com", "Editor", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Test Template", nil)
	defer testhelper.CleanupTemplate(t, pool, templateID)

	sourceVersionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "Source Version", entity.VersionStatusDraft)
	defer testhelper.CleanupTemplateVersion(t, pool, sourceVersionID)

	t.Run("success", func(t *testing.T) {
		req := dto.CreateVersionFromExistingRequest{
			SourceVersionID: sourceVersionID,
			Name:            "Cloned Version",
		}
		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/from-existing", req)

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var versionResp dto.TemplateVersionResponse
		err := json.Unmarshal(body, &versionResp)
		require.NoError(t, err)
		assert.Equal(t, "Cloned Version", versionResp.Name)
		assert.Equal(t, "DRAFT", versionResp.Status)
	})

	t.Run("source version not found", func(t *testing.T) {
		req := dto.CreateVersionFromExistingRequest{
			SourceVersionID: "00000000-0000-0000-0000-000000000000",
			Name:            "Cloned",
		}
		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/from-existing", req)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestTemplateVersionController_GetVersion(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup
	tenantID := testhelper.CreateTestTenant(t, pool, "Test Tenant", "TVGT01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Test Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	viewer := testhelper.CreateTestUser(t, pool, "viewer-get@test.com", "Viewer", nil)
	defer testhelper.CleanupUser(t, pool, viewer.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Test Template", nil)
	defer testhelper.CleanupTemplate(t, pool, templateID)

	versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "v1.0", entity.VersionStatusDraft)
	defer testhelper.CleanupTemplateVersion(t, pool, versionID)

	t.Run("success with VIEWER", func(t *testing.T) {
		resp, body := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/content/templates/" + templateID + "/versions/" + versionID)

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var detailResp dto.TemplateVersionDetailResponse
		err := json.Unmarshal(body, &detailResp)
		require.NoError(t, err)
		assert.Equal(t, versionID, detailResp.ID)
		assert.Equal(t, "v1.0", detailResp.Name)
	})

	t.Run("not found", func(t *testing.T) {
		resp, _ := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/content/templates/" + templateID + "/versions/00000000-0000-0000-0000-000000000000")

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestTemplateVersionController_UpdateVersion(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup
	tenantID := testhelper.CreateTestTenant(t, pool, "Test Tenant", "TVUP01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Test Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	editor := testhelper.CreateTestUser(t, pool, "editor-upd@test.com", "Editor", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Test Template", nil)
	defer testhelper.CleanupTemplate(t, pool, templateID)

	t.Run("success with EDITOR", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "Original", entity.VersionStatusDraft)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		name := "Updated Name"
		desc := "Updated description"
		req := dto.UpdateVersionRequest{
			Name:        &name,
			Description: &desc,
		}
		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT("/api/v1/content/templates/"+templateID+"/versions/"+versionID, req)

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var versionResp dto.TemplateVersionResponse
		err := json.Unmarshal(body, &versionResp)
		require.NoError(t, err)
		assert.Equal(t, "Updated Name", versionResp.Name)
	})

	t.Run("cannot edit published version", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 2, "Published", entity.VersionStatusPublished)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		name := "Try Update"
		req := dto.UpdateVersionRequest{Name: &name}
		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT("/api/v1/content/templates/"+templateID+"/versions/"+versionID, req)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestTemplateVersionController_DeleteVersion(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup
	tenantID := testhelper.CreateTestTenant(t, pool, "Test Tenant", "TVDL01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Test Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	admin := testhelper.CreateTestUser(t, pool, "admin-del@test.com", "Admin", nil)
	defer testhelper.CleanupUser(t, pool, admin.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

	editor := testhelper.CreateTestUser(t, pool, "editor-del@test.com", "Editor", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Test Template", nil)
	defer testhelper.CleanupTemplate(t, pool, templateID)

	t.Run("success with ADMIN", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "To Delete", entity.VersionStatusDraft)

		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE("/api/v1/content/templates/" + templateID + "/versions/" + versionID)

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("forbidden with EDITOR", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 2, "To Delete", entity.VersionStatusDraft)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE("/api/v1/content/templates/" + templateID + "/versions/" + versionID)

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}

// --- Lifecycle Tests ---

func TestTemplateVersionController_PublishVersion(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup
	tenantID := testhelper.CreateTestTenant(t, pool, "Test Tenant", "TVPB01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Test Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	admin := testhelper.CreateTestUser(t, pool, "admin-pub@test.com", "Admin", nil)
	defer testhelper.CleanupUser(t, pool, admin.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

	editor := testhelper.CreateTestUser(t, pool, "editor-pub@test.com", "Editor", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Test Template", nil)
	defer testhelper.CleanupTemplate(t, pool, templateID)

	t.Run("success with ADMIN", func(t *testing.T) {
		versionID := testhelper.CreateTestPublishableTemplateVersion(t, pool, templateID, 1, "v1.0")
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/publish", "")

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("forbidden with EDITOR", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 2, "v2.0", entity.VersionStatusDraft)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/publish", "")

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("already published", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 3, "v3.0", entity.VersionStatusPublished)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/publish", "")

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestTemplateVersionController_ArchiveVersion(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup
	tenantID := testhelper.CreateTestTenant(t, pool, "Test Tenant", "TVAR01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Test Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	admin := testhelper.CreateTestUser(t, pool, "admin-arch@test.com", "Admin", nil)
	defer testhelper.CleanupUser(t, pool, admin.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Test Template", nil)
	defer testhelper.CleanupTemplate(t, pool, templateID)

	t.Run("success with published version", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "v1.0", entity.VersionStatusPublished)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/archive", "")

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("cannot archive draft version", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 2, "v2.0", entity.VersionStatusDraft)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/archive", "")

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestTemplateVersionController_SchedulePublish(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup
	tenantID := testhelper.CreateTestTenant(t, pool, "Test Tenant", "TVSP01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Test Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	admin := testhelper.CreateTestUser(t, pool, "admin-sched@test.com", "Admin", nil)
	defer testhelper.CleanupUser(t, pool, admin.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Test Template", nil)
	defer testhelper.CleanupTemplate(t, pool, templateID)

	t.Run("success", func(t *testing.T) {
		versionID := testhelper.CreateTestPublishableTemplateVersion(t, pool, templateID, 1, "v1.0")
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		futureTime := time.Now().Add(24 * time.Hour).UTC()
		req := dto.SchedulePublishRequest{PublishAt: futureTime}
		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/schedule-publish", req)

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("time in past", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 2, "v2.0", entity.VersionStatusDraft)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		pastTime := time.Now().Add(-1 * time.Hour).UTC()
		req := dto.SchedulePublishRequest{PublishAt: pastTime}
		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/schedule-publish", req)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

func TestTemplateVersionController_ScheduleArchive(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup
	tenantID := testhelper.CreateTestTenant(t, pool, "Test Tenant", "TVSA01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Test Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	admin := testhelper.CreateTestUser(t, pool, "admin-scha@test.com", "Admin", nil)
	defer testhelper.CleanupUser(t, pool, admin.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

	editor := testhelper.CreateTestUser(t, pool, "editor-scha@test.com", "Editor", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Test Template", nil)
	defer testhelper.CleanupTemplate(t, pool, templateID)

	t.Run("success with published version and scheduled replacement", func(t *testing.T) {
		// First create a published version
		publishedVersionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "v1.0", entity.VersionStatusPublished)
		defer testhelper.CleanupTemplateVersion(t, pool, publishedVersionID)

		// Create a scheduled version (replacement) - must exist for ScheduleArchive to work
		scheduledVersionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 2, "v2.0", entity.VersionStatusScheduled)
		defer testhelper.CleanupTemplateVersion(t, pool, scheduledVersionID)

		futureTime := time.Now().Add(24 * time.Hour).UTC()
		req := dto.ScheduleArchiveRequest{ArchiveAt: futureTime}
		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/"+publishedVersionID+"/schedule-archive", req)

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("cannot archive without replacement", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 3, "v3.0", entity.VersionStatusPublished)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		futureTime := time.Now().Add(24 * time.Hour).UTC()
		req := dto.ScheduleArchiveRequest{ArchiveAt: futureTime}
		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/schedule-archive", req)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("forbidden with EDITOR", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 4, "v4.0", entity.VersionStatusPublished)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		futureTime := time.Now().Add(24 * time.Hour).UTC()
		req := dto.ScheduleArchiveRequest{ArchiveAt: futureTime}
		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/schedule-archive", req)

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("time in past", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 5, "v5.0", entity.VersionStatusPublished)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		pastTime := time.Now().Add(-1 * time.Hour).UTC()
		req := dto.ScheduleArchiveRequest{ArchiveAt: pastTime}
		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/schedule-archive", req)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("cannot schedule archive for draft version", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 6, "v6.0", entity.VersionStatusDraft)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		futureTime := time.Now().Add(24 * time.Hour).UTC()
		req := dto.ScheduleArchiveRequest{ArchiveAt: futureTime}
		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/schedule-archive", req)

		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("not found", func(t *testing.T) {
		futureTime := time.Now().Add(24 * time.Hour).UTC()
		req := dto.ScheduleArchiveRequest{ArchiveAt: futureTime}
		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/00000000-0000-0000-0000-000000000000/schedule-archive", req)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestTemplateVersionController_CancelSchedule(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup
	tenantID := testhelper.CreateTestTenant(t, pool, "Test Tenant", "TVCS01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Test Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	admin := testhelper.CreateTestUser(t, pool, "admin-cancel@test.com", "Admin", nil)
	defer testhelper.CleanupUser(t, pool, admin.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, admin.ID, entity.WorkspaceRoleAdmin, nil)

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Test Template", nil)
	defer testhelper.CleanupTemplate(t, pool, templateID)

	t.Run("success with scheduled version", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "v1.0", entity.VersionStatusScheduled)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		resp, _ := client.
			WithAuth(admin.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE("/api/v1/content/templates/" + templateID + "/versions/" + versionID + "/schedule")

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})
}

func TestTemplateVersionController_PromoteVersionAsNewTemplateUsesPersistedTemplateID(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Promotion Tenant", "TVPR01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	prodWorkspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Promotion Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, prodWorkspaceID)

	sandboxWorkspaceID := testhelper.CreateTestSandboxWorkspace(t, pool, prodWorkspaceID)
	defer testhelper.CleanupWorkspace(t, pool, sandboxWorkspaceID)

	editor := testhelper.CreateTestUser(t, pool, "editor-promote@test.com", "Editor", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, prodWorkspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	sourceTemplateID := testhelper.CreateTestTemplate(t, pool, sandboxWorkspaceID, "Sandbox Promotion Source", nil)
	defer testhelper.CleanupTemplate(t, pool, sourceTemplateID)

	sourceVersionID := testhelper.CreateTestTemplateVersion(
		t,
		pool,
		sourceTemplateID,
		1,
		"Published Sandbox Version",
		entity.VersionStatusPublished,
	)

	req := dto.PromoteVersionRequest{
		Mode:        dto.PromotionModeNewTemplate,
		VersionName: ptrString("Promoted Version"),
	}
	resp, body := client.
		WithAuth(editor.BearerHeader).
		WithWorkspaceID(prodWorkspaceID).
		WithHeader("X-Sandbox-Mode", "false").
		POST("/api/v1/content/templates/"+sourceTemplateID+"/versions/"+sourceVersionID+"/promote", req)

	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))

	var promoteResp dto.PromoteAsNewTemplateResponse
	err := json.Unmarshal(body, &promoteResp)
	require.NoError(t, err)
	defer testhelper.CleanupTemplate(t, pool, promoteResp.Template.ID)

	assert.Equal(t, prodWorkspaceID, promoteResp.Template.WorkspaceID)
	assert.Equal(t, promoteResp.Template.ID, promoteResp.Version.TemplateID)

	var persistedTemplateID string
	err = pool.QueryRow(
		context.Background(),
		`SELECT template_id FROM content.template_versions WHERE id = $1`,
		promoteResp.Version.ID,
	).Scan(&persistedTemplateID)
	require.NoError(t, err)
	assert.Equal(t, promoteResp.Template.ID, persistedTemplateID)
}

func ptrString(value string) *string {
	return &value
}

// --- Injectable Tests ---

func TestTemplateVersionController_AddInjectable(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup
	tenantID := testhelper.CreateTestTenant(t, pool, "Test Tenant", "TVAI01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Test Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	editor := testhelper.CreateTestUser(t, pool, "editor-inj@test.com", "Editor", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	viewer := testhelper.CreateTestUser(t, pool, "viewer-inj@test.com", "Viewer", nil)
	defer testhelper.CleanupUser(t, pool, viewer.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Test Template", nil)
	defer testhelper.CleanupTemplate(t, pool, templateID)

	injectableID := testhelper.CreateTestInjectable(t, pool, &workspaceID, "client_name", "Client Name", entity.InjectableDataTypeText)
	defer testhelper.CleanupInjectable(t, pool, injectableID)

	t.Run("success with EDITOR", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "v1.0", entity.VersionStatusDraft)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		req := dto.AddVersionInjectableRequest{
			InjectableDefinitionID: injectableID,
			IsRequired:             true,
		}
		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/injectables", req)

		assert.Equal(t, http.StatusCreated, resp.StatusCode)
	})

	t.Run("forbidden with VIEWER", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 2, "v2.0", entity.VersionStatusDraft)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		req := dto.AddVersionInjectableRequest{
			InjectableDefinitionID: injectableID,
			IsRequired:             true,
		}
		resp, _ := client.
			WithAuth(viewer.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/injectables", req)

		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("injectable already linked", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 3, "v3.0", entity.VersionStatusDraft)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		// Link injectable first
		testhelper.CreateTestVersionInjectable(t, pool, versionID, injectableID, true)

		// Try to link again
		req := dto.AddVersionInjectableRequest{
			InjectableDefinitionID: injectableID,
			IsRequired:             true,
		}
		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/injectables", req)

		assert.Equal(t, http.StatusConflict, resp.StatusCode)
	})
}

func TestTemplateVersionController_RemoveInjectable(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup
	tenantID := testhelper.CreateTestTenant(t, pool, "Test Tenant", "TVRI01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Test Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	editor := testhelper.CreateTestUser(t, pool, "editor-rminj@test.com", "Editor", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Test Template", nil)
	defer testhelper.CleanupTemplate(t, pool, templateID)

	injectableID := testhelper.CreateTestInjectable(t, pool, &workspaceID, "client_address", "Client Address", entity.InjectableDataTypeText)
	defer testhelper.CleanupInjectable(t, pool, injectableID)

	t.Run("success with EDITOR", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "v1.0", entity.VersionStatusDraft)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		versionInjectableID := testhelper.CreateTestVersionInjectable(t, pool, versionID, injectableID, true)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE("/api/v1/content/templates/" + templateID + "/versions/" + versionID + "/injectables/" + versionInjectableID)

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("not found", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 2, "v2.0", entity.VersionStatusDraft)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE("/api/v1/content/templates/" + templateID + "/versions/" + versionID + "/injectables/00000000-0000-0000-0000-000000000000")

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// --- Signer Role Tests ---

func TestTemplateVersionController_AddSignerRole(t *testing.T) {
	t.Skip("signer roles are now extracted from PortableDocument content during publish")
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup
	tenantID := testhelper.CreateTestTenant(t, pool, "Test Tenant", "TVSR01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Test Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	editor := testhelper.CreateTestUser(t, pool, "editor-sr@test.com", "Editor", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Test Template", nil)
	defer testhelper.CleanupTemplate(t, pool, templateID)

	t.Run("success with EDITOR", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "v1.0", entity.VersionStatusDraft)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		req := dto.AddVersionSignerRoleRequest{
			RoleName:     "Buyer",
			AnchorString: "__sig_buyer__",
			SignerOrder:  1,
		}
		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/signer-roles", req)

		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var roleResp dto.TemplateVersionSignerRoleResponse
		err := json.Unmarshal(body, &roleResp)
		require.NoError(t, err)
		assert.Equal(t, "Buyer", roleResp.RoleName)
		assert.Equal(t, "__sig_buyer__", roleResp.AnchorString)
		assert.Equal(t, 1, roleResp.SignerOrder)
	})

	t.Run("duplicate anchor", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 2, "v2.0", entity.VersionStatusDraft)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		// Add first role
		testhelper.CreateTestSignerRole(t, pool, versionID, "Seller", "__sig_seller__", 1)

		// Try to add with same anchor
		req := dto.AddVersionSignerRoleRequest{
			RoleName:     "Different",
			AnchorString: "__sig_seller__",
			SignerOrder:  2,
		}
		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/signer-roles", req)

		assert.Equal(t, http.StatusConflict, resp.StatusCode)
	})

	t.Run("duplicate order", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 3, "v3.0", entity.VersionStatusDraft)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		// Add first role with order 1
		testhelper.CreateTestSignerRole(t, pool, versionID, "First", "__sig_first__", 1)

		// Try to add with same order
		req := dto.AddVersionSignerRoleRequest{
			RoleName:     "Second",
			AnchorString: "__sig_second__",
			SignerOrder:  1,
		}
		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			POST("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/signer-roles", req)

		assert.Equal(t, http.StatusConflict, resp.StatusCode)
	})
}

func TestTemplateVersionController_UpdateSignerRole(t *testing.T) {
	t.Skip("signer roles are now extracted from PortableDocument content during publish")
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup
	tenantID := testhelper.CreateTestTenant(t, pool, "Test Tenant", "TVUS01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Test Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	editor := testhelper.CreateTestUser(t, pool, "editor-usr@test.com", "Editor", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Test Template", nil)
	defer testhelper.CleanupTemplate(t, pool, templateID)

	t.Run("success with EDITOR", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "v1.0", entity.VersionStatusDraft)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		roleID := testhelper.CreateTestSignerRole(t, pool, versionID, "Original", "__sig_original__", 1)

		req := dto.UpdateVersionSignerRoleRequest{
			RoleName:    "Updated Role",
			SignerOrder: 1,
		}
		resp, body := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/signer-roles/"+roleID, req)

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var roleResp dto.TemplateVersionSignerRoleResponse
		err := json.Unmarshal(body, &roleResp)
		require.NoError(t, err)
		assert.Equal(t, "Updated Role", roleResp.RoleName)
	})

	t.Run("not found", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 2, "v2.0", entity.VersionStatusDraft)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		req := dto.UpdateVersionSignerRoleRequest{
			RoleName:    "Updated",
			SignerOrder: 1,
		}
		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			PUT("/api/v1/content/templates/"+templateID+"/versions/"+versionID+"/signer-roles/00000000-0000-0000-0000-000000000000", req)

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestTemplateVersionController_RemoveSignerRole(t *testing.T) {
	t.Skip("signer roles are now extracted from PortableDocument content during publish")
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Setup
	tenantID := testhelper.CreateTestTenant(t, pool, "Test Tenant", "TVRS01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Test Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	editor := testhelper.CreateTestUser(t, pool, "editor-rmsr@test.com", "Editor", nil)
	defer testhelper.CleanupUser(t, pool, editor.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, editor.ID, entity.WorkspaceRoleEditor, nil)

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Test Template", nil)
	defer testhelper.CleanupTemplate(t, pool, templateID)

	t.Run("success with EDITOR", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "v1.0", entity.VersionStatusDraft)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		roleID := testhelper.CreateTestSignerRole(t, pool, versionID, "To Delete", "__sig_delete__", 1)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE("/api/v1/content/templates/" + templateID + "/versions/" + versionID + "/signer-roles/" + roleID)

		assert.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("not found", func(t *testing.T) {
		versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 2, "v2.0", entity.VersionStatusDraft)
		defer testhelper.CleanupTemplateVersion(t, pool, versionID)

		resp, _ := client.
			WithAuth(editor.BearerHeader).
			WithWorkspaceID(workspaceID).
			DELETE("/api/v1/content/templates/" + templateID + "/versions/" + versionID + "/signer-roles/00000000-0000-0000-0000-000000000000")

		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}
