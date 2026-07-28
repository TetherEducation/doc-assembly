package pdfrenderer

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// --- Helpers ---

func textNode(s string) portabledoc.Node {
	return portabledoc.Node{Type: portabledoc.NodeTypeText, Text: &s}
}

func markedTextNode(s string, marks ...portabledoc.Mark) portabledoc.Node {
	return portabledoc.Node{Type: portabledoc.NodeTypeText, Text: &s, Marks: marks}
}

func markOf(typ string, attrs ...map[string]any) portabledoc.Mark {
	m := portabledoc.Mark{Type: typ}
	if len(attrs) > 0 {
		m.Attrs = attrs[0]
	}
	return m
}

func paragraphNode(children ...portabledoc.Node) portabledoc.Node {
	return portabledoc.Node{Type: portabledoc.NodeTypeParagraph, Content: children}
}

func newTestConverter(injectables map[string]any, defaults map[string]string) *typstConverter {
	if injectables == nil {
		injectables = map[string]any{}
	}
	if defaults == nil {
		defaults = map[string]string{}
	}
	factory := NewTypstConverterFactory(DefaultDesignTokens())
	conv := factory(injectables, defaults, nil, nil, nil)
	return conv.(*typstConverter)
}

// --- Text & Marks ---

func TestTypstConverter_TextPlain(t *testing.T) {
	c := newTestConverter(nil, nil)
	got := c.convertNode(textNode("Hello world"))
	if got != "Hello world" {
		t.Errorf("got %q, want %q", got, "Hello world")
	}
}

func TestTypstConverter_TextNil(t *testing.T) {
	c := newTestConverter(nil, nil)
	got := c.convertNode(portabledoc.Node{Type: portabledoc.NodeTypeText})
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestTypstConverter_TextEscaping(t *testing.T) {
	c := newTestConverter(nil, nil)
	got := c.convertNode(textNode("price is $10 #tag *bold* _italic_"))
	for _, special := range []string{"\\$", "\\#", "\\*", "\\_"} {
		if !strings.Contains(got, special) {
			t.Errorf("expected %q to contain %q", got, special)
		}
	}
}

func TestTypstConverter_MarkBold(t *testing.T) {
	c := newTestConverter(nil, nil)
	got := c.convertNode(markedTextNode("hello", markOf(portabledoc.MarkTypeBold)))
	if got != "#strong[hello]" {
		t.Errorf("got %q, want %q", got, "#strong[hello]")
	}
}

func TestTypstConverter_MarkItalic(t *testing.T) {
	c := newTestConverter(nil, nil)
	got := c.convertNode(markedTextNode("hello", markOf(portabledoc.MarkTypeItalic)))
	if got != "#emph[hello]" {
		t.Errorf("got %q, want %q", got, "#emph[hello]")
	}
}

func TestTypstConverter_MarkStrike(t *testing.T) {
	c := newTestConverter(nil, nil)
	got := c.convertNode(markedTextNode("hello", markOf(portabledoc.MarkTypeStrike)))
	if got != "#strike[hello]" {
		t.Errorf("got %q, want %q", got, "#strike[hello]")
	}
}

func TestTypstConverter_MarkCode(t *testing.T) {
	c := newTestConverter(nil, nil)
	got := c.convertNode(markedTextNode("x := 1", markOf(portabledoc.MarkTypeCode)))
	if got != "`x := 1`" {
		t.Errorf("got %q, want %q", got, "`x := 1`")
	}
}

func TestTypstConverter_MarkUnderline(t *testing.T) {
	c := newTestConverter(nil, nil)
	got := c.convertNode(markedTextNode("hello", markOf(portabledoc.MarkTypeUnderline)))
	if got != "#underline[hello]" {
		t.Errorf("got %q, want %q", got, "#underline[hello]")
	}
}

func TestTypstConverter_MarkHighlight(t *testing.T) {
	c := newTestConverter(nil, nil)
	got := c.convertNode(markedTextNode("hello", markOf(portabledoc.MarkTypeHighlight)))
	if !strings.Contains(got, "#highlight") && !strings.Contains(got, "#ffeb3b") {
		t.Errorf("expected highlight markup, got %q", got)
	}
}

func TestTypstConverter_MarkHighlightCustomColor(t *testing.T) {
	c := newTestConverter(nil, nil)
	got := c.convertNode(markedTextNode("hello", markOf(portabledoc.MarkTypeHighlight, map[string]any{"color": "#ff0000"})))
	if !strings.Contains(got, "#ff0000") {
		t.Errorf("expected custom color, got %q", got)
	}
}

func TestTypstConverter_MarkLink(t *testing.T) {
	c := newTestConverter(nil, nil)
	got := c.convertNode(markedTextNode("click", markOf(portabledoc.MarkTypeLink, map[string]any{"href": "https://example.com"})))
	want := `#link("https://example.com")[click]`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTypstConverter_MarkLinkEmptyHref(t *testing.T) {
	c := newTestConverter(nil, nil)
	got := c.convertNode(markedTextNode("click", markOf(portabledoc.MarkTypeLink, map[string]any{"href": ""})))
	if got != "click" {
		t.Errorf("got %q, want %q", got, "click")
	}
}

func TestTypstConverter_NestedMarks(t *testing.T) {
	c := newTestConverter(nil, nil)
	got := c.convertNode(markedTextNode("hello", markOf(portabledoc.MarkTypeBold), markOf(portabledoc.MarkTypeItalic)))
	if got != "#emph[#strong[hello]]" {
		t.Errorf("got %q, want %q", got, "#emph[#strong[hello]]")
	}
}

// --- Paragraph ---

func TestTypstConverter_Paragraph(t *testing.T) {
	c := newTestConverter(nil, nil)
	got := c.convertNode(paragraphNode(textNode("Hello")))
	if !strings.Contains(got, "Hello") {
		t.Fatalf("expected paragraph content, got %q", got)
	}
	if !strings.Contains(got, "#set text(top-edge: 0.8em, bottom-edge: -0.2em)") {
		t.Fatalf("expected paragraph text edge rule, got %q", got)
	}
	if !strings.Contains(got, "#set par(leading: 0.50em)") {
		t.Fatalf("expected default paragraph leading, got %q", got)
	}
}

func TestTypstConverter_ParagraphEmpty(t *testing.T) {
	c := newTestConverter(nil, nil)
	got := c.convertNode(paragraphNode())
	want := fmt.Sprintf("#v(%s)", c.tokens.ParagraphSpacing)
	if !strings.Contains(got, want) {
		t.Errorf("empty paragraph should produce %q, got %q", want, got)
	}
}

func TestTypstConverter_ParagraphWithLocalLineSpacing(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:    portabledoc.NodeTypeParagraph,
		Attrs:   map[string]any{"lineSpacing": "relaxed"},
		Content: []portabledoc.Node{textNode("Hello")},
	}

	got := c.convertNode(node)

	if !strings.Contains(got, "#set par(leading: 1.00em)") {
		t.Fatalf("expected local leading rule, got %q", got)
	}
	if !strings.Contains(got, "#set text(top-edge: 0.8em, bottom-edge: -0.2em)") {
		t.Fatalf("expected text edge rule inside scoped paragraph, got %q", got)
	}
	if !strings.Contains(got, "Hello") {
		t.Fatalf("expected paragraph content, got %q", got)
	}
}

func TestTypstConverter_ParagraphWithAlignAndLocalLineSpacing(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeParagraph,
		Attrs: map[string]any{
			"lineSpacing": "loose",
			"textAlign":   "right",
		},
		Content: []portabledoc.Node{textNode("Aligned")},
	}

	got := c.convertNode(node)

	if !strings.Contains(got, "#align(right)[") {
		t.Fatalf("expected aligned paragraph wrapper, got %q", got)
	}
	if !strings.Contains(got, "#set par(leading: 1.50em)") {
		t.Fatalf("expected scoped leading inside aligned paragraph, got %q", got)
	}
	if !strings.Contains(got, "#set text(top-edge: 0.8em, bottom-edge: -0.2em)") {
		t.Fatalf("expected text edge rule inside aligned paragraph, got %q", got)
	}
}

