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

// tenantCatalogRows returns the tenants' document types and processes, and the
// templates of their workspaces, as one JSON document, so a test can tell
// whether a request changed any of them.
func tenantCatalogRows(t *testing.T, pool *pgxpool.Pool, tenantIDs ...string) string {
	t.Helper()
	var rows string
	err := pool.QueryRow(context.Background(), `
		SELECT json_build_object(
			'documentTypes', (SELECT COALESCE(json_agg(dt ORDER BY dt.id), '[]')
				FROM content.document_types dt WHERE dt.tenant_id = ANY($1::uuid[])),
			'processes', (SELECT COALESCE(json_agg(p ORDER BY p.id), '[]')
				FROM content.processes p WHERE p.tenant_id = ANY($1::uuid[])),
			'templates', (SELECT COALESCE(json_agg(tpl ORDER BY tpl.id), '[]')
				FROM content.templates tpl
				JOIN tenancy.workspaces w ON w.id = tpl.workspace_id
				WHERE w.tenant_id = ANY($1::uuid[]))
		)::text`, tenantIDs).Scan(&rows)
	require.NoError(t, err)
	return rows
}

// createTestProcess creates a process for a tenant, removed when the (sub)test ends.
func createTestProcess(t *testing.T, pool *pgxpool.Pool, tenantID, code string) string {
	t.Helper()
	processID := uuid.NewString()
	name, err := json.Marshal(map[string]string{"en": code})
	require.NoError(t, err)
	_, err = pool.Exec(context.Background(), `
		INSERT INTO content.processes (id, tenant_id, code, process_type, name)
		VALUES ($1, $2, $3, 'CANONICAL_NAME', $4)`,
		processID, tenantID, code, name)
	require.NoError(t, err, "failed to create test process")
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM content.processes WHERE id = $1`, processID)
	})
	return processID
}

// scopedCatalog is what one tenant offers the routes under test: a document
// type and a process, plus a spare document type and, in a workspace of the
// tenant, a template the first type is assigned to.
type scopedCatalog struct {
	typeID      string
	spareTypeID string
	processID   string
	templateID  string
}

// missingScopedCatalog names document types, a process and a template that do
// not exist.
func missingScopedCatalog() scopedCatalog {
	return scopedCatalog{
		typeID:      uuid.NewString(),
		spareTypeID: uuid.NewString(),
		processID:   uuid.NewString(),
		templateID:  uuid.NewString(),
	}
}

// ids pairs each of c's IDs with the placeholder a route names it by: {type},
// {process}, ... with prefix inside the braces ({own:type}).
func (c scopedCatalog) ids(prefix string) []string {
	return []string{
		"{" + prefix + "type}", c.typeID,
		"{" + prefix + "spareType}", c.spareTypeID,
		"{" + prefix + "process}", c.processID,
		"{" + prefix + "template}", c.templateID,
	}
}

// catalogRoute is a request whose path (under /api/v1) and JSON body name IDs
// by placeholder.
type catalogRoute struct {
	method string
	path   string
	body   map[string]any
}

// name is the route's method and path, plus any ID its body names.
func (r catalogRoute) name() string {
	name := r.method + " " + r.path
	for _, key := range slices.Sorted(maps.Keys(r.body)) {
		if value, ok := r.body[key].(string); ok && strings.HasPrefix(value, "{") {
			name += " " + key + "=" + value
		}
	}
	return name
}

// send makes the request with each placeholder replaced by its ID.
func (r catalogRoute) send(t *testing.T, client *testhelper.HTTPClient, ids *strings.Replacer) (*http.Response, []byte) {
	t.Helper()
	var body []byte
	if r.body != nil {
		raw, err := json.Marshal(r.body)
		require.NoError(t, err)
		body = []byte(ids.Replace(string(raw)))
	}
	return client.DoRaw(r.method, "/api/v1"+ids.Replace(r.path), body)
}

// catalogScopeEnv is two tenants: the victim's, which owns the document types
// and processes under test, and the outsider's, whose TENANT_OWNER makes every
// request with its own tenant and workspace in the headers. The outsider has
// no role in the victim's tenant.
type catalogScopeEnv struct {
	pool                *pgxpool.Pool
	victimTenantID      string
	victimWorkspaceID   string
	outsiderTenantID    string
	outsiderWorkspaceID string
	systemTenantID      string
	outsider            *testhelper.HTTPClient
}

func setupCatalogScopeEnv(t *testing.T) *catalogScopeEnv {
	t.Helper()
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())
	env := &catalogScopeEnv{pool: pool}

	env.victimTenantID = testhelper.CreateTestTenant(t, pool, "Catalog Scope Victim", "CTSCV1")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, env.victimTenantID) })
	env.victimWorkspaceID = testhelper.CreateTestWorkspace(t, pool, &env.victimTenantID, "Victim Workspace", entity.WorkspaceTypeClient)

	env.outsiderTenantID = testhelper.CreateTestTenant(t, pool, "Catalog Scope Outsider", "CTSCO1")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, env.outsiderTenantID) })
	env.outsiderWorkspaceID = testhelper.CreateTestWorkspace(t, pool, &env.outsiderTenantID, "Outsider Workspace", entity.WorkspaceTypeClient)

	err := pool.QueryRow(context.Background(),
		`SELECT id FROM tenancy.tenants WHERE is_system = TRUE LIMIT 1`).Scan(&env.systemTenantID)
	require.NoError(t, err, "failed to find the system tenant")

	outsider := testhelper.CreateTestUser(t, pool, "outsider-catalog-scope@test.com", "Outsider Owner", nil)
	t.Cleanup(func() { testhelper.CleanupUser(t, pool, outsider.ID) })
	testhelper.CreateTestTenantMember(t, pool, env.outsiderTenantID, outsider.ID, entity.TenantRoleOwner, nil)
	env.outsider = client.WithAuth(outsider.BearerHeader).
		WithTenantID(env.outsiderTenantID).
		WithWorkspaceID(env.outsiderWorkspaceID)

	return env
}

// createCatalog creates a scopedCatalog for tenantID, with its template in
// workspaceID, removed when the (sub)test ends.
func (env *catalogScopeEnv) createCatalog(t *testing.T, tenantID, workspaceID string) scopedCatalog {
	t.Helper()
	pool := env.pool
	suffix := strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:12])

	var catalog scopedCatalog
	catalog.typeID = testhelper.CreateTestDocumentType(t, pool, tenantID, "TYPE_"+suffix, "Type "+suffix)
	catalog.spareTypeID = testhelper.CreateTestDocumentType(t, pool, tenantID, "SPARE_"+suffix, "Spare "+suffix)
	catalog.processID = createTestProcess(t, pool, tenantID, "PROCESS_"+suffix)
	catalog.templateID = testhelper.CreateTestTemplate(t, pool, workspaceID, "Typed "+suffix, nil)
	testhelper.SetTemplateDocumentType(t, pool, catalog.templateID, catalog.typeID)

	t.Cleanup(func() {
		testhelper.CleanupTemplate(t, pool, catalog.templateID)
		testhelper.CleanupDocumentType(t, pool, catalog.typeID)
		testhelper.CleanupDocumentType(t, pool, catalog.spareTypeID)
	})
	return catalog
}

// templateDocumentType returns the document type a template is assigned, or "".
func templateDocumentType(t *testing.T, pool *pgxpool.Pool, templateID string) string {
	t.Helper()
	var documentTypeID *string
	err := pool.QueryRow(context.Background(),
		`SELECT document_type_id FROM content.templates WHERE id = $1`, templateID).Scan(&documentTypeID)
	require.NoError(t, err)
	if documentTypeID == nil {
		return ""
	}
	return *documentTypeID
}

// TestTenantCatalog_ResourceFromAnotherTenant_NotFound covers the routes that
// name a document type or a process by ID, in the path or in the body: the
// owner of one tenant cannot read, change, delete or refer to another tenant's.
// Each route answers exactly what it answers for an ID that does not exist, so
// the ID cannot be probed, and no document type, process or template row of
// either tenant changes.
func TestTenantCatalog_ResourceFromAnotherTenant_NotFound(t *testing.T) {
	env := setupCatalogScopeEnv(t)
	missing := missingScopedCatalog()
	renamed := map[string]any{"name": map[string]string{"en": "Renamed"}}

	routes := []catalogRoute{
		// A document type or process named in the path.
		{http.MethodGet, "/tenant/document-types/{type}", nil},
		{http.MethodPut, "/tenant/document-types/{type}", renamed},
		{http.MethodDelete, "/tenant/document-types/{type}", map[string]any{"force": true}},
		{http.MethodGet, "/tenant/processes/{process}", nil},
		{http.MethodPut, "/tenant/processes/{process}", renamed},
		{http.MethodDelete, "/tenant/processes/{process}", map[string]any{"force": true}},
		// A document type named in the body of a request on the caller's own
		// document type or template.
		{http.MethodDelete, "/tenant/document-types/{own:type}", map[string]any{"replaceWithId": "{type}"}},
		{http.MethodPut, "/content/templates/{own:template}/document-type", map[string]any{"documentTypeId": "{type}"}},
	}

	for _, route := range routes {
		t.Run(route.name(), func(t *testing.T) {
			own := env.createCatalog(t, env.outsiderTenantID, env.outsiderWorkspaceID).ids("own:")
			victim := env.createCatalog(t, env.victimTenantID, env.victimWorkspaceID).ids("")
			before := tenantCatalogRows(t, env.pool, env.victimTenantID, env.outsiderTenantID)

			resp, body := route.send(t, env.outsider, strings.NewReplacer(append(own, victim...)...))
			assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))

			missingResp, missingBody := route.send(t, env.outsider, strings.NewReplacer(append(own, missing.ids("")...)...))
			assert.Equal(t, missingResp.StatusCode, resp.StatusCode, "status differs from a missing ID's")
			assert.Equal(t, string(missingBody), string(body), "body differs from a missing ID's")

			assert.JSONEq(t, before, tenantCatalogRows(t, env.pool, env.victimTenantID, env.outsiderTenantID),
				"a document type, process or template row changed")
		})
	}
}

// TestTenantCatalog_OwnAndGlobalResources covers what the tenant checks let
// through: a tenant reads and changes its own document types and processes,
// and reads and uses, but cannot change, the global ones of the system tenant.
func TestTenantCatalog_OwnAndGlobalResources(t *testing.T) {
	env := setupCatalogScopeEnv(t)
	renamed := map[string]any{"name": map[string]string{"en": "Renamed"}}

	t.Run("own document type and process", func(t *testing.T) {
		own := env.createCatalog(t, env.outsiderTenantID, env.outsiderWorkspaceID)

		resp, body := env.outsider.GET("/api/v1/tenant/document-types/" + own.typeID)
		assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		resp, body = env.outsider.PUT("/api/v1/tenant/document-types/"+own.typeID, renamed)
		assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))

		resp, body = env.outsider.GET("/api/v1/tenant/processes/" + own.processID)
		assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		resp, body = env.outsider.PUT("/api/v1/tenant/processes/"+own.processID, renamed)
		assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))

		// Deleting a type in use, replaced by another type of the tenant.
		resp, body = env.outsider.Do(http.MethodDelete, "/api/v1/tenant/document-types/"+own.typeID,
			map[string]any{"replaceWithId": own.spareTypeID})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.JSONEq(t, `true`, string(testhelper.ParseJSON[map[string]json.RawMessage](t, body)["deleted"]))
		assert.Equal(t, own.spareTypeID, templateDocumentType(t, env.pool, own.templateID))

		resp, body = env.outsider.Do(http.MethodDelete, "/api/v1/tenant/processes/"+own.processID, nil)
		assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	})

	t.Run("global document type and process", func(t *testing.T) {
		own := env.createCatalog(t, env.outsiderTenantID, env.outsiderWorkspaceID)
		suffix := strings.ToUpper(strings.ReplaceAll(uuid.NewString(), "-", "")[:12])
		globalTypeID := testhelper.CreateTestDocumentType(t, env.pool, env.systemTenantID, "GLOBAL_"+suffix, "Global "+suffix)
		t.Cleanup(func() { testhelper.CleanupDocumentType(t, env.pool, globalTypeID) })
		globalProcessID := createTestProcess(t, env.pool, env.systemTenantID, "GLOBAL_"+suffix)
		before := tenantCatalogRows(t, env.pool, env.systemTenantID)

		resp, body := env.outsider.GET("/api/v1/tenant/document-types/" + globalTypeID)
		assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		resp, body = env.outsider.GET("/api/v1/tenant/processes/" + globalProcessID)
		assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))

		// Changing a global one is refused, by name.
		for _, request := range []struct{ method, path, message string }{
			{http.MethodPut, "/api/v1/tenant/document-types/" + globalTypeID, entity.ErrCannotModifyGlobalType.Error()},
			{http.MethodDelete, "/api/v1/tenant/document-types/" + globalTypeID, entity.ErrCannotModifyGlobalType.Error()},
			{http.MethodPut, "/api/v1/tenant/processes/" + globalProcessID, entity.ErrCannotModifyGlobalProcess.Error()},
			{http.MethodDelete, "/api/v1/tenant/processes/" + globalProcessID, entity.ErrCannotModifyGlobalProcess.Error()},
		} {
			resp, body = env.outsider.Do(request.method, request.path, renamed)
			assert.Equal(t, http.StatusBadRequest, resp.StatusCode, request.method+" "+request.path+": "+string(body))
			assert.Contains(t, string(body), request.message)
		}
		assert.JSONEq(t, before, tenantCatalogRows(t, env.pool, env.systemTenantID), "a global row changed")

		// A template takes a global type.
		resp, body = env.outsider.PUT("/api/v1/content/templates/"+own.templateID+"/document-type",
			map[string]any{"documentTypeId": globalTypeID})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.Equal(t, globalTypeID, templateDocumentType(t, env.pool, own.templateID))
	})
}
