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

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

// automationWorkspaceRoutePrefix is the path of the automation routes that are
// addressed by workspace.
const automationWorkspaceRoutePrefix = "/api/v1/automation/workspaces/:workspaceId"

// automationWorkspaceRoutes lists those routes, each with a body its handler
// accepts, so that a request that gets past the scope checks reads or changes
// the workspace's templates.
var automationWorkspaceRoutes = []struct {
	method string
	suffix string
	body   any
}{
	{http.MethodGet, "/injectables", nil},
	{http.MethodGet, "/templates", nil},
	{http.MethodPost, "/templates", map[string]any{"name": "Created over the automation API"}},
	{http.MethodGet, "/templates/:templateId", nil},
	{http.MethodPatch, "/templates/:templateId", map[string]any{"name": "Renamed over the automation API"}},
}

// automationWorkspaceRouteDenied is what the automation API answers a key limited to
// tenants on a resource of another tenant.
const automationWorkspaceRouteDenied = `{"error":"API key does not have access to this tenant"}`

// workspaceRouteFixture is a tenant with one CLIENT workspace, the sandbox the
// database creates for it, an injectable, and a template in each of the two.
type workspaceRouteFixture struct {
	tenantID          string
	workspaceID       string
	sandboxID         string
	injectableKey     string
	templateID        string
	sandboxTemplateID string
}