func TestTypstConverter_ParagraphWithJustifyAndLocalLineSpacing(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeParagraph,
		Attrs: map[string]any{
			"lineSpacing": "tight",
			"textAlign":   "justify",
		},
		Content: []portabledoc.Node{textNode("Justified")},
	}

	got := c.convertNode(node)

	if !strings.Contains(got, "#par(justify: true)[Justified]") {
		t.Fatalf("expected justified paragraph, got %q", got)
	}
	if !strings.Contains(got, "#set par(leading: 0em)") {
		t.Fatalf("expected scoped leading inside justified paragraph, got %q", got)
	}
	if !strings.Contains(got, "#set text(top-edge: 0.8em, bottom-edge: -0.2em)") {
		t.Fatalf("expected text edge rule inside justified paragraph, got %q", got)
	}
}

// --- HardBreak ---

func TestTypstConverter_HardBreak(t *testing.T) {
	c := newTestConverter(nil, nil)
	got := c.convertNode(portabledoc.Node{Type: portabledoc.NodeTypeHardBreak})
	want := "\\\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTypstConverter_ParagraphWithHardBreaks(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeParagraph,
		Content: []portabledoc.Node{
			textNode("Line 1"),
			{Type: portabledoc.NodeTypeHardBreak},
			textNode("Line 2"),
			{Type: portabledoc.NodeTypeHardBreak},
			textNode("Line 3"),
		},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "Line 1\\\nLine 2\\\nLine 3") {
		t.Errorf("expected hard breaks within paragraph, got %q", got)
	}
	if !strings.HasSuffix(got, "\n\n") {
		t.Errorf("expected paragraph to end with double newline, got %q", got)
	}
}

func TestTypstConverter_TableCellWithLineBreaks(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeTable,
		Content: []portabledoc.Node{
			{
				Type: portabledoc.NodeTypeTableRow,
				Content: []portabledoc.Node{
					{
						Type: portabledoc.NodeTypeTableCell,
						Content: []portabledoc.Node{
							paragraphNode(
								textNode("First line"),
								portabledoc.Node{Type: portabledoc.NodeTypeHardBreak},
								textNode("Second line"),
							),
						},
					},
				},
			},
		},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "First line\\\nSecond line") {
		t.Errorf("expected line breaks preserved in table cell, got %q", got)
	}
}

func TestTypstConverter_TableCellMultipleParagraphs(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeTable,
		Content: []portabledoc.Node{
			{
				Type: portabledoc.NodeTypeTableRow,
				Content: []portabledoc.Node{
					{
						Type: portabledoc.NodeTypeTableCell,
						Content: []portabledoc.Node{
							paragraphNode(textNode("Paragraph 1")),
							paragraphNode(textNode("Paragraph 2")),
						},
					},
				},
			},
		},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "Paragraph 1") || !strings.Contains(got, "Paragraph 2") {
		t.Errorf("expected both paragraphs in table cell, got %q", got)
	}
}

// --- Heading ---

func TestTypstConverter_Headings(t *testing.T) {
	tests := []struct {
		level float64
		want  string
	}{
		{1, "= Title\n"},
		{2, "== Title\n"},
		{3, "=== Title\n"},
		{4, "==== Title\n"},
		{5, "===== Title\n"},
		{6, "====== Title\n"},
	}

	for _, tt := range tests {
		c := newTestConverter(nil, nil)
		node := portabledoc.Node{
			Type:    portabledoc.NodeTypeHeading,
			Attrs:   map[string]any{"level": tt.level},
			Content: []portabledoc.Node{textNode("Title")},
		}
		got := c.convertNode(node)
		if !strings.Contains(got, tt.want) {
			t.Errorf("level %.0f: got %q, want content %q", tt.level, got, tt.want)
		}
		if !strings.Contains(got, "#set par(leading: 0.50em)") {
			t.Errorf("level %.0f: expected default heading leading, got %q", tt.level, got)
		}
	}
}

func TestTypstConverter_HeadingDefaultLevel(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:    portabledoc.NodeTypeHeading,
		Content: []portabledoc.Node{textNode("Title")},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "= Title") {
		t.Errorf("expected level 1 heading, got %q", got)
	}
}

func TestTypstConverter_HeadingWithLocalLineSpacing(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeHeading,
		Attrs: map[string]any{
			"level":       float64(2),
			"lineSpacing": "compact",
		},
		Content: []portabledoc.Node{textNode("Wrapped title")},
	}

	got := c.convertNode(node)

	if !strings.Contains(got, "== Wrapped title") {
		t.Fatalf("expected heading markup, got %q", got)
	}
	if !strings.Contains(got, "#set par(leading: 0.15em)") {
		t.Fatalf("expected scoped heading leading, got %q", got)
	}
	if !strings.Contains(got, "#set text(top-edge: 0.8em, bottom-edge: -0.2em)") {
		t.Fatalf("expected text edge rule inside heading, got %q", got)
	}
}

func TestTypstConverter_HeadingWithAlignAndLocalLineSpacing(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeHeading,
		Attrs: map[string]any{
			"level":       float64(3),
			"textAlign":   "center",
			"lineSpacing": "loose",
		},
		Content: []portabledoc.Node{textNode("Centered title")},
	}

	got := c.convertNode(node)

	if !strings.Contains(got, "#align(center)[") {
		t.Fatalf("expected aligned heading wrapper, got %q", got)
	}
	if !strings.Contains(got, "#set par(leading: 1.50em)") {
		t.Fatalf("expected scoped heading leading inside align wrapper, got %q", got)
	}
	if !strings.Contains(got, "#set text(top-edge: 0.8em, bottom-edge: -0.2em)") {
		t.Fatalf("expected text edge rule inside aligned heading, got %q", got)
	}
}

// --- Blockquote ---

func TestTypstConverter_Blockquote(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:    portabledoc.NodeTypeBlockquote,
		Content: []portabledoc.Node{paragraphNode(textNode("quote"))},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "#block(") || !strings.Contains(got, "#emph") {
		t.Errorf("expected blockquote markup, got %q", got)
	}
}

// --- CodeBlock ---

func TestTypstConverter_CodeBlock(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:    portabledoc.NodeTypeCodeBlock,
		Content: []portabledoc.Node{textNode("fmt.Println()")},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "```") {
		t.Errorf("expected code block markup, got %q", got)
	}
}

func TestTypstConverter_CodeBlockWithLanguage(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:    portabledoc.NodeTypeCodeBlock,
		Attrs:   map[string]any{"language": "go"},
		Content: []portabledoc.Node{textNode("fmt.Println()")},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "```go") {
		t.Errorf("expected language annotation, got %q", got)
	}
}

// --- Horizontal Rule ---

func TestTypstConverter_HorizontalRule(t *testing.T) {
	c := newTestConverter(nil, nil)
	got := c.convertNode(portabledoc.Node{Type: portabledoc.NodeTypeHR})
	if !strings.Contains(got, "#line(") {
		t.Errorf("expected line markup, got %q", got)
	}
}

// --- Lists ---

func TestTypstConverter_BulletList(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeBulletList,
		Content: []portabledoc.Node{
			{Type: portabledoc.NodeTypeListItem, Content: []portabledoc.Node{paragraphNode(textNode("item1"))}},
			{Type: portabledoc.NodeTypeListItem, Content: []portabledoc.Node{paragraphNode(textNode("item2"))}},
		},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "- #[") || !strings.Contains(got, "item1") || !strings.Contains(got, "item2") {
		t.Errorf("expected bullet list items, got %q", got)
	}
}

func TestTypstConverter_OrderedList(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeOrderedList,
		Content: []portabledoc.Node{
			{Type: portabledoc.NodeTypeListItem, Content: []portabledoc.Node{paragraphNode(textNode("first"))}},
			{Type: portabledoc.NodeTypeListItem, Content: []portabledoc.Node{paragraphNode(textNode("second"))}},
		},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "+ #[") || !strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Errorf("expected ordered list items, got %q", got)
	}
}

func TestTypstConverter_OrderedListCustomStart(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeOrderedList,
		Attrs: map[string]any{"start": float64(5)},
		Content: []portabledoc.Node{
			{Type: portabledoc.NodeTypeListItem, Content: []portabledoc.Node{paragraphNode(textNode("fifth"))}},
		},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "#set enum(start: 5)") {
		t.Errorf("expected custom start, got %q", got)
	}
}

