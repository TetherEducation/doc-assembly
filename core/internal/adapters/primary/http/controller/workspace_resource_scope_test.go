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

// workspaceResourceRows returns the folders, tags, members and templates of
// the workspaces, with the templates' tag links, as one JSON document, so a
// test can tell whether a request changed any of them.
func workspaceResourceRows(t *testing.T, pool *pgxpool.Pool, workspaceIDs ...string) string {
	t.Helper()
	var rows string
	err := pool.QueryRow(context.Background(), `
		WITH tpl AS (SELECT * FROM content.templates WHERE workspace_id = ANY($1::uuid[]))
		SELECT json_build_object(
			'folders', (SELECT COALESCE(json_agg(f ORDER BY f.id), '[]')
				FROM organizer.folders f WHERE f.workspace_id = ANY($1::uuid[])),
			'tags', (SELECT COALESCE(json_agg(tg ORDER BY tg.id), '[]')
				FROM organizer.tags tg WHERE tg.workspace_id = ANY($1::uuid[])),
			'members', (SELECT COALESCE(json_agg(m ORDER BY m.id), '[]')
				FROM identity.workspace_members m WHERE m.workspace_id = ANY($1::uuid[])),
			'templates', (SELECT COALESCE(json_agg(tpl ORDER BY tpl.id), '[]') FROM tpl),
			'templateTags', (SELECT COALESCE(json_agg(tt ORDER BY tt.template_id, tt.tag_id), '[]')
				FROM content.template_tags tt WHERE tt.template_id IN (SELECT id FROM tpl))
		)::text`, workspaceIDs).Scan(&rows)
	require.NoError(t, err)
	return rows
}

// scopedResources is what one workspace offers the routes under test: a folder
// inside a parent folder, a tag no template uses, a member, and a template
// with a published version.
type scopedResources struct {
	parentFolderID string
	folderID       string
	tagID          string
	memberID       string
	templateID     string
	publishedID    string
}

// missingScopedResources names folders, a tag, a member, a template and a
// version that do not exist.
func missingScopedResources() scopedResources {
	return scopedResources{
		parentFolderID: uuid.NewString(),
		folderID:       uuid.NewString(),
		tagID:          uuid.NewString(),
		memberID:       uuid.NewString(),
		templateID:     uuid.NewString(),
		publishedID:    uuid.NewString(),
	}
}

// ids pairs each of r's IDs with the placeholder a route names it by:
// {folder}, {tag}, ... with prefix inside the braces ({own:folder}).
func (r scopedResources) ids(prefix string) []string {
	return []string{
		"{" + prefix + "parentFolder}", r.parentFolderID,
		"{" + prefix + "folder}", r.folderID,
		"{" + prefix + "tag}", r.tagID,
		"{" + prefix + "member}", r.memberID,
		"{" + prefix + "template}", r.templateID,
		"{" + prefix + "published}", r.publishedID,
	}
}

// resourceRoute is a request whose path (under /api/v1) and JSON body name
// IDs by placeholder.
type resourceRoute struct {
	method string
	path   string
	body   map[string]any
}

// name is the route's method and path, plus any ID its body names.
func (r resourceRoute) name() string {
	name := r.method + " " + r.path
	for _, key := range slices.Sorted(maps.Keys(r.body)) {
		switch value := r.body[key].(type) {
		case string:
			if strings.HasPrefix(value, "{") {
				name += " " + key + "=" + value
			}
		case []string:
			name += " " + key + "=" + strings.Join(value, ",")
		}
	}
	return name
}

// send makes the request with each placeholder replaced by its ID.
func (r resourceRoute) send(t *testing.T, client *testhelper.HTTPClient, ids *strings.Replacer) (*http.Response, []byte) {
	t.Helper()
	var body []byte
	if r.body != nil {
		raw, err := json.Marshal(r.body)
		require.NoError(t, err)
		body = []byte(ids.Replace(string(raw)))
	}
	return client.DoRaw(r.method, "/api/v1"+ids.Replace(r.path), body)
}

// resourceScopeEnv is two workspaces of one tenant: the other workspace, which
// owns the folders, tags and members under test, and the caller's own, whose
// OWNER makes every request. The caller has no access to the other workspace.
type resourceScopeEnv struct {
	pool             *pgxpool.Pool
	otherWorkspaceID string
	ownWorkspaceID   string
	ownSandboxID     string
	owner            *testhelper.HTTPClient
}

