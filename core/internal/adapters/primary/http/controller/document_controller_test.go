//go:build integration

package controller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
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
	docTypeID   string
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
		docTypeID:   docTypeID,
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

// setDocumentStatusForDocumentController forces the status of a document directly in the database,
// bypassing the service-level state machine. Used by controller integration tests that need to
// exercise endpoints whose preconditions require a specific terminal status (e.g. deprecate).
func setDocumentStatusForDocumentController(t *testing.T, documentID string, status entity.DocumentStatus) {
	t.Helper()
	_, err := testhelper.GetTestPool(t).Exec(
		context.Background(),
		`UPDATE execution.documents SET status = $2 WHERE id = $1`,
		documentID,
		status,
	)
	require.NoError(t, err)
}

// giveRecipientSigningURL attaches an active SIGNING_READY attempt to the document
// carrying signingURL for recipientID, as if the provider had accepted the
// submission, so GET .../signing-url has a live link to hand out.
func giveRecipientSigningURL(t *testing.T, documentID, recipientID, signingURL string) {
	t.Helper()
	pool := testhelper.GetTestPool(t)
	ctx := context.Background()

	var attemptID string
	require.NoError(t, pool.QueryRow(ctx, `
		INSERT INTO execution.signing_attempts (document_id, sequence, status)
		VALUES ($1, (SELECT COALESCE(MAX(sequence), 0) + 1 FROM execution.signing_attempts WHERE document_id = $1), $2)
		RETURNING id
	`, documentID, entity.SigningAttemptStatusSigningReady).Scan(&attemptID))

	tag, err := pool.Exec(ctx, `
		INSERT INTO execution.signing_attempt_recipients (
			attempt_id, document_recipient_id, template_version_role_id, signer_order, email, name, signing_url
		)
		SELECT $1::uuid, dr.id, dr.template_version_role_id, role.signer_order, dr.email, dr.name, $3::text
		FROM execution.document_recipients dr
		JOIN content.template_version_signer_roles role ON role.id = dr.template_version_role_id
		WHERE dr.id = $2::uuid
	`, attemptID, recipientID, signingURL)
	require.NoError(t, err)
	require.Equal(t, int64(1), tag.RowsAffected(), "recipient %s not found", recipientID)

	_, err = pool.Exec(ctx, `UPDATE execution.documents SET active_attempt_id = $2 WHERE id = $1`, documentID, attemptID)
	require.NoError(t, err)
}

// sandboxDocFixture holds sandbox workspace template/version/signer IDs for document API tests.
type sandboxDocFixture struct {
	workspaceID string
	versionID   string
	signerRole1 string
	signerRole2 string
}

// setupSandboxDocFixture creates a sandbox workspace with a published template and signer roles.
func setupSandboxDocFixture(t *testing.T, env *documentTestEnv) *sandboxDocFixture {
	t.Helper()
	pool := testhelper.GetTestPool(t)
	ctx := context.Background()

	sandboxID := testhelper.CreateTestSandboxWorkspace(t, pool, env.workspaceID)
	t.Cleanup(func() { testhelper.CleanupWorkspace(t, pool, sandboxID) })

	testhelper.CreateTestWorkspaceMember(t, pool, sandboxID, env.operator.ID, entity.WorkspaceRoleOperator, nil)
	testhelper.CreateTestWorkspaceMember(t, pool, sandboxID, env.viewer.ID, entity.WorkspaceRoleViewer, nil)

	var docTypeID string
	err := pool.QueryRow(ctx,
		`SELECT document_type_id FROM content.templates WHERE id = $1`,
		env.templateID,
	).Scan(&docTypeID)
	require.NoError(t, err)
	require.NotEmpty(t, docTypeID)

	tplID := testhelper.CreateTestTemplate(t, pool, sandboxID, "Sandbox Doc Template", nil)
	t.Cleanup(func() { testhelper.CleanupTemplate(t, pool, tplID) })
	testhelper.SetTemplateDocumentType(t, pool, tplID, docTypeID)

	verID := testhelper.CreateTestTemplateVersion(t, pool, tplID, 1, "v1.0", entity.VersionStatusDraft)
	t.Cleanup(func() { testhelper.CleanupTemplateVersion(t, pool, verID) })
	testhelper.PublishTestVersion(t, pool, verID)

	role1 := testhelper.CreateTestSignerRole(t, pool, verID, "Signer", "__sig_sbx_1__", 1)
	t.Cleanup(func() { testhelper.CleanupSignerRole(t, pool, role1) })
	role2 := testhelper.CreateTestSignerRole(t, pool, verID, "Co-Signer", "__sig_sbx_2__", 2)
	t.Cleanup(func() { testhelper.CleanupSignerRole(t, pool, role2) })

	return &sandboxDocFixture{
		workspaceID: sandboxID,
		versionID:   verID,
		signerRole1: role1,
		signerRole2: role2,
	}
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

func TestDocumentController_CreateDocument_DuplicateRecipientRole(t *testing.T) {
	env := setupDocumentEnv(t)
	req := env.createDocumentReq("Duplicate Role")
	// Two recipients for the first role leave the second role unfilled.
	req.Recipients[1].RoleID = env.signerRole1

	resp, body := env.operatorClient().POST("/api/v1/documents", req)

	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))
	assert.Contains(t, string(body), entity.ErrDuplicateRecipientRole.Error())
}