func TestTypstConverter_TaskList(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeTaskList,
		Content: []portabledoc.Node{
			{Type: portabledoc.NodeTypeTaskItem, Attrs: map[string]any{"checked": true}, Content: []portabledoc.Node{paragraphNode(textNode("done"))}},
			{Type: portabledoc.NodeTypeTaskItem, Attrs: map[string]any{"checked": false}, Content: []portabledoc.Node{paragraphNode(textNode("todo"))}},
		},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "\u2611") || !strings.Contains(got, "\u2610") {
		t.Errorf("expected task markers, got %q", got)
	}
}

// --- Page Break ---

func TestTypstConverter_PageBreak(t *testing.T) {
	c := newTestConverter(nil, nil)
	got := c.convertNode(portabledoc.Node{Type: portabledoc.NodeTypePageBreak})
	if got != "#pagebreak()\n" {
		t.Errorf("got %q, want %q", got, "#pagebreak()\n")
	}
	if c.GetCurrentPage() != 2 {
		t.Errorf("page count should be 2 after pagebreak, got %d", c.GetCurrentPage())
	}
}

func TestTypstConverter_MultiplePageBreaks(t *testing.T) {
	c := newTestConverter(nil, nil)
	c.convertNode(portabledoc.Node{Type: portabledoc.NodeTypePageBreak})
	c.convertNode(portabledoc.Node{Type: portabledoc.NodeTypePageBreak})
	c.convertNode(portabledoc.Node{Type: portabledoc.NodeTypePageBreak})
	if c.GetCurrentPage() != 4 {
		t.Errorf("expected page 4, got %d", c.GetCurrentPage())
	}
}

// --- Image ---

func TestTypstConverter_Image(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeImage,
		Attrs: map[string]any{"src": "https://example.com/img.png", "width": float64(200)},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "#image(") || !strings.Contains(got, "150pt") {
		t.Errorf("expected image with 150pt width (200*0.75), got %q", got)
	}
	if !strings.Contains(got, "img_1.png") {
		t.Errorf("expected local filename, got %q", got)
	}
	if !strings.Contains(got, `scaling: "smooth"`) {
		t.Errorf("expected raster images to request smooth scaling, got %q", got)
	}
}

func TestTypstConverter_ImageCenter(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeImage,
		Attrs: map[string]any{"src": "https://example.com/img.png", "align": "center"},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "#align(center)") {
		t.Errorf("expected center alignment, got %q", got)
	}
	if !strings.Contains(got, "img_1.png") {
		t.Errorf("expected local filename for remote URL, got %q", got)
	}
	if !strings.Contains(got, `scaling: "smooth"`) {
		t.Errorf("expected raster images to request smooth scaling, got %q", got)
	}
}

func TestTypstConverter_ImageSVGDoesNotForceSmoothScaling(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeImage,
		Attrs: map[string]any{"src": "https://example.com/logo.svg", "width": float64(120)},
	}
	got := c.convertNode(node)
	if strings.Contains(got, `scaling: "smooth"`) {
		t.Errorf("expected svg images to omit raster scaling hint, got %q", got)
	}
}

func TestTypstConverter_ImageEmptySrc(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeImage,
		Attrs: map[string]any{"src": ""},
	}
	got := c.convertNode(node)
	if got != "" {
		t.Errorf("expected empty output for empty src, got %q", got)
	}
}

func TestTypstConverter_ImageInjectable(t *testing.T) {
	c := newTestConverter(map[string]any{"img1": "https://resolved.com/photo.jpg"}, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeImage,
		Attrs: map[string]any{"src": "", "injectableId": "img1"},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "img_1.jpg") {
		t.Errorf("expected local image filename, got %q", got)
	}
	if len(c.RemoteImages()) != 1 {
		t.Errorf("expected 1 remote image registered, got %d", len(c.RemoteImages()))
	}
}

func TestTypstConverter_ImageInjectableUsesContainForFixedBox(t *testing.T) {
	c := newTestConverter(map[string]any{"img1": "https://resolved.com/photo.jpg"}, nil)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeImage,
		Attrs: map[string]any{
			"src":          "",
			"injectableId": "img1",
			"width":        float64(240),
			"height":       float64(120),
		},
	}

	got := c.convertNode(node)
	if !strings.Contains(got, `width: 180pt`) || !strings.Contains(got, `height: 90pt`) {
		t.Fatalf("expected fixed image box dimensions in typst output, got %q", got)
	}
	if !strings.Contains(got, `fit: "contain"`) {
		t.Fatalf("expected injectable image to use contain fit, got %q", got)
	}
}

func TestTypstConverter_ImageStorageURL(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeImage,
		Attrs: map[string]any{"src": "storage://asset-key", "width": float64(200)},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "img_1") {
		t.Fatalf("expected generated local filename, got %q", got)
	}
	if _, ok := c.RemoteImages()["storage://asset-key"]; !ok {
		t.Fatalf("expected storage URL to be tracked as remote image")
	}
}

// --- Injector ---

func TestTypstConverter_InjectorWithValue(t *testing.T) {
	c := newTestConverter(map[string]any{"var1": "John Doe"}, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeInjector,
		Attrs: map[string]any{"variableId": "var1", "label": "Name"},
	}
	got := c.convertNode(node)
	if got != "John Doe" {
		t.Errorf("got %q, want %q", got, "John Doe")
	}
}

func TestTypstConverter_InjectorWithDefault(t *testing.T) {
	c := newTestConverter(nil, map[string]string{"var1": "Default Name"})
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeInjector,
		Attrs: map[string]any{"variableId": "var1", "label": "Name"},
	}
	got := c.convertNode(node)
	if got != "Default Name" {
		t.Errorf("got %q, want %q", got, "Default Name")
	}
}

func TestTypstConverter_InjectorEmpty(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeInjector,
		Attrs: map[string]any{"variableId": "var1", "label": "Name"},
	}
	got := c.convertNode(node)
	if got != "" {
		t.Errorf("expected empty string for missing injectable, got %q", got)
	}
}

func TestTypstConverter_InjectorCurrency(t *testing.T) {
	c := newTestConverter(map[string]any{"price": float64(99.5)}, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeInjector,
		Attrs: map[string]any{"variableId": "price", "type": "CURRENCY", "format": "$"},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "99.50") {
		t.Errorf("expected currency formatted value, got %q", got)
	}
}

func TestTypstConverter_InjectorBoolean(t *testing.T) {
	c := newTestConverter(map[string]any{"active": true}, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeInjector,
		Attrs: map[string]any{"variableId": "active"},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "S") { // "Si" or "Sí" depending on locale
		t.Errorf("expected boolean true value, got %q", got)
	}
}

func TestTypstConverter_InjectorNumber(t *testing.T) {
	c := newTestConverter(map[string]any{"count": float64(42)}, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeInjector,
		Attrs: map[string]any{"variableId": "count"},
	}
	got := c.convertNode(node)
	if got != "42" {
		t.Errorf("got %q, want %q", got, "42")
	}
}

// --- Injector with Labels ---

