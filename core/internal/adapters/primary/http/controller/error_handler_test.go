package controller

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// sqlDetail is the kind of text a database error carries.
const sqlDetail = `creating template: ERROR: insert or update on table "templates" violates foreign key constraint "fk_templates_folder_id" (SQLSTATE 23503)`

// captureErrorLogs sends the default logger's output to a buffer for the test.
func captureErrorLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &logs
}

func newErrorHandlerContext(t *testing.T) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/content/templates", nil)
	return ctx, recorder
}

// The text of a server error can carry SQL, constraint names or paths. The
// response says only that the server failed, and the error goes to the log.
func TestHandleError_ServerErrorIsLoggedNotSent(t *testing.T) {
	logs := captureErrorLogs(t)
	ctx, recorder := newErrorHandlerContext(t)

	HandleError(ctx, errors.New(sqlDetail))

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.JSONEq(t, `{"error":"internal server error"}`, recorder.Body.String())
	assert.Contains(t, logs.String(), "fk_templates_folder_id", "the error must be logged")
}

// A handler that answers 500 itself goes through the same door.
func TestRespondError_ServerErrorIsLoggedNotSent(t *testing.T) {
	logs := captureErrorLogs(t)
	ctx, recorder := newErrorHandlerContext(t)

	respondError(ctx, http.StatusInternalServerError, errors.New(sqlDetail))

	assert.Equal(t, http.StatusInternalServerError, recorder.Code)
	assert.JSONEq(t, `{"error":"internal server error"}`, recorder.Body.String())
	assert.Contains(t, logs.String(), "fk_templates_folder_id", "the error must be logged")
}

// A 503 names the condition it was mapped from, whose text is written for the
// client, without what the error was wrapped in.
func TestHandleError_UnavailableNamesTheCondition(t *testing.T) {
	for _, condition := range unavailableErrors {
		t.Run(condition.Error(), func(t *testing.T) {
			logs := captureErrorLogs(t)
			ctx, recorder := newErrorHandlerContext(t)

			HandleError(ctx, fmt.Errorf("rendering /var/lib/doc-engine/tmp/preview-17.typ: %w", condition))

			assert.Equal(t, http.StatusServiceUnavailable, recorder.Code)
			assert.JSONEq(t, fmt.Sprintf(`{"error":%q}`, condition.Error()), recorder.Body.String())
			assert.Contains(t, logs.String(), "preview-17.typ", "the error must be logged")
		})
	}
}

// Below 500 the response carries the error's own text, as before.
func TestHandleError_ClientErrorKeepsItsText(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{fmt.Errorf("finding tag: %w", entity.ErrTagNotFound), http.StatusNotFound},
		{entity.ErrTemplateAlreadyExists, http.StatusConflict},
		{entity.ErrFolderHasChildren, http.StatusBadRequest},
		{entity.ErrInsufficientRole, http.StatusForbidden},
		{entity.ErrUnauthorized, http.StatusUnauthorized},
		{entity.ErrTooManyRequests, http.StatusTooManyRequests},
	}
	for _, tc := range cases {
		t.Run(tc.err.Error(), func(t *testing.T) {
			ctx, recorder := newErrorHandlerContext(t)

			HandleError(ctx, tc.err)

			assert.Equal(t, tc.status, recorder.Code)
			assert.JSONEq(t, fmt.Sprintf(`{"error":%q}`, tc.err.Error()), recorder.Body.String())
		})
	}
}
