package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublicPageEntrySPAMiddleware_ServesHTMLForSigningEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(publicPageEntrySPAMiddleware(testFrontendFS(), "/doc-assembly"))
	router.GET("/doc-assembly/public/sign/:token", func(c *gin.Context) {
		c.Header("X-Handler", "json")
		c.JSON(http.StatusOK, gin.H{"step": "preview"})
	})

	req := httptest.NewRequest(http.MethodGet, "/doc-assembly/public/sign/token-1", nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	assert.NotEqual(t, "json", rec.Header().Get("X-Handler"))
	assert.Contains(t, rec.Body.String(), "<div id=\"app\"></div>")
}

func TestPublicPageEntrySPAMiddleware_PreservesJSONForSigningEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(publicPageEntrySPAMiddleware(testFrontendFS(), "/doc-assembly"))
	router.GET("/doc-assembly/public/sign/:token", func(c *gin.Context) {
		c.Header("X-Handler", "json")
		c.JSON(http.StatusOK, gin.H{"step": "preview"})
	})

	req := httptest.NewRequest(http.MethodGet, "/doc-assembly/public/sign/token-1", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "json", rec.Header().Get("X-Handler"))
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	assert.JSONEq(t, `{"step":"preview"}`, rec.Body.String())
}

func TestPublicPageEntrySPAMiddleware_ServesHTMLForDocumentEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(publicPageEntrySPAMiddleware(testFrontendFS(), "/doc-assembly"))
	router.GET("/doc-assembly/public/doc/:documentId", func(c *gin.Context) {
		c.Header("X-Handler", "json")
		c.JSON(http.StatusOK, gin.H{"documentTitle": "doc"})
	})

	req := httptest.NewRequest(http.MethodGet, "/doc-assembly/public/doc/doc-1", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	assert.NotEqual(t, "json", rec.Header().Get("X-Handler"))
	assert.Contains(t, rec.Body.String(), "<div id=\"app\"></div>")
}

func TestPublicPageEntrySPAMiddleware_ServesHTMLForReadOnlyViewEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(publicPageEntrySPAMiddleware(testFrontendFS(), "/doc-assembly"))
	router.GET("/doc-assembly/public/view/:token", func(c *gin.Context) {
		c.Header("X-Handler", "json")
		c.JSON(http.StatusOK, gin.H{"mode": "content"})
	})

	req := httptest.NewRequest(http.MethodGet, "/doc-assembly/public/view/token-1", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	assert.NotEqual(t, "json", rec.Header().Get("X-Handler"))
	assert.Contains(t, rec.Body.String(), "<div id=\"app\"></div>")
}

func TestPublicPageEntrySPAMiddleware_PreservesJSONForReadOnlyViewEntry(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(publicPageEntrySPAMiddleware(testFrontendFS(), "/doc-assembly"))
	router.GET("/doc-assembly/public/view/:token", func(c *gin.Context) {
		c.Header("X-Handler", "json")
		c.JSON(http.StatusOK, gin.H{"mode": "content"})
	})

	req := httptest.NewRequest(http.MethodGet, "/doc-assembly/public/view/token-1", nil)
	req.Header.Set("Accept", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "json", rec.Header().Get("X-Handler"))
	assert.Contains(t, rec.Header().Get("Content-Type"), "application/json")
	assert.JSONEq(t, `{"mode":"content"}`, rec.Body.String())
}

func TestSigningCSPMiddleware_UsesConfiguredFrameAncestors(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(signingCSPMiddleware(
		"https://sign.tether.education/",
		[]string{
			"https://applications.tether.education/",
			"http://localhost:5173/",
			"https://applications.tether.education",
			"invalid-origin",
		},
	))
	router.GET("/doc-assembly/public/sign/:token", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/doc-assembly/public/sign/token-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	csp := rec.Header().Get("Content-Security-Policy")
	assert.Equal(
		t,
		"frame-src https://sign.tether.education; frame-ancestors https://applications.tether.education http://localhost:5173",
		csp,
	)
}

func TestSigningCSPMiddleware_DefaultsFrameAncestorsToSelf(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(signingCSPMiddleware("https://sign.tether.education", nil))
	router.GET("/public/sign/:token", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/public/sign/token-1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(
		t,
		"frame-src https://sign.tether.education; frame-ancestors 'self'",
		rec.Header().Get("Content-Security-Policy"),
	)
}

func testFrontendFS() fs.FS {
	return fstest.MapFS{
		"index.html": {
			Data: []byte(`<!doctype html><html><body><div id="app"></div></body></html>`),
		},
	}
}

func TestRequestWantsHTML(t *testing.T) {
	assert.True(t, requestWantsHTML("text/html,application/xhtml+xml"))
	assert.True(t, requestWantsHTML("application/json,text/html;q=0.9"))
	assert.False(t, requestWantsHTML("application/json, text/plain, */*"))
	assert.False(t, requestWantsHTML(strings.Repeat(" ", 3)))
}
