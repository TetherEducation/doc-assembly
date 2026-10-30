package riverqueue

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

func TestPayloadString(t *testing.T) {
	raw := json.RawMessage(`{"language":"es","pdfChecksum":"abc","count":3}`)
	cases := []struct {
		name string
		raw  json.RawMessage
		key  string
		want string
	}{
		{"string value", raw, "language", "es"},
		{"non-string value", raw, "count", ""},
		{"missing key", raw, "redirectUrl", ""},
		{"empty payload", nil, "language", ""},
		{"invalid JSON", json.RawMessage(`not json`), "language", ""},
	}
	for _, tc := range cases {
		if got := payloadString(tc.raw, tc.key); got != tc.want {
			t.Errorf("%s: payloadString(%s, %q) = %q, want %q", tc.name, tc.raw, tc.key, got, tc.want)
		}
	}
}

// Test doubles for what renderPDF reads. Each embeds its port interface and
// implements only the method the render path calls.
type stubVersionRepo struct {
	port.TemplateVersionRepository
	version *entity.TemplateVersion
}

func (r stubVersionRepo) FindByID(context.Context, string) (*entity.TemplateVersion, error) {
	return r.version, nil
}

type stubRecipientRepo struct {
	port.DocumentRecipientRepository
}

func (stubRecipientRepo) FindByDocumentID(context.Context, string) ([]*entity.DocumentRecipient, error) {
	return nil, nil
}

type stubSignerRoleRepo struct {
	port.TemplateVersionSignerRoleRepository
}

func (stubSignerRoleRepo) FindByVersionID(context.Context, string) ([]*entity.TemplateVersionSignerRole, error) {
	return nil, nil
}

type stubFieldResponseRepo struct {
	port.DocumentFieldResponseRepository
}

func (stubFieldResponseRepo) FindByDocumentID(context.Context, string) ([]entity.DocumentFieldResponse, error) {
	return nil, nil
}

type stubRenderer struct{ port.PDFRenderer }

func (stubRenderer) RenderPreview(context.Context, *port.RenderPreviewRequest) (*port.RenderPreviewResult, error) {
	return &port.RenderPreviewResult{PDF: []byte("%PDF-1.7")}, nil
}

func TestRenderAttemptArtifactsReturnsTemplateLanguage(t *testing.T) {
	for _, language := range []string{"es", "en", ""} {
		content, err := json.Marshal(map[string]any{
			"version": "1.2.0",
			"meta":    map[string]any{"title": "Carta", "language": language},
		})
		if err != nil {
			t.Fatal(err)
		}
		executor := NewSigningAttemptExecutor(SigningAttemptExecutorConfig{
			VersionRepo:       stubVersionRepo{version: &entity.TemplateVersion{ID: "version-1", ContentStructure: content}},
			RecipientRepo:     stubRecipientRepo{},
			SignerRoleRepo:    stubSignerRoleRepo{},
			FieldResponseRepo: stubFieldResponseRepo{},
			PDFRenderer:       stubRenderer{},
		})

		pdf, _, got, err := executor.renderAttemptArtifacts(context.Background(), &entity.Document{ID: "doc-1", TemplateVersionID: "version-1"})
		if err != nil {
			t.Fatalf("language %q: renderAttemptArtifacts() error = %v", language, err)
		}
		if string(pdf) != "%PDF-1.7" {
			t.Fatalf("language %q: pdf = %q", language, pdf)
		}
		if got != language {
			t.Fatalf("renderAttemptArtifacts() language = %q, want %q", got, language)
		}
	}
}
