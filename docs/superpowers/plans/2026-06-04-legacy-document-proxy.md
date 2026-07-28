# Legacy Document Proxy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a formal `doc-assembly` SDK extension point and API endpoint for host-provided legacy document access negotiation, without creating an alternate path for Doc Assembly Documents.

**Architecture:** The host app registers a single `LegacyDocumentHandler` through `engine.SetLegacyDocumentHandler(handler)`. When registered, the engine mounts `POST /api/v1/legacy-documents/proxy`; the library validates only method, `X-Workspace-Code`, `X-Environment`, and request body size, then delegates auth, request interpretation, legacy lookup, and JSON response shape to the handler. If no handler is registered, the route is not mounted.

**Tech Stack:** Go 1.25, Gin, Viper configuration, Swag/OpenAPI generation, `encoding/json`, standard `net/http` status semantics.

---

## Scope

Only `doc-assembly/` is in scope.

Do not modify:

- `tools-doc-assembly/`
- `crm-front/`
- `applications-front/`
- any deployment workflow

This plan intentionally does not adapt the existing `tools-doc-assembly` PR. That must happen in a separate plan after this library contract lands.

## Decisions Already Resolved

- Canonical concept: `Legacy Document Proxy`.
- This is not a custom route framework.
- This is not an alternate access path for Doc Assembly Documents.
- The endpoint is neutral, not Tether/PandaDoc/GCS-specific.
- Final path: `POST /api/v1/legacy-documents/proxy`.
- The endpoint is mounted only when `SetLegacyDocumentHandler` is called.
- Without a registered handler, the route is not mounted.
- Non-POST methods return `405 Method Not Allowed` when the route is mounted.
- The library requires only `X-Workspace-Code` and `X-Environment`.
- The library does not require `Authorization`, `X-Tenant-ID`, `X-Workspace-ID`, JSON request body, document id, tenant, or content type.
- The handler receives full request headers and raw request body.
- The handler owns authentication, authorization, parsing, legacy lookup, and response content.
- Response is JSON only: `StatusCode`, `Headers`, `Body any`.
- `Body` may be `nil`, scalar, array, object, struct, pointer, or `json.RawMessage`.
- Handler technical errors are logged and returned as `500`.
- Request body size is configurable by YAML, default `64 KiB`.
- The library prevents handler response headers from overwriting system-controlled headers.
- The endpoint must be covered in tests, Swagger/OpenAPI, backend docs, and the repo consumer skill.

## File Structure

Create:

- `core/internal/core/port/legacy_document_handler.go` — public core port for legacy document proxy request/response and handler.
- `core/internal/adapters/primary/http/controller/legacy_document_controller.go` — HTTP adapter for the legacy route, validation, body limiting, handler invocation, JSON serialization, and response header guardrails.
- `core/internal/adapters/primary/http/controller/legacy_document_controller_test.go` — focused controller/router tests for route behavior, validation, handler request shape, response serialization, headers, and errors.

Modify:

- `core/cmd/api/bootstrap/engine.go` — store handler on `Engine`, expose `SetLegacyDocumentHandler`, expose optional getter for tests.
- `core/cmd/api/bootstrap/initializer.go` — pass the registered handler into server bootstrap.
- `core/internal/infra/server/http.go` — accept the handler and register the route only when present.
- `core/internal/infra/config/types.go` — add `LegacyDocumentsConfig` with max body size.
- `core/internal/infra/config/config.go` — set `legacy_documents.max_body_bytes` default to `65536`.
- `core/settings/app.yaml` — document the configurable body limit.
- `core/sdk/interfaces.go` — expose `LegacyDocumentHandler`, request, and response types.
- `docs/backend/authorization-matrix.md` — document the formal endpoint and its boundaries.
- `skills/doc-assembly/SKILL.md` — mention the new consumer-facing capability.
- `skills/doc-assembly/references/engine-api.md` — document `SetLegacyDocumentHandler`.
- `skills/doc-assembly/references/adapters.md` — document the handler contract and boundaries.
- `core/docs/swagger.yaml` and generated swagger files — regenerate after adding annotations.

## Task 1: Add the Legacy Document Handler Port

**Files:**

- Create: `core/internal/core/port/legacy_document_handler.go`
- Modify: `core/sdk/interfaces.go`
- Test: compile-only through `go test -C core ./internal/core/port ./sdk`

- [ ] **Step 1: Write the port file**

Create `core/internal/core/port/legacy_document_handler.go`:

