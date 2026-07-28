package pdfrenderer

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// pxToPt converts pixels (at 96 DPI) to typographic points.
const pxToPt = 0.75 // 1px at 96 DPI = 0.75pt

const (
	headerImageMinWidthPx = 32.0
	headerTextMinWidthPx  = 240.0
	headerImageGapPx      = 16.0
	headerImageHeightPx   = 96.0
	headerTextHeightPx    = 96.0
	headerTextBaseFontPt  = 10.5
	headerSurfacePadPx    = 12.0
	headerSurfaceMinPx    = headerTextHeightPx + (headerSurfacePadPx * 2)
)

type headerRenderMetrics struct {
	surfaceMinHeightPt   float64
	surfaceVerticalPadPt float64
	textSlotHeightPt     float64
	imageGapPt           float64
	headerVisualOffsetPt float64
}

// TypstBuilder constructs complete Typst documents from portable documents.
// It generates the document preamble (page setup, fonts, heading styles)
// and delegates node-by-node conversion to a TypstConverter.
type TypstBuilder struct {
	converter TypstConverter
	tokens    TypstDesignTokens
}

// NewTypstBuilder creates a new Typst builder with the given converter and design tokens.
func NewTypstBuilder(converter TypstConverter, tokens TypstDesignTokens) *TypstBuilder {
	return &TypstBuilder{
		converter: converter,
		tokens:    tokens,
	}
}

// Build creates a complete Typst document from a portable document.
// Returns the Typst source, page count, and signature fields.
func (b *TypstBuilder) Build(doc *portabledoc.Document) (string, int, []port.SignatureField) {
	var sb strings.Builder

	// Package imports
	sb.WriteString("#import \"@preview/wrap-it:0.1.1\": wrap-content\n\n")

	// Page configuration
	hasHeader := doc.Header != nil && doc.Header.Enabled
	sb.WriteString(b.pageSetup(&doc.PageConfig, hasHeader))

	// Base typography
	sb.WriteString(b.typographySetup())

	// Heading styles
	sb.WriteString(b.headingStyles())

	// Set page dimensions for column and signature field calculations
	b.converter.SetPageWidthPx(doc.PageConfig.Width)
	b.converter.SetContentWidthPx(doc.PageConfig.Width - doc.PageConfig.Margins.Left - doc.PageConfig.Margins.Right)

	// Render header block (letterhead, first page only)
	if doc.Header != nil && doc.Header.Enabled {
		sb.WriteString(b.headerBlock(doc.Header, &doc.PageConfig))
	}

	// Render content via converter
	if doc.Content != nil {
		typstContent, signatureFields := b.converter.ConvertNodes(doc.Content.Content)
		sb.WriteString(typstContent)
		pageCount := b.converter.GetCurrentPage()
		return sb.String(), pageCount, signatureFields
	}

	return sb.String(), 1, nil
}

// pageSetup generates #set page(...) directive from PageConfig.
func (b *TypstBuilder) pageSetup(config *portabledoc.PageConfig, hasHeader bool) string {
	marginTopPt := config.Margins.Top * pxToPt
	if hasHeader {
		marginTopPt /= 2
	}
	var sb strings.Builder

	// Check if this matches a standard paper size
	paper := detectPaperSize(config.FormatID)
	if paper != "" {
		fmt.Fprintf(&sb, "#set page(\n  paper: %q,\n", paper)
	} else {
		widthPt := config.Width * pxToPt
		heightPt := config.Height * pxToPt
		fmt.Fprintf(&sb, "#set page(\n  width: %.1fpt,\n  height: %.1fpt,\n", widthPt, heightPt)
	}

	fmt.Fprintf(&sb, "  margin: (top: %.1fpt, bottom: %.1fpt, left: %.1fpt, right: %.1fpt),\n",
		marginTopPt,
		config.Margins.Bottom*pxToPt,
		config.Margins.Left*pxToPt,
		config.Margins.Right*pxToPt,
	)

	// Page numbering (always enabled)
	sb.WriteString("  numbering: \"1 / 1\",\n")

	sb.WriteString(")\n\n")
	return sb.String()
}