func TestTypstConverter_InjectorWithPrefix(t *testing.T) {
	c := newTestConverter(map[string]any{"name": "John"}, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeInjector,
		Attrs: map[string]any{"variableId": "name", "prefix": "Name: "},
	}
	got := c.convertNode(node)
	want := "Name: John"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTypstConverter_InjectorWithSuffix(t *testing.T) {
	c := newTestConverter(map[string]any{"name": "John"}, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeInjector,
		Attrs: map[string]any{"variableId": "name", "suffix": " is the name"},
	}
	got := c.convertNode(node)
	want := "John is the name"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTypstConverter_InjectorWithBothPrefixAndSuffix(t *testing.T) {
	c := newTestConverter(map[string]any{"amount": float64(150)}, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeInjector,
		Attrs: map[string]any{"variableId": "amount", "prefix": "Total: ", "suffix": " USD"},
	}
	got := c.convertNode(node)
	want := "Total: 150 USD"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTypstConverter_InjectorEmptyValueShowLabel(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeInjector,
		Attrs: map[string]any{"variableId": "missing", "prefix": "Total: ", "showLabelIfEmpty": true},
	}
	got := c.convertNode(node)
	want := "Total: "
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTypstConverter_InjectorEmptyValueShowBothLabels(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeInjector,
		Attrs: map[string]any{"variableId": "missing", "prefix": "Total: ", "suffix": " USD", "showLabelIfEmpty": true},
	}
	got := c.convertNode(node)
	want := "Total:  USD"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTypstConverter_InjectorEmptyValueHideLabel(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeInjector,
		Attrs: map[string]any{"variableId": "missing", "prefix": "Total: ", "showLabelIfEmpty": false},
	}
	got := c.convertNode(node)
	if got != "" {
		t.Errorf("expected empty string when value is missing and showLabelIfEmpty=false, got %q", got)
	}
}

func TestTypstConverter_InjectorNodeDefaultValue(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeInjector,
		Attrs: map[string]any{"variableId": "missing", "prefix": "Total: ", "defaultValue": "N/A"},
	}
	got := c.convertNode(node)
	want := "Total: N/A"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTypstConverter_InjectorNodeDefaultOverridesGlobalDefault(t *testing.T) {
	c := newTestConverter(nil, map[string]string{"var1": "Global Default"})
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeInjector,
		Attrs: map[string]any{"variableId": "var1", "defaultValue": "Node Default"},
	}
	got := c.convertNode(node)
	want := "Node Default"
	if got != want {
		t.Errorf("node default should override global default, got %q, want %q", got, want)
	}
}

func TestTypstConverter_InjectorPrefixWithSpecialChars(t *testing.T) {
	c := newTestConverter(map[string]any{"val": "test"}, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeInjector,
		Attrs: map[string]any{"variableId": "val", "prefix": "Price: $"},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "\\$") {
		t.Errorf("expected escaped $, got %q", got)
	}
}

func TestTypstConverter_InjectorBackwardCompatNoLabels(t *testing.T) {
	c := newTestConverter(map[string]any{"var1": "Value"}, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeInjector,
		Attrs: map[string]any{"variableId": "var1"},
	}
	got := c.convertNode(node)
	want := "Value"
	if got != want {
		t.Errorf("backward compat test failed, got %q, want %q", got, want)
	}
}

func TestTypstConverter_InjectorWidth(t *testing.T) {
	c := newTestConverter(map[string]any{"name": "John"}, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeInjector,
		Attrs: map[string]any{"variableId": "name", "width": float64(200)},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "#box(width: 150.0pt)") {
		t.Errorf("expected box with 150pt width (200*0.75), got %q", got)
	}
	if !strings.Contains(got, "John") {
		t.Errorf("expected value inside box, got %q", got)
	}
}

// --- Conditional ---

func TestTypstConverter_ConditionalTrue(t *testing.T) {
	c := newTestConverter(map[string]any{"status": "active"}, nil)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeConditional,
		Attrs: map[string]any{
			"conditions": map[string]any{
				"logic": "AND",
				"children": []any{
					map[string]any{
						"type":       "rule",
						"variableId": "status",
						"operator":   "eq",
						"value":      map[string]any{"mode": "text", "value": "active"},
					},
				},
			},
		},
		Content: []portabledoc.Node{paragraphNode(textNode("Visible"))},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "Visible") {
		t.Errorf("expected visible content, got %q", got)
	}
}

func TestTypstConverter_ConditionalFalse(t *testing.T) {
	c := newTestConverter(map[string]any{"status": "inactive"}, nil)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeConditional,
		Attrs: map[string]any{
			"conditions": map[string]any{
				"logic": "AND",
				"children": []any{
					map[string]any{
						"type":       "rule",
						"variableId": "status",
						"operator":   "eq",
						"value":      map[string]any{"mode": "text", "value": "active"},
					},
				},
			},
		},
		Content: []portabledoc.Node{paragraphNode(textNode("Hidden"))},
	}
	got := c.convertNode(node)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

func TestTypstConverter_ConditionalOR(t *testing.T) {
	c := newTestConverter(map[string]any{"a": "no", "b": "yes"}, nil)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeConditional,
		Attrs: map[string]any{
			"conditions": map[string]any{
				"logic": "OR",
				"children": []any{
					map[string]any{"type": "rule", "variableId": "a", "operator": "eq", "value": map[string]any{"mode": "text", "value": "yes"}},
					map[string]any{"type": "rule", "variableId": "b", "operator": "eq", "value": map[string]any{"mode": "text", "value": "yes"}},
				},
			},
		},
		Content: []portabledoc.Node{paragraphNode(textNode("OR result"))},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "OR result") {
		t.Errorf("OR condition should pass, got %q", got)
	}
}

func TestTypstConverter_ConditionalEmpty(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeConditional,
		Attrs: map[string]any{
			"conditions": map[string]any{
				"logic": "AND",
				"children": []any{
					map[string]any{"type": "rule", "variableId": "x", "operator": "empty", "value": map[string]any{"mode": "text", "value": ""}},
				},
			},
		},
		Content: []portabledoc.Node{paragraphNode(textNode("Empty"))},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "Empty") {
		t.Errorf("empty operator should match nil, got %q", got)
	}
}

func TestTypstConverter_ConditionalNoConditions(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:    portabledoc.NodeTypeConditional,
		Attrs:   map[string]any{},
		Content: []portabledoc.Node{paragraphNode(textNode("Always"))},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "Always") {
		t.Errorf("no conditions should default to true, got %q", got)
	}
}

// --- Table (user-created) ---

func TestTypstConverter_Table(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeTable,
		Content: []portabledoc.Node{
			{
				Type: portabledoc.NodeTypeTableRow,
				Content: []portabledoc.Node{
					{Type: portabledoc.NodeTypeTableHeader, Content: []portabledoc.Node{paragraphNode(textNode("Name"))}},
					{Type: portabledoc.NodeTypeTableHeader, Content: []portabledoc.Node{paragraphNode(textNode("Age"))}},
				},
			},
			{
				Type: portabledoc.NodeTypeTableRow,
				Content: []portabledoc.Node{
					{Type: portabledoc.NodeTypeTableCell, Content: []portabledoc.Node{paragraphNode(textNode("Alice"))}},
					{Type: portabledoc.NodeTypeTableCell, Content: []portabledoc.Node{paragraphNode(textNode("30"))}},
				},
			},
		},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "#table(") {
		t.Errorf("expected #table(, got %q", got)
	}
	if !strings.Contains(got, "Name") || !strings.Contains(got, "Age") {
		t.Errorf("expected header labels, got %q", got)
	}
	if !strings.Contains(got, "Alice") || !strings.Contains(got, "30") {
		t.Errorf("expected data cells, got %q", got)
	}
}

func TestTypstConverter_TableWithColspan(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeTable,
		Content: []portabledoc.Node{
			{
				Type: portabledoc.NodeTypeTableRow,
				Content: []portabledoc.Node{
					{Type: portabledoc.NodeTypeTableHeader, Attrs: map[string]any{"colspan": float64(2)}, Content: []portabledoc.Node{paragraphNode(textNode("Merged"))}},
				},
			},
		},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "colspan: 2") {
		t.Errorf("expected colspan: 2, got %q", got)
	}
}

// --- Table Column Widths ---