```go
package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// LegacyDocumentRequest contains the minimum doc-assembly context plus the
// raw request data a host application needs to resolve a legacy document.
type LegacyDocumentRequest struct {
	WorkspaceCode string
	Environment   entity.Environment
	Headers       map[string][]string
	RawBody       []byte
}

// LegacyDocumentResponse is a host-provided JSON response for a legacy
// document request. Body is serialized as JSON by doc-assembly.
type LegacyDocumentResponse struct {
	StatusCode int
	Headers    map[string][]string
	Body       any
}

// LegacyDocumentHandler resolves requests for documents outside the current
// doc-assembly document lifecycle.
type LegacyDocumentHandler interface {
	HandleLegacyDocument(ctx context.Context, req *LegacyDocumentRequest) (*LegacyDocumentResponse, error)
}
```

- [ ] **Step 2: Expose SDK aliases**

Modify `core/sdk/interfaces.go` by adding this block after the read-only view link aliases:

```go
// LegacyDocumentHandler provides host-defined access negotiation for documents
// outside the current doc-assembly document lifecycle.
type LegacyDocumentHandler = port.LegacyDocumentHandler

// LegacyDocumentRequest is passed to a LegacyDocumentHandler.
type LegacyDocumentRequest = port.LegacyDocumentRequest

// LegacyDocumentResponse is returned by a LegacyDocumentHandler and serialized
// as JSON by doc-assembly.
type LegacyDocumentResponse = port.LegacyDocumentResponse
```

- [ ] **Step 3: Run focused compile tests**

Run:

```bash
go test -C core ./internal/core/port ./sdk
```

Expected:

```text
ok  	github.com/TetherEducation/doc-assembly/core/internal/core/port
ok  	github.com/TetherEducation/doc-assembly/core/sdk
```

- [ ] **Step 4: Commit**

```bash
git add core/internal/core/port/legacy_document_handler.go core/sdk/interfaces.go
git commit -m "feat: add legacy document handler contract"
```

## Task 2: Add Config for Legacy Body Limit

**Files:**

- Modify: `core/internal/infra/config/types.go`
- Modify: `core/internal/infra/config/config.go`
- Modify: `core/settings/app.yaml`
- Test: `core/internal/infra/config/config_test.go`

- [ ] **Step 1: Add failing config tests**

Append these tests to `core/internal/infra/config/config_test.go`:

```go
func TestLegacyDocumentsConfig_DefaultMaxBodyBytes(t *testing.T) {
	cfg := LegacyDocumentsConfig{}

	assert.Equal(t, int64(65536), cfg.MaxBodyBytesOrDefault())
}

func TestLegacyDocumentsConfig_UsesConfiguredMaxBodyBytes(t *testing.T) {
	cfg := LegacyDocumentsConfig{MaxBodyBytes: 4096}

	assert.Equal(t, int64(4096), cfg.MaxBodyBytesOrDefault())
}

func TestLegacyDocumentsConfig_NonPositiveFallsBackToDefault(t *testing.T) {
	for _, value := range []int64{0, -1} {
		cfg := LegacyDocumentsConfig{MaxBodyBytes: value}

		assert.Equal(t, int64(65536), cfg.MaxBodyBytesOrDefault())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test -C core ./internal/infra/config -run TestLegacyDocumentsConfig -count=1
```

Expected:

```text
FAIL
undefined: LegacyDocumentsConfig
```

- [ ] **Step 3: Add config type**

Modify `core/internal/infra/config/types.go`.

Add this field to `Config` after `PublicAccess`:

```go
	LegacyDocuments  LegacyDocumentsConfig  `mapstructure:"legacy_documents"`
```

Add this type near `PublicAccessConfig`:

```go
const defaultLegacyDocumentMaxBodyBytes int64 = 64 * 1024

// LegacyDocumentsConfig configures compatibility access for documents outside
// the current doc-assembly document lifecycle.
type LegacyDocumentsConfig struct {
	MaxBodyBytes int64 `mapstructure:"max_body_bytes"`
}

// MaxBodyBytesOrDefault returns the configured request body limit, falling
// back to 64 KiB for missing or invalid values.
func (c LegacyDocumentsConfig) MaxBodyBytesOrDefault() int64 {
	if c.MaxBodyBytes > 0 {
		return c.MaxBodyBytes
	}
	return defaultLegacyDocumentMaxBodyBytes
}
```

- [ ] **Step 4: Add Viper default**

Modify `core/internal/infra/config/config.go` in `setDefaults` after public access or internal API defaults:

```go
	v.SetDefault("legacy_documents.max_body_bytes", 64*1024)
```

- [ ] **Step 5: Document YAML setting**

Add this block to `core/settings/app.yaml` after `internal_api`:

