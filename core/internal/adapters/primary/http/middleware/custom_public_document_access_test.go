package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

type fakePublicDocAuth struct {
	claims *port.PublicDocumentAccessClaims
	err    error
	abort  bool
}

func (f *fakePublicDocAuth) Authenticate(c *gin.Context, _ *port.AuthenticateRequest) (*port.PublicDocumentAccessClaims, error) {
	if f.abort {
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return nil, errors.New("aborted unauthorized")
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.claims, nil
}

func TestCustomPublicDocumentAccess_SetsClaimsOnSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/public/doc/:documentId",
		CustomPublicDocumentAccess(&fakePublicDocAuth{
			claims: &port.PublicDocumentAccessClaims{Email: "alice@example.com"},
		}),
		func(c *gin.Context) {
			claims, ok := GetPublicDocumentAccessClaims(c)
			if !ok {
				c.JSON(http.StatusOK, gin.H{"email": ""})
				return
			}
			c.JSON(http.StatusOK, gin.H{"email": claims.Email})
		},
	)

	req := httptest.NewRequest(http.MethodGet, "/public/doc/doc-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"email":"alice@example.com"}`, w.Body.String())
}

func TestCustomPublicDocumentAccess_FallsBackWhenAuthFails(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/public/doc/:documentId",
		CustomPublicDocumentAccess(&fakePublicDocAuth{
			err: errors.New("invalid jwt"),
		}),
		func(c *gin.Context) {
			_, ok := GetPublicDocumentAccessClaims(c)
			c.JSON(http.StatusOK, gin.H{"hasClaims": ok})
		},
	)

	req := httptest.NewRequest(http.MethodGet, "/public/doc/doc-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"hasClaims":false}`, w.Body.String())
}

func TestCustomPublicDocumentAccess_FallsBackWhenClaimsEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/public/doc/:documentId",
		CustomPublicDocumentAccess(&fakePublicDocAuth{
			claims: &port.PublicDocumentAccessClaims{Email: ""},
		}),
		func(c *gin.Context) {
			_, ok := GetPublicDocumentAccessClaims(c)
			c.JSON(http.StatusOK, gin.H{"hasClaims": ok})
		},
	)

	req := httptest.NewRequest(http.MethodGet, "/public/doc/doc-1", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"hasClaims":false}`, w.Body.String())
}

func TestCustomPublicDocumentAccess_SetsClaimsOnPOST(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/public/doc/:documentId/request-access",
		CustomPublicDocumentAccess(&fakePublicDocAuth{
			claims: &port.PublicDocumentAccessClaims{Email: "bob@example.com"},
		}),
		func(c *gin.Context) {
			claims, ok := GetPublicDocumentAccessClaims(c)
			if !ok {
				c.JSON(http.StatusOK, gin.H{"email": ""})
				return
			}
			c.JSON(http.StatusOK, gin.H{"email": claims.Email})
		},
	)

	req := httptest.NewRequest(http.MethodPost, "/public/doc/doc-1/request-access", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.JSONEq(t, `{"email":"bob@example.com"}`, w.Body.String())
}

func TestCustomPublicDocumentAccess_StopsWhenAuthenticatorAborts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.POST("/public/doc/:documentId/request-access",
		CustomPublicDocumentAccess(&fakePublicDocAuth{
			abort: true,
		}),
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"handler": "should-not-run"})
		},
	)

	req := httptest.NewRequest(http.MethodPost, "/public/doc/doc-1/request-access", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.JSONEq(t, `{"error":"unauthorized"}`, w.Body.String())
}
