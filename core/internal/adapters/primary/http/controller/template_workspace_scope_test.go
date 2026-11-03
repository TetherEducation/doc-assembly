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
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

const templatesPath = "/api/v1/content/templates/"

// scopedTemplate is a template with something for every template route to
// read or change: a published, a draft and a scheduled version, a document
// type, a linked tag and a free one, and an injectable linked to the draft
// plus a free one.
type scopedTemplate struct {
	templateID       string
	publishedID      string
	draftID          string
	scheduledID      string
	linkedTagID      string
	freeTagID        string
	injectableLinkID string
	freeInjectableID string
}

// missingScopedTemplate names a template, versions, tags and injectables that
// do not exist.
func missingScopedTemplate() scopedTemplate {
	return scopedTemplate{
		templateID:       uuid.NewString(),
		publishedID:      uuid.NewString(),
		draftID:          uuid.NewString(),
		scheduledID:      uuid.NewString(),
		linkedTagID:      uuid.NewString(),
		freeTagID:        uuid.NewString(),
		injectableLinkID: uuid.NewString(),
		freeInjectableID: uuid.NewString(),
	}
}

// ids pairs each of tpl's IDs with the placeholder a route names it by:
// {template}, {draft}, ... with prefix inside the braces ({own:draft}).
func (tpl scopedTemplate) ids(prefix string) []string {
	return []string{
		"{" + prefix + "template}", tpl.templateID,
		"{" + prefix + "published}", tpl.publishedID,
		"{" + prefix + "draft}", tpl.draftID,
		"{" + prefix + "scheduled}", tpl.scheduledID,
		"{" + prefix + "linkedTag}", tpl.linkedTagID,
		"{" + prefix + "freeTag}", tpl.freeTagID,
		"{" + prefix + "link}", tpl.injectableLinkID,
		"{" + prefix + "freeInjectable}", tpl.freeInjectableID,
	}
}

// scopeRoute is a template request whose path (under /content/templates/) and
// JSON body name IDs by placeholder.
type scopeRoute struct {
	method string
	path   string
	body   map[string]any
}

// name is the route's method and path, plus any ID its body names.
func (r scopeRoute) name() string {
	name := r.method + " " + r.path
	for _, key := range slices.Sorted(maps.Keys(r.body)) {
		if value, ok := r.body[key].(string); ok && strings.HasPrefix(value, "{") {
			name += " " + key + "=" + value
		}
	}
	return name
}

// send makes the request with each placeholder replaced by its ID.
func (r scopeRoute) send(t *testing.T, client *testhelper.HTTPClient, ids *strings.Replacer) (*http.Response, []byte) {
	t.Helper()
	var body []byte
	if r.body != nil {
		raw, err := json.Marshal(r.body)
		require.NoError(t, err)
		body = []byte(ids.Replace(string(raw)))
	}
	return client.DoRaw(r.method, templatesPath+ids.Replace(r.path), body)
}

// templateScopeEnv is two workspaces in two tenants: the victim's, which owns
// the templates under test, and the outsider's, whose ADMIN makes every request.
// The outsider is not a member of the victim's workspace.
type templateScopeEnv struct {
	pool                *pgxpool.Pool
	victimWorkspaceID   string
	victimDocTypeID     string
	outsiderWorkspaceID string
	outsiderDocTypeID   string
	outsiderSandboxID   string
	outsider            *testhelper.HTTPClient
}

