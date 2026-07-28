package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

type fakeReadOnlyViewLinkAuth struct {
	req    *port.ReadOnlyViewLinkAuthenticateRequest
	claims *port.ReadOnlyViewLinkAuthClaims
	err    error
}

func (f *fakeReadOnlyViewLinkAuth) Authenticate(_ *gin.Context, req *port.ReadOnlyViewLinkAuthenticateRequest) (*port.ReadOnlyViewLinkAuthClaims, error) {
	f.req = req
	if f.err != nil {
		return nil, f.err
	}
	return f.claims, nil
}

func TestReadOnlyViewLinkCustomAuthPassesRequestContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth := &fakeReadOnlyViewLinkAuth{claims: &port.ReadOnlyViewLinkAuthClaims{
		Email:    "alice@example.test",
		Subject:  "subject-1",
		Provider: "keycloak",
	}}
	router := gin.New()
	router.POST(
		"/api/v1/documents/:documentId/view-link",
		ReadOnlyViewLinkCustomAuth(auth),
		func(c *gin.Context) {
			claims, ok := GetReadOnlyViewLinkAuthClaims(c)
			require.True(t, ok)
			assert.Equal(t, "alice@example.test", claims.Email)
			c.Status(http.StatusNoContent)
		},
	)

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/doc-1/view-link", nil)
	req.Header.Set("X-Workspace-Code", "campus-code-1")
	req.Header.Set("X-Environment", "staging")
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusNoContent, recorder.Code, recorder.Body.String())
	require.NotNil(t, auth.req)
	assert.Equal(t, "doc-1", auth.req.DocumentID)
	assert.Equal(t, "campus-code-1", auth.req.WorkspaceCode)
	assert.Equal(t, entity.EnvironmentDev, auth.req.Environment)
}

func TestReadOnlyViewLinkCustomAuthMapsForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth := &fakeReadOnlyViewLinkAuth{err: entity.ErrForbidden}
	router := gin.New()
	router.POST("/api/v1/documents/:documentId/view-link", ReadOnlyViewLinkCustomAuth(auth), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/doc-1/view-link", nil)
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
}

func TestReadOnlyViewLinkCustomAuthMapsUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	auth := &fakeReadOnlyViewLinkAuth{err: errors.New("invalid token")}
	router := gin.New()
	router.POST("/api/v1/documents/:documentId/view-link", ReadOnlyViewLinkCustomAuth(auth), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/documents/doc-1/view-link", nil)
	router.ServeHTTP(recorder, req)

	require.Equal(t, http.StatusUnauthorized, recorder.Code, recorder.Body.String())
}
