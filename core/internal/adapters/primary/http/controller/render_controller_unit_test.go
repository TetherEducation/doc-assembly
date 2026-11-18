package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

// fakePreviewVersionUseCase serves one version. Methods the preview does not
// call fall through to the nil embedded interface and panic.
type fakePreviewVersionUseCase struct {
	templateuc.TemplateVersionUseCase
	details *entity.TemplateVersionWithDetails
}

func (f *fakePreviewVersionUseCase) GetVersionWithDetails(context.Context, string) (*entity.TemplateVersionWithDetails, error) {
	return f.details, nil
}

// recordingPDFRenderer records the render requests it receives.
type recordingPDFRenderer struct {
	port.PDFRenderer
	requests []*port.RenderPreviewRequest
}

func (r *recordingPDFRenderer) RenderPreview(_ context.Context, req *port.RenderPreviewRequest) (*port.RenderPreviewResult, error) {
	r.requests = append(r.requests, req)
	return &port.RenderPreviewResult{PDF: []byte("%PDF-1.7"), Filename: "preview.pdf"}, nil
}

// The preview is rendered for the caller's workspace, as the request context
// holds it after the workspace and sandbox middleware: the workspace of
// X-Workspace-ID or, in sandbox mode, its sandbox. The renderer resolves
// stored images for that workspace only.
func TestPreviewVersion_RendersForTheCallersWorkspace(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const workspaceID = "5f0c9a52-7c3e-4c7b-9d55-2f4f6a1f0e11"

	content, err := json.Marshal(map[string]any{
		"version": "1.2.0",
		"meta":    map[string]any{"title": "Carta", "language": "es"},
		"content": map[string]any{"type": "doc", "content": []any{}},
	})
	require.NoError(t, err)
	renderer := &recordingPDFRenderer{}
	ctrl := NewRenderController(&fakePreviewVersionUseCase{details: &entity.TemplateVersionWithDetails{
		TemplateVersion: entity.TemplateVersion{ID: "version-1", ContentStructure: content},
	}}, renderer)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/v1/content/templates/template-1/versions/version-1/preview",
		strings.NewReader(`{"injectables":{"client_name":"Jane Doe"}}`))
	ctx.Request.Header.Set("Content-Type", "application/json")
	ctx.Params = gin.Params{{Key: "templateId", Value: "template-1"}, {Key: "versionId", Value: "version-1"}}
	ctx.Set("workspace_id", workspaceID)

	ctrl.PreviewVersion(ctx)

	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	require.Len(t, renderer.requests, 1)
	assert.Equal(t, workspaceID, renderer.requests[0].WorkspaceID)
}