func setupTemplateScopeEnv(t *testing.T) *templateScopeEnv {
	t.Helper()
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())
	env := &templateScopeEnv{pool: pool}

	victimTenantID := testhelper.CreateTestTenant(t, pool, "Template Scope Victim", "TPSCV1")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, victimTenantID) })
	env.victimWorkspaceID = testhelper.CreateTestWorkspace(t, pool, &victimTenantID, "Victim Workspace", entity.WorkspaceTypeClient)
	env.victimDocTypeID = testhelper.CreateTestDocumentType(t, pool, victimTenantID, "TPSCV_TYPE", "Victim Type")
	t.Cleanup(func() { testhelper.CleanupDocumentType(t, pool, env.victimDocTypeID) })

	outsiderTenantID := testhelper.CreateTestTenant(t, pool, "Template Scope Outsider", "TPSCO1")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, outsiderTenantID) })
	env.outsiderWorkspaceID = testhelper.CreateTestWorkspace(t, pool, &outsiderTenantID, "Outsider Workspace", entity.WorkspaceTypeClient)
	env.outsiderSandboxID = testhelper.CreateTestSandboxWorkspace(t, pool, env.outsiderWorkspaceID)
	env.outsiderDocTypeID = testhelper.CreateTestDocumentType(t, pool, outsiderTenantID, "TPSCO_TYPE", "Outsider Type")
	t.Cleanup(func() { testhelper.CleanupDocumentType(t, pool, env.outsiderDocTypeID) })

	// Runs before the tenants' cleanup: also removes whatever a request copied
	// into either workspace, so the cleanup cannot trip over it.
	t.Cleanup(func() {
		cleanupWorkspaceTemplates(t, pool, env.victimWorkspaceID, env.outsiderWorkspaceID, env.outsiderSandboxID)
	})

	outsider := testhelper.CreateTestUser(t, pool, "outsider-template-scope@test.com", "Outsider Admin", nil)
	t.Cleanup(func() { testhelper.CleanupUser(t, pool, outsider.ID) })
	testhelper.CreateTestWorkspaceMember(t, pool, env.outsiderWorkspaceID, outsider.ID, entity.WorkspaceRoleAdmin, nil)
	env.outsider = client.WithAuth(outsider.BearerHeader).WithWorkspaceID(env.outsiderWorkspaceID)

	return env
}

// createTemplate creates a scopedTemplate in workspaceID, removed when the
// (sub)test ends.
func (env *templateScopeEnv) createTemplate(t *testing.T, workspaceID, documentTypeID string) scopedTemplate {
	t.Helper()
	pool := env.pool
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]

	tpl := scopedTemplate{templateID: testhelper.CreateTestTemplate(t, pool, workspaceID, "Scoped "+suffix, nil)}
	testhelper.SetTemplateDocumentType(t, pool, tpl.templateID, documentTypeID)
	tpl.publishedID = testhelper.CreateTestTemplateVersion(t, pool, tpl.templateID, 1, "Published", entity.VersionStatusPublished)
	tpl.draftID = testhelper.CreateTestPublishableTemplateVersion(t, pool, tpl.templateID, 2, "Draft")
	tpl.scheduledID = testhelper.CreateTestTemplateVersion(t, pool, tpl.templateID, 3, "Scheduled", entity.VersionStatusScheduled)

	tpl.linkedTagID = testhelper.CreateTestTag(t, pool, workspaceID, "linked-"+suffix, "#112233")
	tpl.freeTagID = testhelper.CreateTestTag(t, pool, workspaceID, "free-"+suffix, "#445566")
	testhelper.CreateTestTemplateTag(t, pool, tpl.templateID, tpl.linkedTagID)

	linkedDefinitionID := testhelper.CreateTestInjectable(t, pool, &workspaceID, "linked_"+suffix, "Linked", entity.InjectableDataTypeText)
	tpl.freeInjectableID = testhelper.CreateTestInjectable(t, pool, &workspaceID, "free_"+suffix, "Free", entity.InjectableDataTypeText)
	tpl.injectableLinkID = testhelper.CreateTestVersionInjectable(t, pool, tpl.draftID, linkedDefinitionID, false)

	t.Cleanup(func() {
		testhelper.CleanupTemplate(t, pool, tpl.templateID)
		testhelper.CleanupTag(t, pool, tpl.linkedTagID)
		testhelper.CleanupTag(t, pool, tpl.freeTagID)
		testhelper.CleanupInjectable(t, pool, linkedDefinitionID)
		testhelper.CleanupInjectable(t, pool, tpl.freeInjectableID)
	})
	return tpl
}

