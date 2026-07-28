//go:build integration

package controller_test

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

func TestGalleryController_WorkspaceScopedAssetLifecycle(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Gallery Tenant", "GALT01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Gallery Workspace", entity.WorkspaceTypeClient)

	owner := testhelper.CreateTestUser(t, pool, "gallery-owner@test.com", "Gallery Owner", nil)
	defer testhelper.CleanupUser(t, pool, owner.ID)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)

	uploadResp, uploadBody := uploadGalleryAsset(t, ts.URL(), owner.BearerHeader, tenantID, workspaceID, "logo.png", []byte("fake-image-png"), "image/png")
	assert.Equal(t, http.StatusCreated, uploadResp.StatusCode)

	uploaded := testhelper.ParseJSON[dto.GalleryAssetResponse](t, uploadBody)
	require.NotEmpty(t, uploaded.Key)
	assert.Equal(t, "logo.png", uploaded.Filename)

	listResp, listBody := client.
		WithAuth(owner.BearerHeader).
		WithWorkspaceID(workspaceID).
		GET("/api/v1/workspace/gallery?page=1&perPage=20")
	assert.Equal(t, http.StatusOK, listResp.StatusCode)

	listResult := testhelper.ParseJSON[dto.GalleryListResponse](t, listBody)
	require.Len(t, listResult.Items, 1)
	assert.Equal(t, uploaded.Key, listResult.Items[0].Key)

	searchResp, searchBody := client.
		WithAuth(owner.BearerHeader).
		WithWorkspaceID(workspaceID).
		GET("/api/v1/workspace/gallery/search?q=logo")
	assert.Equal(t, http.StatusOK, searchResp.StatusCode)

	searchResult := testhelper.ParseJSON[dto.GalleryListResponse](t, searchBody)
	require.Len(t, searchResult.Items, 1)
	assert.Equal(t, uploaded.Key, searchResult.Items[0].Key)

	urlResp, urlBody := client.
		WithAuth(owner.BearerHeader).
		WithWorkspaceID(workspaceID).
		GET("/api/v1/workspace/gallery/url?key=" + url.QueryEscape(uploaded.Key))
	assert.Equal(t, http.StatusOK, urlResp.StatusCode)

	resolved := testhelper.ParseJSON[dto.GalleryURLResponse](t, urlBody)
	assert.True(t, strings.Contains(resolved.URL, "/api/v1/workspace/gallery/serve?key="), "expected local serve URL, got %q", resolved.URL)

	serveResp, serveBody := client.
		WithAuth(owner.BearerHeader).
		WithWorkspaceID(workspaceID).
		GET("/api/v1/workspace/gallery/serve?key=" + url.QueryEscape(uploaded.Key))
	assert.Equal(t, http.StatusOK, serveResp.StatusCode)
	assert.Equal(t, "image/png", serveResp.Header.Get("Content-Type"))
	assert.Equal(t, []byte("fake-image-png"), serveBody)

	deleteResp, _ := client.
		WithAuth(owner.BearerHeader).
		WithWorkspaceID(workspaceID).
		DELETE("/api/v1/workspace/gallery?key=" + url.QueryEscape(uploaded.Key))
	assert.Equal(t, http.StatusNoContent, deleteResp.StatusCode)

	notFoundResp, _ := client.
		WithAuth(owner.BearerHeader).
		WithWorkspaceID(workspaceID).
		GET("/api/v1/workspace/gallery/url?key=" + url.QueryEscape(uploaded.Key))
	assert.Equal(t, http.StatusNotFound, notFoundResp.StatusCode)
}

func TestGalleryController_RejectsCrossWorkspaceAccessByKey(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Gallery Tenant 2", "GALT02")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceA := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace A", entity.WorkspaceTypeClient)
	workspaceB := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Workspace B", entity.WorkspaceTypeClient)

	owner := testhelper.CreateTestUser(t, pool, "gallery-cross@test.com", "Gallery Cross", nil)
	defer testhelper.CleanupUser(t, pool, owner.ID)
	defer testhelper.CleanupWorkspace(t, pool, workspaceB)
	defer testhelper.CleanupWorkspace(t, pool, workspaceA)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceA, owner.ID, entity.WorkspaceRoleOwner, nil)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceB, owner.ID, entity.WorkspaceRoleOwner, nil)

	uploadResp, uploadBody := uploadGalleryAsset(t, ts.URL(), owner.BearerHeader, tenantID, workspaceA, "proof.png", []byte("fake-proof"), "image/png")
	assert.Equal(t, http.StatusCreated, uploadResp.StatusCode)
	uploaded := testhelper.ParseJSON[dto.GalleryAssetResponse](t, uploadBody)

	urlResp, _ := client.
		WithAuth(owner.BearerHeader).
		WithWorkspaceID(workspaceB).
		GET("/api/v1/workspace/gallery/url?key=" + url.QueryEscape(uploaded.Key))
	assert.Equal(t, http.StatusNotFound, urlResp.StatusCode)

	serveResp, _ := client.
		WithAuth(owner.BearerHeader).
		WithWorkspaceID(workspaceB).
		GET("/api/v1/workspace/gallery/serve?key=" + url.QueryEscape(uploaded.Key))
	assert.Equal(t, http.StatusNotFound, serveResp.StatusCode)

	deleteResp, _ := client.
		WithAuth(owner.BearerHeader).
		WithWorkspaceID(workspaceB).
		DELETE("/api/v1/workspace/gallery?key=" + url.QueryEscape(uploaded.Key))
	assert.Equal(t, http.StatusNotFound, deleteResp.StatusCode)

	ownerURLResp, _ := client.
		WithAuth(owner.BearerHeader).
		WithWorkspaceID(workspaceA).
		GET("/api/v1/workspace/gallery/url?key=" + url.QueryEscape(uploaded.Key))
	assert.Equal(t, http.StatusOK, ownerURLResp.StatusCode)
}

func uploadGalleryAsset(
	t *testing.T,
	baseURL, authHeader, tenantID, workspaceID, filename string,
	content []byte,
	contentType string,
) (*http.Response, []byte) {
	t.Helper()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	headers := textproto.MIMEHeader{}
	headers.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename="%s"`, filename))
	headers.Set("Content-Type", contentType)

	part, err := writer.CreatePart(headers)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req, err := http.NewRequest(http.MethodPost, baseURL+"/api/v1/workspace/gallery", &body)
	require.NoError(t, err)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("X-Tenant-ID", tenantID)
	req.Header.Set("X-Workspace-ID", workspaceID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	copyResp := *resp
	copyResp.Body = http.NoBody
	return &copyResp, respBody
}
