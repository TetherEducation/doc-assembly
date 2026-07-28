package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	"github.com/TetherEducation/doc-assembly/core/internal/infra/config"
)

type serverLegacyDocumentHandler struct {
	called bool
}

func (h *serverLegacyDocumentHandler) HandleLegacyDocument(
	context.Context,
	*port.LegacyDocumentRequest,
) (*port.LegacyDocumentResponse, error) {
	h.called = true
	return &port.LegacyDocumentResponse{StatusCode: http.StatusOK, Body: map[string]any{"ok": true}}, nil
}

func TestHTTPServer_DoesNotMountLegacyDocumentRouteWithoutHandler(t *testing.T) {
	router := legacyDocumentServerTestRouter(nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", strings.NewReader(`{}`))
	req.Header.Set("X-Workspace-Code", "CAMPUS_1")
	req.Header.Set("X-Environment", "prod")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHTTPServer_MountsLegacyDocumentRouteWithHandler(t *testing.T) {
	handler := &serverLegacyDocumentHandler{}
	router := legacyDocumentServerTestRouter(handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", strings.NewReader(`{}`))
	req.Header.Set("X-Workspace-Code", "CAMPUS_1")
	req.Header.Set("X-Environment", "prod")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.True(t, handler.called)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"ok":true}`, rec.Body.String())
}

func legacyDocumentServerTestRouter(handler port.LegacyDocumentHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerLegacyDocumentRoute(router.Group(""), minimalLegacyDocumentServerConfig(), 28*time.Second, handler)
	return router
}

func minimalLegacyDocumentServerConfig() *config.Config {
	return &config.Config{
		Environment: "development",
		Server: config.ServerConfig{
			WriteTimeout: 30,
			CORS: config.CORSConfig{
				AllowedOrigins: []string{"*"},
			},
		},
		LegacyDocuments: config.LegacyDocumentsConfig{
			MaxBodyBytes: 65536,
		},
	}
}