func setupResourceScopeEnv(t *testing.T) *resourceScopeEnv {
	t.Helper()
	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())
	env := &resourceScopeEnv{pool: pool}

	tenantID := testhelper.CreateTestTenant(t, pool, "Workspace Resource Scope", "WRSC01")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, tenantID) })
	env.otherWorkspaceID = testhelper.CreateTestWorkspace(t, pool, &tenantID, "Other Workspace", entity.WorkspaceTypeClient)
	env.ownWorkspaceID = testhelper.CreateTestWorkspace(t, pool, &tenantID, "Own Workspace", entity.WorkspaceTypeClient)
	env.ownSandboxID = testhelper.CreateTestSandboxWorkspace(t, pool, env.ownWorkspaceID)

	owner := testhelper.CreateTestUser(t, pool, "owner-resource-scope@test.com", "Owner", nil)
	t.Cleanup(func() { testhelper.CleanupUser(t, pool, owner.ID) })
	testhelper.CreateTestWorkspaceMember(t, pool, env.ownWorkspaceID, owner.ID, entity.WorkspaceRoleOwner, nil)
	env.owner = client.WithAuth(owner.BearerHeader).WithWorkspaceID(env.ownWorkspaceID)

	// Runs before the user's and the tenant's cleanup: also removes whatever a
	// request created in a workspace, so those cleanups cannot trip over it.
	t.Cleanup(func() {
		cleanupWorkspaceTemplates(t, pool, env.otherWorkspaceID, env.ownWorkspaceID, env.ownSandboxID)
	})

	return env
}

// createResources creates scopedResources in workspaceID, removed when the
// (sub)test ends.
func (env *resourceScopeEnv) createResources(t *testing.T, workspaceID string) scopedResources {
	t.Helper()
	pool := env.pool
	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]

	var res scopedResources
	res.parentFolderID = testhelper.CreateTestFolder(t, pool, workspaceID, "Parent "+suffix, nil)
	res.folderID = testhelper.CreateTestFolder(t, pool, workspaceID, "Folder "+suffix, &res.parentFolderID)
	res.tagID = testhelper.CreateTestTag(t, pool, workspaceID, "tag-"+suffix, "#112233")
	member := testhelper.CreateTestUser(t, pool, "member-"+suffix+"@test.com", "Member "+suffix, nil)
	res.memberID = testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, member.ID, entity.WorkspaceRoleViewer, nil)
	res.templateID = testhelper.CreateTestTemplate(t, pool, workspaceID, "Template "+suffix, nil)
	res.publishedID = testhelper.CreateTestTemplateVersion(t, pool, res.templateID, 1, "Published", entity.VersionStatusPublished)

	t.Cleanup(func() {
		testhelper.CleanupTemplate(t, pool, res.templateID)
		testhelper.CleanupTag(t, pool, res.tagID)
		testhelper.CleanupFolder(t, pool, res.parentFolderID) // takes the folder inside it along
		testhelper.CleanupUser(t, pool, member.ID)
	})
	return res
}