// With signing.pdf_send off (the default, and the test server's setup) the
// "Send a PDF" route does not exist.
func TestDocumentController_SendPDF_AbsentWhenOff(t *testing.T) {
	env := setupDocumentEnv(t)

	resp, body := env.operatorClient().POST("/api/v1/documents/from-pdf", nil)

	assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
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

		var listedDoc *entity.DocumentListItem
		for _, d := range docs {
			if d.ID == doc1.ID {
				listedDoc = d
				break
			}
		}

		require.NotNil(t, listedDoc)
		assert.Equal(t, env.workspaceID, listedDoc.WorkspaceID)
		assert.Equal(t, env.versionID, listedDoc.TemplateVersionID)
		assert.NotEmpty(t, listedDoc.DocumentTypeID)
		assert.Equal(t, "Test Document", *listedDoc.DocumentTypeName)
		assert.Equal(t, "Test Doc Template", listedDoc.TemplateName)
		require.NotNil(t, listedDoc.Title)
		assert.Equal(t, "List Doc 1", *listedDoc.Title)
		assert.Len(t, listedDoc.Recipients, 2)
		assert.Equal(t, "alice@test.com", listedDoc.Recipients[0].Email)
		assert.Equal(t, "bob@test.com", listedDoc.Recipients[1].Email)
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

	t.Run("filter by multiple statuses", func(t *testing.T) {
		setDocumentStatusForDocumentController(t, doc2.ID, entity.DocumentStatusReadyToSign)
		t.Cleanup(func() {
			setDocumentStatusForDocumentController(t, doc2.ID, entity.DocumentStatusAwaitingInput)
		})

		resp, body := env.viewerClient().GET(
			"/api/v1/documents?status=AWAITING_INPUT%2CREADY_TO_SIGN",
		)
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var docs []*entity.DocumentListItem
		require.NoError(t, json.Unmarshal(body, &docs))
		got := make(map[string]entity.DocumentStatus, len(docs))
		for _, d := range docs {
			if d.ID == doc1.ID || d.ID == doc2.ID {
				got[d.ID] = d.Status
			}
		}
		require.Contains(t, got, doc1.ID)
		require.Contains(t, got, doc2.ID)
		assert.Equal(t, entity.DocumentStatusAwaitingInput, got[doc1.ID])
		assert.Equal(t, entity.DocumentStatusReadyToSign, got[doc2.ID])
	})

	t.Run("invalid status in filter returns 400", func(t *testing.T) {
		resp, _ := env.viewerClient().GET("/api/v1/documents?status=AWAITING_INPUT%2CNOT_A_STATUS")
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
	})

	t.Run("search by title", func(t *testing.T) {
		resp, body := env.viewerClient().GET("/api/v1/documents?search=List%20Doc%201")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var docs []*entity.DocumentListItem
		require.NoError(t, json.Unmarshal(body, &docs))
		assert.GreaterOrEqual(t, len(docs), 1)
	})

	t.Run("search by signer email substring", func(t *testing.T) {
		resp, body := env.viewerClient().GET("/api/v1/documents?search=alice%40test.com")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var docs []*entity.DocumentListItem
		require.NoError(t, json.Unmarshal(body, &docs))
		require.NotEmpty(t, docs)
		found := false
		for _, d := range docs {
			if d.ID == doc1.ID {
				found = true
				break
			}
		}
		assert.True(t, found, "expected doc1 to match recipient alice@test.com")
	})

	t.Run("pagination", func(t *testing.T) {
		resp, body := env.viewerClient().GET("/api/v1/documents?limit=1&offset=0")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var docs []*entity.DocumentListItem
		require.NoError(t, json.Unmarshal(body, &docs))
		assert.LessOrEqual(t, len(docs), 1)
	})

	t.Run("filter by document type ids", func(t *testing.T) {
		pool := testhelper.GetTestPool(t)
		docTypeAlt := testhelper.CreateTestDocumentType(t, pool, env.tenantID, "OTHER_DT", "Employment Agreement")
		t.Cleanup(func() { testhelper.CleanupDocumentType(t, pool, docTypeAlt) })

		_, err := pool.Exec(context.Background(), `UPDATE execution.documents SET document_type_id = $2 WHERE id = $1`, doc2.ID, docTypeAlt)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `UPDATE execution.documents SET document_type_id = $2 WHERE id = $1`, doc2.ID, env.docTypeID)
		})

		q := url.Values{}
		q.Set("documentTypeIds", env.docTypeID)
		resp, body := env.viewerClient().GET("/api/v1/documents?" + q.Encode())
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var docs []*entity.DocumentListItem
		require.NoError(t, json.Unmarshal(body, &docs))
		ids := make(map[string]struct{}, len(docs))
		for _, d := range docs {
			ids[d.ID] = struct{}{}
			if d.ID == doc1.ID || d.ID == doc2.ID {
				assert.Equal(t, env.docTypeID, d.DocumentTypeID)
			}
		}
		_, hasDoc1 := ids[doc1.ID]
		_, hasDoc2 := ids[doc2.ID]
		assert.True(t, hasDoc1)
		assert.False(t, hasDoc2, "doc2 was reassigned to alternate document type")
	})

	t.Run("document type options", func(t *testing.T) {
		pool := testhelper.GetTestPool(t)
		docTypeAlt := testhelper.CreateTestDocumentType(t, pool, env.tenantID, "OPTS_DT", "Service Agreement")
		t.Cleanup(func() { testhelper.CleanupDocumentType(t, pool, docTypeAlt) })
		_, err := pool.Exec(context.Background(), `UPDATE execution.documents SET document_type_id = $2 WHERE id = $1`, doc2.ID, docTypeAlt)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `UPDATE execution.documents SET document_type_id = $2 WHERE id = $1`, doc2.ID, env.docTypeID)
		})

		resp, body := env.viewerClient().GET("/api/v1/documents/document-type-options")
		assert.Equal(t, http.StatusOK, resp.StatusCode)

		var opts []*entity.DocumentTypeFilterOption
		require.NoError(t, json.Unmarshal(body, &opts))
		assert.GreaterOrEqual(t, len(opts), 2)
		seen := map[string]bool{}
		for _, o := range opts {
			seen[o.ID] = true
		}
		assert.True(t, seen[env.docTypeID])
		assert.True(t, seen[docTypeAlt])
	})

	t.Run("invalid document type id returns 400", func(t *testing.T) {
		resp, _ := env.viewerClient().GET("/api/v1/documents?documentTypeIds=not-a-uuid")
		assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
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

		resp, body := env.operatorClient().GET("/api/v1/documents/" + doc.ID + "/recipients/" + recipientID + "/signing-url")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.Contains(t, string(body), "signing attempt not found")
	})

	t.Run("recipient not on the document", func(t *testing.T) {
		resp, body := env.operatorClient().GET("/api/v1/documents/" + doc.ID + "/recipients/00000000-0000-0000-0000-000000000000/signing-url")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode)
		assert.Contains(t, string(body), "document recipient not found")
	})

	t.Run("forbidden for viewer", func(t *testing.T) {
		// The link lets whoever holds it sign as the recipient.
		resp, _ := env.viewerClient().GET("/api/v1/documents/" + doc.ID + "/recipients/" + doc.Recipients[0].ID + "/signing-url")
		assert.Equal(t, http.StatusForbidden, resp.StatusCode)
	})
}