```yaml
# Legacy Document Proxy compatibility endpoint.
# The endpoint is mounted only when a host application registers
# SetLegacyDocumentHandler. This setting only controls the maximum request body
# size passed to that handler.
legacy_documents:
  max_body_bytes: 65536 # DOC_ENGINE_LEGACY_DOCUMENTS_MAX_BODY_BYTES - Default 64 KiB
```

- [ ] **Step 6: Run focused config tests**

Run:

```bash
go test -C core ./internal/infra/config -run TestLegacyDocumentsConfig -count=1
```

Expected:

```text
ok  	github.com/TetherEducation/doc-assembly/core/internal/infra/config
```

- [ ] **Step 7: Commit**

```bash
git add core/internal/infra/config/types.go core/internal/infra/config/config.go core/internal/infra/config/config_test.go core/settings/app.yaml
git commit -m "feat: configure legacy document body limit"
```

## Task 3: Add Engine SDK Hook

**Files:**

- Modify: `core/cmd/api/bootstrap/engine.go`
- Test: `core/cmd/api/bootstrap/engine_legacy_document_test.go`

- [ ] **Step 1: Write failing engine tests**

Create `core/cmd/api/bootstrap/engine_legacy_document_test.go`:

```go
package bootstrap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

type testLegacyDocumentHandler struct{}

func (testLegacyDocumentHandler) HandleLegacyDocument(context.Context, *port.LegacyDocumentRequest) (*port.LegacyDocumentResponse, error) {
	return &port.LegacyDocumentResponse{StatusCode: 200, Body: map[string]any{"ok": true}}, nil
}

func TestEngine_SetLegacyDocumentHandler(t *testing.T) {
	handler := &testLegacyDocumentHandler{}

	engine := New().SetLegacyDocumentHandler(handler)

	assert.Same(t, handler, engine.GetLegacyDocumentHandler())
}
```

- [ ] **Step 2: Run test to verify it fails**

Run:

```bash
go test -C core ./cmd/api/bootstrap -run TestEngine_SetLegacyDocumentHandler -count=1
```

Expected:

```text
FAIL
engine.SetLegacyDocumentHandler undefined
```

- [ ] **Step 3: Add engine field and methods**

Modify `core/cmd/api/bootstrap/engine.go`.

Add this field after `readOnlyViewAuth`:

```go
	legacyDocumentHandler port.LegacyDocumentHandler
```

Add these methods after `GetReadOnlyViewLinkAuthenticator`:

```go
// SetLegacyDocumentHandler sets the host-defined handler for
// /api/v1/legacy-documents/proxy.
func (e *Engine) SetLegacyDocumentHandler(handler port.LegacyDocumentHandler) *Engine {
	e.legacyDocumentHandler = handler
	return e
}

// GetLegacyDocumentHandler returns the registered legacy document handler, or
// nil if not set.
func (e *Engine) GetLegacyDocumentHandler() port.LegacyDocumentHandler {
	return e.legacyDocumentHandler
}
```

- [ ] **Step 4: Run focused engine test**

Run:

```bash
go test -C core ./cmd/api/bootstrap -run TestEngine_SetLegacyDocumentHandler -count=1
```

Expected:

```text
ok  	github.com/TetherEducation/doc-assembly/core/cmd/api/bootstrap
```

- [ ] **Step 5: Commit**

```bash
git add core/cmd/api/bootstrap/engine.go core/cmd/api/bootstrap/engine_legacy_document_test.go
git commit -m "feat: add legacy document sdk hook"
```

## Task 4: Build the Legacy Document Controller with TDD

**Files:**

- Create: `core/internal/adapters/primary/http/controller/legacy_document_controller.go`
- Create: `core/internal/adapters/primary/http/controller/legacy_document_controller_test.go`

- [ ] **Step 1: Write controller tests**

Create `core/internal/adapters/primary/http/controller/legacy_document_controller_test.go`:

```go
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

func (f *fakeLegacyDocumentHandler) HandleLegacyDocument(_ context.Context, req *port.LegacyDocumentRequest) (*port.LegacyDocumentResponse, error) {
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
				"X-Operation-ID":              {"malicious"},
				"Access-Control-Allow-Origin": {"https://evil.test"},
				"Vary":                        {"SomethingElse"},
				"Cache-Control":               {"public"},
				"Pragma":                      {"cache"},
				"Expires":                     {"tomorrow"},
				"Set-Cookie":                  {"session=evil"},
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run:

```bash
go test -C core ./internal/adapters/primary/http/controller -run TestLegacyDocumentController -count=1
```

Expected:

```text
FAIL
undefined: NewLegacyDocumentController
```

- [ ] **Step 3: Implement controller**

Create `core/internal/adapters/primary/http/controller/legacy_document_controller.go`:

```go
package controller

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

