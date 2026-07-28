//go:build integration

package controller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

// TestAdminController_Tenants tests all tenant-related endpoints.
func TestAdminController_Tenants(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	// Create test users with different roles
	superAdmin := testhelper.CreateTestUser(t, pool,
		"superadmin-tenants@test.com", "Super Admin",
		testhelper.Ptr(entity.SystemRoleSuperAdmin))
	defer testhelper.CleanupUser(t, pool, superAdmin.ID)

	platformAdmin := testhelper.CreateTestUser(t, pool,
		"platformadmin-tenants@test.com", "Platform Admin",
		testhelper.Ptr(entity.SystemRolePlatformAdmin))
	defer testhelper.CleanupUser(t, pool, platformAdmin.ID)

	regularUser := testhelper.CreateTestUser(t, pool,
		"regular-tenants@test.com", "Regular User", nil)
	defer testhelper.CleanupUser(t, pool, regularUser.ID)

	t.Run("Create", func(t *testing.T) {
		t.Run("success with SUPERADMIN", func(t *testing.T) {
			req := dto.CreateTenantRequest{
				Name:        "Test Tenant Create",
				Code:        "CREA01",
				Description: "A test tenant",
			}

			resp, body := client.WithAuth(superAdmin.BearerHeader).
				POST("/api/v1/system/tenants", req)

			assert.Equal(t, http.StatusCreated, resp.StatusCode)

			var tenant dto.TenantResponse
			err := json.Unmarshal(body, &tenant)
			require.NoError(t, err)

			assert.NotEmpty(t, tenant.ID)
			assert.Equal(t, "Test Tenant Create", tenant.Name)
			assert.Equal(t, "CREA01", tenant.Code)

			defer testhelper.CleanupTenant(t, pool, tenant.ID)
		})

		t.Run("forbidden with PLATFORM_ADMIN", func(t *testing.T) {
			req := dto.CreateTenantRequest{
				Name: "Forbidden Tenant",
				Code: "FORB01",
			}

			resp, _ := client.WithAuth(platformAdmin.BearerHeader).
				POST("/api/v1/system/tenants", req)

			assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		})

		t.Run("forbidden without system role", func(t *testing.T) {
			req := dto.CreateTenantRequest{
				Name: "No Role Tenant",
				Code: "NORL01",
			}

			resp, _ := client.WithAuth(regularUser.BearerHeader).
				POST("/api/v1/system/tenants", req)

			assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		})

		t.Run("unauthorized without token", func(t *testing.T) {
			req := dto.CreateTenantRequest{
				Name: "Unauth Tenant",
				Code: "UNAU01",
			}

			resp, _ := client.POST("/api/v1/system/tenants", req)

			assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
		})

		t.Run("validation empty name", func(t *testing.T) {
			req := dto.CreateTenantRequest{
				Name: "",
				Code: "VALID1",
			}

			resp, _ := client.WithAuth(superAdmin.BearerHeader).
				POST("/api/v1/system/tenants", req)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("validation code too short", func(t *testing.T) {
			req := dto.CreateTenantRequest{
				Name: "Short Code Tenant",
				Code: "A",
			}

			resp, _ := client.WithAuth(superAdmin.BearerHeader).
				POST("/api/v1/system/tenants", req)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("validation code too long", func(t *testing.T) {
			req := dto.CreateTenantRequest{
				Name: "Long Code Tenant",
				Code: "VERYLONGCODE",
			}

			resp, _ := client.WithAuth(superAdmin.BearerHeader).
				POST("/api/v1/system/tenants", req)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("conflict duplicate code", func(t *testing.T) {
			// First create a tenant
			tenantID := testhelper.CreateTestTenant(t, pool, "First Tenant", "DUP001")
			defer testhelper.CleanupTenant(t, pool, tenantID)

			// Try to create another with same code
			req := dto.CreateTenantRequest{
				Name: "Second Tenant",
				Code: "DUP001",
			}

			resp, _ := client.WithAuth(superAdmin.BearerHeader).
				POST("/api/v1/system/tenants", req)

			assert.Equal(t, http.StatusConflict, resp.StatusCode)
		})
	})

	t.Run("ListPaginated", func(t *testing.T) {
		t.Run("success with default pagination", func(t *testing.T) {
			// Create some tenants
			tenant1ID := testhelper.CreateTestTenant(t, pool, "List Tenant 1", "LIST01")
			defer testhelper.CleanupTenant(t, pool, tenant1ID)

			tenant2ID := testhelper.CreateTestTenant(t, pool, "List Tenant 2", "LIST02")
			defer testhelper.CleanupTenant(t, pool, tenant2ID)

			resp, body := client.WithAuth(superAdmin.BearerHeader).
				GET("/api/v1/system/tenants")

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var listResp dto.PaginatedTenantsResponse
			err := json.Unmarshal(body, &listResp)
			require.NoError(t, err)

			assert.GreaterOrEqual(t, len(listResp.Data), 2)
			assert.GreaterOrEqual(t, listResp.Pagination.Total, int64(2))
		})

		t.Run("success with custom limit and offset", func(t *testing.T) {
			resp, body := client.WithAuth(superAdmin.BearerHeader).
				GET("/api/v1/system/tenants?perPage=5&page=1")

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var listResp dto.PaginatedTenantsResponse
			err := json.Unmarshal(body, &listResp)
			require.NoError(t, err)

			assert.LessOrEqual(t, len(listResp.Data), 5)
			assert.Equal(t, 5, listResp.Pagination.PerPage)
		})

		t.Run("success with PLATFORM_ADMIN", func(t *testing.T) {
			resp, _ := client.WithAuth(platformAdmin.BearerHeader).
				GET("/api/v1/system/tenants")

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})

		t.Run("forbidden without role", func(t *testing.T) {
			resp, _ := client.WithAuth(regularUser.BearerHeader).
				GET("/api/v1/system/tenants")

			assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		})
	})

	t.Run("Search", func(t *testing.T) {
		t.Run("success search by name", func(t *testing.T) {
			// Create tenants with similar names
			tenant1ID := testhelper.CreateTestTenant(t, pool, "Search Test Alpha", "SRCH01")
			defer testhelper.CleanupTenant(t, pool, tenant1ID)

			tenant2ID := testhelper.CreateTestTenant(t, pool, "Search Test Beta", "SRCH02")
			defer testhelper.CleanupTenant(t, pool, tenant2ID)

			resp, body := client.WithAuth(superAdmin.BearerHeader).
				GET("/api/v1/system/tenants?q=Search%20Test")

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var listResp dto.PaginatedTenantsResponse
			err := json.Unmarshal(body, &listResp)
			require.NoError(t, err)

			assert.GreaterOrEqual(t, len(listResp.Data), 2)
		})

		t.Run("success search by code", func(t *testing.T) {
			tenantID := testhelper.CreateTestTenant(t, pool, "Code Search Tenant", "CSRCH1")
			defer testhelper.CleanupTenant(t, pool, tenantID)

			resp, body := client.WithAuth(superAdmin.BearerHeader).
				GET("/api/v1/system/tenants?q=CSRCH")

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var listResp dto.PaginatedTenantsResponse
			err := json.Unmarshal(body, &listResp)
			require.NoError(t, err)

			assert.GreaterOrEqual(t, len(listResp.Data), 1)
		})

		t.Run("empty results", func(t *testing.T) {
			resp, body := client.WithAuth(superAdmin.BearerHeader).
				GET("/api/v1/system/tenants?q=ZZZZNONEXISTENT")

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var listResp dto.PaginatedTenantsResponse
			err := json.Unmarshal(body, &listResp)
			require.NoError(t, err)

			assert.Equal(t, 0, len(listResp.Data))
		})

		t.Run("empty query returns paginated list", func(t *testing.T) {
			resp, _ := client.WithAuth(superAdmin.BearerHeader).
				GET("/api/v1/system/tenants")

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})

		t.Run("success with PLATFORM_ADMIN", func(t *testing.T) {
			resp, _ := client.WithAuth(platformAdmin.BearerHeader).
				GET("/api/v1/system/tenants?q=Test")

			assert.Equal(t, http.StatusOK, resp.StatusCode)
		})

		t.Run("forbidden without role", func(t *testing.T) {
			resp, _ := client.WithAuth(regularUser.BearerHeader).
				GET("/api/v1/system/tenants?q=Test")

			assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		})
	})

	t.Run("Get", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			tenantID := testhelper.CreateTestTenant(t, pool, "Get Tenant", "GET001")
			defer testhelper.CleanupTenant(t, pool, tenantID)

			resp, body := client.WithAuth(superAdmin.BearerHeader).
				GET("/api/v1/system/tenants/" + tenantID)

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var tenant dto.TenantResponse
			err := json.Unmarshal(body, &tenant)
			require.NoError(t, err)

			assert.Equal(t, tenantID, tenant.ID)
			assert.Equal(t, "Get Tenant", tenant.Name)
		})

		t.Run("not found", func(t *testing.T) {
			resp, _ := client.WithAuth(superAdmin.BearerHeader).
				GET("/api/v1/system/tenants/00000000-0000-0000-0000-000000000000")

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	})

	t.Run("Update", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			tenantID := testhelper.CreateTestTenant(t, pool, "Update Tenant", "UPD001")
			defer testhelper.CleanupTenant(t, pool, tenantID)

			req := dto.UpdateTenantRequest{
				Name:        "Updated Tenant Name",
				Description: "Updated description",
			}

			resp, body := client.WithAuth(superAdmin.BearerHeader).
				PUT("/api/v1/system/tenants/"+tenantID, req)

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var tenant dto.TenantResponse
			err := json.Unmarshal(body, &tenant)
			require.NoError(t, err)

			assert.Equal(t, "Updated Tenant Name", tenant.Name)
		})

		t.Run("not found", func(t *testing.T) {
			req := dto.UpdateTenantRequest{
				Name: "Not Found Tenant",
			}

			resp, _ := client.WithAuth(superAdmin.BearerHeader).
				PUT("/api/v1/system/tenants/00000000-0000-0000-0000-000000000000", req)

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})

		t.Run("validation empty name", func(t *testing.T) {
			tenantID := testhelper.CreateTestTenant(t, pool, "Empty Name Tenant", "EMPT01")
			defer testhelper.CleanupTenant(t, pool, tenantID)

			req := dto.UpdateTenantRequest{
				Name: "",
			}

			resp, _ := client.WithAuth(superAdmin.BearerHeader).
				PUT("/api/v1/system/tenants/"+tenantID, req)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})
	})

	t.Run("Delete", func(t *testing.T) {
		t.Run("success with SUPERADMIN", func(t *testing.T) {
			tenantID := testhelper.CreateTestTenant(t, pool, "Delete Tenant", "DEL001")
			// No defer cleanup since we're deleting

			resp, _ := client.WithAuth(superAdmin.BearerHeader).
				DELETE("/api/v1/system/tenants/" + tenantID)

			assert.Equal(t, http.StatusNoContent, resp.StatusCode)
		})

		t.Run("forbidden with PLATFORM_ADMIN", func(t *testing.T) {
			tenantID := testhelper.CreateTestTenant(t, pool, "Forbid Delete Tenant", "DEL002")
			defer testhelper.CleanupTenant(t, pool, tenantID)

			resp, _ := client.WithAuth(platformAdmin.BearerHeader).
				DELETE("/api/v1/system/tenants/" + tenantID)

			assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		})

		t.Run("not found", func(t *testing.T) {
			resp, _ := client.WithAuth(superAdmin.BearerHeader).
				DELETE("/api/v1/system/tenants/00000000-0000-0000-0000-000000000000")

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})
	})
}

// TestAdminController_SystemRoles tests system role management endpoints.
func TestAdminController_SystemRoles(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	superAdmin := testhelper.CreateTestUser(t, pool,
		"superadmin-roles@test.com", "Super Admin",
		testhelper.Ptr(entity.SystemRoleSuperAdmin))
	defer testhelper.CleanupUser(t, pool, superAdmin.ID)

	platformAdmin := testhelper.CreateTestUser(t, pool,
		"platformadmin-roles@test.com", "Platform Admin",
		testhelper.Ptr(entity.SystemRolePlatformAdmin))
	defer testhelper.CleanupUser(t, pool, platformAdmin.ID)

	t.Run("ListUsers", func(t *testing.T) {
		t.Run("success with users", func(t *testing.T) {
			resp, body := client.WithAuth(superAdmin.BearerHeader).
				GET("/api/v1/system/users")

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var listResp dto.ListResponse[dto.SystemRoleWithUserResponse]
			err := json.Unmarshal(body, &listResp)
			require.NoError(t, err)

			// At least superAdmin and platformAdmin
			assert.GreaterOrEqual(t, len(listResp.Data), 2)
		})

		t.Run("forbidden PLATFORM_ADMIN", func(t *testing.T) {
			resp, _ := client.WithAuth(platformAdmin.BearerHeader).
				GET("/api/v1/system/users")

			assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		})
	})

	t.Run("Assign", func(t *testing.T) {
		t.Run("success via email creates invited shadow user", func(t *testing.T) {
			req := dto.AddSystemRoleRequest{
				Email:    "new-system-member@test.com",
				FullName: "New System Member",
				Role:     "PLATFORM_ADMIN",
			}

			resp, body := client.WithAuth(superAdmin.BearerHeader).
				POST("/api/v1/system/users", req)

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var roleResp dto.SystemRoleResponse
			err := json.Unmarshal(body, &roleResp)
			require.NoError(t, err)
			assert.Equal(t, "PLATFORM_ADMIN", roleResp.Role)

			row := pool.QueryRow(context.Background(), `
				SELECT email, full_name, status
				FROM identity.users
				WHERE id = $1
			`, roleResp.UserID)

			var email, fullName, status string
			err = row.Scan(&email, &fullName, &status)
			require.NoError(t, err)
			assert.Equal(t, req.Email, email)
			assert.Equal(t, req.FullName, fullName)
			assert.Equal(t, "INVITED", status)

			defer testhelper.CleanupUser(t, pool, roleResp.UserID)
		})

		t.Run("matches existing user case-insensitively", func(t *testing.T) {
			existingUser := testhelper.CreateTestUser(t, pool,
				"existing-case@test.com", "Existing Case", nil)
			defer testhelper.CleanupUser(t, pool, existingUser.ID)

			req := dto.AddSystemRoleRequest{
				Email: "Existing-Case@Test.com",
				Role:  "PLATFORM_ADMIN",
			}

			resp, body := client.WithAuth(superAdmin.BearerHeader).
				POST("/api/v1/system/users", req)

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var roleResp dto.SystemRoleResponse
			err := json.Unmarshal(body, &roleResp)
			require.NoError(t, err)
			assert.Equal(t, existingUser.ID, roleResp.UserID)

			row := pool.QueryRow(context.Background(), `
				SELECT COUNT(*)
				FROM identity.users
				WHERE LOWER(email) = LOWER($1)
			`, req.Email)

			var count int
			err = row.Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 1, count)
		})

		t.Run("success SUPERADMIN role", func(t *testing.T) {
			targetUser := testhelper.CreateTestUser(t, pool,
				"target-super@test.com", "Target Super", nil)
			defer testhelper.CleanupUser(t, pool, targetUser.ID)

			req := dto.AssignSystemRoleRequest{
				Role: "SUPERADMIN",
			}

			resp, body := client.WithAuth(superAdmin.BearerHeader).
				POST("/api/v1/system/users/"+targetUser.ID+"/role", req)

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var roleResp dto.SystemRoleResponse
			err := json.Unmarshal(body, &roleResp)
			require.NoError(t, err)

			assert.Equal(t, targetUser.ID, roleResp.UserID)
			assert.Equal(t, "SUPERADMIN", roleResp.Role)
		})

		t.Run("success PLATFORM_ADMIN role", func(t *testing.T) {
			targetUser := testhelper.CreateTestUser(t, pool,
				"target-platform@test.com", "Target Platform", nil)
			defer testhelper.CleanupUser(t, pool, targetUser.ID)

			req := dto.AssignSystemRoleRequest{
				Role: "PLATFORM_ADMIN",
			}

			resp, body := client.WithAuth(superAdmin.BearerHeader).
				POST("/api/v1/system/users/"+targetUser.ID+"/role", req)

			assert.Equal(t, http.StatusOK, resp.StatusCode)

			var roleResp dto.SystemRoleResponse
			err := json.Unmarshal(body, &roleResp)
			require.NoError(t, err)

			assert.Equal(t, "PLATFORM_ADMIN", roleResp.Role)
		})

		t.Run("user not found", func(t *testing.T) {
			req := dto.AssignSystemRoleRequest{
				Role: "SUPERADMIN",
			}

			resp, _ := client.WithAuth(superAdmin.BearerHeader).
				POST("/api/v1/system/users/00000000-0000-0000-0000-000000000000/role", req)

			assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		})

		t.Run("invalid role", func(t *testing.T) {
			targetUser := testhelper.CreateTestUser(t, pool,
				"target-invalid@test.com", "Target Invalid", nil)
			defer testhelper.CleanupUser(t, pool, targetUser.ID)

			req := dto.AssignSystemRoleRequest{
				Role: "INVALID_ROLE",
			}

			resp, _ := client.WithAuth(superAdmin.BearerHeader).
				POST("/api/v1/system/users/"+targetUser.ID+"/role", req)

			assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
		})

		t.Run("forbidden PLATFORM_ADMIN", func(t *testing.T) {
			targetUser := testhelper.CreateTestUser(t, pool,
				"target-forbid@test.com", "Target Forbid", nil)
			defer testhelper.CleanupUser(t, pool, targetUser.ID)

			req := dto.AssignSystemRoleRequest{
				Role: "PLATFORM_ADMIN",
			}

			resp, _ := client.WithAuth(platformAdmin.BearerHeader).
				POST("/api/v1/system/users/"+targetUser.ID+"/role", req)

			assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		})

		t.Run("add forbidden PLATFORM_ADMIN", func(t *testing.T) {
			req := dto.AddSystemRoleRequest{
				Email: "forbidden-system-member@test.com",
				Role:  "PLATFORM_ADMIN",
			}

			resp, _ := client.WithAuth(platformAdmin.BearerHeader).
				POST("/api/v1/system/users", req)

			assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		})
	})

	t.Run("Revoke", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			targetUser := testhelper.CreateTestUser(t, pool,
				"target-revoke@test.com", "Target Revoke",
				testhelper.Ptr(entity.SystemRolePlatformAdmin))
			defer testhelper.CleanupUser(t, pool, targetUser.ID)

			resp, _ := client.WithAuth(superAdmin.BearerHeader).
				DELETE("/api/v1/system/users/" + targetUser.ID + "/role")

			assert.Equal(t, http.StatusNoContent, resp.StatusCode)
		})

		t.Run("forbidden PLATFORM_ADMIN", func(t *testing.T) {
			targetUser := testhelper.CreateTestUser(t, pool,
				"target-revoke-forbid@test.com", "Target Revoke Forbid",
				testhelper.Ptr(entity.SystemRolePlatformAdmin))
			defer testhelper.CleanupUser(t, pool, targetUser.ID)

			resp, _ := client.WithAuth(platformAdmin.BearerHeader).
				DELETE("/api/v1/system/users/" + targetUser.ID + "/role")

			assert.Equal(t, http.StatusForbidden, resp.StatusCode)
		})
	})
}
