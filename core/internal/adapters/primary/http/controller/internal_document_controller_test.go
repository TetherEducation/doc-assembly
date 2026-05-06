//go:build integration

package controller_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	"github.com/rendis/doc-assembly/core/internal/testing/testhelper"
)

const systemWorkspaceCode = "SYS_WRKSP"

type resolverFunc func(context.Context, *port.TemplateResolverRequest, port.InternalTemplateContextSearchAdapter) (*port.InternalTemplateContext, error)

func (f resolverFunc) Resolve(ctx context.Context, req *port.TemplateResolverRequest, adapter port.InternalTemplateContextSearchAdapter) (*port.InternalTemplateContext, error) {
	return f(ctx, req, adapter)
}

type internalCreateEnv struct {
	ts               *testhelper.TestServer
	client           *testhelper.HTTPClient
	pool             *pgxpool.Pool
	tenantID         string
	tenantCode       string
	workspaceID      string
	workspaceCode    string
	documentTypeID   string
	documentTypeCode string
}

func setupInternalCreateEnv(t *testing.T, templateResolver port.TemplateResolver, createTenantDocType bool) *internalCreateEnv {
	t.Helper()

	pool := testhelper.GetTestPool(t)
	ts := testhelper.NewTestServerWithResolver(t, pool, templateResolver)
	client := testhelper.NewHTTPClient(t, ts.URL())

	tenantCode := fmt.Sprintf("I%09d", time.Now().UnixNano()%1_000_000_000)
	tenantID := testhelper.CreateTestTenant(t, pool, "Internal Tenant", tenantCode)
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, tenantID) })

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Internal Client Workspace", entity.WorkspaceTypeClient)
	workspaceCode := getWorkspaceCode(t, pool, workspaceID)
	t.Cleanup(func() { testhelper.CleanupWorkspace(t, pool, workspaceID) })

	docTypeCode := fmt.Sprintf("DOC_%d", time.Now().UnixNano())
	docTypeID := ""
	if createTenantDocType {
		docTypeID = testhelper.CreateTestDocumentType(t, pool, tenantID, docTypeCode, "Contract")
		t.Cleanup(func() { testhelper.CleanupDocumentType(t, pool, docTypeID) })
	}

	return &internalCreateEnv{
		ts:               ts,
		client:           client,
		pool:             pool,
		tenantID:         tenantID,
		tenantCode:       tenantCode,
		workspaceID:      workspaceID,
		workspaceCode:    workspaceCode,
		documentTypeID:   docTypeID,
		documentTypeCode: docTypeCode,
	}
}

func (e *internalCreateEnv) createPublishedTemplate(t *testing.T, workspaceID, documentTypeID string) (string, string) {
	t.Helper()

	templateID := testhelper.CreateTestTemplate(t, e.pool, workspaceID, "Internal Template", nil)
	testhelper.SetTemplateDocumentType(t, e.pool, templateID, documentTypeID)
	versionID := testhelper.CreateTestTemplateVersion(t, e.pool, templateID, 1, "v1", entity.VersionStatusDraft)
	testhelper.PublishTestVersion(t, e.pool, versionID)
	setTemplateVersionContentInternal(t, e.pool, versionID, `{}`)

	t.Cleanup(func() { testhelper.CleanupTemplate(t, e.pool, templateID) })
	return templateID, versionID
}

func (e *internalCreateEnv) postCreate(
	t *testing.T,
	docTypeCode,
	externalID,
	transactionalID string,
	forceCreate *bool,
	supersedeReason *string,
	payload any,
) (*http.Response, []byte, dto.InternalCreateDocumentWithRecipientsResponse) {
	t.Helper()
	return e.postCreateWithEnv(t, "prod", docTypeCode, externalID, transactionalID, forceCreate, supersedeReason, payload)
}