// cleanupWorkspaceTemplates removes every template of the workspaces, with
// their versions, links and tags.
func cleanupWorkspaceTemplates(t *testing.T, pool *pgxpool.Pool, workspaceIDs ...string) {
	t.Helper()
	rows, err := pool.Query(context.Background(),
		`SELECT id FROM content.templates WHERE workspace_id = ANY($1::uuid[])`, workspaceIDs)
	require.NoError(t, err)
	var templateIDs []string
	for rows.Next() {
		var id string
		require.NoError(t, rows.Scan(&id))
		templateIDs = append(templateIDs, id)
	}
	rows.Close()
	require.NoError(t, rows.Err())
	for _, id := range templateIDs {
		testhelper.CleanupTemplate(t, pool, id)
	}
}

// templateRows returns every template row of the workspaces, with their
// versions, tag links, injectable links and signer roles, as one JSON
// document, so a test can tell whether a request changed any of them.
func templateRows(t *testing.T, pool *pgxpool.Pool, workspaceIDs ...string) string {
	t.Helper()
	var rows string
	err := pool.QueryRow(context.Background(), `
		WITH tpl AS (SELECT * FROM content.templates WHERE workspace_id = ANY($1::uuid[])),
		     ver AS (SELECT * FROM content.template_versions WHERE template_id IN (SELECT id FROM tpl))
		SELECT json_build_object(
			'templates', (SELECT COALESCE(json_agg(tpl ORDER BY tpl.id), '[]') FROM tpl),
			'versions', (SELECT COALESCE(json_agg(ver ORDER BY ver.id), '[]') FROM ver),
			'tags', (SELECT COALESCE(json_agg(tt ORDER BY tt.template_id, tt.tag_id), '[]')
				FROM content.template_tags tt WHERE tt.template_id IN (SELECT id FROM tpl)),
			'injectables', (SELECT COALESCE(json_agg(tvi ORDER BY tvi.id), '[]')
				FROM content.template_version_injectables tvi WHERE tvi.template_version_id IN (SELECT id FROM ver)),
			'signerRoles', (SELECT COALESCE(json_agg(sr ORDER BY sr.id), '[]')
				FROM content.template_version_signer_roles sr WHERE sr.template_version_id IN (SELECT id FROM ver))
		)::text`, workspaceIDs).Scan(&rows)
	require.NoError(t, err)
	return rows
}

// at formats a time d from now for a schedule request.
func at(d time.Duration) string { return time.Now().Add(d).UTC().Format(time.RFC3339) }