func TestTypstConverter_TableWithExplicitColwidths(t *testing.T) {
	c := newTestConverter(nil, nil)
	c.SetContentWidthPx(650)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeTable,
		Content: []portabledoc.Node{
			{
				Type: portabledoc.NodeTypeTableRow,
				Content: []portabledoc.Node{
					{Type: portabledoc.NodeTypeTableHeader, Attrs: map[string]any{"colwidth": []any{float64(200)}}, Content: []portabledoc.Node{paragraphNode(textNode("A"))}},
					{Type: portabledoc.NodeTypeTableHeader, Attrs: map[string]any{"colwidth": []any{float64(100)}}, Content: []portabledoc.Node{paragraphNode(textNode("B"))}},
					{Type: portabledoc.NodeTypeTableHeader, Attrs: map[string]any{"colwidth": []any{float64(300)}}, Content: []portabledoc.Node{paragraphNode(textNode("C"))}},
				},
			},
			{
				Type: portabledoc.NodeTypeTableRow,
				Content: []portabledoc.Node{
					{Type: portabledoc.NodeTypeTableCell, Attrs: map[string]any{"colwidth": []any{float64(200)}}, Content: []portabledoc.Node{paragraphNode(textNode("a1"))}},
					{Type: portabledoc.NodeTypeTableCell, Attrs: map[string]any{"colwidth": []any{float64(100)}}, Content: []portabledoc.Node{paragraphNode(textNode("b1"))}},
					{Type: portabledoc.NodeTypeTableCell, Attrs: map[string]any{"colwidth": []any{float64(300)}}, Content: []portabledoc.Node{paragraphNode(textNode("c1"))}},
				},
			},
		},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "columns: (200fr, 100fr, 300fr)") {
		t.Errorf("expected proportional column widths, got %q", got)
	}
}

func TestTypstConverter_TableWithNilColwidths(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeTable,
		Content: []portabledoc.Node{
			{
				Type: portabledoc.NodeTypeTableRow,
				Content: []portabledoc.Node{
					{Type: portabledoc.NodeTypeTableCell, Content: []portabledoc.Node{paragraphNode(textNode("a"))}},
					{Type: portabledoc.NodeTypeTableCell, Content: []portabledoc.Node{paragraphNode(textNode("b"))}},
					{Type: portabledoc.NodeTypeTableCell, Content: []portabledoc.Node{paragraphNode(textNode("c"))}},
				},
			},
		},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "columns: (1fr, 1fr, 1fr)") {
		t.Errorf("expected equal column widths for nil colwidths, got %q", got)
	}
}

func TestTypstConverter_TableWithPartialColwidths(t *testing.T) {
	c := newTestConverter(nil, nil)
	c.SetContentWidthPx(600)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeTable,
		Content: []portabledoc.Node{
			{
				Type: portabledoc.NodeTypeTableRow,
				Content: []portabledoc.Node{
					{Type: portabledoc.NodeTypeTableCell, Attrs: map[string]any{"colwidth": []any{float64(200)}}, Content: []portabledoc.Node{paragraphNode(textNode("a"))}},
					{Type: portabledoc.NodeTypeTableCell, Content: []portabledoc.Node{paragraphNode(textNode("b"))}},
					{Type: portabledoc.NodeTypeTableCell, Attrs: map[string]any{"colwidth": []any{float64(100)}}, Content: []portabledoc.Node{paragraphNode(textNode("c"))}},
				},
			},
		},
	}
	got := c.convertNode(node)
	// Missing column should get (600 - 200 - 100) / 1 = 300
	if !strings.Contains(got, "columns: (200fr, 300fr, 100fr)") {
		t.Errorf("expected calculated fill for missing colwidth, got %q", got)
	}
}

func TestTypstConverter_TableColwidthWithColspan(t *testing.T) {
	c := newTestConverter(nil, nil)
	c.SetContentWidthPx(600)
	node := portabledoc.Node{
		Type: portabledoc.NodeTypeTable,
		Content: []portabledoc.Node{
			{
				Type: portabledoc.NodeTypeTableRow,
				Content: []portabledoc.Node{
					{Type: portabledoc.NodeTypeTableCell, Attrs: map[string]any{"colspan": float64(2), "colwidth": []any{float64(200), float64(150)}}, Content: []portabledoc.Node{paragraphNode(textNode("merged"))}},
					{Type: portabledoc.NodeTypeTableCell, Attrs: map[string]any{"colwidth": []any{float64(250)}}, Content: []portabledoc.Node{paragraphNode(textNode("c"))}},
				},
			},
		},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "columns: (200fr, 150fr, 250fr)") {
		t.Errorf("expected colwidths from colspan cell, got %q", got)
	}
}

// --- Table Injector ---

func TestTypstConverter_TableInjector(t *testing.T) {
	tv := entity.NewTableValue()
	tv.AddColumn("name", map[string]string{"en": "Name", "es": "Nombre"}, entity.ValueTypeString)
	tv.AddColumn("amount", map[string]string{"en": "Amount"}, entity.ValueTypeNumber)
	tv.AddRow(
		entity.Cell(entity.StringValue("Item A")),
		entity.Cell(entity.NumberValue(100)),
	)
	tv.AddRow(
		entity.Cell(entity.StringValue("Item B")),
		entity.Cell(entity.NumberValue(200)),
	)

	c := newTestConverter(map[string]any{"table1": tv}, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeTableInjector,
		Attrs: map[string]any{"variableId": "table1", "lang": "en"},
	}
	got := c.convertNode(node)

	if !strings.Contains(got, "#table(") {
		t.Errorf("expected #table(, got %q", got)
	}
	if !strings.Contains(got, "Name") || !strings.Contains(got, "Amount") {
		t.Errorf("expected column headers, got %q", got)
	}
	if !strings.Contains(got, "Item A") || !strings.Contains(got, "Item B") {
		t.Errorf("expected row data, got %q", got)
	}
}

func TestTypstConverter_TableInjectorSpanish(t *testing.T) {
	tv := entity.NewTableValue()
	tv.AddColumn("name", map[string]string{"en": "Name", "es": "Nombre"}, entity.ValueTypeString)
	tv.AddRow(entity.Cell(entity.StringValue("X")))

	c := newTestConverter(map[string]any{"t1": tv}, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeTableInjector,
		Attrs: map[string]any{"variableId": "t1", "lang": "es"},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "Nombre") {
		t.Errorf("expected Spanish label, got %q", got)
	}
}

func TestTypstConverter_TableInjectorMissing(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeTableInjector,
		Attrs: map[string]any{"variableId": "missing", "label": "My Table"},
	}
	got := c.convertNode(node)
	// Doc-assembly shows placeholder for missing table injectables
	if !strings.Contains(got, "Table") && got != "" {
		t.Errorf("expected placeholder or empty for missing table injectable, got %q", got)
	}
}

// --- Escaping ---

