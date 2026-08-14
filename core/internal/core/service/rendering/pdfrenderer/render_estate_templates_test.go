package pdfrenderer

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// TestRenderEstateTemplates renders real template JSON through the production converter.
//
// Not a unit test — a harness. Around twenty templates were written for Chilean schools and
// verified only as text. Four of their constructs are first-use in that estate and none has
// ever reached Typst:
//
//   - a document header bound to an image injectable (campus_logo) — no template anywhere
//     contained an image node before
//   - interactiveField checkboxes carrying a legal decision (religion classes, leaving school
//     unaccompanied)
//   - list markers carried as literal text ("a.", "I.") because orderedList always emits 1,2,3
//   - accented Spanish, just written into 19 live templates
//
// A silent failure in any of these — a dropped header, a checkbox that renders as nothing,
// mojibake — would reach families as a signed legal document. Rendering is the only way to
// find out, and every template is still DRAFT, which is the moment to look.
//
//	TEMPLATE_DIR=/path/to/json TEMPLATE_OUT=/path/to/pdfs \
//	  go test ./core/internal/core/service/rendering/pdfrenderer/ -run TestRenderEstateTemplates -v
func TestRenderEstateTemplates(t *testing.T) {
	dir := os.Getenv("TEMPLATE_DIR")
	if dir == "" {
		t.Skip("TEMPLATE_DIR not set")
	}
	outDir := os.Getenv("TEMPLATE_OUT")
	if outDir == "" {
		outDir = dir
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("cannot create output dir: %v", err)
	}

	opts := DefaultTypstOptions()
	if bin := os.Getenv("TYPST_BIN"); bin != "" {
		opts.BinPath = bin
	}

	// A real image cache, so the campus_logo header actually fetches over the network the
	// way it will in production. Without it the header silently renders empty and the test
	// would pass while proving nothing.
	cache, err := NewImageCache(ImageCacheOptions{Dir: filepath.Join(outDir, ".imgcache")})
	if err != nil {
		t.Fatalf("image cache: %v", err)
	}

	service, err := NewService(opts, cache, NewTypstConverterFactory(DefaultDesignTokens()),
		DefaultDesignTokens(), nil)
	if err != nil {
		t.Fatalf("renderer: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}

	var rendered, failed int
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".json")

		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, entry.Name()))
			if err != nil {
				t.Fatalf("read: %v", err)
			}

			// Each file is {"structure": <content_structure>, "injectables": {...}}
			var payload struct {
				Structure   json.RawMessage `json:"structure"`
				Injectables map[string]any  `json:"injectables"`
			}
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			doc, err := portabledoc.Parse(payload.Structure)
			if err != nil {
				failed++
				t.Fatalf("parse content structure: %v", err)
			}
			if doc == nil {
				failed++
				t.Fatal("parsed to nil document")
			}

			result, err := service.RenderPreview(context.Background(), &port.RenderPreviewRequest{
				Document:    doc,
				Injectables: payload.Injectables,
			})
			if err != nil {
				failed++
				t.Fatalf("render: %v", err)
			}
			if len(result.PDF) == 0 {
				failed++
				t.Fatal("rendered zero bytes")
			}

			out := filepath.Join(outDir, name+".pdf")
			if err := os.WriteFile(out, result.PDF, 0o644); err != nil {
				t.Fatalf("write pdf: %v", err)
			}
			rendered++
			t.Logf("%d bytes -> %s", len(result.PDF), out)
		})
	}
	t.Logf("rendered %d, failed %d", rendered, failed)
}