func (e *internalCreateEnv) postCreateWithEnv(
	t *testing.T,
	environment,
	docTypeCode,
	externalID,
	transactionalID string,
	forceCreate *bool,
	supersedeReason *string,
	payload any,
) (*http.Response, []byte, dto.InternalCreateDocumentWithRecipientsResponse) {
	t.Helper()

	req := map[string]any{
		"payload": payload,
	}
	if forceCreate != nil {
		req["forceCreate"] = *forceCreate
	}
	if supersedeReason != nil {
		req["supersedeReason"] = *supersedeReason
	}

	resp, body := e.client.
		WithHeader("X-API-Key", testhelper.TestInternalAPIKey).
		WithHeader("X-Tenant-Code", e.tenantCode).
		WithHeader("X-Workspace-Code", e.workspaceCode).
		WithHeader("X-Document-Type", docTypeCode).
		WithHeader("X-External-ID", externalID).
		WithHeader("X-Transactional-ID", transactionalID).
		WithHeader("X-Environment", environment).
		POST("/api/v1/internal/documents/create", req)

	var parsed dto.InternalCreateDocumentWithRecipientsResponse
	if resp.StatusCode == http.StatusCreated || resp.StatusCode == http.StatusOK {
		require.NoError(t, json.Unmarshal(body, &parsed), "body: %s", string(body))
	}

	return resp, body, parsed
}

func (e *internalCreateEnv) postCreateRaw(
	t *testing.T,
	headers map[string]string,
	payload any,
) (*http.Response, []byte) {
	t.Helper()

	req := map[string]any{
		"payload": payload,
	}

	c := e.client
	for k, v := range headers {
		c = c.WithHeader(k, v)
	}

	return c.POST("/api/v1/internal/documents/create", req)
}

func TestInternalDocumentController_CreateAndReplay(t *testing.T) {
	env := setupInternalCreateEnv(t, nil, true)
	_, versionID := env.createPublishedTemplate(t, env.workspaceID, env.documentTypeID)

	resp, body, first := env.postCreate(t, env.documentTypeCode, "ext-1", "tx-1", nil, nil, map[string]any{"customer": "ACME"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	assert.Equal(t, versionID, first.TemplateVersionID)
	assert.False(t, first.IdempotentReplay)

	resp, body, replay := env.postCreate(t, env.documentTypeCode, "ext-1", "tx-1", nil, nil, map[string]any{"customer": "ACME"})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.True(t, replay.IdempotentReplay)
	assert.Equal(t, first.ID, replay.ID)

	assert.Equal(t, 1, countActiveDocumentsByLogicalKey(t, env.pool, env.workspaceID, env.documentTypeID, "ext-1"))
}

func TestInternalDocumentController_StrictReplayByTransactionalIDMismatch(t *testing.T) {
	env := setupInternalCreateEnv(t, nil, true)
	env.createPublishedTemplate(t, env.workspaceID, env.documentTypeID)

	resp, body, first := env.postCreate(t, env.documentTypeCode, "ext-A", "tx-shared", nil, nil, map[string]any{"k": "v1"})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))

	resp, body, replay := env.postCreate(t, env.documentTypeCode, "ext-B", "tx-shared", nil, nil, map[string]any{"k": "v2"})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.True(t, replay.IdempotentReplay)
	assert.Equal(t, first.ID, replay.ID)
}

func TestInternalDocumentController_NoForceReturnsActiveDocument(t *testing.T) {
	env := setupInternalCreateEnv(t, nil, true)
	env.createPublishedTemplate(t, env.workspaceID, env.documentTypeID)

	_, _, first := env.postCreate(t, env.documentTypeCode, "ext-1", "tx-1", nil, nil, map[string]any{"a": 1})

	resp, body, replay := env.postCreate(t, env.documentTypeCode, "ext-1", "tx-2", nil, nil, map[string]any{"a": 2})
	require.Equal(t, http.StatusOK, resp.StatusCode, string(body))
	assert.True(t, replay.IdempotentReplay)
	assert.Equal(t, first.ID, replay.ID)
}

