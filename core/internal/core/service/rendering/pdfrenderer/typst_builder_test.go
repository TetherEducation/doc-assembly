package pdfrenderer

import (
	"strings"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

type typstBuilderConverterStub struct {
	convertedInputs [][]portabledoc.Node
	remoteImages    map[string]string
}

func (s *typstBuilderConverterStub) ConvertNodes(nodes []portabledoc.Node) (string, []port.SignatureField) {
	s.convertedInputs = append(s.convertedInputs, nodes)
	return "HEADER_TEXT", nil
}

func (s *typstBuilderConverterStub) GetCurrentPage() int {
	return 1
}

func (s *typstBuilderConverterStub) RemoteImages() map[string]string {
	if s.remoteImages == nil {
		s.remoteImages = map[string]string{}
	}
	return s.remoteImages
}

func (s *typstBuilderConverterStub) SetContentWidthPx(float64) {}

func (s *typstBuilderConverterStub) SetPageWidthPx(float64) {}

func (s *typstBuilderConverterStub) RegisterRemoteImage(url string) string {
	if s.remoteImages == nil {
		s.remoteImages = map[string]string{}
	}
	filename := "remote-image-1"
	s.remoteImages[url] = filename
	return filename
}

func (s *typstBuilderConverterStub) ResolveImageSource(attrs map[string]any) string {
	if src, ok := attrs["src"].(string); ok && src != "" {
		return src
	}
	return ""
}

func TestTypstBuilderHeaderBlock_ImageLeftWithText(t *testing.T) {
	converter := &typstBuilderConverterStub{}
	builder := NewTypstBuilder(converter, DefaultDesignTokens())
	imageURL := "https://example.com/logo.png"
	headerText := "Header text"
	imageWidth := 180
	imageHeight := 96
	pageConfig := &portabledoc.PageConfig{
		FormatID: portabledoc.PageFormatA4,
		Width:    794,
		Height:   1123,
		Margins: portabledoc.Margins{
			Top:    72,
			Bottom: 72,
			Left:   72,
			Right:  72,
		},
	}

	got := builder.headerBlock(&portabledoc.DocumentHeader{
		Enabled:     true,
		Layout:      portabledoc.HeaderLayoutImageLeft,
		ImageURL:    &imageURL,
		ImageWidth:  &imageWidth,
		ImageHeight: &imageHeight,
		Content: &portabledoc.ProseMirrorDoc{
			Type: "doc",
			Content: []portabledoc.Node{
				{
					Type:    portabledoc.NodeTypeParagraph,
					Content: []portabledoc.Node{{Type: portabledoc.NodeTypeText, Text: &headerText}},
				},
			},
		},
	}, pageConfig)

	if !strings.Contains(got, "columns: (135.0pt, 1fr)") {
		t.Fatalf("expected image-left grid columns, got %q", got)
	}
	if !strings.Contains(got, "#block(width: 100%)[\n          #grid(\n            columns: (135.0pt, 1fr)") {
		t.Fatalf("expected lateral header grid to span full usable width, got %q", got)
	}
	if !strings.Contains(got, "#block(width: 100%, height: 90.0pt)") {
		t.Fatalf("expected fixed header surface height, got %q", got)
	}
	if !strings.Contains(got, "#place(top + left, dy: -27.0pt)") {
		t.Fatalf("expected header visual offset via place, got %q", got)
	}
	if !strings.Contains(got, "#block(width: 100%)") {
		t.Fatalf("expected placed header content to keep full header width for center/right alignment, got %q", got)
	}
	if !strings.Contains(got, "column-gutter: 12.0pt") {
		t.Fatalf("expected header image gap to match frontend metrics, got %q", got)
	}
	if !strings.Contains(got, "#block(width: 135.0pt, height: 72.0pt)") {
		t.Fatalf("expected fixed-size image slot, got %q", got)
	}
	if !strings.Contains(got, "#image(\"remote-image-1\", height: 72.0pt, width: 135.0pt, fit: \"stretch\")") {
		t.Fatalf("expected registered image in header block to avoid cropping and match editor fill behavior, got %q", got)
	}
	if !strings.Contains(got, "#set par(spacing: 0pt)") {
		t.Fatalf("expected compact header text paragraph spacing, got %q", got)
	}
	if !strings.Contains(got, "#set text(size: 10.5pt)") {
		t.Fatalf("expected header text block to use editor-sized base font, got %q", got)
	}
	if !strings.Contains(got, "#set par(linebreaks: \"simple\")") {
		t.Fatalf("expected header text block to use simple line breaking, got %q", got)
	}
	if !strings.Contains(got, "HEADER_TEXT") {
		t.Fatalf("expected converted header text in block, got %q", got)
	}
	if strings.Contains(got, "#v(1em)") {
		t.Fatalf("expected legacy header spacer to be removed, got %q", got)
	}
	if converter.RemoteImages()[imageURL] != "remote-image-1" {
		t.Fatalf("expected remote image registration for %q", imageURL)
	}
}

func TestTypstBuilderHeaderBlock_ClampsImageWidthToHeaderContentWidth(t *testing.T) {
	converter := &typstBuilderConverterStub{}
	builder := NewTypstBuilder(converter, DefaultDesignTokens())
	imageURL := "https://example.com/logo.png"
	imageWidth := 600
	imageHeight := 96
	headerText := "Header text"
	pageConfig := &portabledoc.PageConfig{
		FormatID: portabledoc.PageFormatA4,
		Width:    794,
		Height:   1123,
		Margins: portabledoc.Margins{
			Top:    72,
			Bottom: 72,
			Left:   72,
			Right:  72,
		},
	}

	got := builder.headerBlock(&portabledoc.DocumentHeader{
		Enabled:     true,
		Layout:      portabledoc.HeaderLayoutImageRight,
		ImageURL:    &imageURL,
		ImageWidth:  &imageWidth,
		ImageHeight: &imageHeight,
		Content: &portabledoc.ProseMirrorDoc{
			Type: "doc",
			Content: []portabledoc.Node{
				{
					Type:    portabledoc.NodeTypeParagraph,
					Content: []portabledoc.Node{{Type: portabledoc.NodeTypeText, Text: &headerText}},
				},
			},
		},
	}, pageConfig)

	if !strings.Contains(got, "columns: (1fr, 295.5pt)") {
		t.Fatalf("expected clamped right-side image column, got %q", got)
	}
	if !strings.Contains(got, "#block(width: 100%)[\n          #grid(\n            columns: (1fr, 295.5pt)") {
		t.Fatalf("expected right-side header grid to span full usable width, got %q", got)
	}
	if !strings.Contains(got, "#block(width: 295.5pt, height: 72.0pt)") {
		t.Fatalf("expected clamped image slot width to match grid column, got %q", got)
	}
	if !strings.Contains(got, "#image(\"remote-image-1\", height: 72.0pt, width: 295.5pt, fit: \"stretch\")") {
		t.Fatalf("expected clamped image to use stretch fit and avoid cover cropping, got %q", got)
	}
}

func TestTypstBuilderHeaderBlock_ImageCenterUsesDedicatedCenteredLayout(t *testing.T) {
	converter := &typstBuilderConverterStub{}
	builder := NewTypstBuilder(converter, DefaultDesignTokens())
	imageURL := "https://example.com/logo.png"
	imageWidth := 180
	imageHeight := 96
	pageConfig := &portabledoc.PageConfig{
		FormatID: portabledoc.PageFormatA4,
		Width:    794,
		Height:   1123,
		Margins: portabledoc.Margins{
			Top:    72,
			Bottom: 72,
			Left:   72,
			Right:  72,
		},
	}

	got := builder.headerBlock(&portabledoc.DocumentHeader{
		Enabled:     true,
		Layout:      portabledoc.HeaderLayoutImageCenter,
		ImageURL:    &imageURL,
		ImageWidth:  &imageWidth,
		ImageHeight: &imageHeight,
	}, pageConfig)

	if !strings.Contains(got, "#align(center)[#block(width: 135.0pt, height: 72.0pt)") {
		t.Fatalf("expected centered image slot for center layout, got %q", got)
	}
	if strings.Contains(got, "#grid(") {
		t.Fatalf("expected center layout to avoid lateral grid composition, got %q", got)
	}
}

func TestTypstBuilderHeaderBlock_InjectableImageUsesContainFit(t *testing.T) {
	converter := &typstBuilderConverterStub{}
	builder := NewTypstBuilder(converter, DefaultDesignTokens())
	imageURL := "https://example.com/dynamic-logo.png"
	imageWidth := 180
	imageHeight := 96
	pageConfig := &portabledoc.PageConfig{
		FormatID: portabledoc.PageFormatA4,
		Width:    794,
		Height:   1123,
		Margins: portabledoc.Margins{
			Top:    72,
			Bottom: 72,
			Left:   72,
			Right:  72,
		},
	}

	got := builder.headerBlock(&portabledoc.DocumentHeader{
		Enabled:           true,
		Layout:            portabledoc.HeaderLayoutImageLeft,
		ImageURL:          &imageURL,
		ImageInjectableID: ptrTo("company_logo"),
		ImageWidth:        &imageWidth,
		ImageHeight:       &imageHeight,
	}, pageConfig)

	if !strings.Contains(got, "#image(\"remote-image-1\", height: 72.0pt, width: 135.0pt, fit: \"contain\")") {
		t.Fatalf("expected injectable header image to preserve its box with contain fit, got %q", got)
	}
}

func ptrTo[T any](value T) *T {
	return &value
}

func TestTypstBuilderHeaderBlock_TextOnlyDoesNotForceImageRightAlignment(t *testing.T) {
	converter := &typstBuilderConverterStub{}
	builder := NewTypstBuilder(converter, DefaultDesignTokens())
	headerText := "Header text"
	pageConfig := &portabledoc.PageConfig{
		FormatID: portabledoc.PageFormatA4,
		Width:    794,
		Height:   1123,
		Margins: portabledoc.Margins{
			Top:    72,
			Bottom: 72,
			Left:   72,
			Right:  72,
		},
	}

	got := builder.headerBlock(&portabledoc.DocumentHeader{
		Enabled: true,
		Layout:  portabledoc.HeaderLayoutImageRight,
		Content: &portabledoc.ProseMirrorDoc{
			Type: "doc",
			Content: []portabledoc.Node{
				{
					Type:    portabledoc.NodeTypeParagraph,
					Content: []portabledoc.Node{{Type: portabledoc.NodeTypeText, Text: &headerText}},
				},
			},
		},
	}, pageConfig)

	if strings.Contains(got, "#align(right)") {
		t.Fatalf("expected text-only header to preserve text slot layout without forced right alignment, got %q", got)
	}
	if !strings.Contains(got, "#block(width: 100%, height: 72.0pt)") {
		t.Fatalf("expected fixed-height header text slot, got %q", got)
	}
}

func TestTypstBuilderHeaderBlock_NormalizesConsecutiveParagraphsIntoHardBreaks(t *testing.T) {
	converter := &typstBuilderConverterStub{}
	builder := NewTypstBuilder(converter, DefaultDesignTokens())
	line1 := "Line 1"
	line2 := "Line 2"
	pageConfig := &portabledoc.PageConfig{
		FormatID: portabledoc.PageFormatA4,
		Width:    794,
		Height:   1123,
		Margins: portabledoc.Margins{
			Top:    72,
			Bottom: 72,
			Left:   72,
			Right:  72,
		},
	}

	builder.headerBlock(&portabledoc.DocumentHeader{
		Enabled: true,
		Layout:  portabledoc.HeaderLayoutImageLeft,
		Content: &portabledoc.ProseMirrorDoc{
			Type: "doc",
			Content: []portabledoc.Node{
				{
					Type:    portabledoc.NodeTypeParagraph,
					Content: []portabledoc.Node{{Type: portabledoc.NodeTypeText, Text: &line1}},
				},
				{
					Type:    portabledoc.NodeTypeParagraph,
					Content: []portabledoc.Node{{Type: portabledoc.NodeTypeText, Text: &line2}},
				},
			},
		},
	}, pageConfig)

	if len(converter.convertedInputs) != 1 {
		t.Fatalf("expected one header conversion call, got %d", len(converter.convertedInputs))
	}

	got := converter.convertedInputs[0]
	if len(got) != 1 {
		t.Fatalf("expected header paragraphs to be normalized into one node, got %#v", got)
	}
	if got[0].Type != portabledoc.NodeTypeParagraph {
		t.Fatalf("expected normalized node to remain a paragraph, got %#v", got[0])
	}
	if len(got[0].Content) != 3 {
		t.Fatalf("expected merged paragraph with hard break, got %#v", got[0].Content)
	}
	if got[0].Content[1].Type != portabledoc.NodeTypeHardBreak {
		t.Fatalf("expected hard break between merged header lines, got %#v", got[0].Content)
	}
}

func TestTypstBuilderHeaderBlock_DoesNotMergeParagraphsWithDifferentAttrs(t *testing.T) {
	converter := &typstBuilderConverterStub{}
	builder := NewTypstBuilder(converter, DefaultDesignTokens())
	line1 := "Line 1"
	line2 := "Line 2"
	pageConfig := &portabledoc.PageConfig{
		FormatID: portabledoc.PageFormatA4,
		Width:    794,
		Height:   1123,
		Margins: portabledoc.Margins{
			Top:    72,
			Bottom: 72,
			Left:   72,
			Right:  72,
		},
	}

	builder.headerBlock(&portabledoc.DocumentHeader{
		Enabled: true,
		Layout:  portabledoc.HeaderLayoutImageLeft,
		Content: &portabledoc.ProseMirrorDoc{
			Type: "doc",
			Content: []portabledoc.Node{
				{
					Type:    portabledoc.NodeTypeParagraph,
					Attrs:   map[string]any{"lineSpacing": portabledoc.LineSpacingCompact},
					Content: []portabledoc.Node{{Type: portabledoc.NodeTypeText, Text: &line1}},
				},
				{
					Type:    portabledoc.NodeTypeParagraph,
					Attrs:   map[string]any{"lineSpacing": portabledoc.LineSpacingLoose},
					Content: []portabledoc.Node{{Type: portabledoc.NodeTypeText, Text: &line2}},
				},
			},
		},
	}, pageConfig)

	got := converter.convertedInputs[0]
	if len(got) != 2 {
		t.Fatalf("expected paragraphs with different attrs to stay separate, got %#v", got)
	}
}

func TestTypstBuilderPageSetupHalvesTopMarginWhenHeaderEnabled(t *testing.T) {
	builder := NewTypstBuilder(&typstBuilderConverterStub{}, DefaultDesignTokens())
	pageConfig := &portabledoc.PageConfig{
		FormatID: portabledoc.PageFormatA4,
		Width:    794,
		Height:   1123,
		Margins: portabledoc.Margins{
			Top:    72,
			Bottom: 72,
			Left:   72,
			Right:  72,
		},
	}

	withHeader := builder.pageSetup(pageConfig, true)
	withoutHeader := builder.pageSetup(pageConfig, false)

	if !strings.Contains(withHeader, "top: 27.0pt") {
		t.Fatalf("expected top margin to be halved when header is enabled, got %q", withHeader)
	}
	if !strings.Contains(withoutHeader, "top: 54.0pt") {
		t.Fatalf("expected original top margin when header is disabled, got %q", withoutHeader)
	}
}

func TestTypstBuilderTypographySetup_ConfiguresTextEdgesForCssLikeLineHeight(t *testing.T) {
	builder := NewTypstBuilder(&typstBuilderConverterStub{}, DefaultDesignTokens())

	got := builder.typographySetup(portabledoc.LanguageSpanish)

	if !strings.Contains(got, "top-edge: 0.8em") {
		t.Fatalf("expected typography setup to set text top edge, got %q", got)
	}
	if !strings.Contains(got, "bottom-edge: -0.2em") {
		t.Fatalf("expected typography setup to set text bottom edge, got %q", got)
	}
	if !strings.Contains(got, "#set par(leading: 0.50em, spacing: 1.5em)") {
		t.Fatalf("expected typography setup to preserve global paragraph leading, got %q", got)
	}
	if strings.Contains(got, "linebreaks: \"simple\"") {
		t.Fatalf("expected simple line breaking to stay scoped to header text only, got %q", got)
	}
	if strings.Contains(got, "size: 10.5pt") {
		t.Fatalf("expected header base font size override to stay scoped to header text only, got %q", got)
	}
}

func TestTypstBuilderTypographySetup_SetsHyphenationLanguage(t *testing.T) {
	builder := NewTypstBuilder(&typstBuilderConverterStub{}, DefaultDesignTokens())

	// Without a language Typst hyphenates with English rules, which breaks Spanish words
	// at impossible points ("re-spetar", "Establec-imiento") on documents families sign.
	spanish := builder.typographySetup(portabledoc.LanguageSpanish)
	if !strings.Contains(spanish, `lang: "es"`) {
		t.Fatalf("expected Spanish hyphenation language, got %q", spanish)
	}

	english := builder.typographySetup(portabledoc.LanguageEnglish)
	if !strings.Contains(english, `lang: "en"`) {
		t.Fatalf("expected English hyphenation language, got %q", english)
	}

	// Every tenant here is Chilean, so an unset language is far likelier to be a missing
	// field than a genuinely English document.
	for _, absent := range []string{"", "   ", "pt-BR"} {
		got := builder.typographySetup(absent)
		if !strings.Contains(got, `lang: "es"`) {
			t.Fatalf("expected Spanish fallback for %q, got %q", absent, got)
		}
	}
}