// detectPaperSize maps FormatID to Typst paper names.
func detectPaperSize(formatID string) string {
	switch formatID {
	case portabledoc.PageFormatA4:
		return "a4"
	case portabledoc.PageFormatLetter:
		return "us-letter"
	case portabledoc.PageFormatLegal:
		return "us-legal"
	default:
		return "" // Custom -- use explicit width/height
	}
}

// typographySetup generates base text and paragraph settings.
func (b *TypstBuilder) typographySetup() string {
	var sb strings.Builder
	sb.WriteString("#set text(\n")

	// Font stack
	sb.WriteString("  font: (")
	for i, font := range b.tokens.FontStack {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "%q", font)
	}
	sb.WriteString("),\n")

	fmt.Fprintf(&sb, "  size: %s,\n", b.tokens.BaseFontSize)
	fmt.Fprintf(&sb, "  fill: rgb(%q),\n", b.tokens.BaseTextColor)
	sb.WriteString("  top-edge: 0.8em,\n")
	sb.WriteString("  bottom-edge: -0.2em,\n")
	sb.WriteString("  hyphenate: true,\n")
	sb.WriteString("  number-width: \"proportional\",\n")
	sb.WriteString(")\n\n")

	fmt.Fprintf(&sb, "#set par(leading: %s, spacing: %s)\n\n", b.tokens.ParagraphLeading, b.tokens.ParagraphSpacing)

	return sb.String()
}

// headingStyles generates show rules for heading sizes matching the CSS styles.
func (b *TypstBuilder) headingStyles() string {
	var sb strings.Builder
	for i, size := range b.tokens.HeadingSizes {
		fmt.Fprintf(&sb, "#show heading.where(level: %d): set text(size: %s, weight: %s)\n", i+1, size, b.tokens.HeadingWeight)
	}
	sb.WriteString("\n")
	return sb.String()
}

// RemoteImages returns the map of remote image URLs to local filenames
// collected during build by the converter.
func (b *TypstBuilder) RemoteImages() map[string]string {
	return b.converter.RemoteImages()
}

// headerBlock renders the document header (letterhead) as a Typst content block.
// It supports three layout modes: image-left, image-right, and image-center.
func (b *TypstBuilder) headerBlock(header *portabledoc.DocumentHeader, pageConfig *portabledoc.PageConfig) string {
	metrics := resolveHeaderRenderMetrics(pageConfig)
	textTypst := b.renderHeaderText(header.TextNodes(), metrics)
	hasText := strings.TrimSpace(textTypst) != ""
	maxImageWidthPx := resolveHeaderMaxImageWidthPx(pageConfig, hasText)
	imageTypst := b.renderHeaderImage(header, maxImageWidthPx)
	imageWidthPt, hasImageWidth := resolveHeaderImageWidthPt(header, maxImageWidthPx)
	imageSlot := renderHeaderImageSlot(imageTypst, imageWidthPt, hasImageWidth, metrics)

	var content string

	switch header.Layout {
	case portabledoc.HeaderLayoutImageCenter:
		content = renderCenteredHeader(imageSlot, textTypst)
	case portabledoc.HeaderLayoutImageRight:
		content = renderLateralHeader(textTypst, imageSlot, false, imageWidthPt, hasImageWidth, metrics)
	default: // image-left (default)
		content = renderLateralHeader(imageSlot, textTypst, true, imageWidthPt, hasImageWidth, metrics)
	}

	return renderHeaderSurface(content, metrics)
}

