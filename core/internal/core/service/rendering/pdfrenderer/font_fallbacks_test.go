package pdfrenderer

import (
	"strings"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
)

// --- fontWithFallbacks unit tests ---

func TestFontWithFallbacks_KnownFonts(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Courier New", `("Courier New", "Liberation Mono", "DejaVu Sans Mono")`},
		{"Arial", `("Arial", "Liberation Sans", "DejaVu Sans")`},
		{"Times New Roman", `("Times New Roman", "Liberation Serif", "DejaVu Serif")`},
		{"Helvetica", `("Helvetica", "Liberation Sans", "DejaVu Sans")`},
		{"Helvetica Neue", `("Helvetica Neue", "Helvetica", "Liberation Sans")`},
		{"Georgia", `("Georgia", "Noto Serif", "Liberation Serif")`},
		{"Inter", `("Inter", "Arial", "Helvetica Neue", "Liberation Sans")`},
		{"Space Grotesk", `("Space Grotesk", "Inter", "Arial", "Helvetica Neue", "Liberation Sans")`},
	}

	for _, tt := range tests {
		got := fontWithFallbacks(tt.input)
		if got != tt.want {
			t.Errorf("fontWithFallbacks(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFontWithFallbacks_CaseInsensitive(t *testing.T) {
	tests := []struct {
		input string
	}{
		{"courier new"},
		{"COURIER NEW"},
		{"Courier New"},
		{"courier New"},
	}

	for _, tt := range tests {
		got := fontWithFallbacks(tt.input)
		if !strings.Contains(got, "Liberation Mono") {
			t.Errorf("fontWithFallbacks(%q) should match case-insensitively, got %q", tt.input, got)
		}
	}
}

func TestFontWithFallbacks_UnknownFont(t *testing.T) {
	got := fontWithFallbacks("CustomBrandFont")
	want := `"CustomBrandFont"`
	if got != want {
		t.Errorf("fontWithFallbacks(%q) = %q, want %q", "CustomBrandFont", got, want)
	}
}

func TestFontWithFallbacks_TrimsWhitespace(t *testing.T) {
	got := fontWithFallbacks("  Arial  ")
	if !strings.Contains(got, "Liberation Sans") {
		t.Errorf("fontWithFallbacks should trim whitespace, got %q", got)
	}
}

func TestFontWithFallbacks_EmptyString(t *testing.T) {
	got := fontWithFallbacks("")
	want := `""`
	if got != want {
		t.Errorf("fontWithFallbacks(%q) = %q, want %q", "", got, want)
	}
}

// --- Integration: textStyle mark with fontFamily ---

func TestTypstConverter_TextStyleFontFallback(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := markedTextNode("hello", markOf(portabledoc.MarkTypeTextStyle, map[string]any{
		"fontFamily": "Courier New, monospace",
	}))
	got := c.convertNode(node)

	if !strings.Contains(got, `"Courier New"`) {
		t.Errorf("expected Courier New in output, got %q", got)
	}
	if !strings.Contains(got, `"Liberation Mono"`) {
		t.Errorf("expected Liberation Mono fallback, got %q", got)
	}
	if !strings.Contains(got, `"DejaVu Sans Mono"`) {
		t.Errorf("expected DejaVu Sans Mono fallback, got %q", got)
	}
	if !strings.Contains(got, "font: (") {
		t.Errorf("expected font tuple syntax, got %q", got)
	}
}

func TestTypstConverter_TextStyleFontUnknown(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := markedTextNode("hello", markOf(portabledoc.MarkTypeTextStyle, map[string]any{
		"fontFamily": "MyCustomFont",
	}))
	got := c.convertNode(node)

	if !strings.Contains(got, `font: "MyCustomFont"`) {
		t.Errorf("expected single font string for unknown font, got %q", got)
	}
}

func TestTypstConverter_TextStyleFontWithColor(t *testing.T) {
	c := newTestConverter(nil, nil)
	node := markedTextNode("hello", markOf(portabledoc.MarkTypeTextStyle, map[string]any{
		"fontFamily": "Arial, sans-serif",
		"color":      "#ff0000",
	}))
	got := c.convertNode(node)

	if !strings.Contains(got, `fill: rgb("#ff0000")`) {
		t.Errorf("expected color param, got %q", got)
	}
	if !strings.Contains(got, `"Liberation Sans"`) {
		t.Errorf("expected Arial fallback, got %q", got)
	}
}

func TestTypstConverter_TextStyleAllEditorFonts(t *testing.T) {
	editorFonts := []struct {
		cssValue     string
		expectedFont string
		expectedFB   string
	}{
		{"Inter", "Inter", "Helvetica Neue"},
		{"Arial, sans-serif", "Arial", "Liberation Sans"},
		{"Times New Roman, serif", "Times New Roman", "Liberation Serif"},
		{"Helvetica, sans-serif", "Helvetica", "Liberation Sans"},
		{"Georgia, serif", "Georgia", "Noto Serif"},
		{"Courier New, monospace", "Courier New", "Liberation Mono"},
	}

	for _, tt := range editorFonts {
		c := newTestConverter(nil, nil)
		node := markedTextNode("text", markOf(portabledoc.MarkTypeTextStyle, map[string]any{
			"fontFamily": tt.cssValue,
		}))
		got := c.convertNode(node)

		if !strings.Contains(got, tt.expectedFont) {
			t.Errorf("font %q: expected %q in output, got %q", tt.cssValue, tt.expectedFont, got)
		}
		if !strings.Contains(got, tt.expectedFB) {
			t.Errorf("font %q: expected fallback %q in output, got %q", tt.cssValue, tt.expectedFB, got)
		}
	}
}

// --- Integration: list styles with font fallback ---

func TestTypstConverter_ListStyleFontFallback(t *testing.T) {
	c := newTestConverter(nil, nil)
	styles := &entity.ListStyles{}
	family := "Arial"
	styles.FontFamily = &family

	parts := c.collectListStyleParts(styles)

	found := false
	for _, p := range parts {
		if strings.Contains(p, "font:") {
			found = true
			if !strings.Contains(p, "Liberation Sans") {
				t.Errorf("expected fallback in list style font, got %q", p)
			}
			if !strings.Contains(p, "(") {
				t.Errorf("expected tuple syntax in list style font, got %q", p)
			}
		}
	}
	if !found {
		t.Error("expected font param in list style parts")
	}
}

func TestTypstConverter_ListStyleFontUnknown(t *testing.T) {
	c := newTestConverter(nil, nil)
	styles := &entity.ListStyles{}
	family := "BrandFont"
	styles.FontFamily = &family

	parts := c.collectListStyleParts(styles)

	for _, p := range parts {
		if strings.Contains(p, "font:") {
			if strings.Contains(p, "(") {
				t.Errorf("unknown font should not produce tuple, got %q", p)
			}
			if !strings.Contains(p, `"BrandFont"`) {
				t.Errorf("expected single font string, got %q", p)
			}
		}
	}
}

// --- Integration: table style rules with font fallback ---

func TestTypstConverter_TableHeaderFontFallback(t *testing.T) {
	c := newTestConverter(nil, nil)
	family := "Times New Roman"
	headerStyles := &entity.TableStyles{FontFamily: &family}

	got := c.buildTableStyleRules(headerStyles)

	if !strings.Contains(got, "Liberation Serif") {
		t.Errorf("expected fallback in table header font rule, got %q", got)
	}
	if !strings.Contains(got, "Times New Roman") {
		t.Errorf("expected original font in table header font rule, got %q", got)
	}
}

func TestTypstConverter_TableBodyFontFallback(t *testing.T) {
	c := newTestConverter(nil, nil)
	family := "Courier New"
	bodyStyles := &entity.TableStyles{FontFamily: &family}

	got := c.buildTableBodyStyleRules(bodyStyles)

	if !strings.Contains(got, "Liberation Mono") {
		t.Errorf("expected fallback in table body font rule, got %q", got)
	}
	if !strings.Contains(got, "Courier New") {
		t.Errorf("expected original font in table body font rule, got %q", got)
	}
}

// --- Design tokens font stack ---

func TestDefaultDesignTokens_FontStackStartsWithInter(t *testing.T) {
	tokens := DefaultDesignTokens()

	if len(tokens.FontStack) == 0 {
		t.Fatal("default font stack should not be empty")
	}

	if tokens.FontStack[0] != "Inter" {
		t.Errorf("default font stack should start with Inter, got %v", tokens.FontStack)
	}
}

// --- Builder integration: base typography uses font stack ---

func TestTypstBuilder_TypographyIncludesFallbackFonts(t *testing.T) {
	tokens := DefaultDesignTokens()
	factory := NewTypstConverterFactory(tokens)
	conv := factory(map[string]any{}, map[string]string{}, nil, nil, nil)

	doc := &portabledoc.Document{
		Meta:       portabledoc.Meta{Title: "Test", Language: "en"},
		PageConfig: portabledoc.PageConfig{FormatID: portabledoc.PageFormatA4, Width: 794, Height: 1123, Margins: portabledoc.Margins{}},
		Content:    &portabledoc.ProseMirrorDoc{Type: "doc", Content: []portabledoc.Node{}},
	}

	builder := NewTypstBuilder(conv, tokens)
	got, _, _ := builder.Build(doc)

	if !strings.Contains(got, `"Inter"`) {
		t.Errorf("expected Inter in base typography, got:\n%s", got)
	}
	if !strings.Contains(got, `"Arial"`) {
		t.Errorf("expected Arial fallback in base typography, got:\n%s", got)
	}
}

// --- Full document with inline font ---

func TestTypstBuilder_DocumentWithInlineFont(t *testing.T) {
	tokens := DefaultDesignTokens()
	factory := NewTypstConverterFactory(tokens)
	conv := factory(map[string]any{}, map[string]string{}, nil, nil, nil)

	doc := &portabledoc.Document{
		Meta:       portabledoc.Meta{Title: "Test", Language: "en"},
		PageConfig: portabledoc.PageConfig{FormatID: portabledoc.PageFormatA4, Width: 794, Height: 1123, Margins: portabledoc.Margins{}},
		Content: &portabledoc.ProseMirrorDoc{
			Type: "doc",
			Content: []portabledoc.Node{
				paragraphNode(
					markedTextNode("mono text", markOf(portabledoc.MarkTypeTextStyle, map[string]any{
						"fontFamily": "Courier New, monospace",
					})),
				),
			},
		},
	}

	builder := NewTypstBuilder(conv, tokens)
	got, _, _ := builder.Build(doc)

	if !strings.Contains(got, `"Courier New"`) {
		t.Errorf("expected Courier New in output, got:\n%s", got)
	}
	if !strings.Contains(got, `"Liberation Mono"`) {
		t.Errorf("expected Liberation Mono fallback in output, got:\n%s", got)
	}
}
