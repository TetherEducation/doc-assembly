package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// fakeAuditLogRepo hands every created entry to the test over a channel. It must not call
// t.* methods: Create runs on the middleware's goroutine, possibly after the subtest ended.
type fakeAuditLogRepo struct {
	entries chan *entity.AutomationAuditLog
}

func newFakeAuditLogRepo() *fakeAuditLogRepo {
	return &fakeAuditLogRepo{entries: make(chan *entity.AutomationAuditLog, 1)}
}

func (f *fakeAuditLogRepo) Create(_ context.Context, entry *entity.AutomationAuditLog) error {
	f.entries <- entry
	return nil
}

func (f *fakeAuditLogRepo) ListByKeyID(_ context.Context, _ string, _, _ int) ([]*entity.AutomationAuditLog, error) {
	return nil, nil
}

const auditTestPath = "/api/v1/automation/templates/tpl-1/versions/v-1/content"

// newAuditTestRouter wires the audit middleware behind a stub of AutomationKeyAuth (plus any
// middleware in before) and in front of a handler that records every byte it can read from
// the request body.
func newAuditTestRouter(repo *fakeAuditLogRepo, seen *[]byte, before ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	chain := []gin.HandlerFunc{func(c *gin.Context) {
		c.Set(automationKeyIDCtxKey, "key-1")
		c.Set(automationKeyPrefixCtxKey, "doca_test")
	}}
	chain = append(chain, before...)
	chain = append(chain,
		AutomationAuditLogger(repo),
		func(c *gin.Context) {
			body, err := io.ReadAll(c.Request.Body)
			if err != nil {
				c.AbortWithStatus(http.StatusInternalServerError)
				return
			}
			*seen = body
			c.Status(http.StatusOK)
		},
	)
	router.PUT("/api/v1/automation/templates/:templateId/versions/:versionId/content", chain...)
	return router
}

// jsonBodyOfSize returns a valid JSON object of exactly size bytes.
func jsonBodyOfSize(t *testing.T, size int) []byte {
	t.Helper()
	const frame = `{"pad":""}`
	require.Greater(t, size, len(frame))
	body := []byte(`{"pad":"` + strings.Repeat("x", size-len(frame)) + `"}`)
	require.Len(t, body, size)
	require.True(t, json.Valid(body))
	return body
}

// awaitAuditEntry waits for the asynchronous audit write to reach the fake repository.
func awaitAuditEntry(t *testing.T, repo *fakeAuditLogRepo) *entity.AutomationAuditLog {
	t.Helper()
	select {
	case entry := <-repo.entries:
		return entry
	case <-time.After(2 * time.Second):
		require.FailNow(t, "audit entry was not written")
		return nil
	}
}

func TestAutomationAuditLogger_RequestBodyCapture(t *testing.T) {
	cases := []struct {
		name      string
		size      int
		truncated bool
	}{
		{name: "one byte under the limit is stored verbatim", size: bodyCaptureLimitBytes - 1},
		{name: "exactly the limit is stored verbatim", size: bodyCaptureLimitBytes},
		{name: "one byte over the limit is marked truncated", size: bodyCaptureLimitBytes + 1, truncated: true},
		{name: "200 KB body is marked truncated", size: 200 * 1024, truncated: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeAuditLogRepo()
			var seen []byte
			router := newAuditTestRouter(repo, &seen)
			body := jsonBodyOfSize(t, tc.size)

			req := httptest.NewRequest(http.MethodPut, auditTestPath, bytes.NewReader(body))
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			require.Equal(t, http.StatusOK, w.Code)
			assert.True(t, bytes.Equal(body, seen),
				"handler must receive the full request body: got %d of %d bytes", len(seen), len(body))

			entry := awaitAuditEntry(t, repo)
			require.NotNil(t, entry.Action)
			assert.Equal(t, "UPDATE_CONTENT", *entry.Action)
			require.NotNil(t, entry.ResourceID)
			assert.Equal(t, "v-1", *entry.ResourceID)
			assert.Equal(t, http.StatusOK, entry.ResponseStatus)

			assert.True(t, json.Valid(entry.RequestBody), "audit body must be valid JSON")
			assert.LessOrEqual(t, len(entry.RequestBody), bodyCaptureLimitBytes)
			if !tc.truncated {
				assert.True(t, bytes.Equal(body, entry.RequestBody),
					"audit body must be stored verbatim: got %d of %d bytes", len(entry.RequestBody), len(body))
				return
			}

			var marker struct {
				Truncated     bool `json:"truncated"`
				OriginalBytes int  `json:"originalBytes"`
				LimitBytes    int  `json:"limitBytes"`
			}
			require.NoError(t, json.Unmarshal(entry.RequestBody, &marker), "audit body: %s", string(entry.RequestBody))
			assert.True(t, marker.Truncated)
			assert.Equal(t, tc.size, marker.OriginalBytes)
			assert.Equal(t, bodyCaptureLimitBytes, marker.LimitBytes)
		})
	}
}

func TestAutomationAuditLogger_NoBodyStoresNothing(t *testing.T) {
	repo := newFakeAuditLogRepo()
	var seen []byte
	router := newAuditTestRouter(repo, &seen)

	req := httptest.NewRequest(http.MethodPut, auditTestPath, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Empty(t, seen)

	entry := awaitAuditEntry(t, repo)
	assert.Nil(t, entry.RequestBody)
}

func TestAutomationAuditLogger_MalformedBodyIsMarked(t *testing.T) {
	repo := newFakeAuditLogRepo()
	var seen []byte
	router := newAuditTestRouter(repo, &seen)
	body := []byte(`{"pad": "unterminated`)

	req := httptest.NewRequest(http.MethodPut, auditTestPath, bytes.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, body, seen, "handler must still receive the malformed body")

	entry := awaitAuditEntry(t, repo)
	assert.JSONEq(t, fmt.Sprintf(`{"invalidJson":true,"originalBytes":%d}`, len(body)), string(entry.RequestBody))
}

func TestAutomationAuditLogger_OversizedBodyIsRejectedWith413(t *testing.T) {
	repo := newFakeAuditLogRepo()
	var seen []byte
	router := newAuditTestRouter(repo, &seen, RequestBodyLimit(1024))
	body := jsonBodyOfSize(t, 2048)

	req := httptest.NewRequest(http.MethodPut, auditTestPath, bytes.NewReader(body))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusRequestEntityTooLarge, w.Code)
	assert.JSONEq(t, `{"error":"request body exceeds the limit of 1024 bytes"}`, w.Body.String())
	assert.Nil(t, seen, "handler must not run for an oversized body")

	entry := awaitAuditEntry(t, repo)
	assert.Equal(t, http.StatusRequestEntityTooLarge, entry.ResponseStatus)
	assert.Nil(t, entry.RequestBody)
	require.NotNil(t, entry.Action)
	assert.Equal(t, "UPDATE_CONTENT", *entry.Action)
}

func TestAutomationAuditLogger_UnreadableBodyIsRejectedWith400(t *testing.T) {
	repo := newFakeAuditLogRepo()
	var seen []byte
	router := newAuditTestRouter(repo, &seen)

	req := httptest.NewRequest(http.MethodPut, auditTestPath, iotest.ErrReader(errors.New("connection reset")))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.JSONEq(t, `{"error":"failed to read request body"}`, w.Body.String())
	assert.Nil(t, seen, "handler must not run for an unreadable body")

	entry := awaitAuditEntry(t, repo)
	assert.Equal(t, http.StatusBadRequest, entry.ResponseStatus)
	assert.Nil(t, entry.RequestBody)
}