func TestDocumentController_GetDocumentPDF(t *testing.T) {
	env := setupDocumentEnv(t)
	doc := env.createDocument(t, "PDF Doc")

	t.Run("a document that never completed answers 409", func(t *testing.T) {
		resp, body := env.viewerClient().GET("/api/v1/documents/" + doc.ID + "/pdf")
		assert.Equal(t, http.StatusConflict, resp.StatusCode, string(body))
	})

	t.Run("a completed document without a stored or provider PDF answers 404", func(t *testing.T) {
		setDocumentStatusForDocumentController(t, doc.ID, entity.DocumentStatusCompleted)

		resp, body := env.viewerClient().GET("/api/v1/documents/" + doc.ID + "/pdf")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
	})
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

func TestDocumentController_Sandbox_ListAndCreateIsolation(t *testing.T) {
	env := setupDocumentEnv(t)
	sbx := setupSandboxDocFixture(t, env)

	prodDoc := env.createDocument(t, "Prod Only Doc")

	sandboxReq := dto.CreateDocumentRequest{
		TemplateVersionID: sbx.versionID,
		Title:             "Sandbox Only Doc",
		Recipients: []dto.CreateRecipientRequest{
			{RoleID: sbx.signerRole1, Name: "Alice", Email: "alice-sbx@test.com"},
			{RoleID: sbx.signerRole2, Name: "Bob", Email: "bob-sbx@test.com"},
		},
	}

	sandboxOperator := env.operatorClient().WithHeader(middleware.SandboxModeHeader, "true")
	sandboxViewer := env.viewerClient().WithHeader(middleware.SandboxModeHeader, "true")

	t.Run("create routed to sandbox workspace", func(t *testing.T) {
		resp, body := sandboxOperator.POST("/api/v1/documents", sandboxReq)
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))

		var doc entity.DocumentWithRecipients
		require.NoError(t, json.Unmarshal(body, &doc))
		t.Cleanup(func() { testhelper.CleanupDocument(t, testhelper.GetTestPool(t), doc.ID) })

		var workspaceID string
		require.NoError(t, testhelper.GetTestPool(t).QueryRow(context.Background(),
			`SELECT workspace_id FROM execution.documents WHERE id = $1`, doc.ID,
		).Scan(&workspaceID))
		assert.Equal(t, sbx.workspaceID, workspaceID)
		assert.NotEqual(t, env.workspaceID, workspaceID)
	})

	t.Run("list in sandbox excludes prod docs", func(t *testing.T) {
		resp, body := sandboxViewer.GET("/api/v1/documents")
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var docs []*entity.DocumentListItem
		require.NoError(t, json.Unmarshal(body, &docs))
		for _, d := range docs {
			assert.NotEqual(t, prodDoc.ID, d.ID, "prod doc must not appear in sandbox list")
		}
	})

	t.Run("list in production excludes sandbox docs", func(t *testing.T) {
		createResp, createBody := sandboxOperator.POST("/api/v1/documents", sandboxReq)
		require.Equal(t, http.StatusCreated, createResp.StatusCode, string(createBody))
		var sbxDoc entity.DocumentWithRecipients
		require.NoError(t, json.Unmarshal(createBody, &sbxDoc))
		t.Cleanup(func() { testhelper.CleanupDocument(t, testhelper.GetTestPool(t), sbxDoc.ID) })

		resp, body := env.viewerClient().GET("/api/v1/documents")
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var docs []*entity.DocumentListItem
		require.NoError(t, json.Unmarshal(body, &docs))
		for _, d := range docs {
			assert.NotEqual(t, sbxDoc.ID, d.ID, "sandbox doc must not appear in prod list")
		}
	})

	t.Run("each mode reads only its own documents by ID", func(t *testing.T) {
		createResp, createBody := sandboxOperator.POST("/api/v1/documents", sandboxReq)
		require.Equal(t, http.StatusCreated, createResp.StatusCode, string(createBody))
		var sbxDoc entity.DocumentWithRecipients
		require.NoError(t, json.Unmarshal(createBody, &sbxDoc))
		t.Cleanup(func() { testhelper.CleanupDocument(t, testhelper.GetTestPool(t), sbxDoc.ID) })

		resp, body := sandboxViewer.GET("/api/v1/documents/" + sbxDoc.ID)
		assert.Equal(t, http.StatusOK, resp.StatusCode, string(body))

		resp, body = sandboxViewer.GET("/api/v1/documents/" + prodDoc.ID)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "prod doc must not be readable in sandbox mode: %s", string(body))

		resp, body = env.viewerClient().GET("/api/v1/documents/" + sbxDoc.ID)
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, "sandbox doc must not be readable in production mode: %s", string(body))
	})
}

