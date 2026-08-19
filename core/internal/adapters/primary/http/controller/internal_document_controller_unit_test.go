package controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
)

type fakeInternalDocumentUseCase struct {
	results         []*documentuc.InternalCreateResult
	err             error
	calls           int
	received        []documentuc.InternalCreateCommand
	resolveResult   *documentuc.InternalResolveTemplateResult
	resolveErr      error
	resolveReceived []documentuc.InternalResolveTemplateCommand
}

func (f *fakeInternalDocumentUseCase) CreateDocument(_ context.Context, cmd documentuc.InternalCreateCommand) (*documentuc.InternalCreateResult, error) {
	f.calls++
	f.received = append(f.received, cmd)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.results) == 0 {
		return &documentuc.InternalCreateResult{}, nil
	}
	idx := f.calls - 1
	if idx >= len(f.results) {
		idx = len(f.results) - 1
	}
	return f.results[idx], nil
}

func (f *fakeInternalDocumentUseCase) ResetUnsignedDocument(ctx context.Context, cmd documentuc.InternalCreateCommand) (*documentuc.InternalCreateResult, error) {
	return f.CreateDocument(ctx, cmd)
}

func (f *fakeInternalDocumentUseCase) ResolveTemplate(
	_ context.Context,
	cmd documentuc.InternalResolveTemplateCommand,
) (*documentuc.InternalResolveTemplateResult, error) {
	f.resolveReceived = append(f.resolveReceived, cmd)
	if f.resolveErr != nil {
		return nil, f.resolveErr
	}
	if f.resolveResult == nil {
		return &documentuc.InternalResolveTemplateResult{}, nil
	}
	return f.resolveResult, nil
}

func TestValidateAndExtractHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.Header.Set(HeaderTenantCode, "TENANT")
	req.Header.Set(HeaderWorkspaceCode, "WORKSPACE")
	req.Header.Set(HeaderDocumentType, "DOC")
	req.Header.Set(HeaderExternalID, "EXT")
	req.Header.Set(HeaderTransactionalID, "TX")
	req.Header.Set(HeaderEnvironment, "prod")
	ctx.Request = req

	headers, missing := validateAndExtractHeaders(ctx)
	require.Empty(t, missing)
	assert.Equal(t, "TENANT", headers.TenantCode)
	assert.Equal(t, "WORKSPACE", headers.WorkspaceCode)
	assert.Equal(t, "DOC", headers.DocumentType)
	assert.Equal(t, "EXT", headers.ExternalID)
	assert.Equal(t, "TX", headers.TransactionalID)
}

func TestCreateDocument_MissingPayload(t *testing.T) {
	gin.SetMode(gin.TestMode)
	uc := &fakeInternalDocumentUseCase{}
	controller := NewInternalDocumentController(uc)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/internal/documents/create", strings.NewReader(`{"forceCreate":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderTenantCode, "TENANT")
	req.Header.Set(HeaderWorkspaceCode, "WORKSPACE")
	req.Header.Set(HeaderDocumentType, "DOC")
	req.Header.Set(HeaderExternalID, "EXT")
	req.Header.Set(HeaderTransactionalID, "TX")
	req.Header.Set(HeaderEnvironment, "prod")
	ctx.Request = req

	controller.CreateDocument(ctx)

	assert.Equal(t, http.StatusBadRequest, recorder.Code)
	assert.Equal(t, 0, uc.calls)
}

func TestCreateDocument_StatusCodesAndPayloadMapping(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc := &fakeInternalDocumentUseCase{
		results: []*documentuc.InternalCreateResult{
			{
				Document:         &entity.DocumentWithRecipients{Document: entity.Document{ID: "doc-1", WorkspaceID: "ws-1", TemplateVersionID: "tv-1", Status: entity.DocumentStatusDraft}},
				IdempotentReplay: false,
			},
			{
				Document:         &entity.DocumentWithRecipients{Document: entity.Document{ID: "doc-1", WorkspaceID: "ws-1", TemplateVersionID: "tv-1", Status: entity.DocumentStatusDraft}},
				IdempotentReplay: true,
			},
		},
	}
	controller := NewInternalDocumentController(uc)

	firstCode, firstCmd := executeCreateRequest(t, controller, `{"payload":{"customer":"ACME"},"forceCreate":true}`)
	assert.Equal(t, http.StatusCreated, firstCode)
	require.NotNil(t, firstCmd)
	assert.Equal(t, `{"customer":"ACME"}`, string(firstCmd.PayloadRaw))
	assert.True(t, firstCmd.ForceCreate)

	secondCode, _ := executeCreateRequest(t, controller, `{"payload":{"customer":"ACME"}}`)
	assert.Equal(t, http.StatusOK, secondCode)
}

func executeCreateRequest(t *testing.T, controller *InternalDocumentController, body string) (int, *documentuc.InternalCreateCommand) {
	t.Helper()

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, "/internal/documents/create", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(HeaderTenantCode, "TENANT")
	req.Header.Set(HeaderWorkspaceCode, "WORKSPACE")
	req.Header.Set(HeaderDocumentType, "DOC")
	req.Header.Set(HeaderExternalID, "EXT")
	req.Header.Set(HeaderTransactionalID, "TX")
	req.Header.Set(HeaderEnvironment, "prod")
	ctx.Request = req

	fakeUC, ok := controller.internalDocUC.(*fakeInternalDocumentUseCase)
	require.True(t, ok)
	callsBefore := fakeUC.calls

	controller.CreateDocument(ctx)

	if fakeUC.calls == callsBefore {
		return recorder.Code, nil
	}
	cmd := fakeUC.received[len(fakeUC.received)-1]
	return recorder.Code, &cmd
}
