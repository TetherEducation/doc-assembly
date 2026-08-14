package pdfrenderer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// TestLocalRenderCheck renders template content structures to PDF on this machine.
//
// It exists because ~20 templates were authored as JSON and verified only as text. Four
// constructs in them are first-use across the whole estate — a header image bound to an
// injectable, interactiveField checkboxes, literal list markers, and accented characters —
// and none had ever been through Typst. Text verification cannot catch a renderer that
// drops a logo or mangles a diacritic.
//
// Rendering locally rather than through the service keeps this off production entirely:
// no document is created, no family is notified, and the templates stay DRAFT.
//
// Skipped unless RENDER_IN is set, so it never runs in CI:
//
//	RENDER_IN=/path/to/json RENDER_OUT=/path/to/pdfs go test ./core/internal/core/service/rendering/pdfrenderer/ -run TestLocalRenderCheck -v
func TestLocalRenderCheck(t *testing.T) {
	inDir := os.Getenv("RENDER_IN")
	outDir := os.Getenv("RENDER_OUT")
	if inDir == "" || outDir == "" {
		t.Skip("set RENDER_IN and RENDER_OUT to run the local render check")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("cannot create %s: %v", outDir, err)
	}

	entries, err := os.ReadDir(inDir)
	if err != nil {
		t.Fatalf("cannot read %s: %v", inDir, err)
	}

	// The image cache is what fetches a remote logo URL, so it must be present for the
	// header binding to be exercised at all; passing nil would silently skip it.
	cache, err := NewImageCache(ImageCacheOptions{})
	if err != nil {
		t.Fatalf("image cache: %v", err)
	}

	service, err := NewService(DefaultTypstOptions(), cache,
		NewTypstConverterFactory(DefaultDesignTokens()), DefaultDesignTokens(), nil)
	if err != nil {
		t.Fatalf("render service: %v", err)
	}

	// Values a real render would inject. Accented on purpose: if the pipeline damages
	// diacritics, it should show up in injected values as well as in template text.
	injectables := map[string]any{
		"date_now":                       "13/08/2026",
		"academic_period_year":           "2026",
		"campus_name":                    "Colegio Ayelén",
		"campus_address_street":          "Camino San Ramón 3403",
		"campus_address_city":            "Rancagua",
		"rbd":                            "6085",
		"grade_name":                     "1ro Básico",
		"shift_name":                     "Jornada Completa",
		"institution_code":               "20364",
		"legalguardian_first_name":       "María Soledad",
		"legalguardian_first_last_name":  "Fuentes",
		"legalguardian_second_last_name": "Riquelme",
		"legalguardian_id_number":        "12.345.678-9",
		"legalguardian_email":            "apoderada@ejemplo.cl",
		"legalguardian_cellphone":        "+56 9 1234 5678",
		"legalguardian_address_street":   "Avenida Los Ñandúes 1234",
		"legalguardian_address_number":   "1234",
		"legalguardian_address_city":     "Rancagua",
		"student_first_name":             "Tomás",
		"student_first_last_name":        "Fuentes",
		"student_second_last_name":       "Riquelme",
		"student_id_number":              "23.456.789-0",
		"student_birthdate":              "14/03/2018",
		// The school's own mark, exactly as crm-identity-access serves it.
		"campus_logo": "https://osxjjddopxhloclxyfjj.supabase.co/storage/v1/object/public/" +
			"explorer-images/chile/2036400001/images/e0780f04-7883-4f4c-b5ce-8f5664e18700.jpeg",
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(inDir, name))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			var structure json.RawMessage = raw
			doc, err := portabledoc.Parse(structure)
			if err != nil {
				t.Fatalf("parse content structure: %v", err)
			}
			if doc == nil {
				t.Fatal("content structure produced no document")
			}
			// Parse accepts an object with no top-level "content" and returns a non-nil
			// document that renders as a blank page. A wrongly-shaped input therefore
			// looks like a successful render, which is worse than an error — fifteen
			// stale dumps passed this test while producing empty PDFs.
			if doc.Content == nil || len(doc.Content.Content) == 0 {
				t.Fatal("parsed document has no content nodes — check the JSON shape " +
					"(expected the template's content_structure, not a wrapper)")
			}

			result, err := service.RenderPreview(context.Background(), &port.RenderPreviewRequest{
				Document:    doc,
				Injectables: injectables,
			})
			if err != nil {
				t.Fatalf("render: %v", err)
			}
			out := filepath.Join(outDir, name[:len(name)-5]+".pdf")
			if err := os.WriteFile(out, result.PDF, 0o644); err != nil {
				t.Fatalf("write pdf: %v", err)
			}
			t.Logf("rendered %d bytes -> %s", len(result.PDF), out)
		})
	}
}