// TestDocumentController_DocumentFromAnotherWorkspace_NotFound covers
// documentInWorkspace: an operator who is also a member of a second workspace in
// the same tenant cannot read or act on the first workspace's document by its ID,
// through any route that names a document. Each route answers exactly what it
// answers for a document that does not exist, so the ID cannot be probed.
func TestDocumentController_DocumentFromAnotherWorkspace_NotFound(t *testing.T) {
	env := setupDocumentEnv(t)
	pool := testhelper.GetTestPool(t)
	doc := env.createDocument(t, "Workspace Scope Doc")
	recipientID := doc.Recipients[0].ID

	// A live signing link, so the signing-url route has something to leak: from
	// the document's own workspace it answers 200 with the link.
	signingURL := "https://sign.example.test/" + doc.ID
	giveRecipientSigningURL(t, doc.ID, recipientID, signingURL)
	resp, body := env.operatorClient().GET("/api/v1/documents/" + doc.ID + "/recipients/" + recipientID + "/signing-url")
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	require.Contains(t, string(body), signingURL)

	otherWorkspaceID := testhelper.CreateTestWorkspace(t, pool, &env.tenantID, "Other Workspace", entity.WorkspaceTypeClient)
	t.Cleanup(func() { testhelper.CleanupWorkspace(t, pool, otherWorkspaceID) })
	testhelper.CreateTestWorkspaceMember(t, pool, otherWorkspaceID, env.operator.ID, entity.WorkspaceRoleOperator, nil)
	fromOtherWorkspace := env.client.WithAuth(env.operator.BearerHeader).WithWorkspaceID(otherWorkspaceID)
	missingDocumentID := uuid.NewString()

	routes := []struct{ method, path string }{
		{http.MethodGet, ""},
		{http.MethodGet, "/recipients"},
		{http.MethodGet, "/events"},
		{http.MethodGet, "/recipients/" + recipientID + "/signing-url"},
		{http.MethodGet, "/pdf"},
		{http.MethodPost, "/view-link"},
		{http.MethodGet, "/print-pdf"},
		{http.MethodPost, "/refresh"},
		{http.MethodPost, "/cancel"},
		{http.MethodPost, "/deprecate"},
		{http.MethodPost, "/remind"},
		{http.MethodPost, "/invalidate-tokens"},
	}
	for _, route := range routes {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			resp, body := fromOtherWorkspace.Do(route.method, "/api/v1/documents/"+doc.ID+route.path, nil)
			assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))

			missingResp, missingBody := fromOtherWorkspace.Do(route.method, "/api/v1/documents/"+missingDocumentID+route.path, nil)
			assert.Equal(t, missingResp.StatusCode, resp.StatusCode, "status differs from a missing document's")
			assert.Equal(t, string(missingBody), string(body), "body differs from a missing document's")
		})
	}

	t.Run("the document is untouched and still readable from its own workspace", func(t *testing.T) {
		resp, body := env.viewerClient().GET("/api/v1/documents/" + doc.ID)
		require.Equal(t, http.StatusOK, resp.StatusCode, string(body))

		var got entity.DocumentWithRecipients
		require.NoError(t, json.Unmarshal(body, &got))
		assert.Equal(t, doc.Status, got.Status, "the cancel attempted from the other workspace must not have run")
	})

	t.Run("an ID that is not a UUID answers 404", func(t *testing.T) {
		resp, body := env.viewerClient().GET("/api/v1/documents/not-a-uuid")
		assert.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
	})
}

func TestDocumentController_Sandbox_BadRequestForSystemWorkspace(t *testing.T) {
	env := setupDocumentEnv(t)
	pool := testhelper.GetTestPool(t)

	var systemWorkspaceID string
	err := pool.QueryRow(
		context.Background(),
		`SELECT id FROM tenancy.workspaces WHERE tenant_id = $1 AND type = 'SYSTEM' AND is_sandbox = FALSE LIMIT 1`,
		env.tenantID,
	).Scan(&systemWorkspaceID)
	require.NoError(t, err)

	testhelper.CreateTestWorkspaceMember(t, pool, systemWorkspaceID, env.viewer.ID, entity.WorkspaceRoleViewer, nil)

	resp, body := env.client.
		WithAuth(env.viewer.BearerHeader).
		WithWorkspaceID(systemWorkspaceID).
		WithHeader(middleware.SandboxModeHeader, "true").
		GET("/api/v1/documents")

	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))

	var errorResp map[string]string
	require.NoError(t, json.Unmarshal(body, &errorResp))
	assert.Equal(t, entity.ErrSandboxNotSupported.Error(), errorResp["error"])
}