func TestInternalDocumentController_ForceCreateSupersedesPrevious(t *testing.T) {
	env := setupInternalCreateEnv(t, nil, true)
	env.createPublishedTemplate(t, env.workspaceID, env.documentTypeID)

	_, _, first := env.postCreate(t, env.documentTypeCode, "ext-1", "tx-1", nil, nil, map[string]any{"a": 1})

	force := true
	reason := "manual reissue"
	resp, body, second := env.postCreate(t, env.documentTypeCode, "ext-1", "tx-2", &force, &reason, map[string]any{"a": 2})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
	require.NotEqual(t, first.ID, second.ID)
	require.NotNil(t, second.SupersededPreviousDocumentID)
	assert.Equal(t, first.ID, *second.SupersededPreviousDocumentID)

	prev := loadDocumentSupersedeState(t, env.pool, first.ID)
	assert.False(t, prev.IsActive)
	require.NotNil(t, prev.SupersededByDocumentID)
	assert.Equal(t, second.ID, *prev.SupersededByDocumentID)
	require.NotNil(t, prev.SupersedeReason)
	assert.Equal(t, reason, *prev.SupersedeReason)
	require.NotNil(t, prev.SupersededAt)

	assert.Equal(t, 1, countActiveDocumentsByLogicalKey(t, env.pool, env.workspaceID, env.documentTypeID, "ext-1"))
}

func TestInternalDocumentController_ConcurrentForceCreatePreservesSingleActive(t *testing.T) {
	env := setupInternalCreateEnv(t, nil, true)
	env.createPublishedTemplate(t, env.workspaceID, env.documentTypeID)
	force := true

	resp, body, _ := env.postCreate(t, env.documentTypeCode, "ext-concurrent", "tx-init", nil, nil, map[string]any{"seed": true})
	require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))

	const workers = 6
	var wg sync.WaitGroup
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			txID := fmt.Sprintf("tx-force-%d", i)
			resp, body, _ := env.postCreate(t, env.documentTypeCode, "ext-concurrent", txID, &force, nil, map[string]any{"idx": i})
			if resp.StatusCode != http.StatusCreated {
				errCh <- fmt.Errorf("unexpected status %d body=%s", resp.StatusCode, string(body))
			}
		}(i)
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	assert.Equal(t, 1, countActiveDocumentsByLogicalKey(t, env.pool, env.workspaceID, env.documentTypeID, "ext-concurrent"))
}

func TestInternalDocumentController_ConcurrentSameTransactionalID(t *testing.T) {
	env := setupInternalCreateEnv(t, nil, true)
	env.createPublishedTemplate(t, env.workspaceID, env.documentTypeID)

	const workers = 8
	var wg sync.WaitGroup
	statuses := make(chan int, workers)
	ids := make(chan string, workers)
	errCh := make(chan error, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			resp, body, out := env.postCreate(t, env.documentTypeCode, "ext-same-tx", "tx-same", nil, nil, map[string]any{"idx": i})
			if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
				errCh <- fmt.Errorf("unexpected status %d body=%s", resp.StatusCode, string(body))
				return
			}
			statuses <- resp.StatusCode
			ids <- out.ID
		}(i)
	}

	wg.Wait()
	close(statuses)
	close(ids)
	close(errCh)
	for err := range errCh {
		require.NoError(t, err)
	}

	count201 := 0
	count200 := 0
	uniqueIDs := map[string]struct{}{}
	for s := range statuses {
		if s == http.StatusCreated {
			count201++
		}
		if s == http.StatusOK {
			count200++
		}
	}
	for id := range ids {
		uniqueIDs[id] = struct{}{}
	}

	assert.Equal(t, 1, count201)
	assert.Equal(t, workers-1, count200)
	assert.Len(t, uniqueIDs, 1)
	assert.Equal(t, 1, countDocumentsByTransactionalID(t, env.pool, env.workspaceID, "tx-same"))
}

