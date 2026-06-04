package controller_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/controller"
	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
)

type readOnlyViewUCStub struct {
	createCalledWith     string
	createWorkspace      string
	createCodeCalledWith string
	createWorkspaceCode  string
	viewCalledWith       string
	pdfCalledWith        string

	createResult *documentuc.CreateReadOnlyViewLinkResult
	viewResult   *documentuc.ReadOnlyViewResponse
	pdfBytes     []byte
	pdfFilename  string
	err          error

	createCodeCalls    []string
	errByWorkspaceCode map[string]error
}

func (s *readOnlyViewUCStub) CreateReadOnlyViewLink(_ context.Context, workspaceID, documentID string) (*documentuc.CreateReadOnlyViewLinkResult, error) {
	s.createWorkspace = workspaceID
	s.createCalledWith = documentID
	if s.err != nil {
		return nil, s.err
	}
	return s.createResult, nil
}

func (s *readOnlyViewUCStub) CreateReadOnlyViewLinkByWorkspaceCode(_ context.Context, workspaceCode, documentID string) (*documentuc.CreateReadOnlyViewLinkResult, error) {
	s.createWorkspaceCode = workspaceCode
	s.createCodeCalledWith = documentID
	s.createCodeCalls = append(s.createCodeCalls, workspaceCode)
	if s.errByWorkspaceCode != nil {
		if err, ok := s.errByWorkspaceCode[workspaceCode]; ok {
			return nil, err
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	return s.createResult, nil
}

func (s *readOnlyViewUCStub) GetReadOnlyView(_ context.Context, token string) (*documentuc.ReadOnlyViewResponse, error) {
	s.viewCalledWith = token
	if s.err != nil {
		return nil, s.err
	}
	return s.viewResult, nil
}

func (s *readOnlyViewUCStub) GetReadOnlyViewPDF(_ context.Context, token string) ([]byte, string, error) {
	s.pdfCalledWith = token
	if s.err != nil {
		return nil, "", s.err
	}
	return s.pdfBytes, s.pdfFilename, nil
}

type readOnlyViewLinkAuthStub struct {
	claims *port.ReadOnlyViewLinkAuthClaims
	err    error
}

func (s *readOnlyViewLinkAuthStub) Authenticate(_ *gin.Context, _ *port.ReadOnlyViewLinkAuthenticateRequest) (*port.ReadOnlyViewLinkAuthClaims, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.claims, nil
}

func TestDocumentController_CreateReadOnlyViewLink(t *testing.T) {
	gin.SetMode(gin.TestMode)
	expiresAt := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

	t.Run("viewer allowed", func(t *testing.T) {
		uc := &readOnlyViewUCStub{createResult: &documentuc.CreateReadOnlyViewLinkResult{
			URL:       "https://example.test/public/view/view-token",
			Token:     "view-token",
			ExpiresAt: expiresAt,
		}}
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("workspace_id", "workspace-1")
			c.Set("workspace_role", entity.WorkspaceRoleViewer)
			c.Next()
		})
		controller.NewDocumentController(nil, nil, uc, nil).RegisterRoutes(router.Group("/api/v1"))

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/doc-1/view-link", nil)
		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		assert.Equal(t, "doc-1", uc.createCalledWith)
		assert.Equal(t, "workspace-1", uc.createWorkspace)

		var body struct {
			URL       string    `json:"url"`
			Token     string    `json:"token"`
			ExpiresAt time.Time `json:"expiresAt"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		assert.Equal(t, "https://example.test/public/view/view-token", body.URL)
		assert.Equal(t, "view-token", body.Token)
		assert.Equal(t, expiresAt, body.ExpiresAt)
	})

	t.Run("non viewer forbidden", func(t *testing.T) {
		uc := &readOnlyViewUCStub{createResult: &documentuc.CreateReadOnlyViewLinkResult{}}
		router := gin.New()
		controller.NewDocumentController(nil, nil, uc, nil).RegisterRoutes(router.Group("/api/v1"))

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/doc-1/view-link", nil)
		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
		assert.Empty(t, uc.createCalledWith)
	})

	t.Run("missing workspace rejected before use case", func(t *testing.T) {
		uc := &readOnlyViewUCStub{createResult: &documentuc.CreateReadOnlyViewLinkResult{}}
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set("workspace_role", entity.WorkspaceRoleViewer)
			c.Next()
		})
		controller.NewDocumentController(nil, nil, uc, nil).RegisterRoutes(router.Group("/api/v1"))

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/doc-1/view-link", nil)
		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
		assert.Empty(t, uc.createCalledWith)
		assert.Empty(t, uc.createWorkspace)
	})

	t.Run("external auth route uses workspace code header", func(t *testing.T) {
		uc := &readOnlyViewUCStub{createResult: &documentuc.CreateReadOnlyViewLinkResult{
			URL:       "https://example.test/public/view/view-token",
			Token:     "view-token",
			ExpiresAt: expiresAt,
		}}
		router := gin.New()
		router.POST("/api/v1/documents/:documentId/view-link", controller.NewDocumentController(nil, nil, uc, nil).CreateReadOnlyViewLinkByWorkspaceCode)

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/doc-1/view-link", nil)
		req.Header.Set("X-Workspace-Code", "workspace-code-from-header")
		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		assert.Equal(t, "doc-1", uc.createCodeCalledWith)
		assert.Equal(t, "workspace-code-from-header", uc.createWorkspaceCode)
		assert.Empty(t, uc.createCalledWith)
		assert.Empty(t, uc.createWorkspace)
	})

	t.Run("external auth route uses authorized workspace codes after header mismatch", func(t *testing.T) {
		uc := &readOnlyViewUCStub{
			createResult: &documentuc.CreateReadOnlyViewLinkResult{
				URL:       "https://example.test/public/view/view-token",
				Token:     "view-token",
				ExpiresAt: expiresAt,
			},
			errByWorkspaceCode: map[string]error{
				"2518500001": entity.ErrForbidden,
			},
		}
		auth := &readOnlyViewLinkAuthStub{
			claims: &port.ReadOnlyViewLinkAuthClaims{
				Email:                    "admin@example.test",
				Subject:                  "subject-1",
				Provider:                 "test",
				AuthorizedWorkspaceCodes: []string{"SAN_VICENTE_DE_PAUL", "DEFAULT"},
			},
		}
		router := gin.New()
		router.POST(
			"/api/v1/documents/:documentId/view-link",
			middleware.ReadOnlyViewLinkCustomAuth(auth),
			controller.NewDocumentController(nil, nil, uc, nil).CreateReadOnlyViewLinkByWorkspaceCode,
		)

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/doc-1/view-link", nil)
		req.Header.Set("X-Workspace-Code", "2518500001")
		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		assert.Equal(t, []string{"2518500001", "SAN_VICENTE_DE_PAUL"}, uc.createCodeCalls)
		assert.Equal(t, "SAN_VICENTE_DE_PAUL", uc.createWorkspaceCode)
	})

	t.Run("external auth route returns forbidden when authorized workspace codes do not match", func(t *testing.T) {
		uc := &readOnlyViewUCStub{
			createResult: &documentuc.CreateReadOnlyViewLinkResult{},
			errByWorkspaceCode: map[string]error{
				"2518500001":          entity.ErrForbidden,
				"SAN_VICENTE_DE_PAUL": entity.ErrForbidden,
			},
		}
		auth := &readOnlyViewLinkAuthStub{
			claims: &port.ReadOnlyViewLinkAuthClaims{
				Email:                    "admin@example.test",
				Subject:                  "subject-1",
				Provider:                 "test",
				AuthorizedWorkspaceCodes: []string{"SAN_VICENTE_DE_PAUL"},
			},
		}
		router := gin.New()
		router.POST(
			"/api/v1/documents/:documentId/view-link",
			middleware.ReadOnlyViewLinkCustomAuth(auth),
			controller.NewDocumentController(nil, nil, uc, nil).CreateReadOnlyViewLinkByWorkspaceCode,
		)

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/doc-1/view-link", nil)
		req.Header.Set("X-Workspace-Code", "2518500001")
		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
		assert.Equal(t, []string{"2518500001", "SAN_VICENTE_DE_PAUL"}, uc.createCodeCalls)
	})

	t.Run("external auth route rejects missing workspace code header", func(t *testing.T) {
		uc := &readOnlyViewUCStub{createResult: &documentuc.CreateReadOnlyViewLinkResult{}}
		router := gin.New()
		router.POST("/api/v1/documents/:documentId/view-link", controller.NewDocumentController(nil, nil, uc, nil).CreateReadOnlyViewLinkByWorkspaceCode)

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/doc-1/view-link", nil)
		req.Header.Set("X-Workspace-ID", "workspace-id-must-not-be-used")
		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
		assert.Empty(t, uc.createCodeCalledWith)
		assert.Empty(t, uc.createWorkspaceCode)
	})
}

func TestPublicReadOnlyViewController(t *testing.T) {
	gin.SetMode(gin.TestMode)
	expiresAt := time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC)

	t.Run("returns metadata", func(t *testing.T) {
		pdfURL := "/public/view/view-token/pdf"
		uc := &readOnlyViewUCStub{viewResult: &documentuc.ReadOnlyViewResponse{
			Mode:           documentuc.ReadOnlyViewModePDF,
			DocumentID:     "doc-1",
			DocumentTitle:  "Contract",
			DocumentStatus: entity.DocumentStatusCompleted,
			ExpiresAt:      expiresAt,
			PDFURL:         &pdfURL,
		}}
		router := gin.New()
		controller.NewPublicReadOnlyViewController(uc).RegisterRoutes(router)

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/public/view/view-token", nil)
		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		assert.Equal(t, "view-token", uc.viewCalledWith)

		var body struct {
			Mode           string    `json:"mode"`
			DocumentID     string    `json:"documentId"`
			DocumentTitle  string    `json:"documentTitle"`
			DocumentStatus string    `json:"documentStatus"`
			ExpiresAt      time.Time `json:"expiresAt"`
			PDFURL         string    `json:"pdfUrl"`
		}
		require.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &body))
		assert.Equal(t, "pdf", body.Mode)
		assert.Equal(t, "doc-1", body.DocumentID)
		assert.Equal(t, "Contract", body.DocumentTitle)
		assert.Equal(t, string(entity.DocumentStatusCompleted), body.DocumentStatus)
		assert.Equal(t, expiresAt, body.ExpiresAt)
		assert.Equal(t, pdfURL, body.PDFURL)
	})

	t.Run("serves PDF inline", func(t *testing.T) {
		uc := &readOnlyViewUCStub{pdfBytes: []byte("%PDF-1.7"), pdfFilename: "contract.pdf"}
		router := gin.New()
		controller.NewPublicReadOnlyViewController(uc).RegisterRoutes(router)

		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/public/view/view-token/pdf", nil)
		router.ServeHTTP(recorder, req)

		require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
		assert.Equal(t, "view-token", uc.pdfCalledWith)
		assert.Equal(t, "application/pdf", recorder.Header().Get("Content-Type"))
		assert.Equal(t, `inline; filename="contract.pdf"`, recorder.Header().Get("Content-Disposition"))
		assert.True(t, strings.HasPrefix(recorder.Body.String(), "%PDF"))
	})
}