func (b *TypstBuilder) renderHeaderImage(header *portabledoc.DocumentHeader, maxWidthPx float64) string {
	if !header.HasImage() {
		return ""
	}

	attrs := map[string]any{}
	if header.ImageURL != nil {
		attrs["src"] = *header.ImageURL
	}
	if header.ImageInjectableID != nil {
		attrs["injectableId"] = *header.ImageInjectableID
	}

	src := b.converter.ResolveImageSource(attrs)
	if src == "" {
		return ""
	}

	imageFilename := src
	if strings.HasPrefix(src, "http://") ||
		strings.HasPrefix(src, "https://") ||
		strings.HasPrefix(src, "data:") ||
		strings.HasPrefix(src, "storage://") {
		imageFilename = b.converter.RegisterRemoteImage(src)
	}

	heightPx := headerImageHeightPx
	if header.ImageHeight != nil && *header.ImageHeight > 0 {
		heightPx = float64(*header.ImageHeight)
	}

	args := []string{
		fmt.Sprintf("%q", imageFilename),
		fmt.Sprintf("height: %.1fpt", heightPx*pxToPt),
	}

	isInjectableImage := header.ImageInjectableID != nil && *header.ImageInjectableID != ""
	if widthPt, ok := resolveHeaderImageWidthPt(header, maxWidthPx); ok {
		args = append(args, fmt.Sprintf("width: %.1fpt", widthPt))
		if isInjectableImage {
			args = append(args, `fit: "contain"`)
		} else {
			args = append(args, `fit: "stretch"`)
		}
	}

	return fmt.Sprintf("#image(%s)", strings.Join(args, ", "))
}

func (b *TypstBuilder) renderHeaderText(nodes []portabledoc.Node, metrics headerRenderMetrics) string {
	if len(nodes) == 0 {
		return ""
	}

	converted, _ := b.converter.ConvertNodes(normalizeHeaderTextNodes(nodes))
	if strings.TrimSpace(converted) == "" {
		return ""
	}

	return fmt.Sprintf(
		"#block(width: 100%%, height: %.1fpt)[\n#set text(size: %.1fpt)\n#set par(linebreaks: \"simple\")\n#set par(spacing: 0pt)\n%s\n]\n",
		metrics.textSlotHeightPt,
		headerTextBaseFontPt,
		strings.TrimRight(converted, "\n"),
	)
}

func normalizeHeaderTextNodes(nodes []portabledoc.Node) []portabledoc.Node {
	if len(nodes) <= 1 {
		return nodes
	}

	normalized := make([]portabledoc.Node, 0, len(nodes))

	for _, node := range nodes {
		if len(normalized) == 0 {
			normalized = append(normalized, clonePortableNode(node))
			continue
		}

		prev := &normalized[len(normalized)-1]
		if prev.Type == portabledoc.NodeTypeParagraph &&
			node.Type == portabledoc.NodeTypeParagraph &&
			reflect.DeepEqual(prev.Attrs, node.Attrs) {
			if len(prev.Content) > 0 {
				prev.Content = append(prev.Content, portabledoc.Node{Type: portabledoc.NodeTypeHardBreak})
			}
			prev.Content = append(prev.Content, clonePortableNodes(node.Content)...)
			continue
		}

		normalized = append(normalized, clonePortableNode(node))
	}

	return normalized
}

func clonePortableNodes(nodes []portabledoc.Node) []portabledoc.Node {
	if len(nodes) == 0 {
		return nil
	}

	cloned := make([]portabledoc.Node, 0, len(nodes))
	for _, node := range nodes {
		cloned = append(cloned, clonePortableNode(node))
	}
	return cloned
}

func clonePortableNode(node portabledoc.Node) portabledoc.Node {
	cloned := node
	if node.Content != nil {
		cloned.Content = clonePortableNodes(node.Content)
	}
	if node.Attrs != nil {
		cloned.Attrs = clonePortableAttrs(node.Attrs)
	}
	if node.Text != nil {
		text := *node.Text
		cloned.Text = &text
	}
	if node.Marks != nil {
		cloned.Marks = append([]portabledoc.Mark(nil), node.Marks...)
	}

	return cloned
}

func clonePortableAttrs(attrs map[string]any) map[string]any {
	if attrs == nil {
		return nil
	}

	cloned := make(map[string]any, len(attrs))
	for key, value := range attrs {
		cloned[key] = value
	}
	return cloned
}

func renderCenteredHeader(imageSlot, textTypst string) string {
	if imageSlot != "" {
		return fmt.Sprintf("#align(center)[%s]\n", imageSlot)
	}
	if textTypst != "" {
		return textTypst
	}
	return ""
}