func TestInternalDocumentController_DefaultResolverWorkspaceOnly(t *testing.T) {
	t.Run("prod resolves tenant workspace", func(t *testing.T) {
		env := setupInternalCreateEnv(t, nil, true)
		_, versionID := env.createPublishedTemplate(t, env.workspaceID, env.documentTypeID)

		resp, body, out := env.postCreate(t, env.documentTypeCode, "ext-l1", "tx-l1", nil, nil, map[string]any{"level": 1})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		assert.Equal(t, versionID, out.TemplateVersionID)
	})

	t.Run("prod does not fall back to tenant SYS_WRKSP", func(t *testing.T) {
		env := setupInternalCreateEnv(t, nil, true)
		tenantSystemWorkspaceID := getTenantSystemWorkspaceID(t, env.pool, env.tenantID)
		env.createPublishedTemplate(t, tenantSystemWorkspaceID, env.documentTypeID)

		resp, body, _ := env.postCreate(t, env.documentTypeCode, "ext-l2", "tx-l2", nil, nil, map[string]any{"level": 2})
		require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
	})

	t.Run("prod does not fall back to SYS tenant SYS_WRKSP", func(t *testing.T) {
		env := setupInternalCreateEnv(t, nil, false)
		sysTenantID, sysWorkspaceID := getSystemTenantAndWorkspace(t, env.pool)

		docTypeCode := fmt.Sprintf("GLOB_%d", time.Now().UnixNano())
		sysDocTypeID := testhelper.CreateTestDocumentType(t, env.pool, sysTenantID, docTypeCode, "Global Contract")
		t.Cleanup(func() { testhelper.CleanupDocumentType(t, env.pool, sysDocTypeID) })

		env.createPublishedTemplate(t, sysWorkspaceID, sysDocTypeID)

		resp, body, _ := env.postCreate(t, docTypeCode, "ext-l3", "tx-l3", nil, nil, map[string]any{"level": 3})
		require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
	})
}

func TestInternalDocumentController_CustomResolver(t *testing.T) {
	t.Run("custom hit uses returned version", func(t *testing.T) {
		var forcedVersionID string
		resolver := resolverFunc(func(ctx context.Context, req *port.TemplateResolverRequest, adapter port.InternalTemplateContextSearchAdapter) (*port.InternalTemplateContext, error) {
			published := true
			return adapter.ResolveInternalTemplateContext(ctx, port.InternalTemplateContextSearchParams{
				TenantCode:             req.TenantCode,
				RequestedWorkspaceCode: req.WorkspaceCode,
				WorkspaceCodes:         []string{req.WorkspaceCode},
				DocumentType:           req.DocumentType,
				Process:                req.Process,
				Published:              &published,
			})
		})

		env := setupInternalCreateEnv(t, resolver, true)
		_, forcedVersionID = env.createPublishedTemplate(t, env.workspaceID, env.documentTypeID)

		resp, body, out := env.postCreate(t, env.documentTypeCode, "ext-custom-hit", "tx-custom-hit", nil, nil, map[string]any{"custom": true})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		assert.Equal(t, forcedVersionID, out.TemplateVersionID)
	})

	t.Run("custom resolver cannot resolve SYS_WRKSP templates", func(t *testing.T) {
		resolver := resolverFunc(func(ctx context.Context, req *port.TemplateResolverRequest, adapter port.InternalTemplateContextSearchAdapter) (*port.InternalTemplateContext, error) {
			published := true
			return adapter.ResolveInternalTemplateContext(ctx, port.InternalTemplateContextSearchParams{
				TenantCode:             req.TenantCode,
				RequestedWorkspaceCode: req.WorkspaceCode,
				WorkspaceCodes:         []string{systemWorkspaceCode},
				DocumentType:           req.DocumentType,
				Process:                req.Process,
				Published:              &published,
			})
		})

		env := setupInternalCreateEnv(t, resolver, true)
		tenantSystemWorkspaceID := getTenantSystemWorkspaceID(t, env.pool, env.tenantID)
		env.createPublishedTemplate(t, tenantSystemWorkspaceID, env.documentTypeID)

		resp, body, _ := env.postCreate(t, env.documentTypeCode, "ext-custom-system", "tx-custom-system", nil, nil, map[string]any{"custom": "system"})
		require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
	})

	t.Run("custom nil falls back to default", func(t *testing.T) {
		resolver := resolverFunc(func(_ context.Context, _ *port.TemplateResolverRequest, _ port.InternalTemplateContextSearchAdapter) (*port.InternalTemplateContext, error) {
			return nil, nil
		})

		env := setupInternalCreateEnv(t, resolver, true)
		_, expectedVersionID := env.createPublishedTemplate(t, env.workspaceID, env.documentTypeID)

		resp, body, out := env.postCreate(t, env.documentTypeCode, "ext-custom-nil", "tx-custom-nil", nil, nil, map[string]any{"custom": "nil"})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		assert.Equal(t, expectedVersionID, out.TemplateVersionID)
	})

	t.Run("custom error aborts request", func(t *testing.T) {
		resolver := resolverFunc(func(_ context.Context, _ *port.TemplateResolverRequest, _ port.InternalTemplateContextSearchAdapter) (*port.InternalTemplateContext, error) {
			return nil, errors.New("resolver failed")
		})

		env := setupInternalCreateEnv(t, resolver, true)
		env.createPublishedTemplate(t, env.workspaceID, env.documentTypeID)

		resp, body, _ := env.postCreate(t, env.documentTypeCode, "ext-custom-err", "tx-custom-err", nil, nil, map[string]any{"custom": "err"})
		require.Equal(t, http.StatusInternalServerError, resp.StatusCode, string(body))
	})
}

