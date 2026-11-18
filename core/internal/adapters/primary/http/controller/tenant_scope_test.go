//go:build integration

package controller_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

// tenantRows returns the tenants' workspace and tenant-member rows as one JSON
// document, so a test can tell whether a request changed any of them.
func tenantRows(t *testing.T, pool *pgxpool.Pool, tenantIDs ...string) string {
	t.Helper()
	var rows string
	err := pool.QueryRow(context.Background(), `
		SELECT json_build_object(
			'workspaces', (SELECT COALESCE(json_agg(w ORDER BY w.id), '[]')
				FROM tenancy.workspaces w WHERE w.tenant_id = ANY($1::uuid[])),
			'members', (SELECT COALESCE(json_agg(m ORDER BY m.id), '[]')
				FROM identity.tenant_members m WHERE m.tenant_id = ANY($1::uuid[]))
		)::text`, tenantIDs).Scan(&rows)
	require.NoError(t, err)
	return rows
}

// TestTenantController_ResourceFromAnotherTenant_NotFound covers the /tenant
// routes that name a workspace or a tenant member: the owner of one tenant
// cannot change another tenant's workspace, or read or change its members, by
// ID. Each route answers exactly what it answers for an ID that does not
// exist, and no workspace or member row of either tenant changes.
func TestTenantController_ResourceFromAnotherTenant_NotFound(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	victimTenantID := testhelper.CreateTestTenant(t, pool, "Tenant Scope Victim", "TNSCV1")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, victimTenantID) })
	victimAdmin := testhelper.CreateTestUser(t, pool, "victim-admin-tenant-scope@test.com", "Victim Admin", nil)
	t.Cleanup(func() { testhelper.CleanupUser(t, pool, victimAdmin.ID) })
	victimMemberID := testhelper.CreateTestTenantMember(t, pool, victimTenantID, victimAdmin.ID, entity.TenantRoleAdmin, nil)

	outsiderTenantID := testhelper.CreateTestTenant(t, pool, "Tenant Scope Outsider", "TNSCO1")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, outsiderTenantID) })
	outsider := testhelper.CreateTestUser(t, pool, "outsider-owner-tenant-scope@test.com", "Outsider Owner", nil)
	t.Cleanup(func() { testhelper.CleanupUser(t, pool, outsider.ID) })
	testhelper.CreateTestTenantMember(t, pool, outsiderTenantID, outsider.ID, entity.TenantRoleOwner, nil)
	fromOutsider := client.WithAuth(outsider.BearerHeader).WithTenantID(outsiderTenantID)

	routes := []struct {
		method string
		path   string // {workspace} and {member} stand for the victim's IDs
		body   any
	}{
		{http.MethodPatch, "/workspaces/{workspace}/status", map[string]any{"status": "SUSPENDED"}},
		{http.MethodDelete, "/workspaces/{workspace}", nil},
		{http.MethodGet, "/members/{member}", nil},
		{http.MethodPut, "/members/{member}", map[string]any{"role": "TENANT_OWNER"}},
		{http.MethodDelete, "/members/{member}", nil},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			victimWorkspaceID := testhelper.CreateTestWorkspace(t, pool, &victimTenantID, "Victim Workspace "+uuid.NewString()[:8], entity.WorkspaceTypeClient)
			t.Cleanup(func() { testhelper.CleanupWorkspace(t, pool, victimWorkspaceID) })
			before := tenantRows(t, pool, victimTenantID, outsiderTenantID)

			path := func(workspaceID, memberID string) string {
				return "/api/v1/tenant" + strings.NewReplacer("{workspace}", workspaceID, "{member}", memberID).Replace(route.path)
			}
			resp, body := fromOutsider.Do(route.method, path(victimWorkspaceID, victimMemberID), route.body)
			assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))

			missingResp, missingBody := fromOutsider.Do(route.method, path(uuid.NewString(), uuid.NewString()), route.body)
			assert.Equal(t, missingResp.StatusCode, resp.StatusCode, "status differs from a missing ID's")
			assert.Equal(t, string(missingBody), string(body), "body differs from a missing ID's")

			assert.JSONEq(t, before, tenantRows(t, pool, victimTenantID, outsiderTenantID), "a workspace or member row changed")
		})
	}

	// The guard lets a workspace of the caller's own tenant through.
	t.Run("PATCH /workspaces/{own workspace}/status", func(t *testing.T) {
		ownWorkspaceID := testhelper.CreateTestWorkspace(t, pool, &outsiderTenantID, "Own Workspace "+uuid.NewString()[:8], entity.WorkspaceTypeClient)
		t.Cleanup(func() { testhelper.CleanupWorkspace(t, pool, ownWorkspaceID) })

		resp, body := fromOutsider.PATCH("/api/v1/tenant/workspaces/"+ownWorkspaceID+"/status", map[string]any{"status": "SUSPENDED"})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.Contains(t, string(body), `"status":"SUSPENDED"`)
	})
}