func newWorkspaceRouteFixture(t *testing.T, pool *pgxpool.Pool, label string) workspaceRouteFixture {
	t.Helper()
	code := label + strings.ToUpper(uuid.NewString()[:8])
	var f workspaceRouteFixture
	f.tenantID = testhelper.CreateTestTenant(t, pool, "Automation Tenant "+code, code)
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, f.tenantID) })
	f.workspaceID = testhelper.CreateTestWorkspace(t, pool, &f.tenantID, "Automation Workspace "+code, entity.WorkspaceTypeClient)
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT id FROM tenancy.workspaces WHERE sandbox_of_id = $1`, f.workspaceID).Scan(&f.sandboxID))

	f.injectableKey = "automation_scope_" + strings.ToLower(code)
	injectableID := testhelper.CreateTestInjectable(t, pool, &f.workspaceID, f.injectableKey, "Automation Scope "+code, entity.InjectableDataTypeText)
	t.Cleanup(func() { testhelper.CleanupInjectable(t, pool, injectableID) })

	f.templateID = testhelper.CreateTestTemplate(t, pool, f.workspaceID, "Automation Template "+code, nil)
	testhelper.CreateTestTemplateVersion(t, pool, f.templateID, 1, "v1", entity.VersionStatusDraft)
	f.sandboxTemplateID = testhelper.CreateTestTemplate(t, pool, f.sandboxID, "Automation Sandbox Template "+code, nil)
	testhelper.CreateTestTemplateVersion(t, pool, f.sandboxTemplateID, 1, "v1", entity.VersionStatusDraft)
	return f
}

// automationWorkspaceRouteRows returns the tenants' workspaces (sandboxes
// included), templates and template versions as one JSON document, so a test can
// tell whether a request changed any of them.
func automationWorkspaceRouteRows(t *testing.T, pool *pgxpool.Pool, tenantIDs ...string) string {
	t.Helper()
	var rows string
	err := pool.QueryRow(context.Background(), `
		WITH ws AS (SELECT * FROM tenancy.workspaces WHERE tenant_id = ANY($1::uuid[])),
		     tp AS (SELECT t.* FROM content.templates t JOIN ws ON ws.id = t.workspace_id)
		SELECT json_build_object(
			'workspaces', (SELECT COALESCE(json_agg(ws ORDER BY ws.id), '[]') FROM ws),
			'templates', (SELECT COALESCE(json_agg(tp ORDER BY tp.id), '[]') FROM tp),
			'versions', (SELECT COALESCE(json_agg(v ORDER BY v.id), '[]')
				FROM content.template_versions v JOIN tp ON tp.id = v.template_id)
		)::text`, tenantIDs).Scan(&rows)
	require.NoError(t, err)
	return rows
}

// automationWorkspaceRoutePath fills the IDs into a route of the table.
func automationWorkspaceRoutePath(workspaceID, templateID, suffix string) string {
	return strings.NewReplacer(":workspaceId", workspaceID, ":templateId", templateID).
		Replace(automationWorkspaceRoutePrefix + suffix)
}

// TestAutomationController_WorkspaceRoutes_KeyTenantScope covers the automation
// routes addressed by workspace. A key limited to tenants answers 403 on a
// workspace of another tenant, as it does on that tenant's templates, in
// production and in sandbox mode, and nothing of either tenant changes.
func TestAutomationController_WorkspaceRoutes_KeyTenantScope(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)

	own := newWorkspaceRouteFixture(t, pool, "O")
	limitedKeyID, limitedKey := testhelper.CreateTestAutomationKey(t, pool, "workspace-route-limited", []string{own.tenantID})
	t.Cleanup(func() { testhelper.CleanupAutomationKey(t, pool, limitedKeyID) })
	globalKeyID, globalKey := testhelper.CreateTestAutomationKey(t, pool, "workspace-route-global", nil)
	t.Cleanup(func() { testhelper.CleanupAutomationKey(t, pool, globalKeyID) })
	limited := testhelper.NewHTTPClient(t, ts.URL()).WithAutomationKey(limitedKey)
	global := testhelper.NewHTTPClient(t, ts.URL()).WithAutomationKey(globalKey)

	// A route registered under the prefix later has to be added to the table, or
	// it would not be checked here.
	t.Run("every registered route is covered", func(t *testing.T) {
		var registered, covered []string
		for _, route := range ts.Engine.Routes() {
			if strings.HasPrefix(route.Path, automationWorkspaceRoutePrefix) {
				registered = append(registered, route.Method+" "+strings.TrimPrefix(route.Path, automationWorkspaceRoutePrefix))
			}
		}
		for _, route := range automationWorkspaceRoutes {
			covered = append(covered, route.method+" "+route.suffix)
		}
		assert.ElementsMatch(t, covered, registered)
	})

	modes := []struct {
		name    string
		sandbox bool
		client  *testhelper.HTTPClient
	}{
		{"production", false, limited},
		{"sandbox mode", true, limited.WithHeader("X-Sandbox-Mode", "true")},
	}
	for _, route := range automationWorkspaceRoutes {
		for _, mode := range modes {
			t.Run(route.method+" "+route.suffix+" in "+mode.name, func(t *testing.T) {
				other := newWorkspaceRouteFixture(t, pool, "X")
				templateID := other.templateID
				if mode.sandbox {
					templateID = other.sandboxTemplateID
				}
				before := automationWorkspaceRouteRows(t, pool, own.tenantID, other.tenantID)

				resp, body := mode.client.Do(route.method, automationWorkspaceRoutePath(other.workspaceID, templateID, route.suffix), route.body)
				assert.Equal(t, http.StatusForbidden, resp.StatusCode, string(body))
				assert.Equal(t, automationWorkspaceRouteDenied, string(body))

				assert.JSONEq(t, before, automationWorkspaceRouteRows(t, pool, own.tenantID, other.tenantID), "a workspace, template or version row changed")
			})
		}
	}

	t.Run("sandbox workspace addressed by its own ID", func(t *testing.T) {
		other := newWorkspaceRouteFixture(t, pool, "X")
		before := automationWorkspaceRouteRows(t, pool, own.tenantID, other.tenantID)

		for _, route := range automationWorkspaceRoutes {
			resp, body := limited.Do(route.method, automationWorkspaceRoutePath(other.sandboxID, other.sandboxTemplateID, route.suffix), route.body)
			assert.Equal(t, http.StatusForbidden, resp.StatusCode, "%s %s: %s", route.method, route.suffix, body)
		}

		assert.JSONEq(t, before, automationWorkspaceRouteRows(t, pool, own.tenantID, other.tenantID), "a workspace, template or version row changed")
	})

	// The tenant check comes before the sandbox is resolved, so a request in
	// sandbox mode does not create the sandbox of a workspace the key may not use.
	t.Run("sandbox mode on a workspace that has no sandbox yet", func(t *testing.T) {
		other := newWorkspaceRouteFixture(t, pool, "X")
		_, err := pool.Exec(context.Background(), `DELETE FROM tenancy.workspaces WHERE id = $1`, other.sandboxID)
		require.NoError(t, err)
		before := automationWorkspaceRouteRows(t, pool, own.tenantID, other.tenantID)

		resp, body := limited.WithHeader("X-Sandbox-Mode", "true").GET(automationWorkspaceRoutePath(other.workspaceID, "", "/templates"))
		assert.Equal(t, http.StatusForbidden, resp.StatusCode, string(body))

		assert.JSONEq(t, before, automationWorkspaceRouteRows(t, pool, own.tenantID, other.tenantID), "a workspace, template or version row changed")
	})

	// The system tenant is a tenant like any other to a key that does not list it.
	t.Run("workspace of the system tenant", func(t *testing.T) {
		var systemWorkspaceID string
		require.NoError(t, pool.QueryRow(context.Background(), `
			SELECT w.id FROM tenancy.workspaces w JOIN tenancy.tenants t ON t.id = w.tenant_id
			WHERE t.is_system AND w.type = 'SYSTEM'`).Scan(&systemWorkspaceID))

		for _, suffix := range []string{"/injectables", "/templates"} {
			resp, body := limited.GET(automationWorkspaceRoutePath(systemWorkspaceID, "", suffix))
			assert.Equal(t, http.StatusForbidden, resp.StatusCode, "%s: %s", suffix, body)
			assert.Equal(t, automationWorkspaceRouteDenied, string(body), suffix)
		}
	})

	t.Run("workspace that does not exist", func(t *testing.T) {
		for _, route := range automationWorkspaceRoutes {
			for _, mode := range modes {
				for _, workspaceID := range []string{uuid.NewString(), "not-a-uuid"} {
					resp, body := mode.client.Do(route.method, automationWorkspaceRoutePath(workspaceID, own.templateID, route.suffix), route.body)
					assert.Equal(t, http.StatusNotFound, resp.StatusCode, "%s %s in %s on %s: %s", route.method, route.suffix, mode.name, workspaceID, body)
				}
			}
		}
	})

	// The checks let a key through on a workspace of its own tenant, and a global
	// key on any workspace.
	t.Run("workspace of a tenant the key may use", func(t *testing.T) {
		resp, body := limited.GET(automationWorkspaceRoutePath(own.workspaceID, "", "/injectables"))
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.Contains(t, string(body), own.injectableKey)

		resp, body = limited.GET(automationWorkspaceRoutePath(own.workspaceID, "", "/templates"))
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.Contains(t, string(body), own.templateID)
		assert.NotContains(t, string(body), own.sandboxTemplateID)

		resp, body = limited.POST(automationWorkspaceRoutePath(own.workspaceID, "", "/templates"), map[string]any{"name": "Created by its own key"})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		created := testhelper.ParseJSON[dto.TemplateCreateResponse](t, body)
		require.NotNil(t, created.Template)
		assert.Equal(t, own.workspaceID, created.Template.WorkspaceID)

		other := newWorkspaceRouteFixture(t, pool, "X")
		resp, body = global.GET(automationWorkspaceRoutePath(other.workspaceID, "", "/templates"))
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.Contains(t, string(body), other.templateID)
	})
}

// TestAutomationController_WorkspaceRoutes_TemplateOfAnotherWorkspace covers
// the two workspace routes that also name a template. The template has to be in
// the workspace the request resolved to: the one in the URL, or its sandbox with
// X-Sandbox-Mode: true. A template anywhere else answers exactly what a template
// that does not exist answers, for a key limited to the tenant and for a global
// key alike, and no template row changes.
func TestAutomationController_WorkspaceRoutes_TemplateOfAnotherWorkspace(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)

	own := newWorkspaceRouteFixture(t, pool, "O")
	other := newWorkspaceRouteFixture(t, pool, "X")
	siblingWorkspaceID := testhelper.CreateTestWorkspace(t, pool, &own.tenantID, "Automation Sibling Workspace", entity.WorkspaceTypeClient)
	siblingTemplateID := testhelper.CreateTestTemplate(t, pool, siblingWorkspaceID, "Automation Sibling Template", nil)

	limitedKeyID, limitedKey := testhelper.CreateTestAutomationKey(t, pool, "workspace-template-limited", []string{own.tenantID})
	t.Cleanup(func() { testhelper.CleanupAutomationKey(t, pool, limitedKeyID) })
	globalKeyID, globalKey := testhelper.CreateTestAutomationKey(t, pool, "workspace-template-global", nil)
	t.Cleanup(func() { testhelper.CleanupAutomationKey(t, pool, globalKeyID) })
	limited := testhelper.NewHTTPClient(t, ts.URL()).WithAutomationKey(limitedKey)
	global := testhelper.NewHTTPClient(t, ts.URL()).WithAutomationKey(globalKey)

	routes := []struct {
		method string
		body   any
	}{
		{http.MethodGet, nil},
		{http.MethodPatch, map[string]any{"name": "Renamed over the automation API"}},
	}
	targets := []struct {
		name       string
		templateID string
		sandbox    bool
	}{
		{"template of another tenant", other.templateID, false},
		{"template of another tenant, in sandbox mode", other.sandboxTemplateID, true},
		{"template of another workspace of the tenant", siblingTemplateID, false},
		{"sandbox template without sandbox mode", own.sandboxTemplateID, false},
		{"production template in sandbox mode", own.templateID, true},
	}
	callers := []struct {
		name   string
		client *testhelper.HTTPClient
	}{
		{"key limited to the tenant", limited},
		{"global key", global},
	}
	for _, route := range routes {
		for _, target := range targets {
			for _, caller := range callers {
				t.Run(route.method+" "+target.name+" with a "+caller.name, func(t *testing.T) {
					client := caller.client
					if target.sandbox {
						client = client.WithHeader("X-Sandbox-Mode", "true")
					}
					before := automationWorkspaceRouteRows(t, pool, own.tenantID, other.tenantID)

					resp, body := client.Do(route.method, automationWorkspaceRoutePath(own.workspaceID, target.templateID, "/templates/:templateId"), route.body)
					assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))

					missingResp, missingBody := client.Do(route.method, automationWorkspaceRoutePath(own.workspaceID, uuid.NewString(), "/templates/:templateId"), route.body)
					assert.Equal(t, missingResp.StatusCode, resp.StatusCode, "status differs from a missing template's")
					assert.Equal(t, string(missingBody), string(body), "body differs from a missing template's")

					assert.JSONEq(t, before, automationWorkspaceRouteRows(t, pool, own.tenantID, other.tenantID), "a workspace, template or version row changed")
				})
			}
		}
	}

	t.Run("template ID that is not a UUID", func(t *testing.T) {
		resp, body := limited.GET(automationWorkspaceRoutePath(own.workspaceID, "not-a-uuid", "/templates/:templateId"))
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
	})

	// The check lets a template of the resolved workspace through.
	t.Run("template of the workspace", func(t *testing.T) {
		path := automationWorkspaceRoutePath(own.workspaceID, own.templateID, "/templates/:templateId")
		resp, body := limited.GET(path)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.Contains(t, string(body), own.templateID)

		resp, body = limited.PATCH(path, map[string]any{"name": "Renamed by its own key"})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.Contains(t, string(body), `"title":"Renamed by its own key"`)

		// The workspace ID is matched as an ID, not as text.
		resp, body = global.GET(automationWorkspaceRoutePath(strings.ToUpper(own.workspaceID), own.templateID, "/templates/:templateId"))
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

		sandboxPath := automationWorkspaceRoutePath(own.workspaceID, own.sandboxTemplateID, "/templates/:templateId")
		inSandbox := limited.WithHeader("X-Sandbox-Mode", "true")
		resp, body = inSandbox.GET(sandboxPath)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.Contains(t, string(body), own.sandboxTemplateID)

		resp, body = inSandbox.PATCH(sandboxPath, map[string]any{"name": "Renamed in the sandbox"})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.Contains(t, string(body), `"title":"Renamed in the sandbox"`)

		// A sandbox workspace addressed by its own ID is the resolved workspace too.
		resp, body = limited.GET(automationWorkspaceRoutePath(own.sandboxID, own.sandboxTemplateID, "/templates/:templateId"))
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	})
}