func TestInternalDocumentController_LegacyContractAndMissingV2(t *testing.T) {
	env := setupInternalCreateEnv(t, nil, true)
	env.createPublishedTemplate(t, env.workspaceID, env.documentTypeID)

	resp, body := env.client.
		WithHeader("X-API-Key", testhelper.TestInternalAPIKey).
		WithHeader("X-Template-ID", "legacy-template").
		POST("/api/v1/internal/documents/create", map[string]any{"payload": map[string]any{"k": "v"}})
	require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))

	resp, body = env.client.
		WithHeader("X-API-Key", testhelper.TestInternalAPIKey).
		POST("/api/v1/internal/documents/create-v2", map[string]any{"payload": map[string]any{"k": "v"}})
	require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
}

func TestInternalDocumentController_EnvironmentHeaderValidation(t *testing.T) {
	t.Run("missing X-Environment returns 400", func(t *testing.T) {
		env := setupInternalCreateEnv(t, nil, true)
		env.createPublishedTemplate(t, env.workspaceID, env.documentTypeID)

		resp, body := env.postCreateRaw(t, map[string]string{
			"X-API-Key":          testhelper.TestInternalAPIKey,
			"X-Tenant-Code":      env.tenantCode,
			"X-Workspace-Code":   env.workspaceCode,
			"X-Document-Type":    env.documentTypeCode,
			"X-External-ID":      "ext-no-env",
			"X-Transactional-ID": "tx-no-env",
		}, map[string]any{"k": "v"})

		require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))

		var errResp dto.InternalErrorResponse
		require.NoError(t, json.Unmarshal(body, &errResp))
		assert.Equal(t, "MISSING_HEADERS", errResp.Code)
		assert.Contains(t, errResp.Details, "X-Environment")
	})

	t.Run("invalid X-Environment returns 400", func(t *testing.T) {
		env := setupInternalCreateEnv(t, nil, true)
		env.createPublishedTemplate(t, env.workspaceID, env.documentTypeID)

		resp, body, _ := env.postCreateWithEnv(t, "invalid-env", env.documentTypeCode, "ext-invalid-env", "tx-invalid-env", nil, nil, map[string]any{"k": "v"})
		require.Equal(t, http.StatusBadRequest, resp.StatusCode, string(body))

		var errResp dto.InternalErrorResponse
		require.NoError(t, json.Unmarshal(body, &errResp))
		assert.Equal(t, "INVALID_HEADER", errResp.Code)
	})

	t.Run("env prod creates document normally", func(t *testing.T) {
		env := setupInternalCreateEnv(t, nil, true)
		_, versionID := env.createPublishedTemplate(t, env.workspaceID, env.documentTypeID)

		resp, body, out := env.postCreateWithEnv(t, "prod", env.documentTypeCode, "ext-prod", "tx-prod", nil, nil, map[string]any{"env": "prod"})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		assert.Equal(t, versionID, out.TemplateVersionID)
	})
}

