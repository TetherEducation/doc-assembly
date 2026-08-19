package controller

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
)

func newResolveRequest(headers map[string]string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/internal/templates/resolve", nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return req
}

func resolveHeaders() map[string]string {
	return map[string]string{
		HeaderTenantCode:    "CL",
		HeaderWorkspaceCode: "2036400001",
		HeaderDocumentType:  "EDUCATIONAL_SERVICES_AGREEMENT",
		HeaderEnvironment:   "prod",
	}
}

func TestResolveTemplateReturnsResolution(t *testing.T) {
	gin.SetMode(gin.TestMode)

	updatedAt := time.Date(2026, 8, 16, 14, 1, 0, 0, time.UTC)
	uc := &fakeInternalDocumentUseCase{
		resolveResult: &documentuc.InternalResolveTemplateResult{
			TenantCode:             "CL",
			RequestedWorkspaceCode: "2036400001",
			ResolvedWorkspaceCode:  "CORMUN",
			DocumentType:           "EDUCATIONAL_SERVICES_AGREEMENT",
			TemplateID:             "tpl-1",
			VersionID:              "ver-1",
			VersionNumber:          2,
			VersionStatus:          "PUBLISHED",
			UpdatedAt:              &updatedAt,
			SignerRoles: []documentuc.InternalResolvedSignerRole{
				{RoleName: "Socio Educativo", SignerOrder: 1, AnchorString: "__sig_socio_educativo__"},
			},
			Injectables: []documentuc.InternalResolvedInjectable{
				{Key: "legalguardian_id_number", Label: "RUT Apoderado/a", IsRequired: true},
			},
		},
	}
	controller := NewInternalDocumentController(uc)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = newResolveRequest(resolveHeaders())
	controller.ResolveTemplate(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var body dto.InternalResolveTemplateResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	// The requested and resolved workspaces differing is the whole point of the endpoint:
	// this campus is served its network's template, not its own.
	if body.RequestedWorkspaceCode != "2036400001" || body.ResolvedWorkspaceCode != "CORMUN" {
		t.Fatalf("expected requested 2036400001 resolved CORMUN, got %q and %q",
			body.RequestedWorkspaceCode, body.ResolvedWorkspaceCode)
	}
	if body.VersionNumber != 2 || body.VersionStatus != "PUBLISHED" {
		t.Fatalf("unexpected version projection: %+v", body)
	}
	if len(body.SignerRoles) != 1 || body.SignerRoles[0].RoleName != "Socio Educativo" {
		t.Fatalf("unexpected signer roles: %+v", body.SignerRoles)
	}
	if len(body.Injectables) != 1 || !body.Injectables[0].IsRequired {
		t.Fatalf("unexpected injectables: %+v", body.Injectables)
	}
}

func TestResolveTemplateRejectsMissingHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)

	for _, omitted := range []string{
		HeaderTenantCode, HeaderWorkspaceCode, HeaderDocumentType, HeaderEnvironment,
	} {
		headers := resolveHeaders()
		delete(headers, omitted)

		uc := &fakeInternalDocumentUseCase{}
		controller := NewInternalDocumentController(uc)
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = newResolveRequest(headers)
		controller.ResolveTemplate(ctx)

		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("omitting %s: expected 400, got %d", omitted, recorder.Code)
		}
		if len(uc.resolveReceived) != 0 {
			t.Fatalf("omitting %s: use case should not have been called", omitted)
		}
	}
}

// A campus with no template anywhere in its fallback chain is a real answer, and has to
// read as 404 rather than a server fault — the CRM distinguishes "no contract configured"
// from "we could not ask".
func TestResolveTemplateNotFoundMapsTo404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc := &fakeInternalDocumentUseCase{resolveErr: entity.ErrInternalTemplateResolutionNotFound}
	controller := NewInternalDocumentController(uc)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = newResolveRequest(resolveHeaders())
	controller.ResolveTemplate(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestResolveTemplateNormalisesProcessAndPassesEnvironment(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc := &fakeInternalDocumentUseCase{}
	controller := NewInternalDocumentController(uc)

	headers := resolveHeaders()
	headers[HeaderEnvironment] = "dev"
	headers[HeaderProcess] = "ENROLLMENT"

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = newResolveRequest(headers)
	controller.ResolveTemplate(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if len(uc.resolveReceived) != 1 {
		t.Fatalf("expected one call, got %d", len(uc.resolveReceived))
	}
	got := uc.resolveReceived[0]
	if got.Environment != entity.Environment("dev") {
		t.Fatalf("expected dev environment to reach the use case, got %q", got.Environment)
	}
	if got.Process != "ENROLLMENT" {
		t.Fatalf("expected process to reach the use case, got %q", got.Process)
	}
}

func TestResolveTemplateWithoutUseCaseIs500(t *testing.T) {
	gin.SetMode(gin.TestMode)

	controller := &InternalDocumentController{}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = newResolveRequest(resolveHeaders())
	controller.ResolveTemplate(ctx)

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", recorder.Code)
	}
	var body dto.InternalErrorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if body.Code != "INTERNAL_ERROR" {
		t.Fatalf("unexpected error code %q", body.Code)
	}
}
