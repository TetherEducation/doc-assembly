package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

type fakeVersionUseCase struct {
	versions    []*entity.TemplateVersion
	details     *entity.TemplateVersionWithDetails
	listErr     error
	detailErr   error
	listedFor   string
	detailedFor string
}

func (f *fakeVersionUseCase) ListVersions(_ context.Context, templateID string) ([]*entity.TemplateVersion, error) {
	f.listedFor = templateID
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.versions, nil
}

func (f *fakeVersionUseCase) GetVersionWithDetails(_ context.Context, id string) (*entity.TemplateVersionWithDetails, error) {
	f.detailedFor = id
	if f.detailErr != nil {
		return nil, f.detailErr
	}
	return f.details, nil
}

type fakeVersionMapper struct{}

func (fakeVersionMapper) ToListResponse(versions []*entity.TemplateVersion) *dto.ListTemplateVersionsResponse {
	return &dto.ListTemplateVersionsResponse{Total: len(versions)}
}

func (fakeVersionMapper) ToDetailResponse(details *entity.TemplateVersionWithDetails) *dto.TemplateVersionDetailResponse {
	if details == nil {
		return nil
	}
	return &dto.TemplateVersionDetailResponse{}
}

func newTemplateRequest(method, target string, params gin.Params) (*gin.Context, *httptest.ResponseRecorder) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(method, target, nil)
	ctx.Params = params
	return ctx, recorder
}

func TestListVersionsPassesTemplateIDThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc := &fakeVersionUseCase{versions: []*entity.TemplateVersion{{ID: "v1"}, {ID: "v2"}}}
	controller := NewInternalTemplateController(nil, uc, fakeVersionMapper{})

	ctx, recorder := newTemplateRequest(
		http.MethodGet,
		"/api/v1/internal/templates/tpl-1/versions",
		gin.Params{{Key: "templateId", Value: "tpl-1"}},
	)
	controller.ListVersions(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if uc.listedFor != "tpl-1" {
		t.Fatalf("expected the template id to reach the use case, got %q", uc.listedFor)
	}
	var body dto.ListTemplateVersionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if body.Total != 2 {
		t.Fatalf("expected 2 versions, got %d", body.Total)
	}
}

func TestGetVersionContentUsesVersionIDNotTemplateID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	uc := &fakeVersionUseCase{details: &entity.TemplateVersionWithDetails{}}
	controller := NewInternalTemplateController(nil, uc, fakeVersionMapper{})

	ctx, recorder := newTemplateRequest(
		http.MethodGet,
		"/api/v1/internal/templates/tpl-1/versions/ver-9/content",
		gin.Params{{Key: "templateId", Value: "tpl-1"}, {Key: "versionId", Value: "ver-9"}},
	)
	controller.GetVersionContent(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	// Both ids are in the path; looking up by the wrong one would silently return
	// someone else's content, so it is worth pinning.
	if uc.detailedFor != "ver-9" {
		t.Fatalf("expected lookup by version id, got %q", uc.detailedFor)
	}
}

// The two endpoints report a miss differently, and the CRM depends on the distinction:
// a version that does not exist is a 404, while listing an unknown template is simply an
// empty list. Pinned so nobody later "fixes" one to match the other.
func TestMissingVersionIs404(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, recorder := newTemplateRequest(
		http.MethodGet, "/x", gin.Params{{Key: "versionId", Value: "missing"}},
	)
	NewInternalTemplateController(nil, &fakeVersionUseCase{detailErr: entity.ErrVersionNotFound}, fakeVersionMapper{}).
		GetVersionContent(ctx)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestListingAnUnknownTemplateIsAnEmptyList(t *testing.T) {
	gin.SetMode(gin.TestMode)

	ctx, recorder := newTemplateRequest(
		http.MethodGet, "/x", gin.Params{{Key: "templateId", Value: "unknown"}},
	)
	NewInternalTemplateController(nil, &fakeVersionUseCase{}, fakeVersionMapper{}).ListVersions(ctx)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	var body dto.ListTemplateVersionsResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding response: %v", err)
	}
	if body.Total != 0 {
		t.Fatalf("expected an empty list, got %d", body.Total)
	}
}

func TestVersionEndpointsWithoutUseCaseAre500(t *testing.T) {
	gin.SetMode(gin.TestMode)

	listCtx, listRecorder := newTemplateRequest(http.MethodGet, "/x", nil)
	(&InternalTemplateController{}).ListVersions(listCtx)
	if listRecorder.Code != http.StatusInternalServerError {
		t.Fatalf("list: expected 500, got %d", listRecorder.Code)
	}

	detailCtx, detailRecorder := newTemplateRequest(http.MethodGet, "/x", nil)
	(&InternalTemplateController{}).GetVersionContent(detailCtx)
	if detailRecorder.Code != http.StatusInternalServerError {
		t.Fatalf("content: expected 500, got %d", detailRecorder.Code)
	}
}
