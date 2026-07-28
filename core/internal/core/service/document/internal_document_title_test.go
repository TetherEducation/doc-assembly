package document

import (
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
)

// buildInternalDocument only reads from its arguments, so a zero-value
// service is enough to exercise it.
func buildDocForTitleTest(t *testing.T, templateTitle string) *entity.Document {
	t.Helper()

	resolved := &internalResolvedContext{
		workspace:    &entity.Workspace{ID: "ws-1"},
		version:      &entity.TemplateVersionWithDetails{TemplateVersion: entity.TemplateVersion{ID: "ver-1"}},
		documentType: &entity.DocumentType{ID: "dt-1"},
		template:     &entity.Template{ID: "tpl-1", Title: templateTitle},
	}

	doc, err := (&InternalDocumentService{}).buildInternalDocument(
		documentuc.InternalCreateCommand{ExternalID: "0ed0dc30-2da3-4c89-a074-5d34fc1e2bec"},
		resolved,
		&PreparedDocumentData{},
	)
	if err != nil {
		t.Fatalf("buildInternalDocument: %v", err)
	}
	return doc
}

func TestBuildInternalDocumentUsesTemplateTitle(t *testing.T) {
	doc := buildDocForTitleTest(t, "Comprobante de Matrícula")

	if doc.Title == nil || *doc.Title != "Comprobante de Matrícula" {
		t.Fatalf("Title = %v, want the template display title", doc.Title)
	}
	// The UUID fallback is exactly what leaked to signers before this fix.
	if got := documentTitle(doc); got != "Comprobante de Matrícula" {
		t.Fatalf("documentTitle = %q, want the human title, not the ID fallback", got)
	}
}

func TestBuildInternalDocumentBlankTemplateTitleKeepsFallback(t *testing.T) {
	doc := buildDocForTitleTest(t, "   ")

	if doc.Title != nil {
		t.Fatalf("Title = %q, want nil for blank template titles", *doc.Title)
	}
	if got := documentTitle(doc); got != doc.ID {
		t.Fatalf("documentTitle = %q, want the ID fallback when no title exists", got)
	}
}