func renderLateralHeader(
	leftContent, rightContent string,
	imageOnLeft bool,
	imageWidthPt float64,
	hasImageWidth bool,
	metrics headerRenderMetrics,
) string {
	switch {
	case leftContent != "" && rightContent != "":
		columns := "(auto, 1fr)"
		if imageOnLeft {
			if hasImageWidth {
				columns = fmt.Sprintf("(%.1fpt, 1fr)", imageWidthPt)
			}
		} else {
			if hasImageWidth {
				columns = fmt.Sprintf("(1fr, %.1fpt)", imageWidthPt)
			}
		}

		return fmt.Sprintf(
			"#block(width: 100%%)[\n  #grid(\n    columns: %s,\n    column-gutter: %.1fpt,\n    [%s],\n    [%s],\n  )\n]\n",
			columns,
			metrics.imageGapPt,
			leftContent,
			rightContent,
		)
	case leftContent != "":
		if imageOnLeft {
			return fmt.Sprintf("#align(left)[%s]\n", leftContent)
		}
		return leftContent
	case rightContent != "":
		if imageOnLeft {
			return rightContent
		}
		return fmt.Sprintf("#align(right)[%s]\n", rightContent)
	default:
		return ""
	}
}

func renderHeaderImageSlot(imageTypst string, imageWidthPt float64, hasImageWidth bool, metrics headerRenderMetrics) string {
	if imageTypst == "" {
		return ""
	}

	if hasImageWidth {
		return fmt.Sprintf(
			"#block(width: %.1fpt, height: %.1fpt)[\n#align(center + horizon)[%s]\n]\n",
			imageWidthPt,
			metrics.textSlotHeightPt,
			imageTypst,
		)
	}

	return fmt.Sprintf(
		"#block(height: %.1fpt)[\n#align(center + horizon)[%s]\n]\n",
		metrics.textSlotHeightPt,
		imageTypst,
	)
}

func renderHeaderSurface(content string, metrics headerRenderMetrics) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}

	return fmt.Sprintf(
		"#block(width: 100%%, height: %.1fpt)[\n  #place(top + left, dy: -%.1fpt)[\n    #block(width: 100%%)[\n      #pad(y: %.1fpt)[\n%s\n      ]\n    ]\n  ]\n]\n",
		metrics.surfaceMinHeightPt,
		metrics.headerVisualOffsetPt,
		metrics.surfaceVerticalPadPt,
		indentTypstBlock(strings.TrimRight(content, "\n"), "        "),
	)
}

func indentTypstBlock(content, prefix string) string {
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if line == "" {
			lines[i] = prefix
			continue
		}
		lines[i] = prefix + line
	}
	return strings.Join(lines, "\n")
}

func resolveHeaderRenderMetrics(pageConfig *portabledoc.PageConfig) headerRenderMetrics {
	headerVisualOffsetPx := 0.0
	if pageConfig != nil {
		headerVisualOffsetPx = pageConfig.Margins.Top / 2
	}

	return headerRenderMetrics{
		surfaceMinHeightPt:   headerSurfaceMinPx * pxToPt,
		surfaceVerticalPadPt: headerSurfacePadPx * pxToPt,
		textSlotHeightPt:     headerTextHeightPx * pxToPt,
		imageGapPt:           headerImageGapPx * pxToPt,
		headerVisualOffsetPt: headerVisualOffsetPx * pxToPt,
	}
}

func resolveHeaderMaxImageWidthPx(pageConfig *portabledoc.PageConfig, hasText bool) float64 {
	if pageConfig == nil {
		if hasText {
			return headerTextMinWidthPx
		}
		return 0
	}

	usableWidth := pageConfig.Width - pageConfig.Margins.Left - pageConfig.Margins.Right
	if !hasText {
		return max(headerImageMinWidthPx, usableWidth)
	}

	return max(headerImageMinWidthPx, usableWidth-headerImageGapPx-headerTextMinWidthPx)
}

func resolveHeaderImageWidthPt(header *portabledoc.DocumentHeader, maxWidthPx float64) (float64, bool) {
	if header == nil || header.ImageWidth == nil || *header.ImageWidth <= 0 {
		return 0, false
	}

	widthPx := min(float64(*header.ImageWidth), maxWidthPx)
	widthPx = max(headerImageMinWidthPx, widthPx)

	return widthPx * pxToPt, true
}
