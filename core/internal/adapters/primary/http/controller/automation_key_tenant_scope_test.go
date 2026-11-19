//go:build integration

package controller_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/pdfpages/pdfpagestest"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

const (
	automationKeyScopeBase = "/api/v1/automation"

	// automationKeyScopeDenied is what the automation API answers a key limited
	// to tenants on a resource of another tenant.
	automationKeyScopeDenied = `{"error":"API key does not have access to this tenant"}`

	// automationKeyScopeForeignVersion is what it answers when the version in the
	// URL is not a version of the template in the URL.
	automationKeyScopeForeignVersion = `{"error":"version does not belong to the specified template"}`
)

// automationKeyScopeRoute is one automation route with a request its handler
// accepts, so that a request that gets past the scope check changes something.
// The placeholders {tenant}, {workspace}, {template}, {version} and {docType}
// stand for the IDs of the tenant the request is aimed at.
type automationKeyScopeRoute struct {
	method    string
	path      string // as registered, without the /api/v1/automation prefix
	body      any
	published bool // {version} is the PUBLISHED version instead of the DRAFT one
	pdf       bool // the body is a multipart PDF upload
}

// automationKeyScopeTenantRoutes are the routes addressed by tenant.
var automationKeyScopeTenantRoutes = []automationKeyScopeRoute{
	{method: http.MethodGet, path: "/tenants/:tenantId/workspaces"},
	{method: http.MethodPost, path: "/tenants/:tenantId/workspaces", body: map[string]any{"name": "Created over the automation API"}},
	{method: http.MethodPatch, path: "/tenants/:tenantId/workspaces/:workspaceId", body: map[string]any{"name": "Renamed over the automation API"}},
	{method: http.MethodPost, path: "/tenants/:tenantId/workspaces/:workspaceId/suspend"},
	{method: http.MethodPost, path: "/tenants/:tenantId/workspaces/:workspaceId/activate"},
	{method: http.MethodPost, path: "/tenants/:tenantId/workspaces/:workspaceId/archive"},
}

// automationKeyScopeTemplateRoutes are the routes addressed by template.
var automationKeyScopeTemplateRoutes = []automationKeyScopeRoute{
	{method: http.MethodPut, path: "/templates/:templateId/process", body: map[string]any{"process": "AUTOMATION_SCOPE", "processType": "ID"}},
	{method: http.MethodPost, path: "/templates/:templateId/document-type", body: map[string]any{"documentTypeId": "{docType}"}},
	{method: http.MethodGet, path: "/templates/:templateId/versions"},
	{method: http.MethodPost, path: "/templates/:templateId/versions", body: map[string]any{"name": "Created over the automation API"}},
	{method: http.MethodGet, path: "/templates/:templateId/versions/:versionId"},
	{method: http.MethodPatch, path: "/templates/:templateId/versions/:versionId", body: map[string]any{"name": "Renamed over the automation API"}},
	{method: http.MethodPost, path: "/templates/:templateId/versions/:versionId/publish"},
	{method: http.MethodPost, path: "/templates/:templateId/versions/:versionId/archive", published: true},
	{method: http.MethodGet, path: "/templates/:templateId/versions/:versionId/content"},
	{method: http.MethodPut, path: "/templates/:templateId/versions/:versionId/content", body: map[string]any{"contentStructure": minimalPortabledoc()}},
	{method: http.MethodPost, path: "/templates/:templateId/versions/:versionId/source-document", pdf: true},
	{method: http.MethodPut, path: "/templates/:templateId/versions/:versionId/placements", body: map[string]any{"signers": []any{map[string]any{
		"name": "Signer", "order": 1,
		"fields": []any{map[string]any{"type": "signature", "page": 1, "x": 10, "y": 10, "width": 100, "height": 40}},
	}}}},
}