const legacyDocumentsProxyPath = "/legacy-documents/proxy"

// LegacyDocumentController delegates legacy document access negotiation to a
// host-provided handler.
type LegacyDocumentController struct {
	handler      port.LegacyDocumentHandler
	maxBodyBytes int64
}

// NewLegacyDocumentController creates a controller for the legacy document proxy.
func NewLegacyDocumentController(handler port.LegacyDocumentHandler, maxBodyBytes int64) *LegacyDocumentController {
	return &LegacyDocumentController{
		handler:      handler,
		maxBodyBytes: maxBodyBytes,
	}
}

// RegisterRoutes registers the legacy document proxy route. The route is
// intentionally registered for all methods so non-POST calls return 405.
func (c *LegacyDocumentController) RegisterRoutes(rg *gin.RouterGroup) {
	if c == nil || c.handler == nil || rg == nil {
		return
	}
	rg.Any(legacyDocumentsProxyPath, c.Proxy)
}

// Proxy handles legacy document access negotiation.
//
// @Summary      Proxy legacy document access negotiation
// @Description  Delegates legacy document access to a host-registered LegacyDocumentHandler. This endpoint is only for documents outside the current doc-assembly document lifecycle and is mounted only when a handler is registered.
// @Tags         legacy-documents
// @Accept       json
// @Produce      json
// @Param        X-Workspace-Code header string true "Workspace business code"
// @Param        X-Environment header string true "Runtime environment: dev or prod"
// @Param        request body object false "Host-defined JSON payload"
// @Success      200 {object} map[string]interface{} "Host-defined JSON response"
// @Failure      400 {object} map[string]string "Missing or invalid required header"
// @Failure      405 {object} map[string]string "Method not allowed"
// @Failure      413 {object} map[string]string "Request body too large"
// @Failure      500 {object} map[string]string "Handler or JSON serialization failure"
// @Router       /legacy-documents/proxy [post]
func (c *LegacyDocumentController) Proxy(ctx *gin.Context) {
	if ctx.Request.Method != http.MethodPost {
		ctx.JSON(http.StatusMethodNotAllowed, gin.H{"error": "method not allowed"})
		return
	}

	workspaceCode := strings.TrimSpace(ctx.GetHeader(HeaderWorkspaceCode))
	if workspaceCode == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "missing X-Workspace-Code"})
		return
	}

	environment, ok := parseRequiredLegacyEnvironment(ctx)
	if !ok {
		return
	}

	rawBody, ok := c.readRawBody(ctx)
	if !ok {
		return
	}

	resp, err := c.handler.HandleLegacyDocument(ctx.Request.Context(), &port.LegacyDocumentRequest{
		WorkspaceCode: workspaceCode,
		Environment:   environment,
		Headers:       cloneHeaders(ctx.Request.Header),
		RawBody:       rawBody,
	})
	if err != nil {
		slog.ErrorContext(ctx.Request.Context(), "legacy document handler failed",
			slog.String("workspace_code", workspaceCode),
			slog.String("environment", string(environment)),
			slog.String("operation_id", middleware.GetOperationID(ctx)),
			slog.Any("error", err),
		)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "legacy document handler failed"})
		return
	}
	if resp == nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "legacy document handler returned nil response"})
		return
	}

	applyLegacyResponseHeaders(ctx, resp.Headers)
	writeLegacyJSON(ctx, resp.StatusCode, resp.Body)
}

func parseRequiredLegacyEnvironment(ctx *gin.Context) (entity.Environment, bool) {
	raw := strings.TrimSpace(ctx.GetHeader(HeaderEnvironment))
	if raw == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "missing X-Environment"})
		return "", false
	}

	environment, err := entity.ParseEnvironment(raw)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid X-Environment header value (must be 'dev' or 'prod')"})
		return "", false
	}
	return environment, true
}

