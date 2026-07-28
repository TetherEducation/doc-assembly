package controller

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

type fakeLegacyDocumentHandler struct {
	req  *port.LegacyDocumentRequest
	resp *port.LegacyDocumentResponse
	err  error
}

func (f *fakeLegacyDocumentHandler) HandleLegacyDocument(
	_ context.Context,
	req *port.LegacyDocumentRequest,
) (*port.LegacyDocumentResponse, error) {
	f.req = req
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func legacyRouter(handler port.LegacyDocumentHandler, maxBodyBytes int64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1")
	group.Use(middleware.Operation())
	NewLegacyDocumentController(handler, maxBodyBytes).RegisterRoutes(group)
	return router
}

func TestLegacyDocumentController_DoesNotRegisterRouteWithoutDependencies(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("nil controller", func(t *testing.T) {
		router := gin.New()
		group := router.Group("/api/v1")
		var controller *LegacyDocumentController
		controller.RegisterRoutes(group)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("nil handler", func(t *testing.T) {
		router := legacyRouter(nil, 65536)

		req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusNotFound, rec.Code)
	})

	t.Run("nil group", func(t *testing.T) {
		controller := NewLegacyDocumentController(&fakeLegacyDocumentHandler{}, 65536)

		require.NotPanics(t, func() {
			controller.RegisterRoutes(nil)
		})
	})
}

func TestLegacyDocumentController_RejectsMissingWorkspaceCode(t *testing.T) {
	handler := &fakeLegacyDocumentHandler{}
	router := legacyRouter(handler, 65536)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", strings.NewReader(`{}`))
	req.Header.Set(HeaderEnvironment, "prod")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.JSONEq(t, `{"error":"missing X-Workspace-Code"}`, rec.Body.String())
	assert.Nil(t, handler.req)
}

func TestLegacyDocumentController_RejectsMissingEnvironment(t *testing.T) {
	handler := &fakeLegacyDocumentHandler{}
	router := legacyRouter(handler, 65536)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", strings.NewReader(`{}`))
	req.Header.Set(HeaderWorkspaceCode, "CAMPUS_1")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.JSONEq(t, `{"error":"missing X-Environment"}`, rec.Body.String())
	assert.Nil(t, handler.req)
}

func TestLegacyDocumentController_RejectsInvalidEnvironment(t *testing.T) {
	handler := &fakeLegacyDocumentHandler{}
	router := legacyRouter(handler, 65536)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", strings.NewReader(`{}`))
	req.Header.Set(HeaderWorkspaceCode, "CAMPUS_1")
	req.Header.Set(HeaderEnvironment, "invalid")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.JSONEq(t, `{"error":"invalid X-Environment header value (must be 'dev' or 'prod')"}`, rec.Body.String())
	assert.Nil(t, handler.req)
}

func TestLegacyDocumentController_RejectsNonCanonicalEnvironments(t *testing.T) {
	for _, value := range []string{"staging", "production", "DEV", "Prod"} {
		t.Run(value, func(t *testing.T) {
			handler := &fakeLegacyDocumentHandler{}
			router := legacyRouter(handler, 65536)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", strings.NewReader(`{}`))
			req.Header.Set(HeaderWorkspaceCode, "CAMPUS_1")
			req.Header.Set(HeaderEnvironment, value)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.JSONEq(t, `{"error":"invalid X-Environment header value (must be 'dev' or 'prod')"}`, rec.Body.String())
			assert.Nil(t, handler.req)
		})
	}
}

func TestLegacyDocumentController_RejectsBodyOverLimit(t *testing.T) {
	handler := &fakeLegacyDocumentHandler{}
	router := legacyRouter(handler, 8)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", strings.NewReader(`{"too":"large"}`))
	req.Header.Set(HeaderWorkspaceCode, "CAMPUS_1")
	req.Header.Set(HeaderEnvironment, "prod")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.JSONEq(t, `{"error":"request body too large"}`, rec.Body.String())
	assert.Nil(t, handler.req)
}

func TestLegacyDocumentController_PassesRequestToHandler(t *testing.T) {
	handler := &fakeLegacyDocumentHandler{
		resp: &port.LegacyDocumentResponse{StatusCode: http.StatusAccepted, Body: map[string]any{"ok": true}},
	}
	router := legacyRouter(handler, 65536)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", strings.NewReader(`{"documentId":"legacy-1"}`))
	req.Header.Set(HeaderWorkspaceCode, " CAMPUS_1 ")
	req.Header.Set(HeaderEnvironment, "dev")
	req.Header.Add("X-Legacy-Token", "a")
	req.Header.Add("X-Legacy-Token", "b")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.NotNil(t, handler.req)
	assert.Equal(t, "CAMPUS_1", handler.req.WorkspaceCode)
	assert.Equal(t, entity.EnvironmentDev, handler.req.Environment)
	assert.Equal(t, []byte(`{"documentId":"legacy-1"}`), handler.req.RawBody)
	assert.Equal(t, []string{"a", "b"}, handler.req.Headers["X-Legacy-Token"])
	assert.Equal(t, http.StatusAccepted, rec.Code)
	assert.JSONEq(t, `{"ok":true}`, rec.Body.String())
	assert.NotEmpty(t, rec.Header().Get(middleware.OperationIDHeader))
}

func TestLegacyDocumentController_MethodNotAllowed(t *testing.T) {
	handler := &fakeLegacyDocumentHandler{}
	router := legacyRouter(handler, 65536)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/legacy-documents/proxy", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusMethodNotAllowed, rec.Code)
	assert.Equal(t, http.MethodPost, rec.Header().Get("Allow"))
	assert.JSONEq(t, `{"error":"method not allowed"}`, rec.Body.String())
	assert.Nil(t, handler.req)
}

func TestLegacyDocumentController_SerializesJSONBodyVariants(t *testing.T) {
	cases := []struct {
		name string
		body any
		want string
	}{
		{name: "object", body: map[string]any{"url": "https://example.test/doc"}, want: `{"url":"https://example.test/doc"}`},
		{name: "struct", body: struct {
			Token string `json:"token"`
		}{Token: "abc"}, want: `{"token":"abc"}`},
		{name: "pointer", body: &struct {
			Token string `json:"token"`
		}{Token: "abc"}, want: `{"token":"abc"}`},
		{name: "string", body: "ready", want: `"ready"`},
		{name: "number", body: 42, want: `42`},
		{name: "bool", body: true, want: `true`},
		{name: "nil", body: nil, want: `null`},
		{name: "array", body: []any{"a", 1, false}, want: `["a",1,false]`},
		{name: "raw", body: json.RawMessage(`{"raw":true}`), want: `{"raw":true}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := &fakeLegacyDocumentHandler{
				resp: &port.LegacyDocumentResponse{StatusCode: http.StatusOK, Body: tc.body},
			}
			router := legacyRouter(handler, 65536)

			req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", nil)
			req.Header.Set(HeaderWorkspaceCode, "CAMPUS_1")
			req.Header.Set(HeaderEnvironment, "prod")
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
			assert.JSONEq(t, tc.want, rec.Body.String())
		})
	}
}

func TestLegacyDocumentController_UsesDefaultStatusForInvalidHandlerStatus(t *testing.T) {
	handler := &fakeLegacyDocumentHandler{
		resp: &port.LegacyDocumentResponse{StatusCode: 99, Body: map[string]any{"ok": true}},
	}
	router := legacyRouter(handler, 65536)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", nil)
	req.Header.Set(HeaderWorkspaceCode, "CAMPUS_1")
	req.Header.Set(HeaderEnvironment, "prod")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"ok":true}`, rec.Body.String())
}

func TestLegacyDocumentController_CopiesAllowedResponseHeaders(t *testing.T) {
	handler := &fakeLegacyDocumentHandler{
		resp: &port.LegacyDocumentResponse{
			StatusCode: http.StatusOK,
			Headers: map[string][]string{
				"X-Legacy-Provider": {"pandadoc"},
				"Location":          {"https://example.test/legacy"},
			},
			Body: map[string]any{"ok": true},
		},
	}
	router := legacyRouter(handler, 65536)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", nil)
	req.Header.Set(HeaderWorkspaceCode, "CAMPUS_1")
	req.Header.Set(HeaderEnvironment, "prod")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, "pandadoc", rec.Header().Get("X-Legacy-Provider"))
	assert.Equal(t, "https://example.test/legacy", rec.Header().Get("Location"))
}

func TestLegacyDocumentController_DoesNotOverwriteSystemHeaders(t *testing.T) {
	handler := &fakeLegacyDocumentHandler{
		resp: &port.LegacyDocumentResponse{
			StatusCode: http.StatusOK,
			Headers: map[string][]string{
				"Content-Type":                {"text/plain"},
				"Content-Length":              {"999"},
				"Content-Encoding":            {"gzip"},
				"X-Operation-ID":              {"malicious"},
				"Access-Control-Allow-Origin": {"https://evil.test"},
				"Vary":                        {"SomethingElse"},
				"Cache-Control":               {"public"},
				"Pragma":                      {"cache"},
				"Expires":                     {"tomorrow"},
				"Set-Cookie":                  {"session=evil"},
				"Transfer-Encoding":           {"chunked"},
				"Connection":                  {"upgrade"},
				"Upgrade":                     {"websocket"},
				"Trailer":                     {"X-Trailer"},
				"TE":                          {"trailers"},
				"Keep-Alive":                  {"timeout=5"},
			},
			Body: map[string]any{"ok": true},
		},
	}
	router := legacyRouter(handler, 65536)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", nil)
	req.Header.Set(HeaderWorkspaceCode, "CAMPUS_1")
	req.Header.Set(HeaderEnvironment, "prod")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	assert.NotEqual(t, "malicious", rec.Header().Get("X-Operation-ID"))
	assert.Empty(t, rec.Header().Values("Set-Cookie"))
	assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	assert.Empty(t, rec.Header().Get("Vary"))
	assert.Empty(t, rec.Header().Get("Cache-Control"))
	assert.Empty(t, rec.Header().Get("Pragma"))
	assert.Empty(t, rec.Header().Get("Expires"))
	assert.Empty(t, rec.Header().Get("Content-Length"))
	assert.Empty(t, rec.Header().Get("Content-Encoding"))
	assert.Empty(t, rec.Header().Get("Transfer-Encoding"))
	assert.Empty(t, rec.Header().Get("Connection"))
	assert.Empty(t, rec.Header().Get("Upgrade"))
	assert.Empty(t, rec.Header().Get("Trailer"))
	assert.Empty(t, rec.Header().Get("TE"))
	assert.Empty(t, rec.Header().Get("Keep-Alive"))
}

func TestLegacyDocumentController_HandlerErrorReturnsInternalServerError(t *testing.T) {
	handler := &fakeLegacyDocumentHandler{err: errors.New("legacy backend unavailable")}
	router := legacyRouter(handler, 65536)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", nil)
	req.Header.Set(HeaderWorkspaceCode, "CAMPUS_1")
	req.Header.Set(HeaderEnvironment, "prod")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.JSONEq(t, `{"error":"legacy document handler failed"}`, rec.Body.String())
}

func TestLegacyDocumentController_NilHandlerResponseReturnsInternalServerError(t *testing.T) {
	handler := &fakeLegacyDocumentHandler{}
	router := legacyRouter(handler, 65536)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", nil)
	req.Header.Set(HeaderWorkspaceCode, "CAMPUS_1")
	req.Header.Set(HeaderEnvironment, "prod")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.JSONEq(t, `{"error":"legacy document handler returned nil response"}`, rec.Body.String())
}

func TestLegacyDocumentController_UnserializableBodyReturnsInternalServerError(t *testing.T) {
	handler := &fakeLegacyDocumentHandler{
		resp: &port.LegacyDocumentResponse{
			StatusCode: http.StatusOK,
			Body:       map[string]any{"bad": func() {}},
		},
	}
	router := legacyRouter(handler, 65536)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", nil)
	req.Header.Set(HeaderWorkspaceCode, "CAMPUS_1")
	req.Header.Set(HeaderEnvironment, "prod")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.JSONEq(t, `{"error":"legacy document response serialization failed"}`, rec.Body.String())
}