// automationKeyScopeDocumentTypeRoutes carry the tenant in the query string or
// in the body.
var automationKeyScopeDocumentTypeRoutes = []automationKeyScopeRoute{
	{method: http.MethodGet, path: "/document-types?tenantId={tenant}"},
	{method: http.MethodPost, path: "/document-types", body: map[string]any{
		"tenantId": "{tenant}", "code": "AUTOMATION_SCOPE_NEW", "name": map[string]string{"en": "Created over the automation API"},
	}},
}

// keyScopeFixture is a tenant with a document type and one CLIENT workspace
// holding a template with a DRAFT version that can be published and a PUBLISHED
// one, plus a template in the sandbox the database creates for the workspace.
type keyScopeFixture struct {
	tenantID          string
	workspaceID       string
	docTypeID         string
	templateID        string
	draftID           string
	publishedID       string
	sandboxTemplateID string
	sandboxDraftID    string
}

func newKeyScopeFixture(t *testing.T, pool *pgxpool.Pool, label string) keyScopeFixture {
	t.Helper()
	code := label + strings.ToUpper(uuid.NewString()[:8])
	var f keyScopeFixture
	f.tenantID = testhelper.CreateTestTenant(t, pool, "Automation Tenant "+code, code)
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, f.tenantID) })
	f.docTypeID = testhelper.CreateTestDocumentType(t, pool, f.tenantID, "DT_"+code, "Automation Document Type "+code)
	f.workspaceID = testhelper.CreateTestWorkspace(t, pool, &f.tenantID, "Automation Workspace "+code, entity.WorkspaceTypeClient)

	f.templateID = testhelper.CreateTestTemplate(t, pool, f.workspaceID, "Automation Template "+code, nil)
	f.draftID = testhelper.CreateTestPublishableTemplateVersion(t, pool, f.templateID, 1, "draft")
	f.publishedID = testhelper.CreateTestTemplateVersion(t, pool, f.templateID, 2, "published", entity.VersionStatusDraft)
	testhelper.PublishTestVersion(t, pool, f.publishedID)

	var sandboxID string
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT id FROM tenancy.workspaces WHERE sandbox_of_id = $1`, f.workspaceID).Scan(&sandboxID))
	f.sandboxTemplateID = testhelper.CreateTestTemplate(t, pool, sandboxID, "Automation Sandbox Template "+code, nil)
	f.sandboxDraftID = testhelper.CreateTestPublishableTemplateVersion(t, pool, f.sandboxTemplateID, 1, "draft")
	return f
}

// automationKeyScopeRows returns everything the automation API can change for
// the tenants (workspaces, templates, versions with their signer roles,
// injectables and uploaded PDFs, and document types) as one JSON document, so a
// test can tell whether a request changed any of it.
func automationKeyScopeRows(t *testing.T, pool *pgxpool.Pool, tenantIDs ...string) string {
	t.Helper()
	var rows string
	err := pool.QueryRow(context.Background(), `
		WITH ws AS (SELECT * FROM tenancy.workspaces WHERE tenant_id = ANY($1::uuid[])),
		     tp AS (SELECT t.* FROM content.templates t JOIN ws ON ws.id = t.workspace_id),
		     tv AS (SELECT v.* FROM content.template_versions v JOIN tp ON tp.id = v.template_id)
		SELECT json_build_object(
			'workspaces', (SELECT COALESCE(json_agg(ws ORDER BY ws.id), '[]') FROM ws),
			'templates', (SELECT COALESCE(json_agg(tp ORDER BY tp.id), '[]') FROM tp),
			'versions', (SELECT COALESCE(json_agg(tv ORDER BY tv.id), '[]') FROM tv),
			'signerRoles', (SELECT COALESCE(json_agg(r ORDER BY r.id), '[]')
				FROM content.template_version_signer_roles r JOIN tv ON tv.id = r.template_version_id),
			'versionInjectables', (SELECT COALESCE(json_agg(i ORDER BY i.id), '[]')
				FROM content.template_version_injectables i JOIN tv ON tv.id = i.template_version_id),
			'uploadedPDFs', (SELECT COALESCE(json_agg(s ORDER BY s.template_version_id), '[]')
				FROM content.template_version_sources s JOIN tv ON tv.id = s.template_version_id),
			'documentTypes', (SELECT COALESCE(json_agg(d ORDER BY d.id), '[]')
				FROM content.document_types d WHERE d.tenant_id = ANY($1::uuid[]))
		)::text`, tenantIDs).Scan(&rows)
	require.NoError(t, err)
	return rows
}

// automationKeyScopeIDs are the IDs a request is filled with.
type automationKeyScopeIDs struct {
	tenantID    string
	workspaceID string
	templateID  string
	draftID     string
	publishedID string
	docTypeID   string
}

func (f keyScopeFixture) ids() automationKeyScopeIDs {
	return automationKeyScopeIDs{
		tenantID: f.tenantID, workspaceID: f.workspaceID, templateID: f.templateID,
		draftID: f.draftID, publishedID: f.publishedID, docTypeID: f.docTypeID,
	}
}

// automationKeyScopeSend makes the request of a route with the given IDs.
func automationKeyScopeSend(t *testing.T, client *testhelper.HTTPClient, route automationKeyScopeRoute, ids automationKeyScopeIDs) (int, string) {
	t.Helper()
	versionID := ids.draftID
	if route.published {
		versionID = ids.publishedID
	}
	fill := strings.NewReplacer(
		":tenantId", ids.tenantID, "{tenant}", ids.tenantID,
		":workspaceId", ids.workspaceID,
		":templateId", ids.templateID,
		":versionId", versionID,
		"{docType}", ids.docTypeID,
	)
	path := automationKeyScopeBase + fill.Replace(route.path)

	if route.pdf {
		var form bytes.Buffer
		writer := multipart.NewWriter(&form)
		part, err := writer.CreateFormFile("file", "source.pdf")
		require.NoError(t, err)
		_, err = part.Write(pdfpagestest.Pages([2]float64{612, 792}))
		require.NoError(t, err)
		require.NoError(t, writer.Close())
		resp, body := client.WithHeader("Content-Type", writer.FormDataContentType()).DoRaw(route.method, path, form.Bytes())
		return resp.StatusCode, string(body)
	}

	var raw []byte
	if route.body != nil {
		encoded, err := json.Marshal(route.body)
		require.NoError(t, err)
		raw = []byte(fill.Replace(string(encoded)))
	}
	resp, body := client.DoRaw(route.method, path, raw)
	return resp.StatusCode, string(body)
}

// automationKeyScopeRegistered returns "METHOD path" for every automation route
// registered under prefix, without the /api/v1/automation prefix.
func automationKeyScopeRegistered(ts *testhelper.TestServer, prefix string) []string {
	var registered []string
	for _, route := range ts.Engine.Routes() {
		if strings.HasPrefix(route.Path, automationKeyScopeBase+prefix) {
			registered = append(registered, route.Method+" "+strings.TrimPrefix(route.Path, automationKeyScopeBase))
		}
	}
	return registered
}

// automationKeyScopeCovered returns "METHOD path" for every route of a table,
// without the query string.
func automationKeyScopeCovered(routes []automationKeyScopeRoute) []string {
	covered := make([]string, 0, len(routes))
	for _, route := range routes {
		path, _, _ := strings.Cut(route.path, "?")
		covered = append(covered, route.method+" "+path)
	}
	return covered
}

// automationKeyScopeClients returns a client with a key limited to the tenants
// and one with a global key.
func automationKeyScopeClients(t *testing.T, pool *pgxpool.Pool, ts *testhelper.TestServer, tenantIDs ...string) (limited, global *testhelper.HTTPClient) {
	t.Helper()
	limitedKeyID, limitedKey := testhelper.CreateTestAutomationKey(t, pool, "key-scope-limited", tenantIDs)
	t.Cleanup(func() { testhelper.CleanupAutomationKey(t, pool, limitedKeyID) })
	globalKeyID, globalKey := testhelper.CreateTestAutomationKey(t, pool, "key-scope-global", nil)
	t.Cleanup(func() { testhelper.CleanupAutomationKey(t, pool, globalKeyID) })
	return testhelper.NewHTTPClient(t, ts.URL()).WithAutomationKey(limitedKey),
		testhelper.NewHTTPClient(t, ts.URL()).WithAutomationKey(globalKey)
}

// TestAutomationController_KeyLimitedToTenants_AnotherTenant covers the
// automation routes addressed by tenant, by template, and by a tenant in the
// query string or in the body. A key limited to tenants answers 403 on every
// one of them when the tenant, or the template's tenant, is not one of its own,
// and nothing of either tenant changes.
func TestAutomationController_KeyLimitedToTenants_AnotherTenant(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)

	own := newKeyScopeFixture(t, pool, "O")
	limited, global := automationKeyScopeClients(t, pool, ts, own.tenantID)

	// A route registered under these prefixes later has to be added to its table,
	// or it would not be checked here.
	t.Run("every registered route is covered", func(t *testing.T) {
		assert.ElementsMatch(t, automationKeyScopeCovered(automationKeyScopeTenantRoutes), automationKeyScopeRegistered(ts, "/tenants/:tenantId"))
		assert.ElementsMatch(t, automationKeyScopeCovered(automationKeyScopeTemplateRoutes), automationKeyScopeRegistered(ts, "/templates/:templateId"))
		assert.ElementsMatch(t, automationKeyScopeCovered(automationKeyScopeDocumentTypeRoutes), automationKeyScopeRegistered(ts, "/document-types"))
	})

	var routes []automationKeyScopeRoute
	routes = append(routes, automationKeyScopeTenantRoutes...)
	routes = append(routes, automationKeyScopeTemplateRoutes...)
	routes = append(routes, automationKeyScopeDocumentTypeRoutes...)
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			other := newKeyScopeFixture(t, pool, "X")
			before := automationKeyScopeRows(t, pool, own.tenantID, other.tenantID)

			status, body := automationKeyScopeSend(t, limited, route, other.ids())
			assert.Equal(t, http.StatusForbidden, status, body)
			assert.Equal(t, automationKeyScopeDenied, body)

			assert.JSONEq(t, before, automationKeyScopeRows(t, pool, own.tenantID, other.tenantID), "a row of one of the tenants changed")
		})
	}

	// A template in a sandbox belongs to the tenant of the sandbox's workspace.
	t.Run("template in another tenant's sandbox", func(t *testing.T) {
		other := newKeyScopeFixture(t, pool, "X")
		ids := other.ids()
		ids.templateID, ids.draftID = other.sandboxTemplateID, other.sandboxDraftID
		before := automationKeyScopeRows(t, pool, own.tenantID, other.tenantID)

		for _, route := range automationKeyScopeTemplateRoutes {
			if route.published {
				continue
			}
			status, body := automationKeyScopeSend(t, limited, route, ids)
			assert.Equal(t, http.StatusForbidden, status, "%s %s: %s", route.method, route.path, body)
		}

		assert.JSONEq(t, before, automationKeyScopeRows(t, pool, own.tenantID, other.tenantID), "a row of one of the tenants changed")
	})

	// The checks let a key through on its own tenant, and a global key on any.
	t.Run("tenant the key may use", func(t *testing.T) {
		for _, route := range []automationKeyScopeRoute{
			automationKeyScopeTenantRoutes[0],       // list workspaces
			automationKeyScopeTemplateRoutes[2],     // list versions
			automationKeyScopeTemplateRoutes[9],     // replace content
			automationKeyScopeDocumentTypeRoutes[0], // list document types
		} {
			status, body := automationKeyScopeSend(t, limited, route, own.ids())
			assert.Equal(t, http.StatusOK, status, "%s %s: %s", route.method, route.path, body)
		}

		status, body := automationKeyScopeSend(t, limited, automationKeyScopeDocumentTypeRoutes[1], own.ids())
		assert.Equal(t, http.StatusCreated, status, body)

		other := newKeyScopeFixture(t, pool, "X")
		status, body = automationKeyScopeSend(t, global, automationKeyScopeTemplateRoutes[2], other.ids())
		assert.Equal(t, http.StatusOK, status, body)
	})
}

// TestAutomationController_VersionOfAnotherTemplate covers the automation
// routes that name a version under a template. The version has to be a version
// of that template: one of another template, of the same tenant or of another,
// is refused with the same answer for a key limited to tenants and for a global
// key, and no version changes.
func TestAutomationController_VersionOfAnotherTemplate(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)

	own := newKeyScopeFixture(t, pool, "O")
	limited, global := automationKeyScopeClients(t, pool, ts, own.tenantID)

	siblingTemplateID := testhelper.CreateTestTemplate(t, pool, own.workspaceID, "Automation Sibling Template", nil)
	siblingDraftID := testhelper.CreateTestPublishableTemplateVersion(t, pool, siblingTemplateID, 1, "draft")
	siblingPublishedID := testhelper.CreateTestTemplateVersion(t, pool, siblingTemplateID, 2, "published", entity.VersionStatusDraft)
	testhelper.PublishTestVersion(t, pool, siblingPublishedID)

	callers := []struct {
		name   string
		client *testhelper.HTTPClient
	}{
		{"key limited to the tenant", limited},
		{"global key", global},
	}
	checked := 0
	for _, route := range automationKeyScopeTemplateRoutes {
		if !strings.Contains(route.path, ":versionId") {
			continue
		}
		checked++
		for _, caller := range callers {
			t.Run(route.method+" "+route.path+" with a "+caller.name, func(t *testing.T) {
				other := newKeyScopeFixture(t, pool, "X")
				before := automationKeyScopeRows(t, pool, own.tenantID, other.tenantID)

				// The key's own template, with a version of another tenant's template.
				ids := own.ids()
				ids.draftID, ids.publishedID = other.draftID, other.publishedID
				status, body := automationKeyScopeSend(t, caller.client, route, ids)
				assert.Equal(t, http.StatusBadRequest, status, body)
				assert.Equal(t, automationKeyScopeForeignVersion, body)

				// The key's own template, with a version of another of its templates.
				ids.draftID, ids.publishedID = siblingDraftID, siblingPublishedID
				status, body = automationKeyScopeSend(t, caller.client, route, ids)
				assert.Equal(t, http.StatusBadRequest, status, body)
				assert.Equal(t, automationKeyScopeForeignVersion, body)

				assert.JSONEq(t, before, automationKeyScopeRows(t, pool, own.tenantID, other.tenantID), "a row of one of the tenants changed")
			})
		}
	}
	// Guards against the loop silently matching nothing if the routes are renamed.
	require.GreaterOrEqual(t, checked, 8, "routes that name a version")

	// The check lets a version of the template in the URL through.
	t.Run("version of the template", func(t *testing.T) {
		for _, route := range []automationKeyScopeRoute{
			automationKeyScopeTemplateRoutes[4], // get version
			automationKeyScopeTemplateRoutes[5], // rename version
			automationKeyScopeTemplateRoutes[9], // replace content
		} {
			status, body := automationKeyScopeSend(t, limited, route, own.ids())
			assert.Equal(t, http.StatusOK, status, "%s %s: %s", route.method, route.path, body)
		}
		status, body := automationKeyScopeSend(t, limited, automationKeyScopeTemplateRoutes[6], own.ids()) // publish
		assert.Equal(t, http.StatusNoContent, status, body)
	})
}