// TestContentTemplateController_TemplateFromAnotherWorkspace_NotFound covers
// templateInWorkspace: an admin of a workspace in another tenant cannot read,
// change, copy, publish or delete a template by its ID, through any route that
// names a template. Each route answers exactly what it answers for a template
// that does not exist, so the ID cannot be probed, and no template row changes.
func TestContentTemplateController_TemplateFromAnotherWorkspace_NotFound(t *testing.T) {
	env := setupTemplateScopeEnv(t)
	missing := strings.NewReplacer(missingScopedTemplate().ids("")...)

	// Every panel route that names a template, each with a body it would act on.
	routes := []scopeRoute{
		{http.MethodGet, "{template}", nil},
		{http.MethodGet, "{template}/all-versions", nil},
		{http.MethodPut, "{template}", map[string]any{"title": "Taken over"}},
		{http.MethodDelete, "{template}", nil},
		{http.MethodPost, "{template}/clone", map[string]any{"newTitle": "Cloned away", "versionId": "{published}"}},
		{http.MethodPost, "{template}/tags", map[string]any{"tagIds": []string{"{freeTag}"}}},
		{http.MethodDelete, "{template}/tags/{linkedTag}", nil},
		{http.MethodPut, "{template}/process", map[string]any{"process": "taken-over", "processType": "CANONICAL_NAME"}},
		{http.MethodPut, "{template}/document-type", map[string]any{"documentTypeId": nil}},
		{http.MethodGet, "{template}/versions", nil},
		{http.MethodPost, "{template}/versions", map[string]any{"name": "Planted"}},
		{http.MethodPost, "{template}/versions/from-existing", map[string]any{"sourceVersionId": "{draft}", "name": "Planted copy"}},
		{http.MethodGet, "{template}/versions/{draft}", nil},
		{http.MethodPut, "{template}/versions/{draft}", map[string]any{"name": "Renamed"}},
		{http.MethodDelete, "{template}/versions/{draft}", nil},
		{http.MethodPost, "{template}/versions/{draft}/publish", nil},
		{http.MethodPost, "{template}/versions/{published}/archive", nil},
		{http.MethodPost, "{template}/versions/{draft}/schedule-publish", map[string]any{"publishAt": at(48 * time.Hour)}},
		{http.MethodPost, "{template}/versions/{published}/schedule-archive", map[string]any{"archiveAt": at(72 * time.Hour)}},
		{http.MethodDelete, "{template}/versions/{scheduled}/schedule", nil},
		{http.MethodPost, "{template}/versions/{draft}/injectables", map[string]any{"injectableDefinitionId": "{freeInjectable}", "isRequired": true}},
		{http.MethodDelete, "{template}/versions/{draft}/injectables/{link}", nil},
		{http.MethodPost, "{template}/versions/{published}/promote", map[string]any{"mode": "NEW_TEMPLATE", "versionName": "Exfiltrated"}},
		{http.MethodPost, "{template}/versions/{draft}/preview", map[string]any{"injectables": map[string]any{}}},
	}

	for _, route := range routes {
		t.Run(route.name(), func(t *testing.T) {
			victim := strings.NewReplacer(env.createTemplate(t, env.victimWorkspaceID, env.victimDocTypeID).ids("")...)
			before := templateRows(t, env.pool, env.victimWorkspaceID, env.outsiderWorkspaceID)

			resp, body := route.send(t, env.outsider, victim)
			assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))

			missingResp, missingBody := route.send(t, env.outsider, missing)
			assert.Equal(t, missingResp.StatusCode, resp.StatusCode, "status differs from a missing template's")
			assert.Equal(t, string(missingBody), string(body), "body differs from a missing template's")

			assert.JSONEq(t, before, templateRows(t, env.pool, env.victimWorkspaceID, env.outsiderWorkspaceID),
				"a template row changed")
		})
	}

	// In sandbox mode the caller's workspace is its sandbox, and another
	// workspace's sandbox template is just as foreign.
	t.Run("GET {sandbox template} in sandbox mode", func(t *testing.T) {
		victimSandboxID := testhelper.CreateTestSandboxWorkspace(t, env.pool, env.victimWorkspaceID)
		t.Cleanup(func() { cleanupWorkspaceTemplates(t, env.pool, victimSandboxID) })
		sandboxTemplateID := testhelper.CreateTestTemplate(t, env.pool, victimSandboxID, "Victim Sandbox Template", nil)

		fromSandbox := env.outsider.WithHeader("X-Sandbox-Mode", "true")
		resp, body := fromSandbox.GET(templatesPath + sandboxTemplateID)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
		missingResp, missingBody := fromSandbox.GET(templatesPath + missing.Replace("{template}"))
		assert.Equal(t, missingResp.StatusCode, resp.StatusCode, "status differs from a missing template's")
		assert.Equal(t, string(missingBody), string(body), "body differs from a missing template's")
	})
}