func TestInternalDocumentController_EnvironmentDevSandboxFirst(t *testing.T) {
	t.Run("dev resolves sandbox template first", func(t *testing.T) {
		env := setupInternalCreateEnv(t, nil, true)

		// Create sandbox workspace for the prod workspace
		sandboxID := testhelper.CreateTestSandboxWorkspace(t, env.pool, env.workspaceID)
		t.Cleanup(func() { testhelper.CleanupWorkspace(t, env.pool, sandboxID) })

		// Publish template ONLY in sandbox workspace
		_, sandboxVersionID := env.createPublishedTemplate(t, sandboxID, env.documentTypeID)

		resp, body, out := env.postCreateWithEnv(t, "dev", env.documentTypeCode, "ext-sbx-hit", "tx-sbx-hit", nil, nil, map[string]any{"sandbox": true})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		assert.Equal(t, sandboxVersionID, out.TemplateVersionID)
	})

	t.Run("dev does not fall back to prod when sandbox has no template", func(t *testing.T) {
		env := setupInternalCreateEnv(t, nil, true)

		// Create sandbox workspace (empty, no template)
		sandboxID := testhelper.CreateTestSandboxWorkspace(t, env.pool, env.workspaceID)
		t.Cleanup(func() { testhelper.CleanupWorkspace(t, env.pool, sandboxID) })

		// Publish template ONLY in prod workspace
		env.createPublishedTemplate(t, env.workspaceID, env.documentTypeID)

		resp, body, _ := env.postCreateWithEnv(t, "dev", env.documentTypeCode, "ext-sbx-miss", "tx-sbx-miss", nil, nil, map[string]any{"sandbox": false})
		require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
	})

	t.Run("dev without sandbox workspace returns not found", func(t *testing.T) {
		env := setupInternalCreateEnv(t, nil, true)

		// No sandbox workspace created — just prod template
		env.createPublishedTemplate(t, env.workspaceID, env.documentTypeID)

		resp, body, _ := env.postCreateWithEnv(t, "dev", env.documentTypeCode, "ext-no-sbx", "tx-no-sbx", nil, nil, map[string]any{"no_sandbox": true})
		require.Equal(t, http.StatusNotFound, resp.StatusCode, string(body))
	})
}

func TestInternalDocumentController_EnvironmentPropagatedToResolver(t *testing.T) {
	t.Run("dev environment reaches custom resolver", func(t *testing.T) {
		var receivedEnv entity.Environment
		var receivedSandboxCode string
		resolver := resolverFunc(func(_ context.Context, req *port.TemplateResolverRequest, _ port.InternalTemplateContextSearchAdapter) (*port.InternalTemplateContext, error) {
			receivedEnv = req.Environment
			receivedSandboxCode = req.SandboxWorkspaceCode
			return nil, nil // fall back to default
		})

		env := setupInternalCreateEnv(t, resolver, true)
		sandboxID := testhelper.CreateTestSandboxWorkspace(t, env.pool, env.workspaceID)
		t.Cleanup(func() { testhelper.CleanupWorkspace(t, env.pool, sandboxID) })
		env.createPublishedTemplate(t, sandboxID, env.documentTypeID)

		sandboxCode := getWorkspaceCode(t, env.pool, sandboxID)

		resp, body, _ := env.postCreateWithEnv(t, "dev", env.documentTypeCode, "ext-resolver-dev", "tx-resolver-dev", nil, nil, map[string]any{"test": true})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		assert.Equal(t, entity.EnvironmentDev, receivedEnv)
		assert.Equal(t, sandboxCode, receivedSandboxCode)
	})

	t.Run("prod environment reaches custom resolver without sandbox code", func(t *testing.T) {
		var receivedEnv entity.Environment
		var receivedSandboxCode string
		resolver := resolverFunc(func(_ context.Context, req *port.TemplateResolverRequest, _ port.InternalTemplateContextSearchAdapter) (*port.InternalTemplateContext, error) {
			receivedEnv = req.Environment
			receivedSandboxCode = req.SandboxWorkspaceCode
			return nil, nil
		})

		env := setupInternalCreateEnv(t, resolver, true)
		env.createPublishedTemplate(t, env.workspaceID, env.documentTypeID)

		resp, body, _ := env.postCreateWithEnv(t, "prod", env.documentTypeCode, "ext-resolver-prod", "tx-resolver-prod", nil, nil, map[string]any{"test": true})
		require.Equal(t, http.StatusCreated, resp.StatusCode, string(body))
		assert.Equal(t, entity.EnvironmentProd, receivedEnv)
		assert.Empty(t, receivedSandboxCode)
	})
}

