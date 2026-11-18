package riverqueue

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// recordingRenderer records the render requests it receives.
type recordingRenderer struct {
	port.PDFRenderer
	requests []*port.RenderPreviewRequest
}

func (r *recordingRenderer) RenderPreview(_ context.Context, req *port.RenderPreviewRequest) (*port.RenderPreviewResult, error) {
	r.requests = append(r.requests, req)
	return &port.RenderPreviewResult{PDF: []byte("%PDF-1.7")}, nil
}

// The PDF to sign is rendered for the document's workspace. The renderer
// resolves stored images for that workspace only.
func TestRenderAttemptArtifactsRendersForTheDocumentsWorkspace(t *testing.T) {
	const workspaceID = "5f0c9a52-7c3e-4c7b-9d55-2f4f6a1f0e11"
	content, err := json.Marshal(map[string]any{
		"version": "1.2.0",
		"meta":    map[string]any{"title": "Carta", "language": "es"},
	})
	if err != nil {
		t.Fatal(err)
	}
	renderer := &recordingRenderer{}
	executor := NewSigningAttemptExecutor(SigningAttemptExecutorConfig{
		VersionRepo:       stubVersionRepo{version: &entity.TemplateVersion{ID: "version-1", ContentStructure: content}},
		RecipientRepo:     stubRecipientRepo{},
		SignerRoleRepo:    stubSignerRoleRepo{},
		FieldResponseRepo: stubFieldResponseRepo{},
		PDFRenderer:       renderer,
	})

	_, _, _, err = executor.renderAttemptArtifacts(context.Background(), &entity.Document{
		ID:                "doc-1",
		WorkspaceID:       workspaceID,
		TemplateVersionID: "version-1",
	})
	if err != nil {
		t.Fatalf("renderAttemptArtifacts() error = %v", err)
	}

	if len(renderer.requests) != 1 {
		t.Fatalf("render requests = %d, want 1", len(renderer.requests))
	}
	if got := renderer.requests[0].WorkspaceID; got != workspaceID {
		t.Errorf("render request WorkspaceID = %q, want the document's workspace %q", got, workspaceID)
	}
}