func (c *LegacyDocumentController) readRawBody(ctx *gin.Context) ([]byte, bool) {
	limit := c.maxBodyBytes
	if limit <= 0 {
		limit = 64 * 1024
	}

	reader := http.MaxBytesReader(ctx.Writer, ctx.Request.Body, limit)
	body, err := io.ReadAll(reader)
	if err != nil {
		if errors.As(err, new(*http.MaxBytesError)) {
			ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			return nil, false
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read request body"})
		return nil, false
	}
	return body, true
}

func cloneHeaders(headers http.Header) map[string][]string {
	cloned := make(map[string][]string, len(headers))
	for key, values := range headers {
		cloned[key] = append([]string(nil), values...)
	}
	return cloned
}

func applyLegacyResponseHeaders(ctx *gin.Context, headers map[string][]string) {
	for key, values := range headers {
		if isProtectedLegacyResponseHeader(key) {
			continue
		}
		for _, value := range values {
			ctx.Writer.Header().Add(key, value)
		}
	}
}

func isProtectedLegacyResponseHeader(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if normalized == "" {
		return true
	}
	if strings.HasPrefix(normalized, "access-control-") {
		return true
	}

	switch normalized {
	case "content-type",
		"x-operation-id",
		"vary",
		"cache-control",
		"pragma",
		"expires",
		"set-cookie":
		return true
	default:
		return false
	}
}

func writeLegacyJSON(ctx *gin.Context, statusCode int, body any) {
	if statusCode == 0 {
		statusCode = http.StatusOK
	}

	ctx.Header("Content-Type", "application/json; charset=utf-8")

	if raw, ok := body.(json.RawMessage); ok {
		if !json.Valid(raw) {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "legacy document response serialization failed"})
			return
		}
		ctx.Status(statusCode)
		_, _ = ctx.Writer.Write(raw)
		return
	}

	payload, err := json.Marshal(body)
	if err != nil {
		slog.ErrorContext(ctx.Request.Context(), "legacy document response serialization failed",
			slog.String("operation_id", middleware.GetOperationID(ctx)),
			slog.Any("error", err),
		)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "legacy document response serialization failed"})
		return
	}

	ctx.Status(statusCode)
	_, _ = ctx.Writer.Write(payload)
}
```

- [ ] **Step 4: Run controller tests**

Run:

```bash
go test -C core ./internal/adapters/primary/http/controller -run TestLegacyDocumentController -count=1
```

Expected:

```text
ok  	github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/controller
```

- [ ] **Step 5: Commit**

```bash
git add core/internal/adapters/primary/http/controller/legacy_document_controller.go core/internal/adapters/primary/http/controller/legacy_document_controller_test.go
git commit -m "feat: add legacy document proxy controller"
```

## Task 5: Wire Route Mounting into HTTP Server

**Files:**

- Modify: `core/internal/infra/server/http.go`
- Modify: `core/cmd/api/bootstrap/initializer.go`
- Test: `core/internal/infra/server/http_legacy_documents_test.go`

- [ ] **Step 1: Write server routing tests**

Create `core/internal/infra/server/http_legacy_documents_test.go`:

```go
package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	"github.com/TetherEducation/doc-assembly/core/internal/infra/config"
)

type serverLegacyHandler struct {
	called bool
}

func (h *serverLegacyHandler) HandleLegacyDocument(context.Context, *port.LegacyDocumentRequest) (*port.LegacyDocumentResponse, error) {
	h.called = true
	return &port.LegacyDocumentResponse{StatusCode: http.StatusOK, Body: map[string]any{"ok": true}}, nil
}

