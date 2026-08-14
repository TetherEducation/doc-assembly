package pdfrenderer

import (
	"context"
	"strings"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// Body rows are "every row but the header", which table.cell.where() cannot express: its
// `y` selector takes one integer. Emitting `where(y: range(1, none))` made Typst fail the
// whole compile with "expected integer, found none", so a template holding a table was
// unrenderable whenever body styles were set.
func TestBuildTableBodyStyleRules_DoesNotEmitRangeSelector(t *testing.T) {
	c := newTestConverter(nil, nil)
	size := 11
	weight := "regular"
	color := "#333333"
	rules := c.buildTableBodyStyleRules(&entity.TableStyles{
		FontSize: &size, FontWeight: &weight, TextColor: &color,
	})

	if strings.Contains(rules, "range(") {
		t.Fatalf("body style rules must not use a range selector, got:\n%s", rules)
	}
	for _, want := range []string{"11pt", "regular", "#333333"} {
		if !strings.Contains(rules, want) {
			t.Errorf("expected %q in body style rules, got:\n%s", want, rules)
		}
	}
}

func TestBuildTableBodyStyleRules_EmptyWhenNothingSet(t *testing.T) {
	c := newTestConverter(nil, nil)
	if got := c.buildTableBodyStyleRules(nil); got != "" {
		t.Errorf("nil styles should produce no rules, got %q", got)
	}
	if got := c.buildTableBodyStyleRules(&entity.TableStyles{}); got != "" {
		t.Errorf("empty styles should produce no rules, got %q", got)
	}
}

// The unit tests above check the emitted source; this one proves Typst accepts it, which is
// the property that actually broke. It needs the typst binary, so it skips without one.
func TestTableWithBodyStylesCompiles(t *testing.T) {
	service, err := NewService(DefaultTypstOptions(), nil,
		NewTypstConverterFactory(DefaultDesignTokens()), DefaultDesignTokens(), nil)
	if err != nil {
		t.Skipf("typst unavailable: %v", err)
	}

	text := func(s string) portabledoc.Node {
		v := s
		return portabledoc.Node{Type: "text", Text: &v}
	}
	cell := func(s string) portabledoc.Node {
		return portabledoc.Node{Type: "tableCell", Content: []portabledoc.Node{
			{Type: "paragraph", Content: []portabledoc.Node{text(s)}},
		}}
	}

	doc := &portabledoc.Document{
		Content: &portabledoc.ProseMirrorDoc{
			Type: "doc",
			Content: []portabledoc.Node{{
				Type: "table",
				Content: []portabledoc.Node{
					{Type: "tableRow", Content: []portabledoc.Node{cell("Encabezado"), cell("Valor")}},
					{Type: "tableRow", Content: []portabledoc.Node{cell("Matrícula"), cell("2026")}},
				},
			}},
		},
	}

	if _, err := service.RenderPreview(context.Background(), &port.RenderPreviewRequest{
		Document: doc, Injectables: map[string]any{},
	}); err != nil {
		t.Fatalf("a table with body styles must compile: %v", err)
	}
}
