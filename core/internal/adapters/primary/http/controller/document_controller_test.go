//go:build integration

package controller_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/testing/testhelper"
)

// documentTestEnv holds the common fixtures needed for document tests.
type documentTestEnv struct {
	ts          *testhelper.TestServer
	client      *testhelper.HTTPClient
	tenantID    string
	workspaceID string
	templateID  string
	versionID   string
	signerRole1 string
	signerRole2 string
	operator    *testhelper.TestUser
	viewer      *testhelper.TestUser
}

// setupDocumentEnv creates the full fixture stack needed for document tests:
// tenant > workspace > template > published version > 2 signer roles > operator + viewer users.
func setupDocumentEnv(t *testing.T) *documentTestEnv {
	t.Helper()

	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServer(t, pool)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantID := testhelper.CreateTestTenant(t, pool, "Doc Test Tenant", "DOCT01")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, tenantID) })

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Doc Test Workspace", entity.WorkspaceTypeClient)
	t.Cleanup(func() { testhelper.CleanupWorkspace(t, pool, workspaceID) })

	operator := testhelper.CreateTestUser(t, pool, "operator-doc@test.com", "Operator", nil)
	t.Cleanup(func() { testhelper.CleanupUser(t, pool, operator.ID) })
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, operator.ID, entity.WorkspaceRoleOperator, nil)

	viewer := testhelper.CreateTestUser(t, pool, "viewer-doc@test.com", "Viewer", nil)
	t.Cleanup(func() { testhelper.CleanupUser(t, pool, viewer.ID) })
	testhelper.CreateTestWorkspaceMember(t, pool, workspaceID, viewer.ID, entity.WorkspaceRoleViewer, nil)

	docTypeID := testhelper.CreateTestDocumentType(t, pool, tenantID, "TEST_DOC", "Test Document")
	t.Cleanup(func() { testhelper.CleanupDocumentType(t, pool, docTypeID) })

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Test Doc Template", nil)
	t.Cleanup(func() { testhelper.CleanupTemplate(t, pool, templateID) })

	testhelper.SetTemplateDocumentType(t, pool, templateID, docTypeID)

	versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "v1.0", entity.VersionStatusDraft)
	t.Cleanup(func() { testhelper.CleanupTemplateVersion(t, pool, versionID) })

	testhelper.PublishTestVersion(t, pool, versionID)

	signerRole1 := testhelper.CreateTestSignerRole(t, pool, versionID, "Signer", "__sig_rol_1__", 1)
	t.Cleanup(func() { testhelper.CleanupSignerRole(t, pool, signerRole1) })

	signerRole2 := testhelper.CreateTestSignerRole(t, pool, versionID, "Co-Signer", "__sig_rol_2__", 2)
	t.Cleanup(func() { testhelper.CleanupSignerRole(t, pool, signerRole2) })

	return &documentTestEnv{
		ts:          ts,
		client:      client,
		tenantID:    tenantID,
		workspaceID: workspaceID,
		templateID:  templateID,
		versionID:   versionID,
		signerRole1: signerRole1,
		signerRole2: signerRole2,
		operator:    operator,
		viewer:      viewer,
	}
}

// operatorClient returns an HTTP client authenticated as operator with workspace context.
func (env *documentTestEnv) operatorClient() *testhelper.HTTPClient {
	return env.client.WithAuth(env.operator.BearerHeader).WithWorkspaceID(env.workspaceID)
}

// viewerClient returns an HTTP client authenticated as viewer with workspace context.
func (env *documentTestEnv) viewerClient() *testhelper.HTTPClient {
	return env.client.WithAuth(env.viewer.BearerHeader).WithWorkspaceID(env.workspaceID)
}

// createDocumentReq builds a valid CreateDocumentRequest for the test environment.
func (env *documentTestEnv) createDocumentReq(title string) dto.CreateDocumentRequest {
	return dto.CreateDocumentRequest{
		TemplateVersionID: env.versionID,
		Title:             title,
		Recipients: []dto.CreateRecipientRequest{
			{RoleID: env.signerRole1, Name: "Alice", Email: "alice@test.com"},
			{RoleID: env.signerRole2, Name: "Bob", Email: "bob@test.com"},
		},
	}
}

