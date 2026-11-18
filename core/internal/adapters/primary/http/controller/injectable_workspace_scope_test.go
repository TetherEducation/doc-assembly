//go:build integration

package controller_test

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

// injectableScopeRows returns the workspaces' injectable definitions, and the
// injectables linked to the versions of their templates, as one JSON document,
// so a test can tell whether a request changed any of them.
func injectableScopeRows(t *testing.T, pool *pgxpool.Pool, workspaceIDs ...string) string {
	t.Helper()
	var rows string
	err := pool.QueryRow(context.Background(), `
		SELECT json_build_object(
			'definitions', (SELECT COALESCE(json_agg(d ORDER BY d.id), '[]')
				FROM content.injectable_definitions d WHERE d.workspace_id = ANY($1::uuid[])),
			'links', (SELECT COALESCE(json_agg(tvi ORDER BY tvi.id), '[]')
				FROM content.template_version_injectables tvi
				JOIN content.template_versions tv ON tv.id = tvi.template_version_id
				JOIN content.templates tpl ON tpl.id = tv.template_id
				WHERE tpl.workspace_id = ANY($1::uuid[]))
		)::text`, workspaceIDs).Scan(&rows)
	require.NoError(t, err)
	return rows
}

// scopedInjectables is what one workspace offers the routes under test: an
// injectable definition of its own, and a template with a draft version.
type scopedInjectables struct {
	injectableID string
	templateID   string
	draftID      string
}

// ids pairs each of s's IDs with the placeholder a route names it by:
// {injectable}, ... with prefix inside the braces ({own:draft}).
func (s scopedInjectables) ids(prefix string) []string {
	return []string{
		"{" + prefix + "injectable}", s.injectableID,
		"{" + prefix + "template}", s.templateID,
		"{" + prefix + "draft}", s.draftID,
	}
}

// injectableRoute is a request whose path (under /api/v1) and JSON body name
// IDs by placeholder.
type injectableRoute struct {
	method string
	path   string
	body   map[string]any
}

// name is the route's method and path, plus any ID its body names.
func (r injectableRoute) name() string {
	name := r.method + " " + r.path
	for _, key := range slices.Sorted(maps.Keys(r.body)) {
		if value, ok := r.body[key].(string); ok && strings.HasPrefix(value, "{") {
			name += " " + key + "=" + value
		}
	}
	return name
}

// send makes the request with each placeholder replaced by its ID.
func (r injectableRoute) send(t *testing.T, client *testhelper.HTTPClient, ids *strings.Replacer) (*http.Response, []byte) {
	t.Helper()
	var body []byte
	if r.body != nil {
		raw, err := json.Marshal(r.body)
		require.NoError(t, err)
		body = []byte(ids.Replace(string(raw)))
	}
	return client.DoRaw(r.method, "/api/v1"+ids.Replace(r.path), body)
}

// injectableScopeEnv is two workspaces of one tenant: the other workspace,
// which owns the injectable definitions under test, and the caller's own,
// whose OWNER makes every request. The caller has no access to the other
// workspace.
type injectableScopeEnv struct {
	pool             *pgxpool.Pool
	otherWorkspaceID string
	ownWorkspaceID   string
	ownSandboxID     string
	owner            *testhelper.HTTPClient
}

func setupInjectableScopeEnv(t *testing.T) *injectableScopeEnv {
	t.Helper()
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())
	env := &injectableScopeEnv{pool: pool}

	tenantID := testhelper.CreateTestTenant(t, pool, "Injectable Scope", "INJSC1")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, tenantID) })
	env.otherWorkspaceID = testhelper.CreateTestWorkspace(t, pool, &tenantID, "Other Workspace", entity.WorkspaceTypeClient)
	env.ownWorkspaceID = testhelper.CreateTestWorkspace(t, pool, &tenantID, "Own Workspace", entity.WorkspaceTypeClient)
	env.ownSandboxID = testhelper.CreateTestSandboxWorkspace(t, pool, env.ownWorkspaceID)

	owner := testhelper.CreateTestUser(t, pool, "owner-injectable-scope@test.com", "Owner", nil)
	t.Cleanup(func() { testhelper.CleanupUser(t, pool, owner.ID) })
	testhelper.CreateTestWorkspaceMember(t, pool, env.ownWorkspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)
	env.owner = client.WithAuth(owner.BearerHeader).WithWorkspaceID(env.ownWorkspaceID)

	return env
}

// createInjectables creates scopedInjectables in workspaceID, with the
// definition owned by definitionWorkspaceID (nil for a global definition),
// removed when the (sub)test ends.
func (env *injectableScopeEnv) createInjectables(t *testing.T, workspaceID string, definitionWorkspaceID *string) scopedInjectables {
	t.Helper()
	pool := env.pool
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]

	var res scopedInjectables
	res.injectableID = testhelper.CreateTestInjectable(t, pool, definitionWorkspaceID, "scoped_"+suffix, "Scoped "+suffix, entity.InjectableDataTypeText)
	res.templateID = testhelper.CreateTestTemplate(t, pool, workspaceID, "Template "+suffix, nil)
	res.draftID = testhelper.CreateTestTemplateVersion(t, pool, res.templateID, 1, "Draft", entity.VersionStatusDraft)

	t.Cleanup(func() {
		testhelper.CleanupTemplate(t, pool, res.templateID)
		testhelper.CleanupInjectable(t, pool, res.injectableID)
	})
	return res
}

