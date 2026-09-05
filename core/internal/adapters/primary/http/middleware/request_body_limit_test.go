package middleware

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bodyLimitRouter returns a router whose handler records what it could read from the body.
func bodyLimitRouter(maxBytes int64, got *[]byte, readErr *error) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/upload", RequestBodyLimit(maxBytes), func(c *gin.Context) {
		*got, *readErr = io.ReadAll(c.Request.Body)
		c.Status(http.StatusOK)
	})
	return router
}

func TestRequestBodyLimit_AllowsBodiesWithinTheLimit(t *testing.T) {
	var got []byte
	var readErr error
	router := bodyLimitRouter(16, &got, &readErr)
	body := bytes.Repeat([]byte("x"), 16)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body)))

	require.Equal(t, http.StatusOK, w.Code)
	require.NoError(t, readErr)
	assert.Equal(t, body, got)
}

func TestRequestBodyLimit_FailsReadsPastTheLimit(t *testing.T) {
	var got []byte
	var readErr error
	router := bodyLimitRouter(16, &got, &readErr)
	body := bytes.Repeat([]byte("x"), 17)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body)))

	var maxBytesErr *http.MaxBytesError
	require.ErrorAs(t, readErr, &maxBytesErr)
	assert.Equal(t, int64(16), maxBytesErr.Limit)
	assert.Len(t, got, 16, "the reader hands out at most the limit")
}

func TestRequestBodyLimit_NonPositiveLimitDisablesTheCap(t *testing.T) {
	var got []byte
	var readErr error
	router := bodyLimitRouter(0, &got, &readErr)
	body := bytes.Repeat([]byte("x"), 1024)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/upload", bytes.NewReader(body)))

	require.NoError(t, readErr)
	assert.Equal(t, body, got)
}
