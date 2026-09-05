//go:build integration

package controller_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

// automationEnv holds all shared state for automation controller tests.
type automationEnv struct {
	ts          *testhelper.TestServer
	pool        *pgxpool.Pool
	client      *testhelper.HTTPClient
	keyID       string
	tenantID    string
	workspaceID string
}

// newAutomationEnv creates a fully initialised automation test environment.
func newAutomationEnv(t *testing.T) *automationEnv {
	t.Helper()
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)

	keyID, rawKey := testhelper.CreateTestAutomationKey(t, pool, "test-key", nil)
	t.Cleanup(func() { testhelper.CleanupAutomationKey(t, pool, keyID) })

	tenantID := testhelper.CreateTestTenant(t, pool, "Acme Corp", "ACME")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, tenantID) })

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Main WS", entity.WorkspaceTypeClient)
	t.Cleanup(func() { testhelper.CleanupWorkspace(t, pool, workspaceID) })

	client := testhelper.NewHTTPClient(t, ts.URL()).WithAutomationKey(rawKey)

	return &automationEnv{
		ts:          ts,
		pool:        pool,
		client:      client,
		keyID:       keyID,
		tenantID:    tenantID,
		workspaceID: workspaceID,
	}
}

// =============================================================================
// Auth Tests
// =============================================================================