func TestHTTPServer_DoesNotMountLegacyDocumentRouteWithoutHandler(t *testing.T) {
	router := legacyServerTestRouter(nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", strings.NewReader(`{}`))
	req.Header.Set("X-Workspace-Code", "CAMPUS_1")
	req.Header.Set("X-Environment", "prod")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHTTPServer_MountsLegacyDocumentRouteWithHandler(t *testing.T) {
	handler := &serverLegacyHandler{}
	router := legacyServerTestRouter(handler)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/legacy-documents/proxy", strings.NewReader(`{}`))
	req.Header.Set("X-Workspace-Code", "CAMPUS_1")
	req.Header.Set("X-Environment", "prod")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.True(t, handler.called)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"ok":true}`, rec.Body.String())
}

func legacyServerTestRouter(handler port.LegacyDocumentHandler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerLegacyDocumentRoute(router.Group(""), minimalServerConfig(), 28*time.Second, handler)
	return router
}

func minimalServerConfig() *config.Config {
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
```

- [ ] **Step 2: Run server tests to verify they fail**

Run:

```bash
go test -C core ./internal/infra/server -run TestHTTPServer_.*LegacyDocument -count=1
```

Expected:

```text
FAIL
undefined: registerLegacyDocumentRoute
```

- [ ] **Step 3: Add server constructor parameter and route registration**

Modify `core/internal/infra/server/http.go`.

Add this parameter to `NewHTTPServer` after `readOnlyViewLinkAuthenticator`:

```go
	legacyDocumentHandler port.LegacyDocumentHandler,
```

Add this call after `registerReadOnlyViewLinkRoute(...)`:

```go
	registerLegacyDocumentRoute(base, cfg, requestTimeout, legacyDocumentHandler)
```

Add this helper after `registerReadOnlyViewLinkRoute`:

```go
func registerLegacyDocumentRoute(
	router gin.IRouter,
	cfg *config.Config,
	requestTimeout time.Duration,
	handler port.LegacyDocumentHandler,
) {
	if handler == nil {
		return
	}

	v1 := router.Group("/api/v1")
	v1.Use(noCacheAPI())
	v1.Use(middleware.Operation())
	v1.Use(middleware.RequestTimeout(requestTimeout))

	controller.NewLegacyDocumentController(
		handler,
		cfg.LegacyDocuments.MaxBodyBytesOrDefault(),
	).RegisterRoutes(v1)
}
```

- [ ] **Step 4: Pass handler from engine bootstrap**

Modify `core/cmd/api/bootstrap/initializer.go`.

In the `server.NewHTTPServer(...)` call, add `e.legacyDocumentHandler` after `e.readOnlyViewAuth`:

```go
			e.readOnlyViewAuth,
			e.legacyDocumentHandler,
			automationAPIKeyRepo,
```

- [ ] **Step 5: Run server tests**

Run:

```bash
go test -C core ./internal/infra/server -run TestHTTPServer_.*LegacyDocument -count=1
```

Expected:

```text
ok  	github.com/TetherEducation/doc-assembly/core/internal/infra/server
```

- [ ] **Step 6: Run bootstrap compile tests**

Run:

```bash
go test -C core ./cmd/api/bootstrap
```

Expected:

```text
ok  	github.com/TetherEducation/doc-assembly/core/cmd/api/bootstrap
```

- [ ] **Step 7: Commit**

```bash
git add core/internal/infra/server/http.go core/internal/infra/server/http_legacy_documents_test.go core/cmd/api/bootstrap/initializer.go
git commit -m "feat: mount legacy document proxy route"
```

## Task 6: Add Swagger/OpenAPI Coverage

**Files:**

- Modify: `core/internal/adapters/primary/http/controller/legacy_document_controller.go`
- Generated: `core/docs/docs.go`
- Generated: `core/docs/swagger.json`
- Generated: `core/docs/swagger.yaml`

- [ ] **Step 1: Verify controller annotation is present**

Confirm `core/internal/adapters/primary/http/controller/legacy_document_controller.go` includes this exact route annotation:

```go
// @Router       /legacy-documents/proxy [post]
```

And these header annotations:

```go
// @Param        X-Workspace-Code header string true "Workspace business code"
// @Param        X-Environment header string true "Runtime environment: dev or prod"
```

- [ ] **Step 2: Regenerate Swagger**

Run:

```bash
make -C core swagger
```

Expected:

```text
swag init
```

and generated docs updated.

- [ ] **Step 3: Verify swagger contains the endpoint**

Run:

```bash
rg -n 'legacy-documents/proxy|X-Workspace-Code|X-Environment' core/docs/swagger.yaml core/docs/docs.go
```

Expected includes:

```text
core/docs/swagger.yaml:...:/legacy-documents/proxy:
core/docs/swagger.yaml:...:X-Workspace-Code
core/docs/swagger.yaml:...:X-Environment
```

- [ ] **Step 4: Commit**

```bash
git add core/internal/adapters/primary/http/controller/legacy_document_controller.go core/docs/docs.go core/docs/swagger.json core/docs/swagger.yaml
git commit -m "docs: document legacy document proxy api"
```

## Task 7: Update Backend Docs and Consumer Skill

**Files:**

- Modify: `CONTEXT.md`
- Modify: `docs/backend/authorization-matrix.md`
- Modify: `skills/doc-assembly/SKILL.md`
- Modify: `skills/doc-assembly/references/engine-api.md`
- Modify: `skills/doc-assembly/references/adapters.md`

- [ ] **Step 1: Verify glossary is already domain-only**

Open `CONTEXT.md` and confirm it contains:

```md
**Legacy Document Proxy**:
A compatibility capability for documents created outside the current doc-assembly document lifecycle. It lets a host application centralize contract access through doc-assembly without creating a separate service.
_Avoid_: custom routes framework, alternate document access path, doc-assembly document bypass
```

If wording drifted during earlier work, restore the above language.

- [ ] **Step 2: Add authorization matrix entry**

In `docs/backend/authorization-matrix.md`, add a section near other API routes that do not use the normal workspace auth chain:

```md
### Endpoint de Legacy Document Proxy (`/api/v1/legacy-documents/proxy`)

| Método | Endpoint | Descripción | Autorización |
|---|---|---|---|
| POST | `/legacy-documents/proxy` | Delega al host application la resolución de acceso para documentos legacy externos al lifecycle actual de doc-assembly. | Custom del host |

**Headers requeridos por doc-assembly**:

| Header | Descripción |
|---|---|
| `X-Workspace-Code` | Código de workspace usado como contexto mínimo de compatibilidad. |
| `X-Environment` | Ambiente canónico (`dev` o `prod`). |

**No requiere por parte de doc-assembly**: `Authorization`, `X-Tenant-ID`, `X-Workspace-ID`, body JSON, document id, tenant id, o content type específico.

**Boundary**: este endpoint no debe usarse para acceder documentos creados por el lifecycle actual de doc-assembly. Los Doc Assembly Documents deben seguir usando las rutas estándar de documentos, signing o read-only view.
```

- [ ] **Step 3: Update skill index**

Modify `skills/doc-assembly/SKILL.md`.

In the "When to use this skill" paragraph, add `legacy document proxy handlers` to the trigger list:

```md
Triggers include: writing or reviewing custom injectors, plugging a custom signing provider, replacing storage/notification adapters, configuring `settings/app.yaml`, wiring auth, exposing public signing/read-only links, implementing legacy document proxy handlers, customizing PDF look, or handling completion events.
```

- [ ] **Step 4: Update engine API reference**

In `skills/doc-assembly/references/engine-api.md`, add this row to an appropriate table near auth hooks or external adapters:

```md
| `SetLegacyDocumentHandler(handler sdk.LegacyDocumentHandler)` | Mounts `POST /api/v1/legacy-documents/proxy` for host-owned legacy document access negotiation. No handler means no route. |
```

Add this section below the table:

```md
## Legacy Document Proxy

Use `SetLegacyDocumentHandler` only for documents outside the current doc-assembly document lifecycle. It is not a custom route framework and must not be used as an alternate access path for Doc Assembly Documents.

The library validates only:

- method is `POST`
- `X-Workspace-Code` is present
- `X-Environment` is present and parses to `dev` or `prod`
- request body is within `legacy_documents.max_body_bytes`

The handler owns authentication, authorization, request parsing, legacy lookup, and JSON response semantics.
```

- [ ] **Step 5: Update adapters reference**

Append this section to `skills/doc-assembly/references/adapters.md`:

````md
## LegacyDocumentHandler

`LegacyDocumentHandler` lets a wrapper expose a compatibility JSON endpoint for legacy documents:

```go
type LegacyDocumentHandler interface {
    HandleLegacyDocument(ctx context.Context, req *sdk.LegacyDocumentRequest) (*sdk.LegacyDocumentResponse, error)
}
```

Register it before `Run()`:

```go
engine.SetLegacyDocumentHandler(myHandler)
```

When registered, doc-assembly mounts:

```http
POST /api/v1/legacy-documents/proxy
X-Workspace-Code: <workspace-code>
X-Environment: dev|prod
```

The request body is optional and passed as raw bytes. The handler may also read any request headers from `req.Headers`.

Return JSON through `Body any`:

```go
return &sdk.LegacyDocumentResponse{
    StatusCode: http.StatusOK,
    Headers: map[string][]string{
        "X-Legacy-Provider": {"previous-system"},
    },
    Body: map[string]any{
        "url": "https://legacy.example.test/documents/abc",
        "expiresInSeconds": 300,
    },
}, nil
```

Do not return PDFs, byte streams, or Doc Assembly Documents from this handler. Use standard document, signing, and read-only view flows for documents created by doc-assembly.
````

- [ ] **Step 6: Commit docs**

```bash
git add CONTEXT.md docs/backend/authorization-matrix.md skills/doc-assembly/SKILL.md skills/doc-assembly/references/engine-api.md skills/doc-assembly/references/adapters.md
git commit -m "docs: describe legacy document proxy contract"
```

## Task 8: Exhaustive Verification and Gap Review

**Files:**

- Inspect all changed files.
- Run full relevant test suite.
- Do not commit until failures are resolved.

- [ ] **Step 1: Run focused tests**

Run:

```bash
go test -C core ./internal/adapters/primary/http/controller -run TestLegacyDocumentController -count=1
go test -C core ./internal/infra/server -run TestHTTPServer_.*LegacyDocument -count=1
go test -C core ./internal/infra/config -run TestLegacyDocumentsConfig -count=1
go test -C core ./cmd/api/bootstrap -run TestEngine_SetLegacyDocumentHandler -count=1
```

Expected: all pass.

- [ ] **Step 2: Run package tests touched by this change**

Run:

```bash
go test -C core ./cmd/api/bootstrap ./internal/core/port ./sdk ./internal/adapters/primary/http/controller ./internal/infra/server ./internal/infra/config
```

Expected: all pass.

- [ ] **Step 3: Run broader backend tests**

Run:

```bash
go test -C core ./...
```

Expected: all pass.

- [ ] **Step 4: Run lint/build generation checks**

Run:

```bash
make -C core swagger
make -C core wire
make -C core lint
make -C core build
```

Expected: all pass. If `make -C core lint` is unavailable locally because `golangci-lint` is missing, record that exact tool error and still run `go test -C core ./...`.

- [ ] **Step 5: Inspect diff for scope**

Run:

```bash
git diff --stat
git diff -- core/internal/core/port/legacy_document_handler.go core/internal/adapters/primary/http/controller/legacy_document_controller.go core/internal/infra/server/http.go core/cmd/api/bootstrap/engine.go
```

Expected:

- no changes outside `doc-assembly`;
- no `tools-doc-assembly`, `crm-front`, or `applications-front` edits;
- no fallback/legacy PDF implementation in the library;
- no handler-specific auth logic in the library;
- no PDF/byte streaming response path.

- [ ] **Step 6: Self-review against requirements**

Verify each requirement maps to code and tests:

```text
Route only when handler registered -> server route tests.
No handler means no route -> server route tests.
POST only -> method-not-allowed test.
X-Workspace-Code required -> controller test.
X-Environment required/valid -> controller tests.
No auth required by lib -> no auth middleware in registerLegacyDocumentRoute.
Optional raw body -> handler request shape test.
Body limit default/config -> config and 413 tests.
JSON response any -> serialization variants test.
json.RawMessage supported -> serialization variants test.
Handler error logs and 500 -> handler error test.
Unserializable body logs and 500 -> serialization failure test.
Protected headers -> system header test.
Swagger/OpenAPI -> rg verification.
Skill/docs updated -> docs task.
```

- [ ] **Step 7: Fix gaps and rerun relevant tests**

If any gap is found, fix the smallest relevant file and rerun:

```bash
go test -C core ./internal/adapters/primary/http/controller ./internal/infra/server ./internal/infra/config ./cmd/api/bootstrap
```

Expected: all pass.

- [ ] **Step 8: Final commit if verification changes files**

If Task 8 produced follow-up changes:

```bash
git add core docs skills CONTEXT.md
git commit -m "test: cover legacy document proxy contract"
```

If no follow-up changes were needed, do not create an empty commit.

## Task 9: Final Readiness Report

**Files:**

- No file edits.

- [ ] **Step 1: Capture final status**

Run:

```bash
git status --short --branch
git log --oneline -5
```

Expected:

```text
## <branch-name>
```

with only intentional commits on the branch and no untracked temporary files.

- [ ] **Step 2: Summarize verification**

Prepare a concise report containing:

```text
Implemented:
- SDK hook: SetLegacyDocumentHandler
- Route: POST /api/v1/legacy-documents/proxy
- Required headers: X-Workspace-Code, X-Environment
- Config: legacy_documents.max_body_bytes default 65536
- JSON response proxy with protected response headers
- Swagger/docs/skill updates

Verification:
- go test -C core ./...
- make -C core swagger
- make -C core wire
- make -C core lint
- make -C core build

Not in scope:
- tools-doc-assembly handler implementation
- GCS/PandaDoc legacy lookup
- crm-front changes
```

## Self-Review

Spec coverage:

- Legacy-only boundary: covered in `CONTEXT.md`, docs, skill, and no core document lookup in implementation.
- Formal endpoint: covered by controller and Swagger.
- Mounted only by registration: covered by engine hook and server tests.
- Minimal library validation: covered by controller tests.
- No auth/body/content-type requirement: covered by route chain and tests.
- Configurable body limit default 64 KiB: covered by config tests and YAML.
- JSON `any` response variety: covered by serialization tests.
- Handler error logging + 500: covered by tests and `slog.ErrorContext`.
- Protected system headers: covered by tests.
- Exhaustive TDD and review: each task starts with failing tests where code behavior changes, then implementation, then verification.

Placeholder scan:

- No `TBD`.
- No `TODO`.
- No "fill in later".
- No undefined helper names in implementation snippets except existing repo utilities and types verified in the current codebase.

Type consistency:

- `LegacyDocumentHandler`, `LegacyDocumentRequest`, and `LegacyDocumentResponse` are defined in `port` and aliased in `sdk`.
- `SetLegacyDocumentHandler` stores `port.LegacyDocumentHandler`.
- `NewLegacyDocumentController` accepts `port.LegacyDocumentHandler`.
- `registerLegacyDocumentRoute` passes `cfg.LegacyDocuments.MaxBodyBytesOrDefault()`.