// TestInjectable_DefinitionFromAnotherWorkspace_NotFound covers the routes
// that name an injectable definition, in the path or in the body: the owner of
// one workspace cannot read, change or use another workspace's by ID. Each
// route answers what it answers for an ID that does not exist, so the ID
// cannot be probed, and no definition or link of either workspace changes.
func TestInjectable_DefinitionFromAnotherWorkspace_NotFound(t *testing.T) {
	env := setupInjectableScopeEnv(t)

	routes := []injectableRoute{
		// The definitions a workspace can use, read by ID or linked to a version.
		{http.MethodGet, "/content/injectables/{injectable}", nil},
		{http.MethodPost, "/content/templates/{own:template}/versions/{own:draft}/injectables", map[string]any{"injectableDefinitionId": "{injectable}", "isRequired": true}},
		// The workspace's own definitions, which are selected by ID and workspace.
		{http.MethodGet, "/workspace/injectables/{injectable}", nil},
		{http.MethodPut, "/workspace/injectables/{injectable}", map[string]any{"label": "Renamed"}},
		{http.MethodDelete, "/workspace/injectables/{injectable}", nil},
		{http.MethodPost, "/workspace/injectables/{injectable}/activate", nil},
		{http.MethodPost, "/workspace/injectables/{injectable}/deactivate", nil},
	}

	for _, route := range routes {
		t.Run(route.name(), func(t *testing.T) {
			own := env.createInjectables(t, env.ownWorkspaceID, &env.ownWorkspaceID).ids("own:")
			other := env.createInjectables(t, env.otherWorkspaceID, &env.otherWorkspaceID)
			missingID := uuid.NewString()
			before := injectableScopeRows(t, env.pool, env.otherWorkspaceID, env.ownWorkspaceID)

			resp, body := route.send(t, env.owner, strings.NewReplacer(append(own, "{injectable}", other.injectableID)...))
			assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))

			missingResp, missingBody := route.send(t, env.owner, strings.NewReplacer(append(own, "{injectable}", missingID)...))
			assert.Equal(t, missingResp.StatusCode, resp.StatusCode, "status differs from a missing ID's")
			// Some of these routes repeat the ID they were asked for.
			assert.Equal(t,
				strings.ReplaceAll(string(missingBody), missingID, "{injectable}"),
				strings.ReplaceAll(string(body), other.injectableID, "{injectable}"),
				"body differs from a missing ID's")

			assert.JSONEq(t, before, injectableScopeRows(t, env.pool, env.otherWorkspaceID, env.ownWorkspaceID),
				"an injectable definition or link changed")
		})
	}
}

// TestInjectable_DefinitionsAWorkspaceCanUse covers what the workspace check
// lets through: a workspace reads and links its own definitions and the global
// ones, and a sandbox template takes the definitions of the workspace the
// sandbox belongs to.
func TestInjectable_DefinitionsAWorkspaceCanUse(t *testing.T) {
	env := setupInjectableScopeEnv(t)

	link := func(client *testhelper.HTTPClient, target scopedInjectables, injectableID string) (*http.Response, []byte) {
		return client.POST("/api/v1/content/templates/"+target.templateID+"/versions/"+target.draftID+"/injectables",
			map[string]any{"injectableDefinitionId": injectableID, "isRequired": true})
	}

	t.Run("own definition", func(t *testing.T) {
		own := env.createInjectables(t, env.ownWorkspaceID, &env.ownWorkspaceID)

		resp, body := env.owner.GET("/api/v1/content/injectables/" + own.injectableID)
		assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		resp, body = link(env.owner, own, own.injectableID)
		assert.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	})

	t.Run("global definition", func(t *testing.T) {
		global := env.createInjectables(t, env.ownWorkspaceID, nil)

		resp, body := env.owner.GET("/api/v1/content/injectables/" + global.injectableID)
		assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		resp, body = link(env.owner, global, global.injectableID)
		assert.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	})

	t.Run("sandbox template, definition of its workspace", func(t *testing.T) {
		sandbox := env.createInjectables(t, env.ownSandboxID, &env.ownWorkspaceID)

		resp, body := link(env.owner.WithHeader("X-Sandbox-Mode", "true"), sandbox, sandbox.injectableID)
		assert.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	})

	t.Run("sandbox template, definition of another workspace", func(t *testing.T) {
		sandbox := env.createInjectables(t, env.ownSandboxID, &env.otherWorkspaceID)
		fromSandbox := env.owner.WithHeader("X-Sandbox-Mode", "true")
		before := injectableScopeRows(t, env.pool, env.otherWorkspaceID, env.ownWorkspaceID, env.ownSandboxID)

		resp, body := link(fromSandbox, sandbox, sandbox.injectableID)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
		missingResp, missingBody := link(fromSandbox, sandbox, uuid.NewString())
		assert.Equal(t, missingResp.StatusCode, resp.StatusCode, "status differs from a missing ID's")
		assert.Equal(t, string(missingBody), string(body), "body differs from a missing ID's")

		assert.JSONEq(t, before, injectableScopeRows(t, env.pool, env.otherWorkspaceID, env.ownWorkspaceID, env.ownSandboxID),
			"an injectable definition or link changed")
	})
}