func TestAutomationController_Auth(t *testing.T) {
	env := newAutomationEnv(t)

	t.Run("missing key", func(t *testing.T) {
		// A client with no automation key should be rejected.
		noKeyClient := testhelper.NewHTTPClient(t, env.ts.URL())
		resp, _ := noKeyClient.GET("/api/v1/automation/tenants")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("invalid key", func(t *testing.T) {
		// A client with a syntactically valid but unknown key should be rejected.
		badClient := testhelper.NewHTTPClient(t, env.ts.URL()).WithAutomationKey("doca_invalid")
		resp, _ := badClient.GET("/api/v1/automation/tenants")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("revoked key", func(t *testing.T) {
		revokedKeyID, revokedRawKey := testhelper.CreateTestAutomationKey(t, env.pool, "revoked-key", nil)
		t.Cleanup(func() { testhelper.CleanupAutomationKey(t, env.pool, revokedKeyID) })

		// Revoke the key directly in the database.
		_, err := env.pool.Exec(context.Background(),
			`UPDATE automation.api_keys SET revoked_at = NOW(), is_active = false WHERE id = $1`,
			revokedKeyID)
		require.NoError(t, err)

		revokedClient := testhelper.NewHTTPClient(t, env.ts.URL()).WithAutomationKey(revokedRawKey)
		resp, _ := revokedClient.GET("/api/v1/automation/tenants")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// =============================================================================
// Tenant Tests
// =============================================================================

func TestAutomationController_ListTenants(t *testing.T) {
	env := newAutomationEnv(t)

	t.Run("global key sees created tenant", func(t *testing.T) {
		resp, body := env.client.GET("/api/v1/automation/tenants")
		require.Equal(t, http.StatusOK, resp.StatusCode)

		result := testhelper.ParseJSON[dto.ListResponse[dto.TenantResponse]](t, body)
		assert.GreaterOrEqual(t, result.Count, 1)

		found := false
		for _, tenant := range result.Data {
			if tenant.ID == env.tenantID {
				found = true
				break
			}
		}
		assert.True(t, found, "expected to find tenantID=%s in list", env.tenantID)
	})

	t.Run("tenant-scoped key sees only allowed tenant", func(t *testing.T) {
		scopedKeyID, scopedRawKey := testhelper.CreateTestAutomationKey(t, env.pool, "scoped-key", []string{env.tenantID})
		t.Cleanup(func() { testhelper.CleanupAutomationKey(t, env.pool, scopedKeyID) })

		scopedClient := testhelper.NewHTTPClient(t, env.ts.URL()).WithAutomationKey(scopedRawKey)
		resp, body := scopedClient.GET("/api/v1/automation/tenants")
		require.Equal(t, http.StatusOK, resp.StatusCode)

		result := testhelper.ParseJSON[dto.ListResponse[dto.TenantResponse]](t, body)
		require.Equal(t, 1, result.Count)
		assert.Equal(t, env.tenantID, result.Data[0].ID)
	})

	t.Run("tenant-scoped key with different tenant sees filtered results", func(t *testing.T) {
		otherTenantID := testhelper.CreateTestTenant(t, env.pool, "Other Corp", "OTHER")
		t.Cleanup(func() { testhelper.CleanupTenant(t, env.pool, otherTenantID) })

		scopedKeyID, scopedRawKey := testhelper.CreateTestAutomationKey(t, env.pool, "scoped-other-key", []string{otherTenantID})
		t.Cleanup(func() { testhelper.CleanupAutomationKey(t, env.pool, scopedKeyID) })

		scopedClient := testhelper.NewHTTPClient(t, env.ts.URL()).WithAutomationKey(scopedRawKey)
		resp, body := scopedClient.GET("/api/v1/automation/tenants")
		require.Equal(t, http.StatusOK, resp.StatusCode)

		result := testhelper.ParseJSON[dto.ListResponse[dto.TenantResponse]](t, body)
		// Should only contain the other tenant, not env.tenantID.
		for _, tenant := range result.Data {
			assert.NotEqual(t, env.tenantID, tenant.ID, "should not see unallowed tenant")
		}
	})
}

// =============================================================================
// Workspace Tests
// =============================================================================

func TestAutomationController_Workspaces(t *testing.T) {
	env := newAutomationEnv(t)

	t.Run("list workspaces returns created workspace", func(t *testing.T) {
		path := fmt.Sprintf("/api/v1/automation/tenants/%s/workspaces", env.tenantID)
		resp, body := env.client.GET(path)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		result := testhelper.ParseJSON[dto.ListResponse[dto.WorkspaceResponse]](t, body)
		assert.GreaterOrEqual(t, result.Count, 1)

		found := false
		for _, ws := range result.Data {
			if ws.ID == env.workspaceID {
				found = true
				break
			}
		}
		assert.True(t, found, "expected to find workspaceID=%s in list", env.workspaceID)
	})

	t.Run("create workspace returns 201", func(t *testing.T) {
		path := fmt.Sprintf("/api/v1/automation/tenants/%s/workspaces", env.tenantID)
		resp, body := env.client.POST(path, map[string]string{"name": "New WS"})
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		created := testhelper.ParseJSON[dto.WorkspaceResponse](t, body)
		require.NotEmpty(t, created.ID)
		t.Cleanup(func() { testhelper.CleanupWorkspace(t, env.pool, created.ID) })
	})

	t.Run("tenant-scoped key cannot access other tenant workspaces", func(t *testing.T) {
		otherTenantID := testhelper.CreateTestTenant(t, env.pool, "Restricted Corp", "RSTR")
		t.Cleanup(func() { testhelper.CleanupTenant(t, env.pool, otherTenantID) })

		// Key is scoped to otherTenantID, so accessing env.tenantID should be forbidden.
		scopedKeyID, scopedRawKey := testhelper.CreateTestAutomationKey(t, env.pool, "scope-ws-key", []string{otherTenantID})
		t.Cleanup(func() { testhelper.CleanupAutomationKey(t, env.pool, scopedKeyID) })

		scopedClient := testhelper.NewHTTPClient(t, env.ts.URL()).WithAutomationKey(scopedRawKey)
		path := fmt.Sprintf("/api/v1/automation/tenants/%s/workspaces", env.tenantID)
		resp, _ := scopedClient.GET(path)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}

// =============================================================================
// Injectable Tests
// =============================================================================

func TestAutomationController_Injectables(t *testing.T) {
	env := newAutomationEnv(t)

	t.Run("list injectables returns 200", func(t *testing.T) {
		path := fmt.Sprintf("/api/v1/automation/workspaces/%s/injectables", env.workspaceID)
		resp, _ := env.client.GET(path)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

// =============================================================================
// Template Tests
// =============================================================================

func TestAutomationController_Templates(t *testing.T) {
	env := newAutomationEnv(t)

	// Create a template to work with.
	createPath := fmt.Sprintf("/api/v1/automation/workspaces/%s/templates", env.workspaceID)
	createResp, createBody := env.client.POST(createPath, map[string]string{"name": "My Template"})
	require.Equal(t, http.StatusCreated, createResp.StatusCode)

	created := testhelper.ParseJSON[dto.TemplateCreateResponse](t, createBody)
	require.NotNil(t, created.Template)
	templateID := created.Template.ID
	require.NotEmpty(t, templateID)
	t.Cleanup(func() { testhelper.CleanupTemplate(t, env.pool, templateID) })

	t.Run("list templates contains created template", func(t *testing.T) {
		listPath := fmt.Sprintf("/api/v1/automation/workspaces/%s/templates", env.workspaceID)
		resp, body := env.client.GET(listPath)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		result := testhelper.ParseJSON[dto.ListResponse[dto.TemplateListItemResponse]](t, body)
		assert.GreaterOrEqual(t, result.Count, 1)

		found := false
		for _, tpl := range result.Data {
			if tpl.ID == templateID {
				found = true
				break
			}
		}
		assert.True(t, found, "expected to find templateID=%s in list", templateID)
	})

	t.Run("get template returns 200", func(t *testing.T) {
		getPath := fmt.Sprintf("/api/v1/automation/workspaces/%s/templates/%s", env.workspaceID, templateID)
		resp, body := env.client.GET(getPath)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		detail := testhelper.ParseJSON[dto.TemplateWithDetailsResponse](t, body)
		assert.Equal(t, templateID, detail.ID)
	})

	t.Run("update template name returns 200", func(t *testing.T) {
		patchPath := fmt.Sprintf("/api/v1/automation/workspaces/%s/templates/%s", env.workspaceID, templateID)
		updatedName := "Updated Template"
		resp, body := env.client.PATCH(patchPath, map[string]string{"name": updatedName})
		require.Equal(t, http.StatusOK, resp.StatusCode)

		updated := testhelper.ParseJSON[dto.TemplateResponse](t, body)
		assert.Equal(t, updatedName, updated.Title)
	})
}

// =============================================================================
// Document Type Tests
// =============================================================================

func TestAutomationController_DocumentTypes(t *testing.T) {
	env := newAutomationEnv(t)

	t.Run("missing tenantId returns 400", func(t *testing.T) {
		resp, _ := env.client.GET("/api/v1/automation/document-types")
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("with tenantId returns 200", func(t *testing.T) {
		path := fmt.Sprintf("/api/v1/automation/document-types?tenantId=%s", env.tenantID)
		resp, _ := env.client.GET(path)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
	})
}

// =============================================================================
// Version Lifecycle Test
// =============================================================================

func TestAutomationController_VersionLifecycle(t *testing.T) {
	env := newAutomationEnv(t)

	// Step 1: Create template.
	createTplPath := fmt.Sprintf("/api/v1/automation/workspaces/%s/templates", env.workspaceID)
	tplResp, tplBody := env.client.POST(createTplPath, map[string]string{"name": "Lifecycle Template"})
	require.Equal(t, http.StatusCreated, tplResp.StatusCode)

	tplCreated := testhelper.ParseJSON[dto.TemplateCreateResponse](t, tplBody)
	require.NotNil(t, tplCreated.Template)
	templateID := tplCreated.Template.ID
	require.NotEmpty(t, templateID)
	t.Cleanup(func() { testhelper.CleanupTemplate(t, env.pool, templateID) })

	// Step 2: Create version.
	createVerPath := fmt.Sprintf("/api/v1/automation/templates/%s/versions", templateID)
	verResp, verBody := env.client.POST(createVerPath, map[string]string{
		"name":        "v1.0",
		"description": "First",
	})
	require.Equal(t, http.StatusCreated, verResp.StatusCode)

	versionCreated := testhelper.ParseJSON[dto.TemplateVersionResponse](t, verBody)
	versionID := versionCreated.ID
	require.NotEmpty(t, versionID)

	// Step 3: GET version.
	getVerPath := fmt.Sprintf("/api/v1/automation/templates/%s/versions/%s", templateID, versionID)
	resp, _ := env.client.GET(getVerPath)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// Step 4: PATCH version (update name).
	patchVerPath := fmt.Sprintf("/api/v1/automation/templates/%s/versions/%s", templateID, versionID)
	patchResp, patchBody := env.client.PATCH(patchVerPath, map[string]string{"name": "v1.0-updated"})
	require.Equal(t, http.StatusOK, patchResp.StatusCode)
	patched := testhelper.ParseJSON[dto.TemplateVersionResponse](t, patchBody)
	assert.Equal(t, "v1.0-updated", patched.Name)

	// Step 5: GET content.
	getContentPath := fmt.Sprintf("/api/v1/automation/templates/%s/versions/%s/content", templateID, versionID)
	contentResp, _ := env.client.GET(getContentPath)
	assert.Equal(t, http.StatusOK, contentResp.StatusCode)

	// Step 6: PUT content with valid minimal portabledoc JSON.
	putContentPath := fmt.Sprintf("/api/v1/automation/templates/%s/versions/%s/content", templateID, versionID)
	putResp, putBody := env.client.PUT(putContentPath, map[string]interface{}{
		"contentStructure": minimalPortabledoc(),
	})
	assert.Equal(t, http.StatusOK, putResp.StatusCode, "put content body: %s", string(putBody))

	// Step 7: Publish version.
	publishPath := fmt.Sprintf("/api/v1/automation/templates/%s/versions/%s/publish", templateID, versionID)
	publishResp, publishBody := env.client.POST(publishPath, nil)
	assert.Equal(t, http.StatusNoContent, publishResp.StatusCode, "publish body: %s", string(publishBody))

	// Step 8: Archive version.
	archivePath := fmt.Sprintf("/api/v1/automation/templates/%s/versions/%s/archive", templateID, versionID)
	archiveResp, _ := env.client.POST(archivePath, nil)
	assert.Equal(t, http.StatusNoContent, archiveResp.StatusCode)
}

// =============================================================================
// Non-Draft Content Update Test
// =============================================================================

func TestAutomationController_UpdateContentNonDraft(t *testing.T) {
	env := newAutomationEnv(t)

	// Create template.
	createTplPath := fmt.Sprintf("/api/v1/automation/workspaces/%s/templates", env.workspaceID)
	tplResp, tplBody := env.client.POST(createTplPath, map[string]string{"name": "NonDraft Template"})
	require.Equal(t, http.StatusCreated, tplResp.StatusCode)

	tplCreated := testhelper.ParseJSON[dto.TemplateCreateResponse](t, tplBody)
	require.NotNil(t, tplCreated.Template)
	templateID := tplCreated.Template.ID
	require.NotEmpty(t, templateID)
	t.Cleanup(func() { testhelper.CleanupTemplate(t, env.pool, templateID) })

	// Create version via fixture.
	versionID := testhelper.CreateTestTemplateVersion(t, env.pool, templateID, 2, "v2.0", entity.VersionStatusDraft)

	// Publish the version directly via fixture (bypassing API).
	testhelper.PublishTestVersion(t, env.pool, versionID)

	// Attempt to update content of a published version — must fail.
	putContentPath := fmt.Sprintf("/api/v1/automation/templates/%s/versions/%s/content", templateID, versionID)
	resp, _ := env.client.PUT(putContentPath, map[string]interface{}{
		"contentStructure": minimalPortabledoc(),
	})
	assert.NotEqual(t, http.StatusOK, resp.StatusCode, "updating content of a published version should fail")
	assert.GreaterOrEqual(t, resp.StatusCode, http.StatusBadRequest, "expected a 4xx error status")
}

// =============================================================================
// Large Body Content Update Test
// =============================================================================

// auditBodyCaptureLimit mirrors bodyCaptureLimitBytes in middleware/automation_audit.go.
// It bounds what the audit log persists, not what handlers may read: bodies above it must
// still reach updateVersionContent intact.
const auditBodyCaptureLimit = 64 * 1024

func TestAutomationController_UpdateContentLargeBody(t *testing.T) {
	env := newAutomationEnv(t)

	templateID := testhelper.CreateTestTemplate(t, env.pool, env.workspaceID, "Large Body Template", nil)
	t.Cleanup(func() { testhelper.CleanupTemplate(t, env.pool, templateID) })
	versionID := testhelper.CreateTestTemplateVersion(t, env.pool, templateID, 1, "v1.0", entity.VersionStatusDraft)

	// Pad the document with one paragraph whose text node alone exceeds the capture limit.
	doc := minimalPortabledoc()
	content := doc["content"].(map[string]interface{})
	nodes := content["content"].([]interface{})
	nodes = append(nodes, map[string]interface{}{
		"type": "paragraph",
		"content": []interface{}{
			map[string]interface{}{"type": "text", "text": strings.Repeat("x", 100*1024)},
		},
	})
	content["content"] = nodes

	reqBody := map[string]interface{}{"contentStructure": doc}
	raw, err := json.Marshal(reqBody)
	require.NoError(t, err)
	require.Greater(t, len(raw), auditBodyCaptureLimit, "test payload must exceed the audit capture limit")

	putContentPath := fmt.Sprintf("/api/v1/automation/templates/%s/versions/%s/content", templateID, versionID)
	putResp, putBody := env.client.PUT(putContentPath, reqBody)
	require.Equal(t, http.StatusOK, putResp.StatusCode, "put content body: %s", string(putBody))

	// The stored content must round-trip intact through the API.
	getResp, getBody := env.client.GET(putContentPath)
	require.Equal(t, http.StatusOK, getResp.StatusCode)
	detail := testhelper.ParseJSON[dto.TemplateVersionDetailResponse](t, getBody)
	expected, err := json.Marshal(doc)
	require.NoError(t, err)
	assert.JSONEq(t, string(expected), string(detail.ContentStructure))

	// The audit entry is written asynchronously. It must exist (a truncated JSON prefix would
	// be rejected by the JSONB column) and hold at most the capture limit.
	var stored json.RawMessage
	require.Eventually(t, func() bool {
		return env.pool.QueryRow(context.Background(),
			`SELECT request_body FROM automation.audit_log
			 WHERE api_key_id = $1 AND action = 'UPDATE_CONTENT' AND resource_id = $2
			 ORDER BY created_at DESC LIMIT 1`,
			env.keyID, versionID).Scan(&stored) == nil
	}, 5*time.Second, 100*time.Millisecond, "audit row for UPDATE_CONTENT was not written")

	require.True(t, json.Valid(stored), "stored request_body must be valid JSON: %s", string(stored))
	assert.LessOrEqual(t, len(stored), auditBodyCaptureLimit)
	marker := testhelper.ParseJSON[map[string]interface{}](t, stored)
	assert.Equal(t, true, marker["truncated"])
	assert.Equal(t, float64(len(raw)), marker["originalBytes"])
}

// =============================================================================
// Test Helpers
// =============================================================================

// minimalPortabledoc returns the smallest valid portabledoc structure that passes
// both content-update and publish validation:
//   - version, meta.title, pageConfig dimensions (required fields)
//   - one signer role with text name/email fields
//   - one signature node in the content referencing that role
func minimalPortabledoc() map[string]interface{} {
	roleID := "role-001"
	sigID := "sig-001"
	return map[string]interface{}{
		"version": "1.1.0",
		"meta": map[string]interface{}{
			"title":    "Test Document",
			"language": "en",
		},
		"pageConfig": map[string]interface{}{
			"formatId": "A4",
			"width":    595.0,
			"height":   842.0,
			"margins": map[string]interface{}{
				"top":    20.0,
				"bottom": 20.0,
				"left":   20.0,
				"right":  20.0,
			},
		},
		"variableIds": []interface{}{},
		"signerRoles": []interface{}{
			map[string]interface{}{
				"id":    roleID,
				"label": "Signer",
				"order": 1,
				"name": map[string]interface{}{
					"type":  "text",
					"value": "Test Signer",
				},
				"email": map[string]interface{}{
					"type":  "text",
					"value": "signer@test.com",
				},
			},
		},
		"content": map[string]interface{}{
			"type": "doc",
			"content": []interface{}{
				map[string]interface{}{
					"type": "signature",
					"attrs": map[string]interface{}{
						"count":     1,
						"layout":    "single-center",
						"lineWidth": "md",
						"signatures": []interface{}{
							map[string]interface{}{
								"id":     sigID,
								"roleId": roleID,
								"label":  "Signer",
							},
						},
					},
				},
			},
		},
		"exportInfo": map[string]interface{}{
			"exportedAt": "2026-01-01T00:00:00Z",
			"sourceApp":  "automation-test",
		},
	}
}
