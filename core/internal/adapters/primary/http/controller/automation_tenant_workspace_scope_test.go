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

// automationTenantWorkspacePrefix is the path of the automation routes that name
// one workspace of a tenant.
const automationTenantWorkspacePrefix = "/api/v1/automation/tenants/:tenantId/workspaces/:workspaceId"

// automationTenantWorkspaceRoutes lists those routes, each with a body its
// handler accepts, so that a request that gets past the scope check changes the
// workspace.
var automationTenantWorkspaceRoutes = []struct {
	method string
	suffix string
	body   any
}{
	{http.MethodPatch, "", map[string]any{"name": "Renamed over the automation API"}},
	{http.MethodPost, "/suspend", nil},
	{http.MethodPost, "/activate", nil},
	{http.MethodPost, "/archive", nil},
}

// tenantWorkspaceFixture is a tenant with one CLIENT workspace and the sandbox
// the database creates for it.
type tenantWorkspaceFixture struct {
	tenantID    string
	workspaceID string
	sandboxID   string
}

func newTenantWorkspaceFixture(t *testing.T, pool *pgxpool.Pool, label string) tenantWorkspaceFixture {
	t.Helper()
	code := label + strings.ToUpper(uuid.NewString()[:8])
	var f tenantWorkspaceFixture
	f.tenantID = testhelper.CreateTestTenant(t, pool, "Automation Tenant "+code, code)
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, f.tenantID) })
	f.workspaceID = testhelper.CreateTestWorkspace(t, pool, &f.tenantID, "Automation Workspace "+code, entity.WorkspaceTypeClient)
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT id FROM tenancy.workspaces WHERE sandbox_of_id = $1`, f.workspaceID).Scan(&f.sandboxID))
	return f
}

// automationTenantWorkspaceRows returns the tenants' workspace rows, sandboxes
// included, as one JSON document, so a test can tell whether a request changed
// any of them.
func automationTenantWorkspaceRows(t *testing.T, pool *pgxpool.Pool, tenantIDs ...string) string {
	t.Helper()
	var rows string
	err := pool.QueryRow(context.Background(), `
		SELECT COALESCE(json_agg(w ORDER BY w.id), '[]')::text
		FROM tenancy.workspaces w
		WHERE w.tenant_id = ANY($1::uuid[])`, tenantIDs).Scan(&rows)
	require.NoError(t, err)
	return rows
}

// TestAutomationController_WorkspaceOfAnotherTenant_NotFound covers the
// automation routes that name a workspace under a tenant. The key's tenant scope
// is checked against the tenant in the URL, so the workspace has to belong to
// that tenant: a workspace of another tenant answers exactly what a workspace
// that does not exist answers, for a key limited to the tenant and for a global
// key alike, and no workspace row of either tenant changes.
func TestAutomationController_WorkspaceOfAnotherTenant_NotFound(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)

	own := newTenantWorkspaceFixture(t, pool, "O")

	limitedKeyID, limitedKey := testhelper.CreateTestAutomationKey(t, pool, "tenant-workspace-limited", []string{own.tenantID})
	t.Cleanup(func() { testhelper.CleanupAutomationKey(t, pool, limitedKeyID) })
	globalKeyID, globalKey := testhelper.CreateTestAutomationKey(t, pool, "tenant-workspace-global", nil)
	t.Cleanup(func() { testhelper.CleanupAutomationKey(t, pool, globalKeyID) })
	limited := testhelper.NewHTTPClient(t, ts.URL()).WithAutomationKey(limitedKey)
	global := testhelper.NewHTTPClient(t, ts.URL()).WithAutomationKey(globalKey)

	path := func(tenantID, workspaceID, suffix string) string {
		return strings.NewReplacer(":tenantId", tenantID, ":workspaceId", workspaceID).
			Replace(automationTenantWorkspacePrefix) + suffix
	}

	// A route registered under the prefix later has to be added to the table, or
	// it would not be checked here.
	t.Run("every registered route is covered", func(t *testing.T) {
		var registered, covered []string
		for _, route := range ts.Engine.Routes() {
			if strings.HasPrefix(route.Path, automationTenantWorkspacePrefix) {
				registered = append(registered, route.Method+" "+strings.TrimPrefix(route.Path, automationTenantWorkspacePrefix))
			}
		}
		for _, route := range automationTenantWorkspaceRoutes {
			covered = append(covered, route.method+" "+route.suffix)
		}
		assert.ElementsMatch(t, covered, registered)
	})

	callers := []struct {
		name   string
		client *testhelper.HTTPClient
	}{
		{"key limited to the tenant", limited},
		{"global key", global},
	}
	for _, route := range automationTenantWorkspaceRoutes {
		for _, caller := range callers {
			t.Run(route.method+" "+route.suffix+" with a "+caller.name, func(t *testing.T) {
				other := newTenantWorkspaceFixture(t, pool, "X")
				before := automationTenantWorkspaceRows(t, pool, own.tenantID, other.tenantID)

				resp, body := caller.client.Do(route.method, path(own.tenantID, other.workspaceID, route.suffix), route.body)
				assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))

				missingResp, missingBody := caller.client.Do(route.method, path(own.tenantID, uuid.NewString(), route.suffix), route.body)
				assert.Equal(t, missingResp.StatusCode, resp.StatusCode, "status differs from a missing workspace's")
				assert.Equal(t, string(missingBody), string(body), "body differs from a missing workspace's")

				assert.JSONEq(t, before, automationTenantWorkspaceRows(t, pool, own.tenantID, other.tenantID), "a workspace row changed")
			})
		}
	}

	t.Run("sandbox of another tenant's workspace", func(t *testing.T) {
		other := newTenantWorkspaceFixture(t, pool, "X")
		before := automationTenantWorkspaceRows(t, pool, own.tenantID, other.tenantID)

		resp, body := limited.POST(path(own.tenantID, other.sandboxID, "/suspend"), nil)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
		assert.JSONEq(t, before, automationTenantWorkspaceRows(t, pool, own.tenantID, other.tenantID), "a workspace row changed")
	})

	t.Run("tenant that does not exist", func(t *testing.T) {
		before := automationTenantWorkspaceRows(t, pool, own.tenantID)

		resp, body := global.POST(path(uuid.NewString(), own.workspaceID, "/suspend"), nil)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
		assert.JSONEq(t, before, automationTenantWorkspaceRows(t, pool, own.tenantID), "a workspace row changed")
	})

	// The key's own tenant check still comes first: a tenant the key may not use
	// answers 403 whether or not the workspace exists.
	t.Run("tenant the key may not use", func(t *testing.T) {
		other := newTenantWorkspaceFixture(t, pool, "X")
		before := automationTenantWorkspaceRows(t, pool, own.tenantID, other.tenantID)

		for _, route := range automationTenantWorkspaceRoutes {
			resp, body := limited.Do(route.method, path(other.tenantID, other.workspaceID, route.suffix), route.body)
			assert.Equal(t, http.StatusForbidden, resp.StatusCode, "%s %s: %s", route.method, route.suffix, body)

			missingResp, missingBody := limited.Do(route.method, path(other.tenantID, uuid.NewString(), route.suffix), route.body)
			assert.Equal(t, http.StatusForbidden, missingResp.StatusCode, "%s %s: %s", route.method, route.suffix, missingBody)
			assert.Equal(t, string(missingBody), string(body), "%s %s: body differs from a missing workspace's", route.method, route.suffix)
		}

		assert.JSONEq(t, before, automationTenantWorkspaceRows(t, pool, own.tenantID, other.tenantID), "a workspace row changed")
	})

	// The check lets a workspace of the tenant in the URL through.
	t.Run("workspace of the tenant", func(t *testing.T) {
		resp, body := limited.PATCH(path(own.tenantID, own.workspaceID, ""), map[string]any{"name": "Renamed by its own key"})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.Contains(t, string(body), `"name":"Renamed by its own key"`)

		resp, body = limited.POST(path(own.tenantID, own.workspaceID, "/suspend"), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.Contains(t, string(body), `"status":"SUSPENDED"`)

		resp, body = limited.POST(path(own.tenantID, own.sandboxID, "/activate"), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.Contains(t, string(body), `"status":"ACTIVE"`)

		// The tenant ID is matched as an ID, not as text.
		resp, body = global.POST(path(strings.ToUpper(own.tenantID), own.workspaceID, "/activate"), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.Contains(t, string(body), `"status":"ACTIVE"`)

		resp, body = limited.POST(path(own.tenantID, own.workspaceID, "/archive"), nil)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.Contains(t, string(body), `"status":"ARCHIVED"`)
	})
}