// TestWorkspaceController_ResourceFromAnotherWorkspace_NotFound covers the
// routes that name a folder, a tag or a member, in the path or in the body:
// the owner of one workspace cannot read, change or refer to another
// workspace's by ID. Each route answers exactly what it answers for an ID that
// does not exist, so the ID cannot be probed, and no row of either workspace
// changes.
func TestWorkspaceController_ResourceFromAnotherWorkspace_NotFound(t *testing.T) {
	env := setupResourceScopeEnv(t)
	missing := missingScopedResources()

	routes := []resourceRoute{
		// A folder, tag or member named in the path.
		{http.MethodGet, "/workspace/folders/{folder}", nil},
		{http.MethodPut, "/workspace/folders/{folder}", map[string]any{"name": "Renamed"}},
		{http.MethodPatch, "/workspace/folders/{folder}/move", map[string]any{"newParentId": nil}},
		{http.MethodDelete, "/workspace/folders/{folder}", nil},
		{http.MethodGet, "/workspace/tags/{tag}", nil},
		{http.MethodPut, "/workspace/tags/{tag}", map[string]any{"name": "renamed", "color": "#000000"}},
		{http.MethodDelete, "/workspace/tags/{tag}", nil},
		{http.MethodGet, "/workspace/members/{member}", nil},
		{http.MethodPut, "/workspace/members/{member}", map[string]any{"role": "ADMIN"}},
		{http.MethodDelete, "/workspace/members/{member}", nil},
		// A folder or tag named in the body of a request on the caller's own
		// folder or template.
		{http.MethodPost, "/workspace/folders", map[string]any{"name": "Nested", "parentId": "{folder}"}},
		{http.MethodPatch, "/workspace/folders/{own:folder}/move", map[string]any{"newParentId": "{folder}"}},
		{http.MethodPost, "/content/templates", map[string]any{"title": "Filed away", "folderId": "{folder}"}},
		{http.MethodPut, "/content/templates/{own:template}", map[string]any{"folderId": "{folder}"}},
		{http.MethodPost, "/content/templates/{own:template}/clone", map[string]any{"newTitle": "Cloned away", "versionId": "{own:published}", "targetFolderId": "{folder}"}},
		{http.MethodPost, "/content/templates/{own:template}/tags", map[string]any{"tagIds": []string{"{tag}"}}},
	}

	for _, route := range routes {
		t.Run(route.name(), func(t *testing.T) {
			own := env.createResources(t, env.ownWorkspaceID).ids("own:")
			other := env.createResources(t, env.otherWorkspaceID).ids("")
			before := workspaceResourceRows(t, env.pool, env.otherWorkspaceID, env.ownWorkspaceID)

			resp, body := route.send(t, env.owner, strings.NewReplacer(append(own, other...)...))
			assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))

			missingResp, missingBody := route.send(t, env.owner, strings.NewReplacer(append(own, missing.ids("")...)...))
			assert.Equal(t, missingResp.StatusCode, resp.StatusCode, "status differs from a missing ID's")
			assert.Equal(t, string(missingBody), string(body), "body differs from a missing ID's")

			assert.JSONEq(t, before, workspaceResourceRows(t, env.pool, env.otherWorkspaceID, env.ownWorkspaceID),
				"a folder, tag, member or template row changed")
		})
	}

	// Removing a tag from a template deletes the link between the two. A
	// template has no link to another workspace's tag, so the route answers as
	// it does for a tag that does not exist.
	t.Run("DELETE /content/templates/{own:template}/tags/{tag}", func(t *testing.T) {
		own := env.createResources(t, env.ownWorkspaceID)
		other := env.createResources(t, env.otherWorkspaceID)
		before := workspaceResourceRows(t, env.pool, env.otherWorkspaceID, env.ownWorkspaceID)

		tagPath := "/api/v1/content/templates/" + own.templateID + "/tags/"
		resp, body := env.owner.DELETE(tagPath + other.tagID)
		missingResp, missingBody := env.owner.DELETE(tagPath + missing.tagID)
		assert.Equal(t, missingResp.StatusCode, resp.StatusCode, "status differs from a missing tag's")
		assert.Equal(t, string(missingBody), string(body), "body differs from a missing tag's")

		assert.JSONEq(t, before, workspaceResourceRows(t, env.pool, env.otherWorkspaceID, env.ownWorkspaceID),
			"a folder, tag, member or template row changed")
	})

	// Promote runs in production on a template of the workspace's sandbox, and
	// files the new template in a folder of the production workspace.
	promote := func(t *testing.T, folderID string) (*http.Response, []byte) {
		t.Helper()
		sourceTemplateID := testhelper.CreateTestTemplate(t, env.pool, env.ownSandboxID, "Promoted "+uuid.NewString()[:8], nil)
		sourceVersionID := testhelper.CreateTestTemplateVersion(t, env.pool, sourceTemplateID, 1, "Sandbox Published", entity.VersionStatusPublished)
		t.Cleanup(func() { testhelper.CleanupTemplate(t, env.pool, sourceTemplateID) })

		return env.owner.WithHeader("X-Sandbox-Mode", "false").POST(
			"/api/v1/content/templates/"+sourceTemplateID+"/versions/"+sourceVersionID+"/promote",
			map[string]any{"mode": "NEW_TEMPLATE", "targetFolderId": folderID})
	}

	t.Run("POST /content/templates/{own sandbox template}/versions/{published}/promote targetFolderId={folder}", func(t *testing.T) {
		other := env.createResources(t, env.otherWorkspaceID)
		before := workspaceResourceRows(t, env.pool, env.otherWorkspaceID, env.ownWorkspaceID)

		resp, body := promote(t, other.folderID)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
		missingResp, missingBody := promote(t, missing.folderID)
		assert.Equal(t, missingResp.StatusCode, resp.StatusCode, "status differs from a missing folder's")
		assert.Equal(t, string(missingBody), string(body), "body differs from a missing folder's")

		assert.JSONEq(t, before, workspaceResourceRows(t, env.pool, env.otherWorkspaceID, env.ownWorkspaceID),
			"a folder, tag, member or template row changed")
	})

	// The check lets a folder of the production workspace through.
	t.Run("POST .../promote targetFolderId={own:folder} files the template there", func(t *testing.T) {
		own := env.createResources(t, env.ownWorkspaceID)

		resp, body := promote(t, own.folderID)
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		promoted := testhelper.ParseJSON[struct {
			Template struct {
				ID          string  `json:"id"`
				WorkspaceID string  `json:"workspaceId"`
				FolderID    *string `json:"folderId"`
			} `json:"template"`
		}](t, body)
		t.Cleanup(func() { testhelper.CleanupTemplate(t, env.pool, promoted.Template.ID) })

		assert.Equal(t, env.ownWorkspaceID, promoted.Template.WorkspaceID)
		require.NotNil(t, promoted.Template.FolderID)
		assert.Equal(t, own.folderID, *promoted.Template.FolderID)
	})
}

