package middleware

import (
	"bytes"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

func abortedWith(t *testing.T, status int, err error) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/workspace", nil)

	abortWithError(ctx, status, err)

	assert.True(t, ctx.IsAborted())
	return recorder
}

// The text of a server error can carry SQL, constraint names or paths. A
// middleware that fails says only that the server failed, and logs the error.
func TestAbortWithError_ServerErrorIsLoggedNotSent(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	recorder := abortedWith(t, http.StatusInternalServerError,
		errors.New(`finding user by email: ERROR: relation "identity.users" does not exist (SQLSTATE 42P01)`))

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.JSONEq(t, `{"error":"internal server error"}`, recorder.Body.String())
	assert.Contains(t, logs.String(), "SQLSTATE 42P01", "the error must be logged")
}

// Below 500 the response carries the error's own text, as before.
func TestAbortWithError_ClientErrorKeepsItsText(t *testing.T) {
	recorder := abortedWith(t, http.StatusForbidden, entity.ErrWorkspaceAccessDenied)

	assert.Equal(t, http.StatusForbidden, recorder.Code)
	assert.JSONEq(t, `{"error":"workspace access denied"}`, recorder.Body.String())
}