func TestEscapeTypst(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello", "hello"},
		{"$100", "\\$100"},
		{"#tag", "\\#tag"},
		{"*bold*", "\\*bold\\*"},
		{"a_b", "a\\_b"},
		{"<>", "\\<\\>"},
		{"[x]", "\\[x\\]"},
		{"@ref", "\\@ref"},
	}
	for _, tt := range tests {
		got := escapeTypst(tt.input)
		if got != tt.want {
			t.Errorf("escapeTypst(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestUnescapeTypst(t *testing.T) {
	original := "$100 #tag *bold* _underscored_"
	escaped := escapeTypst(original)
	unescaped := unescapeTypst(escaped)
	if unescaped != original {
		t.Errorf("round-trip failed: %q -> %q -> %q", original, escaped, unescaped)
	}
}

func TestEscapeTypstString(t *testing.T) {
	got := escapeTypstString(`he said "hello" and \ that`)
	want := `he said \"hello\" and \\ that`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// --- ConvertNodes batch ---

func TestTypstConverter_ConvertNodes(t *testing.T) {
	c := newTestConverter(nil, nil)
	nodes := []portabledoc.Node{
		paragraphNode(textNode("First")),
		paragraphNode(textNode("Second")),
	}
	got, _ := c.ConvertNodes(nodes)
	if !strings.Contains(got, "First") || !strings.Contains(got, "Second") {
		t.Errorf("expected both paragraphs, got %q", got)
	}
}

// --- Unknown node ---

func TestTypstConverter_UnknownNode(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{
		Type:    "somethingNew",
		Content: []portabledoc.Node{paragraphNode(textNode("inner"))},
	}
	got := c.convertNode(node)
	if !strings.Contains(got, "inner") {
		t.Errorf("unknown node should render children, got %q", got)
	}
}

func TestTypstConverter_UnknownNodeEmpty(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := portabledoc.Node{Type: "somethingNew"}
	got := c.convertNode(node)
	if got != "" {
		t.Errorf("expected empty, got %q", got)
	}
}

// --- Builder ---

func TestTypstBuilder_Build(t *testing.T) {
	doc := &portabledoc.Document{
		Meta: portabledoc.Meta{
			Title:    "Test Doc",
			Language: "en",
		},
		PageConfig: portabledoc.PageConfig{
			FormatID: portabledoc.PageFormatA4,
			Width:    794,
			Height:   1123,
			Margins: portabledoc.Margins{
				Top: 72, Bottom: 72, Left: 72, Right: 72,
			},
			ShowPageNumbers: true,
		},
		Content: &portabledoc.ProseMirrorDoc{
			Type: "doc",
			Content: []portabledoc.Node{
				paragraphNode(textNode("Hello world")),
			},
		},
	}

	factory := NewTypstConverterFactory(DefaultDesignTokens())
	conv := factory(nil, nil, nil, nil, nil)
	builder := NewTypstBuilder(conv, DefaultDesignTokens())
	got, _, _ := builder.Build(doc)

	checks := []string{
		`#set page(`,
		`paper: "a4"`,
		`numbering: "1 / 1"`,
		`#set text(`,
		`size: 12pt`,
		`Hello world`,
		`heading.where(level: 1)`,
	}
	for _, check := range checks {
		if !strings.Contains(got, check) {
			t.Errorf("expected output to contain %q, got:\n%s", check, got)
		}
	}
}

func TestTypstBuilder_CustomPageSize(t *testing.T) {
	doc := &portabledoc.Document{
		Meta: portabledoc.Meta{Title: "Custom", Language: "en"},
		PageConfig: portabledoc.PageConfig{
			FormatID: portabledoc.PageFormatCustom,
			Width:    500,
			Height:   700,
			Margins:  portabledoc.Margins{Top: 48, Bottom: 48, Left: 48, Right: 48},
		},
		Content: &portabledoc.ProseMirrorDoc{Type: "doc", Content: []portabledoc.Node{}},
	}

	factory := NewTypstConverterFactory(DefaultDesignTokens())
	conv := factory(nil, nil, nil, nil, nil)
	builder := NewTypstBuilder(conv, DefaultDesignTokens())
	got, _, _ := builder.Build(doc)

	if strings.Contains(got, "paper:") {
		t.Errorf("custom size should not use paper:, got:\n%s", got)
	}
	if !strings.Contains(got, "width:") || !strings.Contains(got, "height:") {
		t.Errorf("expected explicit width/height, got:\n%s", got)
	}
}

func TestTypstBuilder_PageCount(t *testing.T) {
	doc := &portabledoc.Document{
		Meta:       portabledoc.Meta{Title: "Test", Language: "en"},
		PageConfig: portabledoc.PageConfig{FormatID: portabledoc.PageFormatA4, Width: 794, Height: 1123, Margins: portabledoc.Margins{}},
		Content: &portabledoc.ProseMirrorDoc{
			Type: "doc",
			Content: []portabledoc.Node{
				paragraphNode(textNode("Page 1")),
				{Type: portabledoc.NodeTypePageBreak},
				paragraphNode(textNode("Page 2")),
				{Type: portabledoc.NodeTypePageBreak},
				paragraphNode(textNode("Page 3")),
			},
		},
	}

	factory := NewTypstConverterFactory(DefaultDesignTokens())
	conv := factory(nil, nil, nil, nil, nil)
	builder := NewTypstBuilder(conv, DefaultDesignTokens())
	_, pageCount, _ := builder.Build(doc)

	if pageCount != 3 {
		t.Errorf("expected 3 pages, got %d", pageCount)
	}
}

// --- Signature Block Layouts ---

func makeSigs(n int) []portabledoc.SignatureItem {
	sigs := make([]portabledoc.SignatureItem, 0, n)
	for i := range n {
		sigs = append(sigs, portabledoc.SignatureItem{
			ID:    fmt.Sprintf("sig_%d", i),
			Label: fmt.Sprintf("Signer %d", i),
		})
	}
	return sigs
}

func makeSigsWithSubtitle(n int, subtitleIdx int) []portabledoc.SignatureItem {
	sigs := makeSigs(n)
	sub := "Director"
	sigs[subtitleIdx].Subtitle = &sub
	return sigs
}

func TestRenderSignatureBlock_SingleLayouts(t *testing.T) {
	c := newTestConverter(nil, nil)
	tests := []struct {
		layout    string
		wantAlign string
	}{
		{portabledoc.LayoutSingleLeft, "#align(left)"},
		{portabledoc.LayoutSingleCenter, "#align(center)"},
		{portabledoc.LayoutSingleRight, "#align(right)"},
	}
	for _, tt := range tests {
		t.Run(tt.layout, func(t *testing.T) {
			attrs := portabledoc.SignatureAttrs{Count: 1, Layout: tt.layout, LineWidth: "md", Signatures: makeSigs(1)}
			got := c.renderSignatureBlock(attrs)
			if !strings.Contains(got, tt.wantAlign) {
				t.Errorf("layout %s: expected %q in output:\n%s", tt.layout, tt.wantAlign, got)
			}
			if strings.Contains(got, "#grid") {
				t.Errorf("layout %s: should NOT contain #grid:\n%s", tt.layout, got)
			}
		})
	}
}

func TestRenderSignatureBlock_DualSides(t *testing.T) {
	c := newTestConverter(nil, nil)
	attrs := portabledoc.SignatureAttrs{Count: 2, Layout: portabledoc.LayoutDualSides, LineWidth: "md", Signatures: makeSigs(2)}
	got := c.renderSignatureBlock(attrs)

	if !strings.Contains(got, "#grid") {
		t.Fatalf("dual-sides should contain #grid:\n%s", got)
	}
	if !strings.Contains(got, "columns: (1fr, 1fr)") {
		t.Errorf("dual-sides should have 2 columns:\n%s", got)
	}
}

func TestRenderSignatureBlock_DualStacked(t *testing.T) {
	c := newTestConverter(nil, nil)
	tests := []struct {
		layout    string
		wantAlign string
	}{
		{portabledoc.LayoutDualCenter, "#align(center)"},
		{portabledoc.LayoutDualLeft, "#align(left)"},
		{portabledoc.LayoutDualRight, "#align(right)"},
	}
	for _, tt := range tests {
		t.Run(tt.layout, func(t *testing.T) {
			attrs := portabledoc.SignatureAttrs{Count: 2, Layout: tt.layout, LineWidth: "md", Signatures: makeSigs(2)}
			got := c.renderSignatureBlock(attrs)
			if strings.Contains(got, "#grid") {
				t.Errorf("layout %s should NOT contain #grid:\n%s", tt.layout, got)
			}
			if !strings.Contains(got, tt.wantAlign) {
				t.Errorf("layout %s: expected %q:\n%s", tt.layout, tt.wantAlign, got)
			}
			if !strings.Contains(got, "#v(3em)") {
				t.Errorf("layout %s: expected vertical spacing #v(3em):\n%s", tt.layout, got)
			}
			// Should have 2 align blocks
			if strings.Count(got, tt.wantAlign) < 2 {
				t.Errorf("layout %s: expected 2 align blocks, got %d:\n%s", tt.layout, strings.Count(got, tt.wantAlign), got)
			}
		})
	}
}

func TestRenderSignatureBlock_TripleRow(t *testing.T) {
	c := newTestConverter(nil, nil)
	attrs := portabledoc.SignatureAttrs{Count: 3, Layout: portabledoc.LayoutTripleRow, LineWidth: "md", Signatures: makeSigs(3)}
	got := c.renderSignatureBlock(attrs)

	if !strings.Contains(got, "#grid") {
		t.Fatalf("triple-row should contain #grid:\n%s", got)
	}
	if !strings.Contains(got, "columns: (1fr, 1fr, 1fr)") {
		t.Errorf("triple-row should have 3 columns:\n%s", got)
	}
}

func TestRenderSignatureBlock_TriplePyramid(t *testing.T) {
	c := newTestConverter(nil, nil)
	attrs := portabledoc.SignatureAttrs{Count: 3, Layout: portabledoc.LayoutTriplePyramid, LineWidth: "md", Signatures: makeSigs(3)}
	got := c.renderSignatureBlock(attrs)

	// Should have a 2-col grid for top row
	if !strings.Contains(got, "columns: (1fr, 1fr)") {
		t.Errorf("triple-pyramid should have 2-col grid for top:\n%s", got)
	}
	// Should have centered bottom
	if !strings.Contains(got, "#align(center)") {
		t.Errorf("triple-pyramid should have #align(center) for bottom:\n%s", got)
	}
	// Grid should appear BEFORE align(center)
	gridIdx := strings.Index(got, "#grid")
	alignIdx := strings.LastIndex(got, "#align(center)")
	if gridIdx > alignIdx {
		t.Errorf("triple-pyramid: grid should come before centered align:\n%s", got)
	}
}

func TestRenderSignatureBlock_TripleInverted(t *testing.T) {
	c := newTestConverter(nil, nil)
	attrs := portabledoc.SignatureAttrs{Count: 3, Layout: portabledoc.LayoutTripleInverted, LineWidth: "md", Signatures: makeSigs(3)}
	got := c.renderSignatureBlock(attrs)

	// Should have centered top
	if !strings.Contains(got, "#align(center)") {
		t.Errorf("triple-inverted should have #align(center) for top:\n%s", got)
	}
	// Should have 2-col grid for bottom
	if !strings.Contains(got, "columns: (1fr, 1fr)") {
		t.Errorf("triple-inverted should have 2-col grid for bottom:\n%s", got)
	}
	// Align(center) should appear BEFORE grid
	alignIdx := strings.Index(got, "#align(center)")
	gridIdx := strings.Index(got, "#grid")
	if alignIdx > gridIdx {
		t.Errorf("triple-inverted: centered align should come before grid:\n%s", got)
	}
}

func TestRenderSignatureBlock_QuadGrid(t *testing.T) {
	c := newTestConverter(nil, nil)
	attrs := portabledoc.SignatureAttrs{Count: 4, Layout: portabledoc.LayoutQuadGrid, LineWidth: "md", Signatures: makeSigs(4)}
	got := c.renderSignatureBlock(attrs)

	if !strings.Contains(got, "columns: (1fr, 1fr)") {
		t.Errorf("quad-grid should have 2 columns:\n%s", got)
	}
	if !strings.Contains(got, "row-gutter: 3em") {
		t.Errorf("quad-grid should have row-gutter: 3em:\n%s", got)
	}
}

func TestRenderSignatureBlock_QuadTopHeavy(t *testing.T) {
	c := newTestConverter(nil, nil)
	attrs := portabledoc.SignatureAttrs{Count: 4, Layout: portabledoc.LayoutQuadTopHeavy, LineWidth: "md", Signatures: makeSigs(4)}
	got := c.renderSignatureBlock(attrs)

	// Should have 3-col grid for top
	if !strings.Contains(got, "columns: (1fr, 1fr, 1fr)") {
		t.Errorf("quad-top-heavy should have 3-col grid:\n%s", got)
	}
	// Should have centered bottom
	if !strings.Contains(got, "#align(center)") {
		t.Errorf("quad-top-heavy should have #align(center):\n%s", got)
	}
	// Grid before centered
	gridIdx := strings.Index(got, "#grid")
	alignIdx := strings.LastIndex(got, "#align(center)")
	if gridIdx > alignIdx {
		t.Errorf("quad-top-heavy: grid should come before centered:\n%s", got)
	}
}

func TestRenderSignatureBlock_QuadBottomHeavy(t *testing.T) {
	c := newTestConverter(nil, nil)
	attrs := portabledoc.SignatureAttrs{Count: 4, Layout: portabledoc.LayoutQuadBottomHeavy, LineWidth: "md", Signatures: makeSigs(4)}
	got := c.renderSignatureBlock(attrs)

	// Should have centered top
	if !strings.Contains(got, "#align(center)") {
		t.Errorf("quad-bottom-heavy should have #align(center):\n%s", got)
	}
	// Should have 3-col grid for bottom
	if !strings.Contains(got, "columns: (1fr, 1fr, 1fr)") {
		t.Errorf("quad-bottom-heavy should have 3-col grid:\n%s", got)
	}
	// Centered before grid
	alignIdx := strings.Index(got, "#align(center)")
	gridIdx := strings.Index(got, "#grid")
	if alignIdx > gridIdx {
		t.Errorf("quad-bottom-heavy: centered should come before grid:\n%s", got)
	}
}

func TestRenderSignatureBlock_SubtitleAlignment(t *testing.T) {
	c := newTestConverter(nil, nil)
	// One signature with subtitle, one without - in side-by-side layout
	attrs := portabledoc.SignatureAttrs{
		Count: 2, Layout: portabledoc.LayoutDualSides, LineWidth: "md",
		Signatures: makeSigsWithSubtitle(2, 0),
	}
	got := c.renderSignatureBlock(attrs)

	// Grid should use top alignment to keep lines at same height
	if !strings.Contains(got, "align: center + top") {
		t.Errorf("side-by-side layout should use center + top alignment:\n%s", got)
	}
	// Should contain subtitle text
	if !strings.Contains(got, "Director") {
		t.Errorf("should contain subtitle text:\n%s", got)
	}
}

func TestRenderSignatureBlock_PlacesAnchorOnSignatureLineOutOfFlow(t *testing.T) {
	c := newTestConverter(nil, nil)
	roleID := "role_signer"
	attrs := portabledoc.SignatureAttrs{
		Count:     1,
		Layout:    portabledoc.LayoutSingleCenter,
		LineWidth: "md",
		Signatures: []portabledoc.SignatureItem{{
			ID:     "sig_1",
			RoleID: &roleID,
			Label:  "Signer",
		}},
	}

	got := c.renderSignatureBlock(attrs)

	if !strings.Contains(got, `#place(top + center, dy: 0.0pt)[#text(size: 0.1pt, fill: white)[\_\_sig\_sig\_1\_\_]]`) {
		t.Fatalf("expected out-of-flow anchor placed at the signature line, got:\n%s", got)
	}
	if strings.Contains(got, "    #text(size: 0.1pt, fill: white)[\\_\\_sig\\_sig\\_1\\_\\_]\n    #block") {
		t.Fatalf("anchor must not be a flow text row before the line block, got:\n%s", got)
	}
}

func TestCapSignatureLineWidth(t *testing.T) {
	c := newTestConverter(nil, nil)
	// A4: contentWidthPx=642.5 → contentWidthPt=481.875 → 3-col max=(481.875-22)/3≈153.3pt
	c.SetContentWidthPx(642.5)

	tests := []struct {
		name     string
		lwPt     string
		expected string
	}{
		{"lg gets capped", "250pt", "153.3pt"},
		{"md gets capped", "180pt", "153.3pt"},
		{"sm fits", "120pt", "120pt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.capSignatureLineWidth(tt.lwPt)
			if got != tt.expected {
				t.Errorf("got %s, want %s", got, tt.expected)
			}
		})
	}

	t.Run("no contentWidth keeps raw", func(t *testing.T) {
		c2 := newTestConverter(nil, nil)
		got := c2.capSignatureLineWidth("250pt")
		if got != "250pt" {
			t.Errorf("got %s, want 250pt", got)
		}
	})
}

// --- Interactive Field Tests ---

func newTestConverterWithFieldResponses(fieldResponses map[string]json.RawMessage) *typstConverter {
	factory := NewTypstConverterFactory(DefaultDesignTokens())
	conv := factory(nil, nil, nil, nil, fieldResponses)
	return conv.(*typstConverter)
}

func interactiveFieldNode(attrs map[string]any) portabledoc.Node {
	return portabledoc.Node{
		Type:  portabledoc.NodeTypeInteractiveField,
		Attrs: attrs,
	}
}

func TestTypstConverter_InteractiveFieldCheckbox_NoResponse(t *testing.T) {
	c := newTestConverterWithFieldResponses(nil)
	node := interactiveFieldNode(map[string]any{
		"id":        "field_1",
		"fieldType": "checkbox",
		"label":     "Select toppings",
		"options": []any{
			map[string]any{"id": "opt_a", "label": "Cheese"},
			map[string]any{"id": "opt_b", "label": "Pepperoni"},
		},
	})
	got := c.convertNode(node)

	if !strings.Contains(got, `#text(weight: "bold")[Select toppings]`) {
		t.Errorf("expected label, got:\n%s", got)
	}
	if !strings.Contains(got, "☐ Cheese") {
		t.Errorf("expected unchecked Cheese, got:\n%s", got)
	}
	if !strings.Contains(got, "☐ Pepperoni") {
		t.Errorf("expected unchecked Pepperoni, got:\n%s", got)
	}
	if strings.Contains(got, "☑") {
		t.Errorf("no options should be checked without response, got:\n%s", got)
	}
}

func TestTypstConverter_InteractiveFieldCheckbox_WithResponse(t *testing.T) {
	responses := map[string]json.RawMessage{
		"field_1": json.RawMessage(`{"selectedOptionIds":["opt_b"]}`),
	}
	c := newTestConverterWithFieldResponses(responses)
	node := interactiveFieldNode(map[string]any{
		"id":        "field_1",
		"fieldType": "checkbox",
		"label":     "Select toppings",
		"options": []any{
			map[string]any{"id": "opt_a", "label": "Cheese"},
			map[string]any{"id": "opt_b", "label": "Pepperoni"},
		},
	})
	got := c.convertNode(node)

	if !strings.Contains(got, "☐ Cheese") {
		t.Errorf("expected unchecked Cheese, got:\n%s", got)
	}
	if !strings.Contains(got, "☑ Pepperoni") {
		t.Errorf("expected checked Pepperoni, got:\n%s", got)
	}
}

func TestTypstConverter_InteractiveFieldCheckbox_MultipleSelected(t *testing.T) {
	responses := map[string]json.RawMessage{
		"field_1": json.RawMessage(`{"selectedOptionIds":["opt_a","opt_b"]}`),
	}
	c := newTestConverterWithFieldResponses(responses)
	node := interactiveFieldNode(map[string]any{
		"id":        "field_1",
		"fieldType": "checkbox",
		"options": []any{
			map[string]any{"id": "opt_a", "label": "A"},
			map[string]any{"id": "opt_b", "label": "B"},
			map[string]any{"id": "opt_c", "label": "C"},
		},
	})
	got := c.convertNode(node)

	if !strings.Contains(got, "☑ A") {
		t.Errorf("expected checked A, got:\n%s", got)
	}
	if !strings.Contains(got, "☑ B") {
		t.Errorf("expected checked B, got:\n%s", got)
	}
	if !strings.Contains(got, "☐ C") {
		t.Errorf("expected unchecked C, got:\n%s", got)
	}
}

func TestTypstConverter_InteractiveFieldRadio_NoResponse(t *testing.T) {
	c := newTestConverterWithFieldResponses(nil)
	node := interactiveFieldNode(map[string]any{
		"id":        "field_2",
		"fieldType": "radio",
		"label":     "Pick one",
		"options": []any{
			map[string]any{"id": "opt_x", "label": "Option X"},
			map[string]any{"id": "opt_y", "label": "Option Y"},
		},
	})
	got := c.convertNode(node)

	if !strings.Contains(got, "○ Option X") {
		t.Errorf("expected unselected Option X, got:\n%s", got)
	}
	if !strings.Contains(got, "○ Option Y") {
		t.Errorf("expected unselected Option Y, got:\n%s", got)
	}
	if strings.Contains(got, "◉") {
		t.Errorf("no radio should be selected without response, got:\n%s", got)
	}
}

func TestTypstConverter_InteractiveFieldRadio_WithResponse(t *testing.T) {
	responses := map[string]json.RawMessage{
		"field_2": json.RawMessage(`{"selectedOptionIds":["opt_x"]}`),
	}
	c := newTestConverterWithFieldResponses(responses)
	node := interactiveFieldNode(map[string]any{
		"id":        "field_2",
		"fieldType": "radio",
		"label":     "Pick one",
		"options": []any{
			map[string]any{"id": "opt_x", "label": "Option X"},
			map[string]any{"id": "opt_y", "label": "Option Y"},
		},
	})
	got := c.convertNode(node)

	if !strings.Contains(got, "◉ Option X") {
		t.Errorf("expected selected Option X, got:\n%s", got)
	}
	if !strings.Contains(got, "○ Option Y") {
		t.Errorf("expected unselected Option Y, got:\n%s", got)
	}
}

func TestTypstConverter_InteractiveFieldText_NoResponse(t *testing.T) {
	c := newTestConverterWithFieldResponses(nil)
	node := interactiveFieldNode(map[string]any{
		"id":          "field_3",
		"fieldType":   "text",
		"label":       "Your name",
		"placeholder": "Enter your name",
	})
	got := c.convertNode(node)

	if !strings.Contains(got, `#text(weight: "bold")[Your name]`) {
		t.Errorf("expected label, got:\n%s", got)
	}
	if !strings.Contains(got, `#text(fill: gray)[Enter your name]`) {
		t.Errorf("expected placeholder in gray, got:\n%s", got)
	}
}

func TestTypstConverter_InteractiveFieldText_WithResponse(t *testing.T) {
	responses := map[string]json.RawMessage{
		"field_3": json.RawMessage(`{"text":"John Doe"}`),
	}
	c := newTestConverterWithFieldResponses(responses)
	node := interactiveFieldNode(map[string]any{
		"id":          "field_3",
		"fieldType":   "text",
		"label":       "Your name",
		"placeholder": "Enter your name",
	})
	got := c.convertNode(node)

	if !strings.Contains(got, "John Doe") {
		t.Errorf("expected response text, got:\n%s", got)
	}
	if strings.Contains(got, "placeholder") {
		t.Errorf("should not show placeholder when response exists, got:\n%s", got)
	}
}

func TestTypstConverter_InteractiveFieldText_EmptyPlaceholder(t *testing.T) {
	c := newTestConverterWithFieldResponses(nil)
	node := interactiveFieldNode(map[string]any{
		"id":        "field_4",
		"fieldType": "text",
	})
	got := c.convertNode(node)

	// Should still produce a block, just without placeholder
	if !strings.Contains(got, "#block[") {
		t.Errorf("expected block wrapper, got:\n%s", got)
	}
	if strings.Contains(got, "fill: gray") {
		t.Errorf("should not have gray text without placeholder, got:\n%s", got)
	}
}

func TestTypstConverter_InteractiveFieldNoLabel(t *testing.T) {
	c := newTestConverterWithFieldResponses(nil)
	node := interactiveFieldNode(map[string]any{
		"id":        "field_5",
		"fieldType": "checkbox",
		"options": []any{
			map[string]any{"id": "opt_1", "label": "Yes"},
		},
	})
	got := c.convertNode(node)

	if strings.Contains(got, `weight: "bold"`) {
		t.Errorf("should not render label when empty, got:\n%s", got)
	}
	if !strings.Contains(got, "#stack") {
		t.Errorf("should still render options, got:\n%s", got)
	}
}

func TestTypstConverter_InteractiveFieldText_SpecialChars(t *testing.T) {
	responses := map[string]json.RawMessage{
		"field_6": json.RawMessage(`{"text":"Price is $100 #tag"}`),
	}
	c := newTestConverterWithFieldResponses(responses)
	node := interactiveFieldNode(map[string]any{
		"id":        "field_6",
		"fieldType": "text",
		"label":     "Amount & notes",
	})
	got := c.convertNode(node)

	// Should escape special Typst characters
	if !strings.Contains(got, "\\$") {
		t.Errorf("expected escaped $, got:\n%s", got)
	}
	if !strings.Contains(got, "\\#") {
		t.Errorf("expected escaped #, got:\n%s", got)
	}
}

func TestTypstConverter_InteractiveFieldInvalidAttrs(t *testing.T) {
	c := newTestConverterWithFieldResponses(nil)
	// Missing required attrs - ParseInteractiveFieldAttrs should still work
	// but produce an empty result
	node := portabledoc.Node{
		Type:  portabledoc.NodeTypeInteractiveField,
		Attrs: map[string]any{},
	}
	got := c.convertNode(node)

	// Should produce a block (empty field type defaults to no options/text)
	if !strings.Contains(got, "#block[") {
		t.Errorf("expected block wrapper even for empty attrs, got:\n%s", got)
	}
}

// Ensure unused import is satisfied
var _ = port.SignatureField{}
var _ = entity.ListValue{}