// createDocument creates a document via the API and returns the parsed response.
func (env *documentTestEnv) createDocument(t *testing.T, title string) entity.DocumentWithRecipients {
	t.Helper()
	resp, body := env.operatorClient().POST("/api/v1/documents", env.createDocumentReq(title))
	require.Equal(t, http.StatusCreated, resp.StatusCode, "create document failed: %s", string(body))

	var doc entity.DocumentWithRecipients
	require.NoError(t, json.Unmarshal(body, &doc))
	t.Cleanup(func() { testhelper.CleanupDocument(t, testhelper.GetTestPool(t), doc.ID) })
	return doc
}

func setDocumentStatusForDocumentController(t *testing.T, documentID string, status entity.DocumentStatus) {
	t.Helper()
	_, err := testhelper.GetTestPool(t).Exec(t.Context(), `UPDATE execution.documents SET status = $2 WHERE id = $1`, documentID, status)
	require.NoError(t, err)
}

// --- Tests ---

func TestDocumentController_CreateDocument(t *testing.T) {
	env := setupDocumentEnv(t)

	t.Run("success", func(t *testing.T) {
		req := env.createDocumentReq("My Contract")

		resp, body := env.operatorClient().POST("/api/v1/documents", req)
		assert.Equal(t, http.StatusCreated, resp.StatusCode)

		var doc entity.DocumentWithRecipients
		require.NoError(t, json.Unmarshal(body, &doc))
		defer testhelper.CleanupDocument(t, testhelper.GetTestPool(t), doc.ID)

		assert.NotEmpty(t, doc.ID)
		assert.Equal(t, "My Contract", *doc.Title)
		assert.Equal(t, string(entity.DocumentStatusAwaitingInput), string(doc.Status))
		assert.Len(t, doc.Recipients, 2)
	})

	t.Run("validation missing title", func(t *testing.T) {
		req := dto.CreateDocumentRequest{
			TemplateVersionID: env.versionID,
			Recipients: []dto.CreateRecipientRequest{
				{RoleID: env.signerRole1, Name: "Alice", Email: "alice@test.com"},
				{RoleID: env.signerRole2, Name: "Bob", Email: "bob@test.com"},
			},
		}

		resp, _ := env.operatorClient().POST("/api/v1/documents", req)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("validation missing recipients", func(t *testing.T) {
		req := dto.CreateDocumentRequest{
			TemplateVersionID: env.versionID,
			Title:             "Missing Recipients",
			Recipients:        []dto.CreateRecipientRequest{},
		}

		resp, _ := env.operatorClient().POST("/api/v1/documents", req)
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("validation wrong recipient count", func(t *testing.T) {
		req := dto.CreateDocumentRequest{
			TemplateVersionID: env.versionID,
			Title:             "Wrong Count",
			Recipients: []dto.CreateRecipientRequest{
				{RoleID: env.signerRole1, Name: "Alice", Email: "alice@test.com"},
			},
		}

		resp, _ := env.operatorClient().POST("/api/v1/documents", req)
		assert.Equal(t, http.StatusInternalServerError, resp.StatusCode)
	})

	t.Run("forbidden for viewer", func(t *testing.T) {
		req := env.createDocumentReq("Viewer Attempt")

		resp, _ := env.viewerClient().POST("/api/v1/documents", req)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})

	t.Run("unauthorized without token", func(t *testing.T) {
		req := env.createDocumentReq("No Auth")

		resp, _ := env.client.WithWorkspaceID(env.workspaceID).POST("/api/v1/documents", req)
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestDocumentController_GetDocument(t *testing.T) {
	env := setupDocumentEnv(t)

	t.Run("success", func(t *testing.T) {
		doc := env.createDocument(t, "Get Test Doc")

		resp, body := env.viewerClient().GET("/api/v1/documents/" + doc.ID)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var got entity.DocumentWithRecipients
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, doc.ID, got.ID)
		assert.Equal(t, "Get Test Doc", *got.Title)
	})

	t.Run("not found", func(t *testing.T) {
		resp, _ := env.viewerClient().GET("/api/v1/documents/00000000-0000-0000-0000-000000000000")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestDocumentController_ListDocuments(t *testing.T) {
	env := setupDocumentEnv(t)

	doc1 := env.createDocument(t, "List Doc 1")
	_ = doc1
	doc2 := env.createDocument(t, "List Doc 2")
	_ = doc2

	t.Run("success default", func(t *testing.T) {
		resp, body := env.viewerClient().GET("/api/v1/documents")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var docs []*entity.DocumentListItem
		require.NoError(t, json.Unmarshal(body, &docs))
		assert.GreaterOrEqual(t, len(docs), 2)
	})

	t.Run("filter by status", func(t *testing.T) {
		resp, body := env.viewerClient().GET("/api/v1/documents?status=AWAITING_INPUT")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var docs []*entity.DocumentListItem
		require.NoError(t, json.Unmarshal(body, &docs))
		for _, d := range docs {
			assert.Equal(t, entity.DocumentStatusAwaitingInput, d.Status)
		}
	})

	t.Run("search by title", func(t *testing.T) {
		resp, body := env.viewerClient().GET("/api/v1/documents?search=List%20Doc%201")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var docs []*entity.DocumentListItem
		require.NoError(t, json.Unmarshal(body, &docs))
		assert.GreaterOrEqual(t, len(docs), 1)
	})

	t.Run("pagination", func(t *testing.T) {
		resp, body := env.viewerClient().GET("/api/v1/documents?limit=1&offset=0")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var docs []*entity.DocumentListItem
		require.NoError(t, json.Unmarshal(body, &docs))
		assert.LessOrEqual(t, len(docs), 1)
	})
}

func TestDocumentController_GetRecipients(t *testing.T) {
	env := setupDocumentEnv(t)
	doc := env.createDocument(t, "Recipients Test Doc")

	t.Run("success", func(t *testing.T) {
		resp, body := env.viewerClient().GET("/api/v1/documents/" + doc.ID + "/recipients")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var recipients []*entity.DocumentRecipientWithRole
		require.NoError(t, json.Unmarshal(body, &recipients))
		assert.Len(t, recipients, 2)
	})

	t.Run("not found", func(t *testing.T) {
		resp, _ := env.viewerClient().GET("/api/v1/documents/00000000-0000-0000-0000-000000000000/recipients")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	})
}

func TestDocumentController_GetStatistics(t *testing.T) {
	env := setupDocumentEnv(t)
	_ = env.createDocument(t, "Stats Doc")

	t.Run("success", func(t *testing.T) {
		resp, body := env.viewerClient().GET("/api/v1/documents/statistics")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var stats map[string]any
		require.NoError(t, json.Unmarshal(body, &stats))
		assert.GreaterOrEqual(t, stats["total"].(float64), float64(1))
	})
}

func TestDocumentController_RefreshStatus(t *testing.T) {
	env := setupDocumentEnv(t)
	doc := env.createDocument(t, "Refresh Test Doc")

	t.Run("success", func(t *testing.T) {
		resp, body := env.operatorClient().POST("/api/v1/documents/"+doc.ID+"/refresh", nil)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var refreshed entity.DocumentWithRecipients
		require.NoError(t, json.Unmarshal(body, &refreshed))
		assert.Equal(t, doc.ID, refreshed.ID)
	})

	t.Run("forbidden for viewer", func(t *testing.T) {
		resp, _ := env.viewerClient().POST("/api/v1/documents/"+doc.ID+"/refresh", nil)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}

func TestDocumentController_CancelDocument(t *testing.T) {
	env := setupDocumentEnv(t)

	t.Run("success", func(t *testing.T) {
		doc := env.createDocument(t, "Cancel Test Doc")

		resp, body := env.operatorClient().POST("/api/v1/documents/"+doc.ID+"/cancel", nil)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]string
		require.NoError(t, json.Unmarshal(body, &result))
		assert.Equal(t, "cancelled", result["status"])

		// Verify document is voided
		getResp, getBody := env.viewerClient().GET("/api/v1/documents/" + doc.ID)
		assert.Equal(t, http.StatusOK, getResp.StatusCode)

		var voided entity.DocumentWithRecipients
		require.NoError(t, json.Unmarshal(getBody, &voided))
		assert.Equal(t, entity.DocumentStatusCancelled, voided.Status)
	})

	t.Run("forbidden for viewer", func(t *testing.T) {
		doc := env.createDocument(t, "Cancel Forbidden Doc")

		resp, _ := env.viewerClient().POST("/api/v1/documents/"+doc.ID+"/cancel", nil)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}

func TestDocumentController_DeprecateDocument(t *testing.T) {
	env := setupDocumentEnv(t)

	t.Run("success for completed document", func(t *testing.T) {
		doc := env.createDocument(t, "Deprecate Test Doc")
		setDocumentStatusForDocumentController(t, doc.ID, entity.DocumentStatusCompleted)
		reason := "replacement signed"

		resp, body := env.operatorClient().POST("/api/v1/documents/"+doc.ID+"/deprecate", map[string]any{"reason": reason})
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

		var result dto.InternalDeprecateDocumentResponse
		require.NoError(t, json.Unmarshal(body, &result))
		assert.Equal(t, doc.ID, result.ID)
		assert.Equal(t, string(entity.DocumentStatusInvalidated), result.Status)

		getResp, getBody := env.viewerClient().GET("/api/v1/documents/" + doc.ID)
		require.Equal(t, http.StatusOK, getResp.StatusCode)

		var deprecated entity.DocumentWithRecipients
		require.NoError(t, json.Unmarshal(getBody, &deprecated))
		assert.Equal(t, entity.DocumentStatusInvalidated, deprecated.Status)
	})

	t.Run("rejects unsigned document", func(t *testing.T) {
		doc := env.createDocument(t, "Deprecate Unsigned Doc")

		resp, _ := env.operatorClient().POST("/api/v1/documents/"+doc.ID+"/deprecate", map[string]any{"reason": "not signed"})
		assert.Equal(t, http.StatusConflict, resp.StatusCode)
	})

	t.Run("forbidden for viewer", func(t *testing.T) {
		doc := env.createDocument(t, "Deprecate Forbidden Doc")
		setDocumentStatusForDocumentController(t, doc.ID, entity.DocumentStatusCompleted)

		resp, _ := env.viewerClient().POST("/api/v1/documents/"+doc.ID+"/deprecate", map[string]any{"reason": "no access"})
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}

func TestDocumentController_GetDocumentEvents(t *testing.T) {
	env := setupDocumentEnv(t)
	doc := env.createDocument(t, "Events Test Doc")

	t.Run("success", func(t *testing.T) {
		resp, body := env.viewerClient().GET("/api/v1/documents/" + doc.ID + "/events")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var events []dto.DocumentEventResponse
		require.NoError(t, json.Unmarshal(body, &events))
		// CreateAndSendDocument emits DOCUMENT_CREATED and DOCUMENT_SENT events
		assert.GreaterOrEqual(t, len(events), 1)
	})
}

func TestDocumentController_GetSigningURL(t *testing.T) {
	env := setupDocumentEnv(t)
	doc := env.createDocument(t, "Signing URL Doc")

	t.Run("no active attempt", func(t *testing.T) {
		recipientID := doc.Recipients[0].ID

		resp, body := env.viewerClient().GET("/api/v1/documents/" + doc.ID + "/recipients/" + recipientID + "/signing-url")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.Contains(t, string(body), "signing attempt not found")
	})
}

func TestDocumentController_GetDocumentPDF(t *testing.T) {
	t.Skip("completed PDF download is attempt-owned and covered by signing-attempt River/live validation")
}

func TestDocumentController_SendReminder(t *testing.T) {
	env := setupDocumentEnv(t)
	doc := env.createDocument(t, "Reminder Test Doc")

	t.Run("success", func(t *testing.T) {
		resp, body := env.operatorClient().POST("/api/v1/documents/"+doc.ID+"/remind", nil)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var result map[string]string
		require.NoError(t, json.Unmarshal(body, &result))
		assert.Equal(t, "reminders_sent", result["status"])
	})

	t.Run("forbidden for viewer", func(t *testing.T) {
		resp, _ := env.viewerClient().POST("/api/v1/documents/"+doc.ID+"/remind", nil)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}

func TestDocumentController_BatchCreate(t *testing.T) {
	env := setupDocumentEnv(t)

	t.Run("success", func(t *testing.T) {
		req := dto.BatchCreateDocumentRequest{
			Documents: []dto.CreateDocumentRequest{
				env.createDocumentReq("Batch Doc 1"),
				env.createDocumentReq("Batch Doc 2"),
			},
		}

		resp, body := env.operatorClient().POST("/api/v1/documents/batch", req)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var batchResp dto.BatchCreateDocumentResponse
		require.NoError(t, json.Unmarshal(body, &batchResp))

		assert.Len(t, batchResp.Results, 2)
		for _, r := range batchResp.Results {
			assert.True(t, r.Success)
			assert.NotNil(t, r.Document)
			t.Cleanup(func() { testhelper.CleanupDocument(t, testhelper.GetTestPool(t), r.Document.ID) })
		}
	})

	t.Run("forbidden for viewer", func(t *testing.T) {
		req := dto.BatchCreateDocumentRequest{
			Documents: []dto.CreateDocumentRequest{
				env.createDocumentReq("Batch Forbidden"),
			},
		}

		resp, _ := env.viewerClient().POST("/api/v1/documents/batch", req)
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}
