package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/infra/config"
)

func TestCORSMiddleware_IncludesSDKBaseHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(corsMiddleware(config.CORSConfig{
		AllowedOrigins: []string{"http://localhost:5173"},
	}))
	router.OPTIONS("/cors-test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodOptions, "/cors-test", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "authorization,x-environment,x-automation-key,x-operation-id,content-type")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "http://localhost:5173", rec.Header().Get("Access-Control-Allow-Origin"))

	allowHeaders := rec.Header().Get("Access-Control-Allow-Headers")
	assertHeaderAllowed(t, allowHeaders, "X-Environment")
	assertHeaderAllowed(t, allowHeaders, "X-Automation-Key")
	assertHeaderAllowed(t, allowHeaders, "X-Operation-ID")
}

func TestCORSMiddleware_AppendsConfiguredHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(corsMiddleware(config.CORSConfig{
		AllowedOrigins: []string{"http://localhost:5173"},
		AllowedHeaders: []string{"X-Custom-Header"},
	}))
	router.OPTIONS("/cors-test", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodOptions, "/cors-test", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNoContent, rec.Code)

	allowHeaders := rec.Header().Get("Access-Control-Allow-Headers")
	assertHeaderAllowed(t, allowHeaders, "X-Custom-Header")
	assertHeaderAllowed(t, allowHeaders, "X-Environment")
}

func assertHeaderAllowed(t *testing.T, allowHeaders string, expected string) {
	t.Helper()

	for _, header := range strings.Split(allowHeaders, ",") {
		if strings.EqualFold(strings.TrimSpace(header), expected) {
			return
		}
	}
	t.Fatalf("expected header %q in Access-Control-Allow-Headers: %s", expected, allowHeaders)
}