func setTemplateVersionContentInternal(t *testing.T, pool *pgxpool.Pool, versionID, content string) {
	t.Helper()
	_, err := pool.Exec(
		context.Background(),
		`UPDATE content.template_versions SET content_structure = $1 WHERE id = $2`,
		[]byte(content),
		versionID,
	)
	require.NoError(t, err)
}

func getWorkspaceCode(t *testing.T, pool *pgxpool.Pool, workspaceID string) string {
	t.Helper()
	var code string
	err := pool.QueryRow(context.Background(), `SELECT code FROM tenancy.workspaces WHERE id = $1`, workspaceID).Scan(&code)
	require.NoError(t, err)
	return code
}

func getTenantSystemWorkspaceID(t *testing.T, pool *pgxpool.Pool, tenantID string) string {
	t.Helper()
	var workspaceID string
	err := pool.QueryRow(context.Background(), `
		SELECT id
		FROM tenancy.workspaces
		WHERE tenant_id = $1
		  AND code = 'SYS_WRKSP'
		  AND is_sandbox = FALSE
		LIMIT 1
	`, tenantID).Scan(&workspaceID)
	require.NoError(t, err)
	return workspaceID
}

func getSystemTenantAndWorkspace(t *testing.T, pool *pgxpool.Pool) (string, string) {
	t.Helper()
	var tenantID string
	err := pool.QueryRow(context.Background(), `SELECT id FROM tenancy.tenants WHERE code = 'SYS' LIMIT 1`).Scan(&tenantID)
	require.NoError(t, err)

	var workspaceID string
	err = pool.QueryRow(context.Background(), `
		SELECT id
		FROM tenancy.workspaces
		WHERE tenant_id = $1
		  AND code = 'SYS_WRKSP'
		  AND is_sandbox = FALSE
		LIMIT 1
	`, tenantID).Scan(&workspaceID)
	require.NoError(t, err)

	return tenantID, workspaceID
}

func countActiveDocumentsByLogicalKey(t *testing.T, pool *pgxpool.Pool, workspaceID, documentTypeID, externalID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM execution.documents
		WHERE workspace_id = $1
		  AND document_type_id = $2
		  AND client_external_reference_id = $3
		  AND is_active = TRUE
	`, workspaceID, documentTypeID, externalID).Scan(&count)
	require.NoError(t, err)
	return count
}

func countDocumentsByTransactionalID(t *testing.T, pool *pgxpool.Pool, workspaceID, transactionalID string) int {
	t.Helper()
	var count int
	err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM execution.documents
		WHERE workspace_id = $1
		  AND transactional_id = $2
	`, workspaceID, transactionalID).Scan(&count)
	require.NoError(t, err)
	return count
}

type documentSupersedeState struct {
	IsActive               bool
	SupersededByDocumentID *string
	SupersedeReason        *string
	SupersededAt           *time.Time
}

func loadDocumentSupersedeState(t *testing.T, pool *pgxpool.Pool, documentID string) *documentSupersedeState {
	t.Helper()

	state := &documentSupersedeState{}
	err := pool.QueryRow(context.Background(), `
		SELECT is_active, superseded_by_document_id, supersede_reason, superseded_at
		FROM execution.documents
		WHERE id = $1
	`, documentID).Scan(
		&state.IsActive,
		&state.SupersededByDocumentID,
		&state.SupersedeReason,
		&state.SupersededAt,
	)
	require.NoError(t, err)

	return state
}
