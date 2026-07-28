//go:build integration

package controller_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

// TestMeController_ListMyTenants tests the /me/tenants endpoint with pagination and optional search.
func TestMeController_ListMyTenants(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	t.Run("success with default pagination", func(t *testing.T) {
		// Create user
		user := testhelper.CreateTestUser(t, pool, "list-paginated@test.com", "List Paginated User", nil)
		defer testhelper.CleanupUser(t, pool, user.ID)

		// Create tenants
		tenant1ID := testhelper.CreateTestTenant(t, pool, "Tenant Alpha", "LPAL01")
		defer testhelper.CleanupTenant(t, pool, tenant1ID)

		tenant2ID := testhelper.CreateTestTenant(t, pool, "Tenant Beta", "LPBE02")
		defer testhelper.CleanupTenant(t, pool, tenant2ID)

		// Create memberships with different roles
		testhelper.CreateTestTenantMember(t, pool, tenant1ID, user.ID,
			entity.TenantRoleOwner, nil)
		testhelper.CreateTestTenantMember(t, pool, tenant2ID, user.ID,
			entity.TenantRoleAdmin, nil)

		resp, body := client.WithAuth(user.BearerHeader).GET("/api/v1/me/tenants")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var paginatedResp dto.PaginatedTenantsWithRoleResponse
		err := json.Unmarshal(body, &paginatedResp)
		require.NoError(t, err)

		assert.Len(t, paginatedResp.Data, 2)
		assert.Equal(t, int64(2), paginatedResp.Pagination.Total)
		assert.Equal(t, 1, paginatedResp.Pagination.Page)
		assert.Equal(t, 10, paginatedResp.Pagination.PerPage)
	})

	t.Run("success with custom limit and offset", func(t *testing.T) {
		// Create user
		user := testhelper.CreateTestUser(t, pool, "list-custom@test.com", "List Custom User", nil)
		defer testhelper.CleanupUser(t, pool, user.ID)

		// Create 3 tenants
		tenant1ID := testhelper.CreateTestTenant(t, pool, "Tenant Custom A", "LCUA01")
		defer testhelper.CleanupTenant(t, pool, tenant1ID)

		tenant2ID := testhelper.CreateTestTenant(t, pool, "Tenant Custom B", "LCUB02")
		defer testhelper.CleanupTenant(t, pool, tenant2ID)

		tenant3ID := testhelper.CreateTestTenant(t, pool, "Tenant Custom C", "LCUC03")
		defer testhelper.CleanupTenant(t, pool, tenant3ID)

		testhelper.CreateTestTenantMember(t, pool, tenant1ID, user.ID, entity.TenantRoleOwner, nil)
		testhelper.CreateTestTenantMember(t, pool, tenant2ID, user.ID, entity.TenantRoleAdmin, nil)
		testhelper.CreateTestTenantMember(t, pool, tenant3ID, user.ID, entity.TenantRoleAdmin, nil)

		// Request with limit=2, offset=1
		resp, body := client.WithAuth(user.BearerHeader).GET("/api/v1/me/tenants?perPage=2&page=1")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var paginatedResp dto.PaginatedTenantsWithRoleResponse
		err := json.Unmarshal(body, &paginatedResp)
		require.NoError(t, err)

		assert.Len(t, paginatedResp.Data, 2)
		assert.Equal(t, int64(3), paginatedResp.Pagination.Total)
		assert.Equal(t, 1, paginatedResp.Pagination.Page)
		assert.Equal(t, 2, paginatedResp.Pagination.PerPage)
	})

	t.Run("success only returns active memberships", func(t *testing.T) {
		// Create user
		user := testhelper.CreateTestUser(t, pool, "list-active@test.com", "List Active User", nil)
		defer testhelper.CleanupUser(t, pool, user.ID)

		// Create tenants
		activeTenantID := testhelper.CreateTestTenant(t, pool, "List Active Tenant", "LACT01")
		defer testhelper.CleanupTenant(t, pool, activeTenantID)

		pendingTenantID := testhelper.CreateTestTenant(t, pool, "List Pending Tenant", "LPND01")
		defer testhelper.CleanupTenant(t, pool, pendingTenantID)

		// Create ACTIVE membership
		testhelper.CreateTestTenantMember(t, pool, activeTenantID, user.ID,
			entity.TenantRoleOwner, nil)

		// Create PENDING membership
		testhelper.CreateTestTenantMemberWithStatus(t, pool, pendingTenantID, user.ID,
			entity.TenantRoleAdmin, entity.MembershipStatusPending, nil)

		resp, body := client.WithAuth(user.BearerHeader).GET("/api/v1/me/tenants")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var paginatedResp dto.PaginatedTenantsWithRoleResponse
		err := json.Unmarshal(body, &paginatedResp)
		require.NoError(t, err)

		// Should only return the ACTIVE tenant
		assert.Len(t, paginatedResp.Data, 1)
		assert.Equal(t, int64(1), paginatedResp.Pagination.Total)
		assert.Equal(t, activeTenantID, paginatedResp.Data[0].ID)
	})

	t.Run("success with no tenants", func(t *testing.T) {
		// Create user without any tenant memberships
		user := testhelper.CreateTestUser(t, pool, "list-no-tenants@test.com", "List No Tenant User", nil)
		defer testhelper.CleanupUser(t, pool, user.ID)

		resp, body := client.WithAuth(user.BearerHeader).GET("/api/v1/me/tenants")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var paginatedResp dto.PaginatedTenantsWithRoleResponse
		err := json.Unmarshal(body, &paginatedResp)
		require.NoError(t, err)

		assert.Empty(t, paginatedResp.Data)
		assert.Equal(t, int64(0), paginatedResp.Pagination.Total)
	})

	t.Run("unauthorized without token", func(t *testing.T) {
		resp, _ := client.GET("/api/v1/me/tenants")

		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("success with search filter by name", func(t *testing.T) {
		// Create user
		user := testhelper.CreateTestUser(t, pool, "search-name@test.com", "Search Name User", nil)
		defer testhelper.CleanupUser(t, pool, user.ID)

		// Create tenants with similar names
		tenant1ID := testhelper.CreateTestTenant(t, pool, "Acme Corporation", "SACM01")
		defer testhelper.CleanupTenant(t, pool, tenant1ID)

		tenant2ID := testhelper.CreateTestTenant(t, pool, "Acme Industries", "SACM02")
		defer testhelper.CleanupTenant(t, pool, tenant2ID)

		tenant3ID := testhelper.CreateTestTenant(t, pool, "Other Company", "SOTH01")
		defer testhelper.CleanupTenant(t, pool, tenant3ID)

		testhelper.CreateTestTenantMember(t, pool, tenant1ID, user.ID, entity.TenantRoleOwner, nil)
		testhelper.CreateTestTenantMember(t, pool, tenant2ID, user.ID, entity.TenantRoleAdmin, nil)
		testhelper.CreateTestTenantMember(t, pool, tenant3ID, user.ID, entity.TenantRoleAdmin, nil)

		resp, body := client.WithAuth(user.BearerHeader).GET("/api/v1/me/tenants?q=Acme")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var paginatedResp dto.PaginatedTenantsWithRoleResponse
		err := json.Unmarshal(body, &paginatedResp)
		require.NoError(t, err)

		// Should return tenants matching "Acme"
		assert.GreaterOrEqual(t, len(paginatedResp.Data), 1)
		for _, tenant := range paginatedResp.Data {
			assert.Contains(t, tenant.Name, "Acme")
		}
	})

	t.Run("success with search filter by code", func(t *testing.T) {
		// Create user
		user := testhelper.CreateTestUser(t, pool, "search-code@test.com", "Search Code User", nil)
		defer testhelper.CleanupUser(t, pool, user.ID)

		// Create tenant
		tenantID := testhelper.CreateTestTenant(t, pool, "Some Tenant", "SCODE1")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		testhelper.CreateTestTenantMember(t, pool, tenantID, user.ID, entity.TenantRoleOwner, nil)

		resp, body := client.WithAuth(user.BearerHeader).GET("/api/v1/me/tenants?q=SCODE")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var paginatedResp dto.PaginatedTenantsWithRoleResponse
		err := json.Unmarshal(body, &paginatedResp)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, len(paginatedResp.Data), 1)
	})

	t.Run("search with empty results", func(t *testing.T) {
		// Create user
		user := testhelper.CreateTestUser(t, pool, "search-empty@test.com", "Search Empty User", nil)
		defer testhelper.CleanupUser(t, pool, user.ID)

		// Create tenant
		tenantID := testhelper.CreateTestTenant(t, pool, "Real Tenant", "SREA01")
		defer testhelper.CleanupTenant(t, pool, tenantID)

		testhelper.CreateTestTenantMember(t, pool, tenantID, user.ID, entity.TenantRoleOwner, nil)

		resp, body := client.WithAuth(user.BearerHeader).GET("/api/v1/me/tenants?q=ZZZZNONEXISTENT")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var paginatedResp dto.PaginatedTenantsWithRoleResponse
		err := json.Unmarshal(body, &paginatedResp)
		require.NoError(t, err)

		assert.Equal(t, int64(0), paginatedResp.Pagination.Total)
		assert.Empty(t, paginatedResp.Data)
	})
}

func TestMeController_GetMyRoles(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Roles Tenant", "RLET01")
	defer testhelper.CleanupTenant(t, pool, tenantID)

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Roles Workspace", entity.WorkspaceTypeClient)
	defer testhelper.CleanupWorkspace(t, pool, workspaceID)

	superAdminRole := entity.SystemRoleSuperAdmin
	superAdmin := testhelper.CreateTestUser(t, pool, "roles-superadmin@test.com", "Super Admin", &superAdminRole)
	defer testhelper.CleanupUser(t, pool, superAdmin.ID)

	tenantOwner := testhelper.CreateTestUser(t, pool, "roles-tenant-owner@test.com", "Tenant Owner", nil)
	defer testhelper.CleanupUser(t, pool, tenantOwner.ID)
	testhelper.CreateTestTenantMember(t, pool, tenantID, tenantOwner.ID, entity.TenantRoleOwner, nil)

	tenantAdmin := testhelper.CreateTestUser(t, pool, "roles-tenant-admin@test.com", "Tenant Admin", nil)
	defer testhelper.CleanupUser(t, pool, tenantAdmin.ID)
	testhelper.CreateTestTenantMember(t, pool, tenantID, tenantAdmin.ID, entity.TenantRoleAdmin, nil)

	workspaceAdmin := testhelper.CreateTestUser(t, pool, "roles-workspace-admin@test.com", "Workspace Admin", nil)
	defer testhelper.CleanupUser(t, pool, workspaceAdmin.ID)
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, workspaceAdmin.ID, entity.WorkspaceRoleAdmin, nil)

	t.Run("superadmin receives effective workspace owner role", func(t *testing.T) {
		resp, body := client.
			WithAuth(superAdmin.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/me/roles")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var roles dto.MyRolesResponse
		err := json.Unmarshal(body, &roles)
		require.NoError(t, err)

		require.Len(t, roles.Roles, 2)
		assert.Equal(t, "SYSTEM", roles.Roles[0].Type)
		assert.Equal(t, "SUPERADMIN", roles.Roles[0].Role)
		assert.Equal(t, "WORKSPACE", roles.Roles[1].Type)
		assert.Equal(t, "OWNER", roles.Roles[1].Role)
	})

	t.Run("tenant owner receives effective workspace owner role", func(t *testing.T) {
		resp, body := client.
			WithAuth(tenantOwner.BearerHeader).
			WithTenantID(tenantID).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/me/roles")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var roles dto.MyRolesResponse
		err := json.Unmarshal(body, &roles)
		require.NoError(t, err)

		require.Len(t, roles.Roles, 2)
		assert.Equal(t, "TENANT", roles.Roles[0].Type)
		assert.Equal(t, "TENANT_OWNER", roles.Roles[0].Role)
		assert.Equal(t, "WORKSPACE", roles.Roles[1].Type)
		assert.Equal(t, "OWNER", roles.Roles[1].Role)
	})

	t.Run("tenant admin does not receive inherited workspace role", func(t *testing.T) {
		resp, body := client.
			WithAuth(tenantAdmin.BearerHeader).
			WithTenantID(tenantID).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/me/roles")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var roles dto.MyRolesResponse
		err := json.Unmarshal(body, &roles)
		require.NoError(t, err)

		require.Len(t, roles.Roles, 1)
		assert.Equal(t, "TENANT", roles.Roles[0].Type)
		assert.Equal(t, "TENANT_ADMIN", roles.Roles[0].Role)
	})

	t.Run("direct workspace member keeps direct role", func(t *testing.T) {
		resp, body := client.
			WithAuth(workspaceAdmin.BearerHeader).
			WithWorkspaceID(workspaceID).
			GET("/api/v1/me/roles")

		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var roles dto.MyRolesResponse
		err := json.Unmarshal(body, &roles)
		require.NoError(t, err)

		require.Len(t, roles.Roles, 1)
		assert.Equal(t, "WORKSPACE", roles.Roles[0].Type)
		assert.Equal(t, "ADMIN", roles.Roles[0].Role)
	})
}