// TestWorkspaceController_ResourceScope_SandboxMode covers the same checks
// with X-Sandbox-Mode: the caller's workspace is then its sandbox, which has
// folders of its own, while tags stay those of the workspace the sandbox
// belongs to.
func TestWorkspaceController_ResourceScope_SandboxMode(t *testing.T) {
	env := setupResourceScopeEnv(t)
	missing := missingScopedResources()
	fromSandbox := env.owner.WithHeader("X-Sandbox-Mode", "true")

	otherSandboxID := testhelper.CreateTestSandboxWorkspace(t, env.pool, env.otherWorkspaceID)
	t.Cleanup(func() { cleanupWorkspaceTemplates(t, env.pool, otherSandboxID) })

	// Another workspace's sandbox folder is as foreign as its production one.
	t.Run("GET /workspace/folders/{other sandbox folder}", func(t *testing.T) {
		otherSandbox := env.createResources(t, otherSandboxID)

		resp, body := fromSandbox.GET("/api/v1/workspace/folders/" + otherSandbox.folderID)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
		missingResp, missingBody := fromSandbox.GET("/api/v1/workspace/folders/" + missing.folderID)
		assert.Equal(t, missingResp.StatusCode, resp.StatusCode, "status differs from a missing folder's")
		assert.Equal(t, string(missingBody), string(body), "body differs from a missing folder's")
	})

	// Another workspace's tag cannot be put on a sandbox template either.
	t.Run("POST /content/templates/{own sandbox template}/tags tagIds={tag}", func(t *testing.T) {
		ownSandbox := env.createResources(t, env.ownSandboxID)
		other := env.createResources(t, env.otherWorkspaceID)
		before := workspaceResourceRows(t, env.pool, env.otherWorkspaceID, env.ownWorkspaceID, env.ownSandboxID)

		tagsPath := "/api/v1/content/templates/" + ownSandbox.templateID + "/tags"
		resp, body := fromSandbox.POST(tagsPath, map[string]any{"tagIds": []string{other.tagID}})
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
		missingResp, missingBody := fromSandbox.POST(tagsPath, map[string]any{"tagIds": []string{missing.tagID}})
		assert.Equal(t, missingResp.StatusCode, resp.StatusCode, "status differs from a missing tag's")
		assert.Equal(t, string(missingBody), string(body), "body differs from a missing tag's")

		assert.JSONEq(t, before, workspaceResourceRows(t, env.pool, env.otherWorkspaceID, env.ownWorkspaceID, env.ownSandboxID),
			"a folder, tag, member or template row changed")
	})

	// The checks let the sandbox's own folder through, and a tag of the
	// workspace the sandbox belongs to.
	t.Run("the sandbox's folder and its workspace's tag are the caller's own", func(t *testing.T) {
		own := env.createResources(t, env.ownWorkspaceID)
		ownSandbox := env.createResources(t, env.ownSandboxID)

		resp, body := fromSandbox.GET("/api/v1/workspace/folders/" + ownSandbox.folderID)
		assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))

		resp, body = fromSandbox.PUT("/api/v1/content/templates/"+ownSandbox.templateID,
			map[string]any{"folderId": ownSandbox.folderID})
		assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))

		resp, body = fromSandbox.POST("/api/v1/content/templates/"+ownSandbox.templateID+"/tags",
			map[string]any{"tagIds": []string{own.tagID}})
		assert.Equal(t, http.StatusNoContent, resp.StatusCode, string(body))

		var linked bool
		err := env.pool.QueryRow(context.Background(),
			`SELECT EXISTS(SELECT 1 FROM content.template_tags WHERE template_id = $1 AND tag_id = $2)`,
			ownSandbox.templateID, own.tagID).Scan(&linked)
		require.NoError(t, err)
		assert.True(t, linked, "the workspace's tag was not put on the sandbox template")
	})
}
