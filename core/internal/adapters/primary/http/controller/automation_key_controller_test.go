//go:build integration

package controller_test

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

// superadminClient creates a test server and returns a client authenticated as SUPERADMIN.
func superadminClient(t *testing.T) (*testhelper.TestServer, *testhelper.HTTPClient) {
	t.Helper()
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	superRole := entity.SystemRoleSuperAdmin
	admin := testhelper.CreateTestUser(t, pool, fmt.Sprintf("super-%s@test.com", t.Name()), "Super Admin", &superRole)
	t.Cleanup(func() { testhelper.CleanupUser(t, pool, admin.ID) })
	return ts, testhelper.NewHTTPClient(t, ts.URL()).WithAuth(admin.BearerHeader)
}

// TestAutomationKeyController_CreateKey tests POST /api/v1/automation-keys.
func TestAutomationKeyController_CreateKey(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts, client := superadminClient(t)
	_ = ts

	t.Run("success creates key with correct fields", func(t *testing.T) {
		req := dto.CreateAutomationKeyRequest{
			Name:    "my-key",
			KeyType: "automation",
		}

		resp, body := client.POST("/api/v1/automation-keys/", req)
		require.Equal(t, http.StatusCreated, resp.StatusCode)

		created := testhelper.ParseJSON[dto.CreateAutomationKeyResponse](t, body)

		assert.NotEmpty(t, created.ID)
		assert.Equal(t, "my-key", created.Name)
		assert.Equal(t, "automation", created.KeyType)
		assert.True(t, strings.HasPrefix(created.RawKey, "doca_"), "rawKey should start with doca_")
		assert.NotEmpty(t, created.KeyPrefix)
		assert.True(t, created.IsActive)

		t.Cleanup(func() {
			testhelper.CleanupAutomationKey(t, pool, created.ID)
		})
	})

	t.Run("forbidden for non-superadmin user", func(t *testing.T) {
		regularUser := testhelper.CreateTestUser(t, pool, fmt.Sprintf("regular-%s@test.com", t.Name()), "Regular User", nil)
		t.Cleanup(func() { testhelper.CleanupUser(t, pool, regularUser.ID) })

		regularClient := testhelper.NewHTTPClient(t, ts.URL()).WithAuth(regularUser.BearerHeader)
		req := dto.CreateAutomationKeyRequest{
			Name: "should-fail",
		}

		resp, _ := regularClient.POST("/api/v1/automation-keys/", req)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("bad request when name is missing", func(t *testing.T) {
		resp, _ := client.POST("/api/v1/automation-keys/", map[string]string{})
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})
}

// TestAutomationKeyController_ListKeys tests GET /api/v1/automation-keys.
func TestAutomationKeyController_ListKeys(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts, client := superadminClient(t)
	_ = ts

	// Pre-create 2 keys via fixture
	keyID1, _ := testhelper.CreateTestAutomationKey(t, pool, "list-key-1", nil)
	t.Cleanup(func() { testhelper.CleanupAutomationKey(t, pool, keyID1) })

	keyID2, _ := testhelper.CreateTestAutomationKey(t, pool, "list-key-2", nil)
	t.Cleanup(func() { testhelper.CleanupAutomationKey(t, pool, keyID2) })

	resp, body := client.GET("/api/v1/automation-keys/")
	require.Equal(t, http.StatusOK, resp.StatusCode)

	listResp := testhelper.ParseJSON[dto.ListResponse[dto.AutomationKeyResponse]](t, body)

	assert.GreaterOrEqual(t, listResp.Count, 2)
	assert.GreaterOrEqual(t, len(listResp.Data), 2)

	// Verify both created keys appear by ID matching
	foundIDs := make(map[string]bool)
	for _, k := range listResp.Data {
		foundIDs[k.ID] = true
	}
	assert.True(t, foundIDs[keyID1], "key 1 should appear in list")
	assert.True(t, foundIDs[keyID2], "key 2 should appear in list")
}

// TestAutomationKeyController_GetKey tests GET /api/v1/automation-keys/:id.
func TestAutomationKeyController_GetKey(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts, client := superadminClient(t)
	_ = ts

	keyID, _ := testhelper.CreateTestAutomationKey(t, pool, "get-key-test", nil)
	t.Cleanup(func() { testhelper.CleanupAutomationKey(t, pool, keyID) })

	t.Run("success returns key by id", func(t *testing.T) {
		resp, body := client.GET("/api/v1/automation-keys/" + keyID)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		keyResp := testhelper.ParseJSON[dto.AutomationKeyResponse](t, body)

		assert.Equal(t, keyID, keyResp.ID)
		assert.Equal(t, "get-key-test", keyResp.Name)
		assert.True(t, keyResp.IsActive)
	})

	t.Run("not found for nonexistent id", func(t *testing.T) {
		resp, _ := client.GET("/api/v1/automation-keys/00000000-0000-0000-0000-000000000000")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

// TestAutomationKeyController_UpdateKey tests PATCH /api/v1/automation-keys/:id.
func TestAutomationKeyController_UpdateKey(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts, client := superadminClient(t)
	_ = ts

	keyID, _ := testhelper.CreateTestAutomationKey(t, pool, "original-name", nil)
	t.Cleanup(func() { testhelper.CleanupAutomationKey(t, pool, keyID) })

	t.Run("success updates key name", func(t *testing.T) {
		newName := "new-name"
		req := dto.UpdateAutomationKeyRequest{
			Name: &newName,
		}

		resp, body := client.PATCH("/api/v1/automation-keys/"+keyID, req)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		updated := testhelper.ParseJSON[dto.AutomationKeyResponse](t, body)
		assert.Equal(t, "new-name", updated.Name)
		assert.Equal(t, keyID, updated.ID)
	})

	t.Run("verify name persisted via get", func(t *testing.T) {
		resp, body := client.GET("/api/v1/automation-keys/" + keyID)
		require.Equal(t, http.StatusOK, resp.StatusCode)

		keyResp := testhelper.ParseJSON[dto.AutomationKeyResponse](t, body)
		assert.Equal(t, "new-name", keyResp.Name)
	})
}

// TestAutomationKeyController_RevokeKey tests DELETE /api/v1/automation-keys/:id.
func TestAutomationKeyController_RevokeKey(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts, client := superadminClient(t)

	// Create tenant so we can make automation requests
	tenantID := testhelper.CreateTestTenant(t, pool, "Revoke Test Tenant", "RVKT1")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, tenantID) })

	keyID, rawKey := testhelper.CreateTestAutomationKey(t, pool, "revoke-key-test", nil)
	t.Cleanup(func() { testhelper.CleanupAutomationKey(t, pool, keyID) })

	t.Run("success returns 204", func(t *testing.T) {
		resp, _ := client.DELETE("/api/v1/automation-keys/" + keyID)
		require.Equal(t, http.StatusNoContent, resp.StatusCode)
	})

	t.Run("revoked key returns 401 on automation endpoint", func(t *testing.T) {
		automationClient := testhelper.NewHTTPClient(t, ts.URL()).WithAutomationKey(rawKey)
		resp, _ := automationClient.GET("/api/v1/automation/tenants")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

// TestAutomationKeyController_GetAuditLog tests GET /api/v1/automation-keys/:id/audit.
func TestAutomationKeyController_GetAuditLog(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts, client := superadminClient(t)

	// Create tenant for automation requests
	tenantID := testhelper.CreateTestTenant(t, pool, "Audit Test Tenant", "ADTT1")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, tenantID) })

	keyID, rawKey := testhelper.CreateTestAutomationKey(t, pool, "audit-key-test", nil)
	t.Cleanup(func() { testhelper.CleanupAutomationKey(t, pool, keyID) })

	t.Run("empty list initially", func(t *testing.T) {
		resp, body := client.GET("/api/v1/automation-keys/" + keyID + "/audit")
		require.Equal(t, http.StatusOK, resp.StatusCode)

		auditResp := testhelper.ParseJSON[dto.ListResponse[dto.AutomationAuditLogResponse]](t, body)
		assert.Equal(t, 0, auditResp.Count)
		assert.Empty(t, auditResp.Data)
	})

	t.Run("audit log populated after automation request", func(t *testing.T) {
		// Make one automation request using the key
		automationClient := testhelper.NewHTTPClient(t, ts.URL()).WithAutomationKey(rawKey)
		automationResp, _ := automationClient.GET("/api/v1/automation/tenants")
		assert.Equal(t, http.StatusOK, automationResp.StatusCode)

		// Wait for async audit write (goroutine with 3s timeout)
		time.Sleep(200 * time.Millisecond)

		resp, body := client.GET("/api/v1/automation-keys/" + keyID + "/audit")
		require.Equal(t, http.StatusOK, resp.StatusCode)

		auditResp := testhelper.ParseJSON[dto.ListResponse[dto.AutomationAuditLogResponse]](t, body)
		assert.GreaterOrEqual(t, auditResp.Count, 1)
		assert.GreaterOrEqual(t, len(auditResp.Data), 1)

		// Verify audit entry references our key
		assert.Equal(t, keyID, auditResp.Data[0].APIKeyID)
	})
}