// TestTemplateVersionController_VersionFromAnotherWorkspace_NotFound covers
// versionInTemplate and the IDs template routes take in their bodies: an admin
// cannot reach another workspace's version, injectable link or template
// through a template of their own. Each route answers exactly what it answers
// for an ID that does not exist, and no template row changes.
func TestTemplateVersionController_VersionFromAnotherWorkspace_NotFound(t *testing.T) {
	env := setupTemplateScopeEnv(t)
	missing := missingScopedTemplate()

	routes := []scopeRoute{
		{http.MethodGet, "{own:template}/versions/{other:draft}", nil},
		{http.MethodPut, "{own:template}/versions/{other:draft}", map[string]any{"name": "Renamed"}},
		{http.MethodDelete, "{own:template}/versions/{other:draft}", nil},
		{http.MethodPost, "{own:template}/versions/{other:draft}/publish", nil},
		{http.MethodPost, "{own:template}/versions/{other:published}/archive", nil},
		{http.MethodPost, "{own:template}/versions/{other:draft}/schedule-publish", map[string]any{"publishAt": at(48 * time.Hour)}},
		{http.MethodPost, "{own:template}/versions/{other:published}/schedule-archive", map[string]any{"archiveAt": at(72 * time.Hour)}},
		{http.MethodDelete, "{own:template}/versions/{other:scheduled}/schedule", nil},
		{http.MethodPost, "{own:template}/versions/{other:draft}/injectables", map[string]any{"injectableDefinitionId": "{own:freeInjectable}", "isRequired": true}},
		{http.MethodDelete, "{own:template}/versions/{own:draft}/injectables/{other:link}", nil},
		{http.MethodPost, "{own:template}/versions/{other:published}/promote", map[string]any{"mode": "NEW_TEMPLATE", "versionName": "Exfiltrated"}},
		{http.MethodPost, "{own:template}/versions/{other:draft}/preview", map[string]any{"injectables": map[string]any{}}},
		{http.MethodPost, "{own:template}/versions/from-existing", map[string]any{"sourceVersionId": "{other:draft}", "name": "Planted copy"}},
		{http.MethodPost, "{own:template}/clone", map[string]any{"newTitle": "Cloned away", "versionId": "{other:published}"}},
	}

	for _, route := range routes {
		t.Run(route.name(), func(t *testing.T) {
			own := env.createTemplate(t, env.outsiderWorkspaceID, env.outsiderDocTypeID).ids("own:")
			victim := env.createTemplate(t, env.victimWorkspaceID, env.victimDocTypeID).ids("other:")
			before := templateRows(t, env.pool, env.victimWorkspaceID, env.outsiderWorkspaceID)

			resp, body := route.send(t, env.outsider, strings.NewReplacer(append(own, victim...)...))
			assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))

			missingResp, missingBody := route.send(t, env.outsider, strings.NewReplacer(append(own, missing.ids("other:")...)...))
			assert.Equal(t, missingResp.StatusCode, resp.StatusCode, "status differs from a missing ID's")
			assert.Equal(t, string(missingBody), string(body), "body differs from a missing ID's")

			assert.JSONEq(t, before, templateRows(t, env.pool, env.victimWorkspaceID, env.outsiderWorkspaceID),
				"a template row changed")
		})
	}

	// The guards let the caller's own versions through.
	t.Run("POST {own:template}/versions/{own:draft}/preview renders", func(t *testing.T) {
		own := env.createTemplate(t, env.outsiderWorkspaceID, env.outsiderDocTypeID)
		resp, body := env.outsider.POST(templatesPath+own.templateID+"/versions/"+own.draftID+"/preview",
			map[string]any{"injectables": map[string]any{}})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
		assert.True(t, strings.HasPrefix(string(body), "%PDF"), "preview is not a PDF")
	})

	// Promoting into an existing template: the target must be one of the
	// caller's production templates.
	t.Run("POST {own sandbox template}/versions/{published}/promote NEW_VERSION targetTemplateId={other:template}", func(t *testing.T) {
		sourceTemplateID := testhelper.CreateTestTemplate(t, env.pool, env.outsiderSandboxID, "Outsider Sandbox Source", nil)
		sourceVersionID := testhelper.CreateTestTemplateVersion(t, env.pool, sourceTemplateID, 1, "Sandbox Published", entity.VersionStatusPublished)
		t.Cleanup(func() { testhelper.CleanupTemplate(t, env.pool, sourceTemplateID) })
		victim := env.createTemplate(t, env.victimWorkspaceID, env.victimDocTypeID)
		before := templateRows(t, env.pool, env.victimWorkspaceID, env.outsiderWorkspaceID)

		promote := func(targetTemplateID string) (*http.Response, []byte) {
			return env.outsider.WithHeader("X-Sandbox-Mode", "false").POST(
				templatesPath+sourceTemplateID+"/versions/"+sourceVersionID+"/promote",
				map[string]any{"mode": "NEW_VERSION", "targetTemplateId": targetTemplateID, "versionName": "Planted"})
		}
		resp, body := promote(victim.templateID)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
		missingResp, missingBody := promote(missing.templateID)
		assert.Equal(t, missingResp.StatusCode, resp.StatusCode, "status differs from a missing template's")
		assert.Equal(t, string(missingBody), string(body), "body differs from a missing template's")

		assert.JSONEq(t, before, templateRows(t, env.pool, env.victimWorkspaceID, env.outsiderWorkspaceID),
			"a template row changed")
	})
}
