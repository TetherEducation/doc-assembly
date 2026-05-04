package pdfrenderer

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

// typstConverter converts ProseMirror/TipTap nodes to Typst markup.
type typstConverter struct {
	injectables              map[string]any
	injectableDefaults       map[string]string
	fieldResponses           map[string]json.RawMessage // fieldID -> response JSON
	tokens                   TypstDesignTokens
	signerRoleValues         map[string]port.SignerRoleValue
	signerRoles              map[string]portabledoc.SignerRole // roleID -> SignerRole
	contentWidthPx           float64                           // page content area width in pixels (for table column calculations)
	pageWidthPx              float64                           // full page width in pixels (for signature field percentage calculations)
	currentPage              int
	signatureFields          []port.SignatureField
	currentTableHeaderStyles *entity.TableStyles
	currentTableBodyStyles   *entity.TableStyles
	remoteImages             map[string]string // URL -> local filename
	imageCounter             int
	listDepth                int // tracks nesting depth for user-built lists
}

// NewTypstConverterFactory returns a ConverterFactory that creates real Typst converters.
func NewTypstConverterFactory(tokens TypstDesignTokens) ConverterFactory {
	return func(
		injectables map[string]any,
		injectableDefaults map[string]string,
		signerRoleValues map[string]port.SignerRoleValue,
		signerRoles []portabledoc.SignerRole,
		fieldResponses map[string]json.RawMessage,
	) TypstConverter {
		roleMap := make(map[string]portabledoc.SignerRole, len(signerRoles))
		for _, role := range signerRoles {
			roleMap[role.ID] = role
		}

		return &typstConverter{
			injectables:        injectables,
			injectableDefaults: injectableDefaults,
			fieldResponses:     fieldResponses,
			tokens:             tokens,
			signerRoleValues:   signerRoleValues,
			signerRoles:        roleMap,
			currentPage:        1,
			signatureFields:    make([]port.SignatureField, 0),
			remoteImages:       make(map[string]string),
		}
	}
}

// GetCurrentPage returns the current page number.
func (c *typstConverter) GetCurrentPage() int {
	return c.currentPage
}

// RemoteImages returns the map of remote image URLs to local filenames.
func (c *typstConverter) RemoteImages() map[string]string {
	return c.remoteImages
}

// ResolveImageSource resolves the final image source after injectable substitution.
func (c *typstConverter) ResolveImageSource(attrs map[string]any) string {
	return c.resolveImageSource(attrs)
}

// SetContentWidthPx sets the page content area width in pixels.
func (c *typstConverter) SetContentWidthPx(width float64) {
	c.contentWidthPx = width
}

// SetPageWidthPx sets the full page width in pixels (including margins).
func (c *typstConverter) SetPageWidthPx(width float64) {
	c.pageWidthPx = width
}

// RegisterRemoteImage registers a remote URL or data URL and returns a local filename.
func (c *typstConverter) RegisterRemoteImage(url string) string {
	if existing, ok := c.remoteImages[url]; ok {
		return existing
	}
	c.imageCounter++
	ext := detectExtFromURL(url)
	filename := fmt.Sprintf("img_%d%s", c.imageCounter, ext)
	c.remoteImages[url] = filename
	return filename
}

// ConvertNodes converts a slice of nodes to Typst markup.
// It uses look-ahead to group inline images with their following paragraphs
// for text wrapping via the wrap-it package.
// Returns the Typst source string and any signature fields found during conversion.
func (c *typstConverter) ConvertNodes(nodes []portabledoc.Node) (string, []port.SignatureField) {
	var sb strings.Builder
	for i := 0; i < len(nodes); i++ {
		node := nodes[i]
		if c.isInlineImage(node) {
			// Collect consecutive paragraphs after the inline image as wrap body
			var body []portabledoc.Node
			for j := i + 1; j < len(nodes) && nodes[j].Type == portabledoc.NodeTypeParagraph; j++ {
				body = append(body, nodes[j])
			}
			if len(body) > 0 {
				sb.WriteString(c.wrapImage(node, body))
				i += len(body) // skip consumed paragraphs
			} else {
				sb.WriteString(c.image(node)) // no body, fallback to block
			}
		} else {
			sb.WriteString(c.convertNode(node))
		}
	}
	return sb.String(), c.signatureFields
}

// convertNode converts a single node to Typst markup.
func (c *typstConverter) convertNode(node portabledoc.Node) string {
	if handler := c.getNodeHandler(node.Type); handler != nil {
		return handler(node)
	}
	return c.handleUnknownNode(node)
}

// convertNodes converts a slice of nodes to Typst markup (internal, no signature fields return).
func (c *typstConverter) convertNodes(nodes []portabledoc.Node) string {
	var sb strings.Builder
	for _, node := range nodes {
		sb.WriteString(c.convertNode(node))
	}
	return sb.String()
}

type typstNodeHandler func(node portabledoc.Node) string

func (c *typstConverter) getNodeHandler(nodeType string) typstNodeHandler {
	handlers := map[string]typstNodeHandler{
		portabledoc.NodeTypeParagraph:        c.paragraph,
		portabledoc.NodeTypeHeading:          c.heading,
		portabledoc.NodeTypeBlockquote:       c.blockquote,
		portabledoc.NodeTypeCodeBlock:        c.codeBlock,
		portabledoc.NodeTypeHR:               c.horizontalRule,
		portabledoc.NodeTypeBulletList:       c.bulletList,
		portabledoc.NodeTypeOrderedList:      c.orderedList,
		portabledoc.NodeTypeTaskList:         c.taskList,
		portabledoc.NodeTypeListItem:         c.listItem,
		portabledoc.NodeTypeTaskItem:         c.taskItem,
		portabledoc.NodeTypeInjector:         c.injector,
		portabledoc.NodeTypeConditional:      c.conditional,
		portabledoc.NodeTypeSignature:        c.signature,
		portabledoc.NodeTypePageBreak:        c.pageBreak,
		portabledoc.NodeTypeImage:            c.image,
		portabledoc.NodeTypeCustomImage:      c.image,
		portabledoc.NodeTypeText:             c.text,
		portabledoc.NodeTypeListInjector:     c.listInjector,
		portabledoc.NodeTypeTableInjector:    c.tableInjector,
		portabledoc.NodeTypeTable:            c.table,
		portabledoc.NodeTypeTableRow:         c.tableRow,
		portabledoc.NodeTypeTableCell:        c.tableCellData,
		portabledoc.NodeTypeTableHeader:      c.tableCellHeader,
		portabledoc.NodeTypeHardBreak:        c.hardBreak,
		portabledoc.NodeTypeInteractiveField: c.interactiveField,
	}
	return handlers[nodeType]
}

func (c *typstConverter) handleUnknownNode(node portabledoc.Node) string {
	if len(node.Content) > 0 {
		return c.convertNodes(node.Content)
	}
	return ""
}

// --- Content Nodes ---

func (c *typstConverter) paragraph(node portabledoc.Node) string {
	content := c.convertNodes(node.Content)
	if content == "" {
		return fmt.Sprintf("#v(%s)\n", c.tokens.ParagraphSpacing)
	}

	leading := c.resolveLineSpacing(node.Attrs)
	body := c.applyLocalParagraphFormatting(content, node.Attrs, leading)
	return body + "\n\n"
}

func (c *typstConverter) heading(node portabledoc.Node) string {
	level := c.parseHeadingLevel(node.Attrs)
	content := c.convertNodes(node.Content)
	prefix := strings.Repeat("=", level)
	heading := fmt.Sprintf("%s %s", prefix, content)
	leading := c.resolveLineSpacing(node.Attrs)
	body := c.applyLocalParagraphFormatting(heading, node.Attrs, leading)
	return body + "\n"
}

func (c *typstConverter) parseHeadingLevel(attrs map[string]any) int {
	level := 1
	if l, ok := attrs["level"].(float64); ok {
		level = int(l)
	}
	return clamp(level, 1, 6)
}

func (c *typstConverter) resolveLineSpacing(attrs map[string]any) string {
	if attrs == nil {
		return c.tokens.ParagraphLeading
	}

	raw, ok := attrs["lineSpacing"].(string)
	if !ok || raw == "" {
		return c.tokens.ParagraphLeading
	}

	switch portabledoc.NormalizeLineSpacing(raw) {
	case portabledoc.LineSpacingTight:
		return "0em"
	case portabledoc.LineSpacingCompact:
		return "0.15em"
	case portabledoc.LineSpacingRelaxed:
		return "1.00em"
	case portabledoc.LineSpacingLoose:
		return "1.50em"
	default:
		return c.tokens.ParagraphLeading
	}
}

func (c *typstConverter) applyLocalParagraphFormatting(
	content string,
	attrs map[string]any,
	leading string,
) string {
	align, _ := attrs["textAlign"].(string)

	if align == "justify" {
		body := fmt.Sprintf("#par(justify: true)[%s]", content)
		return wrapTypstBlockWithLineSpacing(body, leading)
	}

	body := wrapTypstBlockWithLineSpacing(content, leading)
	if typstAlign := toTypstAlign(align); typstAlign != "" {
		return fmt.Sprintf("#align(%s)[%s]", typstAlign, body)
	}

	return body
}

func wrapTypstBlockWithLineSpacing(content, leading string) string {
	return fmt.Sprintf(
		"#[\n#set text(top-edge: 0.8em, bottom-edge: -0.2em)\n#set par(leading: %s)\n%s\n]",
		leading,
		strings.TrimRight(content, "\n"),
	)
}

func (c *typstConverter) blockquote(node portabledoc.Node) string {
	content := c.convertNodes(node.Content)
	return fmt.Sprintf(
		"#block(width: 100%%, inset: (left: 1em, top: 0.5em, bottom: 0.5em, right: 1em), stroke: (left: 2pt + %s), fill: rgb(\"%s\"), above: 0.75em, below: 0.75em)[#emph[%s]]\n",
		c.tokens.BlockquoteStrokeColor, c.tokens.BlockquoteFill, content,
	)
}

func (c *typstConverter) codeBlock(node portabledoc.Node) string {
	language, _ := node.Attrs["language"].(string)
	content := c.convertNodes(node.Content)

	if language != "" {
		return fmt.Sprintf("```%s\n%s\n```\n", language, content)
	}
	return fmt.Sprintf("```\n%s\n```\n", content)
}

func (c *typstConverter) horizontalRule(_ portabledoc.Node) string {
	return fmt.Sprintf("#line(length: 100%%, stroke: 0.5pt + %s)\n", c.tokens.HRStrokeColor)
}

// --- List Nodes ---

func (c *typstConverter) bulletList(node portabledoc.Node) string {
	var sb strings.Builder
	for _, child := range node.Content {
		c.renderUserListItem(&sb, child, "- ")
	}
	if c.listDepth == 0 {
		sb.WriteString("\n")
	}
	return sb.String()
}

func (c *typstConverter) orderedList(node portabledoc.Node) string {
	start := 1
	if s, ok := node.Attrs["start"].(float64); ok {
		start = int(s)
	}

	var sb strings.Builder
	needsBlock := start != 1 && c.listDepth == 0
	if needsBlock {
		sb.WriteString("#block[\n")
	}
	if start != 1 {
		fmt.Fprintf(&sb, "#set enum(start: %d)\n", start)
	}
	for _, child := range node.Content {
		c.renderUserListItem(&sb, child, "+ ")
	}
	if needsBlock {
		sb.WriteString("]\n")
	} else if c.listDepth == 0 {
		sb.WriteString("\n")
	}
	return sb.String()
}

func (c *typstConverter) taskList(node portabledoc.Node) string {
	var sb strings.Builder
	for _, child := range node.Content {
		checked, _ := child.Attrs["checked"].(bool)
		marker := "- \u2610 " // ☐
		if checked {
			marker = "- \u2611 " // ☑
		}
		c.renderUserListItem(&sb, child, marker)
	}
	if c.listDepth == 0 {
		sb.WriteString("\n")
	}
	return sb.String()
}

// renderUserListItem renders a listItem/taskItem node with depth-aware indentation.
// It separates text content from nested lists to produce proper Typst nesting.
func (c *typstConverter) renderUserListItem(sb *strings.Builder, node portabledoc.Node, marker string) {
	indent := strings.Repeat("  ", c.listDepth)

	var textParts []string
	var nestedLists []portabledoc.Node

	for _, child := range node.Content {
		switch child.Type {
		case portabledoc.NodeTypeBulletList, portabledoc.NodeTypeOrderedList, portabledoc.NodeTypeTaskList:
			nestedLists = append(nestedLists, child)
		default:
			textParts = append(textParts, strings.TrimSpace(c.convertNode(child)))
		}
	}

	text := strings.Join(textParts, " ")
	fmt.Fprintf(sb, "%s%s%s\n", indent, marker, text)

	c.listDepth++
	for _, nested := range nestedLists {
		sb.WriteString(c.convertNode(nested))
	}
	c.listDepth--
}

// listItem is a fallback -- normally handled inline by bulletList/orderedList.
func (c *typstConverter) listItem(node portabledoc.Node) string {
	content := c.convertNodes(node.Content)
	return fmt.Sprintf("- %s\n", strings.TrimSpace(content))
}

// taskItem is a fallback -- normally handled inline by taskList.
func (c *typstConverter) taskItem(node portabledoc.Node) string {
	checked, _ := node.Attrs["checked"].(bool)
	content := c.convertNodes(node.Content)
	marker := "\u2610" // ☐
	if checked {
		marker = "\u2611" // ☑
	}
	return fmt.Sprintf("- %s %s\n", marker, strings.TrimSpace(content))
}

// --- Dynamic Nodes ---

func (c *typstConverter) injector(node portabledoc.Node) string {
	variableID, _ := node.Attrs["variableId"].(string)
	isRoleVar, _ := node.Attrs["isRoleVariable"].(bool)
	prefix, _ := node.Attrs["prefix"].(string)
	suffix, _ := node.Attrs["suffix"].(string)
	showLabelIfEmpty, _ := node.Attrs["showLabelIfEmpty"].(bool)
	nodeDefaultValue, _ := node.Attrs["defaultValue"].(string)
	widthPx, hasWidth := node.Attrs["width"].(float64)

	// Resolve value with priority: injected > node default > global default
	value := c.resolveInjectorValue(variableID, isRoleVar, node.Attrs)
	if value == "" {
		if nodeDefaultValue != "" {
			value = nodeDefaultValue
		} else {
			value = c.getDefaultValue(variableID)
		}
	}

	// Empty value handling
	if value == "" {
		if showLabelIfEmpty {
			return escapeTypst(prefix) + escapeTypst(suffix)
		}
		return ""
	}

	// Build output: prefix + value + suffix
	content := c.buildInjectorContent(prefix, value, suffix)

	if hasWidth && widthPx > 0 {
		widthPt := widthPx * pxToPt
		return fmt.Sprintf("#box(width: %.1fpt)[%s]", widthPt, content)
	}

	return content
}

func (c *typstConverter) buildInjectorContent(prefix, value, suffix string) string {
	var parts []string
	if prefix != "" {
		parts = append(parts, escapeTypst(prefix))
	}
	parts = append(parts, escapeTypst(value))
	if suffix != "" {
		parts = append(parts, escapeTypst(suffix))
	}
	return strings.Join(parts, "")
}

func (c *typstConverter) resolveInjectorValue(variableID string, isRoleVar bool, attrs map[string]any) string {
	if !isRoleVar {
		return c.resolveRegularInjectable(variableID, attrs)
	}
	return c.resolveRoleVariable(variableID, attrs)
}

func (c *typstConverter) resolveRegularInjectable(variableID string, attrs map[string]any) string {
	if v, ok := c.injectables[variableID]; ok {
		return c.formatInjectableValue(v, attrs)
	}
	return ""
}

func (c *typstConverter) resolveRoleVariable(variableID string, attrs map[string]any) string {
	roleID, _ := attrs["roleId"].(string)
	propertyKey, _ := attrs["propertyKey"].(string)

	if roleValue, ok := c.signerRoleValues[roleID]; ok {
		if value := c.getRolePropertyValue(roleValue, propertyKey); value != "" {
			return value
		}
	}

	// Fallback: try injectables directly for cases like ROLE.Rol_1.email
	if v, ok := c.injectables[variableID]; ok {
		return c.formatInjectableValue(v, attrs)
	}
	return ""
}

func (c *typstConverter) getRolePropertyValue(roleValue port.SignerRoleValue, propertyKey string) string {
	switch propertyKey {
	case portabledoc.RolePropertyName:
		return roleValue.Name
	case portabledoc.RolePropertyEmail:
		return roleValue.Email
	default:
		return ""
	}
}

func (c *typstConverter) getDefaultValue(variableID string) string {
	if defaultVal, ok := c.injectableDefaults[variableID]; ok && defaultVal != "" {
		return defaultVal
	}
	return ""
}

func (c *typstConverter) formatInjectableValue(value any, attrs map[string]any) string {
	injectorType, _ := attrs["type"].(string)
	format, _ := attrs["format"].(string)

	switch v := value.(type) {
	case string:
		return v
	case float64:
		return c.formatFloat64(v, injectorType, format)
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case bool:
		return formatBool(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func (c *typstConverter) formatFloat64(v float64, injectorType, format string) string {
	if injectorType == portabledoc.InjectorTypeCurrency {
		if format != "" {
			return fmt.Sprintf("%s %.2f", format, v)
		}
		return fmt.Sprintf("%.2f", v)
	}

	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func (c *typstConverter) conditional(node portabledoc.Node) string {
	if c.evaluateCondition(node.Attrs) {
		return c.convertNodes(node.Content)
	}
	return ""
}

func (c *typstConverter) pageBreak(_ portabledoc.Node) string {
	c.currentPage++
	return "#pagebreak()\n"
}

// --- Image Nodes ---

// resolveImageSource resolves the final image source from node attributes.
// Handles injectable bindings before any remote/local path conversion.
func (c *typstConverter) resolveImageSource(attrs map[string]any) string {
	src, _ := attrs["src"].(string)

	if injectableId, ok := attrs["injectableId"].(string); ok && injectableId != "" {
		if resolved, exists := c.injectables[injectableId]; exists {
			src = fmt.Sprintf("%v", resolved)
		} else if defaultVal, exists := c.injectableDefaults[injectableId]; exists {
			src = defaultVal
		}
	}

	if src == "" {
		return ""
	}

	return src
}

// resolveImagePath resolves the final local image path from node attributes.
// Handles injectable bindings, remote URLs, and data URLs.
func (c *typstConverter) resolveImagePath(attrs map[string]any) string {
	src := c.resolveImageSource(attrs)
	if src == "" {
		return ""
	}

	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "data:") || strings.HasPrefix(src, "storage://") {
		return c.RegisterRemoteImage(src)

	}
	return src
}

// isInlineImage checks if a node is an image with displayMode "inline" (text wrapping).
func (c *typstConverter) isInlineImage(node portabledoc.Node) bool {
	if node.Type != portabledoc.NodeTypeImage && node.Type != portabledoc.NodeTypeCustomImage {
		return false
	}
	dm, _ := node.Attrs["displayMode"].(string)
	return dm == "inline"
}

// imageMarkup generates just the Typst image/box markup without alignment wrapping.
func (c *typstConverter) imageMarkup(node portabledoc.Node) string {
	src := c.resolveImageSource(node.Attrs)
	imgPath := c.resolveImagePath(node.Attrs)
	if src == "" || imgPath == "" {
		return ""
	}

	width, _ := node.Attrs["width"].(float64)
	height, _ := node.Attrs["height"].(float64)
	shape, _ := node.Attrs["shape"].(string)
	injectableID, _ := node.Attrs["injectableId"].(string)
	isInjectableImage := injectableID != ""

	var markup string
	if width > 0 && height > 0 && isInjectableImage {
		markup = typstImageCall(
			imgPath,
			fmt.Sprintf("width: %.0fpt", width*pxToPt),
			fmt.Sprintf("height: %.0fpt", height*pxToPt),
			`fit: "contain"`,
		)
	} else if width > 0 {
		markup = typstImageCall(imgPath, fmt.Sprintf("width: %.0fpt", width*pxToPt))
	} else {
		markup = typstImageCall(imgPath, "width: 100%")
	}

	if shape != "circle" {
		return markup
	}

	if height <= 0 {
		height = width
	}

	size := math.Min(width, height) * pxToPt
	if size <= 0 {
		return markup
	}

	imageArgs := []string{"width: 100%", "height: 100%"}
	if isInjectableImage {
		imageArgs = append(imageArgs, `fit: "contain"`)
	}

	return fmt.Sprintf(
		"#box(width: %.0fpt, height: %.0fpt, clip: true, radius: 50%%)[%s]",
		size, size, typstImageCall(imgPath, imageArgs...),
	)
}

// wrapImage generates a wrap-content block: image + following paragraphs as body.
func (c *typstConverter) wrapImage(imgNode portabledoc.Node, bodyNodes []portabledoc.Node) string {
	markup := c.imageMarkup(imgNode)
	if markup == "" {
		return ""
	}

	align, _ := imgNode.Attrs["align"].(string)
	typstAlign := "top + left"
	if align == "right" {
		typstAlign = "top + right"
	}

	var body strings.Builder
	for _, n := range bodyNodes {
		body.WriteString(c.convertNode(n))
	}

	return fmt.Sprintf("#wrap-content([%s], align: %s, column-gutter: 0.75em)[%s]\n", markup, typstAlign, body.String())
}

// image converts an image node to block-mode Typst markup.
func (c *typstConverter) image(node portabledoc.Node) string {
	markup := c.imageMarkup(node)
	if markup == "" {
		return ""
	}

	align, _ := node.Attrs["align"].(string)

	switch align {
	case "center":
		return fmt.Sprintf("#align(center)[%s]\n", markup)
	case "right":
		return fmt.Sprintf("#align(right)[%s]\n", markup)
	default:
		return markup + "\n"
	}
}

// --- Hard Break Node ---

func (c *typstConverter) hardBreak(_ portabledoc.Node) string {
	// Typst line break: backslash at end of line
	// This creates a hard line break within the same paragraph
	return "\\\n"
}

// --- Interactive Field Nodes ---

// interactiveField renders an interactive field node (checkbox, radio, or text) to Typst.
func (c *typstConverter) interactiveField(node portabledoc.Node) string {
	attrs, err := portabledoc.ParseInteractiveFieldAttrs(node.Attrs)
	if err != nil {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("#block[\n")

	// Render label if present
	if attrs.Label != "" {
		fmt.Fprintf(&sb, "  #text(weight: \"bold\")[%s]\n", escapeTypst(attrs.Label))
		sb.WriteString("  #v(2pt)\n")
	}

	switch attrs.FieldType {
	case portabledoc.InteractiveFieldTypeCheckbox:
		sb.WriteString(c.renderCheckboxField(attrs))
	case portabledoc.InteractiveFieldTypeRadio:
		sb.WriteString(c.renderRadioField(attrs))
	case portabledoc.InteractiveFieldTypeText:
		sb.WriteString(c.renderTextField(attrs))
	}

	sb.WriteString("]\n")
	return sb.String()
}

// renderCheckboxField renders checkbox options with checked/unchecked indicators.
func (c *typstConverter) renderCheckboxField(attrs *portabledoc.InteractiveFieldAttrs) string {
	selectedIDs := c.resolveSelectedOptionIDs(attrs.ID)

	items := make([]string, 0, len(attrs.Options))
	for _, opt := range attrs.Options {
		sym := "☐"
		if selectedIDs[opt.ID] {
			sym = "☑"
		}
		items = append(items, fmt.Sprintf("[%s %s]", sym, escapeTypst(opt.Label)))
	}

	return c.renderOptionsStack(attrs.GetOptionsLayout(), items)
}

// renderRadioField renders radio options with selected/unselected indicators.
func (c *typstConverter) renderRadioField(attrs *portabledoc.InteractiveFieldAttrs) string {
	selectedIDs := c.resolveSelectedOptionIDs(attrs.ID)

	items := make([]string, 0, len(attrs.Options))
	for _, opt := range attrs.Options {
		sym := "○"
		if selectedIDs[opt.ID] {
			sym = "◉"
		}
		items = append(items, fmt.Sprintf("[%s %s]", sym, escapeTypst(opt.Label)))
	}

	return c.renderOptionsStack(attrs.GetOptionsLayout(), items)
}

// renderOptionsStack renders options using #stack with direction and spacing based on layout.
func (c *typstConverter) renderOptionsStack(layout string, items []string) string {
	dir := "ttb"
	spacing := "4pt"
	if layout == portabledoc.InteractiveFieldLayoutInline {
		dir = "ltr"
		spacing = "8pt"
	}
	return fmt.Sprintf("  #stack(dir: %s, spacing: %s, %s)\n", dir, spacing, strings.Join(items, ", "))
}

// renderTextField renders a text field with its response value or placeholder.
func (c *typstConverter) renderTextField(attrs *portabledoc.InteractiveFieldAttrs) string {
	text := c.resolveTextResponse(attrs.ID)
	if text != "" {
		return fmt.Sprintf("  %s\n", escapeTypst(text))
	}

	// No response: show placeholder in gray
	if attrs.Placeholder != "" {
		return fmt.Sprintf("  #text(fill: gray)[%s]\n", escapeTypst(attrs.Placeholder))
	}

	return ""
}

// checkboxResponse is used to unmarshal checkbox/radio field responses.
type checkboxResponse struct {
	SelectedOptionIDs []string `json:"selectedOptionIds"`
}

// textResponse is used to unmarshal text field responses.
type textResponse struct {
	Text string `json:"text"`
}

// resolveSelectedOptionIDs looks up and parses selectedOptionIds from field responses.
func (c *typstConverter) resolveSelectedOptionIDs(fieldID string) map[string]bool {
	raw, ok := c.fieldResponses[fieldID]
	if !ok || len(raw) == 0 {
		return nil
	}

	var resp checkboxResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil
	}

	result := make(map[string]bool, len(resp.SelectedOptionIDs))
	for _, id := range resp.SelectedOptionIDs {
		result[id] = true
	}
	return result
}

// resolveTextResponse looks up and parses text from field responses.
func (c *typstConverter) resolveTextResponse(fieldID string) string {
	raw, ok := c.fieldResponses[fieldID]
	if !ok || len(raw) == 0 {
		return ""
	}

	var resp textResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return ""
	}
	return resp.Text
}

// --- Text Node ---

func (c *typstConverter) text(node portabledoc.Node) string {
	if node.Text == nil {
		return ""
	}

	txt := escapeTypst(*node.Text)
	for _, mark := range node.Marks {
		txt = c.applyMark(txt, mark)
	}
	return txt
}

func (c *typstConverter) applyMark(txt string, mark portabledoc.Mark) string {
	switch mark.Type {
	case portabledoc.MarkTypeBold:
		return fmt.Sprintf("#strong[%s]", txt)
	case portabledoc.MarkTypeItalic:
		return fmt.Sprintf("#emph[%s]", txt)
	case portabledoc.MarkTypeStrike:
		return fmt.Sprintf("#strike[%s]", txt)
	case portabledoc.MarkTypeCode:
		// Undo escaping for raw code
		return fmt.Sprintf("`%s`", unescapeTypst(txt))
	case portabledoc.MarkTypeUnderline:
		return fmt.Sprintf("#underline[%s]", txt)
	case portabledoc.MarkTypeHighlight:
		return c.applyHighlightMark(txt, mark)
	case portabledoc.MarkTypeLink:
		return c.applyLinkMark(txt, mark)
	case portabledoc.MarkTypeTextStyle:
		return c.applyTextStyleMark(txt, mark)
	default:
		return txt
	}
}

func (c *typstConverter) applyHighlightMark(txt string, mark portabledoc.Mark) string {
	color := c.tokens.HighlightDefaultColor
	if clr, ok := mark.Attrs["color"].(string); ok && clr != "" {
		color = clr
	}
	return fmt.Sprintf("#highlight(fill: rgb(\"%s\"))[%s]", escapeTypstString(color), txt)
}

func (c *typstConverter) applyLinkMark(txt string, mark portabledoc.Mark) string {
	href, _ := mark.Attrs["href"].(string)
	if href == "" {
		return txt
	}
	return fmt.Sprintf("#link(\"%s\")[%s]", escapeTypstString(href), txt)
}

func (c *typstConverter) applyTextStyleMark(txt string, mark portabledoc.Mark) string {
	var params []string

	if color, ok := mark.Attrs["color"].(string); ok && color != "" {
		params = append(params, fmt.Sprintf("fill: rgb(\"%s\")", escapeTypstString(color)))
	}
	if fontSize, ok := mark.Attrs["fontSize"].(string); ok && fontSize != "" {
		// Convert CSS px to Typst pt (1px ~ 0.75pt)
		size := strings.TrimSuffix(fontSize, "px")
		if n, err := strconv.ParseFloat(size, 64); err == nil {
			params = append(params, fmt.Sprintf("size: %.1fpt", n*0.75))
		}
	}
	if fontFamily, ok := mark.Attrs["fontFamily"].(string); ok && fontFamily != "" {
		// Use first font in the family list (e.g., "Times New Roman, serif" -> "Times New Roman")
		family := strings.Split(fontFamily, ",")[0]
		family = strings.TrimSpace(family)
		params = append(params, fmt.Sprintf("font: %s", fontWithFallbacks(family)))
	}

	if len(params) == 0 {
		return txt
	}
	return fmt.Sprintf("#text(%s)[%s]", strings.Join(params, ", "), txt)
}

// --- Signature Nodes ---

func (c *typstConverter) signature(node portabledoc.Node) string {
	attrs := c.parseSignatureAttrs(node.Attrs)
	c.collectSignatureFields(attrs)
	return c.renderSignatureBlock(attrs)
}

// collectSignatureFields extracts signature field positions from the signature block.
func (c *typstConverter) collectSignatureFields(attrs portabledoc.SignatureAttrs) {
	// 6% page height: drives the Documenso field rect size (≈47pt on Letter,
	// extending ≈24pt below the centered line). See labelGapBelowLinePt.
	const defaultHeight = 6.0

	// Calculate field width from line width in points.
	fieldWidth := c.signatureFieldWidthPercent(attrs.LineWidth)

	// Calculate X positions based on layout
	xPositions := c.calculateXPositions(attrs.Layout, attrs.Count)

	// Default Y position - approximation based on typical document layouts
	yPosition := 55.0 // 55% from top

	for i, sig := range attrs.Signatures {
		if sig.RoleID == nil || *sig.RoleID == "" {
			continue
		}

		roleID := *sig.RoleID
		anchorString := c.getAnchorString(&sig)

		// Get X position for this signature index
		xPos := 35.0 // default center
		if i < len(xPositions) {
			xPos = xPositions[i]
		}

		c.signatureFields = append(c.signatureFields, port.SignatureField{
			RoleID:       roleID,
			AnchorString: anchorString,
			Page:         c.currentPage,
			PositionX:    xPos,
			PositionY:    yPosition,
			Width:        fieldWidth,
			Height:       defaultHeight,
		})
	}
}

// signatureFieldWidthPercent converts the signature line width (pt) to a page-width percentage.
func (c *typstConverter) signatureFieldWidthPercent(lineWidth string) float64 {
	const fallbackWidth = 30.0 // 30% fallback

	lwPt := c.signatureLineWidth(lineWidth)
	lwPt = c.capSignatureLineWidth(lwPt)

	val, err := strconv.ParseFloat(strings.TrimSuffix(lwPt, "pt"), 64)
	if err != nil || val <= 0 {
		return fallbackWidth
	}

	// Full page width in pt for percentage calculation (Documenso fields are % of full page).
	pageWidthPt := c.pageWidthPx * pxToPt
	if pageWidthPt <= 0 {
		return fallbackWidth
	}

	pct := (val / pageWidthPt) * 100
	if pct > 100 {
		pct = 100
	}
	return pct
}

// calculateXPositions returns X positions for signatures based on layout.
func (c *typstConverter) calculateXPositions(layout string, count int) []float64 {
	if positions, ok := layoutPositions[layout]; ok {
		return positions
	}
	return c.defaultXPositions(count)
}

// defaultXPositions generates positions based on count when layout is unknown.
func (c *typstConverter) defaultXPositions(count int) []float64 {
	positions := make([]float64, count)
	for i := range positions {
		positions[i] = float64(5 + i*30)
	}
	return positions
}

func (c *typstConverter) parseSignatureAttrs(attrs map[string]any) portabledoc.SignatureAttrs {
	result := portabledoc.SignatureAttrs{
		Count:     getIntAttr(attrs, "count", 1),
		Layout:    getStringAttr(attrs, "layout", portabledoc.LayoutSingleCenter),
		LineWidth: getStringAttr(attrs, "lineWidth", portabledoc.LineWidthMedium),
	}

	if sigsRaw, ok := attrs["signatures"].([]any); ok {
		result.Signatures = c.parseSignatureItems(sigsRaw)
	}
	return result
}

func (c *typstConverter) parseSignatureItems(sigsRaw []any) []portabledoc.SignatureItem {
	items := make([]portabledoc.SignatureItem, 0, len(sigsRaw))
	for _, sigRaw := range sigsRaw {
		sigMap, ok := sigRaw.(map[string]any)
		if !ok {
			continue
		}
		items = append(items, c.parseSignatureItem(sigMap))
	}
	return items
}

func (c *typstConverter) parseSignatureItem(sigMap map[string]any) portabledoc.SignatureItem {
	item := portabledoc.SignatureItem{
		ID:    getStringAttr(sigMap, "id", ""),
		Label: getStringAttr(sigMap, "label", ""),
	}

	item.RoleID = getStringPtrAttr(sigMap, "roleId")
	item.Subtitle = getStringPtrAttr(sigMap, "subtitle")
	item.ImageData = getStringPtrAttr(sigMap, "imageData")
	item.ImageOriginal = getStringPtrAttr(sigMap, "imageOriginal")
	item.ImageOpacity = getFloat64PtrAttr(sigMap, "imageOpacity")
	item.ImageRotation = getIntPtrAttr(sigMap, "imageRotation")
	item.ImageScale = getFloat64PtrAttr(sigMap, "imageScale")
	item.ImageX = getFloat64PtrAttr(sigMap, "imageX")
	item.ImageY = getFloat64PtrAttr(sigMap, "imageY")

	return item
}

func (c *typstConverter) getAnchorString(sig *portabledoc.SignatureItem) string {
	if sig.RoleID != nil && *sig.RoleID != "" {
		if role, ok := c.signerRoles[*sig.RoleID]; ok {
			sanitized := strings.ToLower(role.Label)
			sanitized = strings.ReplaceAll(sanitized, " ", "_")
			return fmt.Sprintf("__sig_%s__", sanitized)
		}
	}
	return fmt.Sprintf("__sig_%s__", sig.ID)
}

// renderSignatureBlock renders a signature block in Typst.
func (c *typstConverter) renderSignatureBlock(attrs portabledoc.SignatureAttrs) string {
	lwPt := c.signatureLineWidth(attrs.LineWidth)
	lwPt = c.capSignatureLineWidth(lwPt)
	body := c.signatureLayoutBody(attrs, lwPt)
	return "#v(2em)\n" + body
}

// capSignatureLineWidth caps the configured line width to fit within
// the most constrained layout (3 columns), ensuring the same width
// is used regardless of layout type.
func (c *typstConverter) capSignatureLineWidth(lwPt string) string {
	if c.contentWidthPx <= 0 {
		return lwPt
	}

	lwVal, err := strconv.ParseFloat(strings.TrimSuffix(lwPt, "pt"), 64)
	if err != nil {
		return lwPt
	}

	contentWidthPt := c.contentWidthPx * pxToPt
	const gutterPt = 11.0 // 1em at 11pt base text size
	const maxCols = 3     // worst case: 3 columns
	maxWidth := (contentWidthPt - float64(maxCols-1)*gutterPt) / float64(maxCols)

	if lwVal > maxWidth {
		return fmt.Sprintf("%.1fpt", maxWidth)
	}
	return lwPt
}

// signatureLayoutBody dispatches to the appropriate layout renderer.
func (c *typstConverter) signatureLayoutBody(attrs portabledoc.SignatureAttrs, lwPt string) string {
	sigs := attrs.Signatures

	switch attrs.Layout {
	// Single signature layouts
	case portabledoc.LayoutSingleLeft:
		return c.renderAlignedSignature(&sigs[0], "left", lwPt)
	case portabledoc.LayoutSingleRight:
		return c.renderAlignedSignature(&sigs[0], "right", lwPt)
	case portabledoc.LayoutSingleCenter:
		return c.renderAlignedSignature(&sigs[0], "center", lwPt)

	// Dual layouts
	case portabledoc.LayoutDualSides:
		return c.renderSignatureGrid(sigs, 2, lwPt)
	case portabledoc.LayoutDualCenter:
		return c.renderStackedSignatures(sigs, "center", lwPt)
	case portabledoc.LayoutDualLeft:
		return c.renderStackedSignatures(sigs, "left", lwPt)
	case portabledoc.LayoutDualRight:
		return c.renderStackedSignatures(sigs, "right", lwPt)

	// Triple layouts
	case portabledoc.LayoutTripleRow:
		return c.renderSignatureGrid(sigs, 3, lwPt)
	case portabledoc.LayoutTriplePyramid:
		return c.renderSplitLayout(sigs[:2], 2, sigs[2:], 0, lwPt)
	case portabledoc.LayoutTripleInverted:
		return c.renderSplitLayout(sigs[:1], 0, sigs[1:], 2, lwPt)

	// Quad layouts
	case portabledoc.LayoutQuadGrid:
		return c.renderSignatureGrid(sigs, 2, lwPt)
	case portabledoc.LayoutQuadTopHeavy:
		return c.renderSplitLayout(sigs[:3], 3, sigs[3:], 0, lwPt)
	case portabledoc.LayoutQuadBottomHeavy:
		return c.renderSplitLayout(sigs[:1], 0, sigs[1:], 3, lwPt)

	default:
		return c.renderSignatureGrid(sigs, len(sigs), lwPt)
	}
}

// signatureLineWidth returns the line width in pt from the lineWidth name.
func (c *typstConverter) signatureLineWidth(lineWidth string) string {
	switch lineWidth {
	case portabledoc.LineWidthSmall:
		return "120pt"
	case portabledoc.LineWidthLarge:
		return "250pt"
	default: // medium
		return "180pt"
	}
}

// renderSignatureGrid renders signatures in a Typst grid layout.
func (c *typstConverter) renderSignatureGrid(sigs []portabledoc.SignatureItem, cols int, lwPt string) string {
	colSpec := strings.TrimRight(strings.Repeat("1fr, ", cols), ", ")

	var sb strings.Builder
	fmt.Fprintf(&sb,
		"#grid(\n  columns: (%s),\n  column-gutter: 1em,\n  row-gutter: 3em,\n  align: center + top,\n",
		colSpec,
	)
	for i := range sigs {
		sb.WriteString("  [\n")
		sb.WriteString(c.renderTypstSignatureItemContent(&sigs[i], lwPt))
		sb.WriteString("  ]")
		if i < len(sigs)-1 {
			sb.WriteString(",\n")
		}
	}
	sb.WriteString("\n)\n")
	return sb.String()
}

// renderAlignedSignature renders a single signature with the given alignment.
func (c *typstConverter) renderAlignedSignature(sig *portabledoc.SignatureItem, alignment, lwPt string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "#align(%s)[\n", alignment)
	sb.WriteString(c.renderTypstSignatureItemContent(sig, lwPt))
	sb.WriteString("]\n")
	return sb.String()
}

// renderStackedSignatures renders signatures vertically stacked with the given alignment.
func (c *typstConverter) renderStackedSignatures(sigs []portabledoc.SignatureItem, alignment, lwPt string) string {
	var sb strings.Builder
	for i := range sigs {
		if i > 0 {
			sb.WriteString("#v(3em)\n")
		}
		sb.WriteString(c.renderAlignedSignature(&sigs[i], alignment, lwPt))
	}
	return sb.String()
}

// renderSplitLayout renders two groups of signatures separated by vertical space.
// cols=0 means single centered signature; cols>0 means grid with that many columns.
func (c *typstConverter) renderSplitLayout(
	topSigs []portabledoc.SignatureItem, topCols int,
	bottomSigs []portabledoc.SignatureItem, bottomCols int,
	lwPt string,
) string {
	var sb strings.Builder
	sb.WriteString(c.renderSignatureGroup(topSigs, topCols, lwPt))
	sb.WriteString("#v(3em)\n")
	sb.WriteString(c.renderSignatureGroup(bottomSigs, bottomCols, lwPt))
	return sb.String()
}

// renderSignatureGroup renders a group of signatures as either a centered single or a grid.
func (c *typstConverter) renderSignatureGroup(sigs []portabledoc.SignatureItem, cols int, lwPt string) string {
	if cols == 0 || len(sigs) == 1 {
		return c.renderAlignedSignature(&sigs[0], "center", lwPt)
	}
	return c.renderSignatureGrid(sigs, cols, lwPt)
}

// renderTypstSignatureItemContent renders the inner content of a signature item.
// The caller is responsible for wrapping in [...] content blocks when needed (e.g., grid items).
func (c *typstConverter) renderTypstSignatureItemContent(sig *portabledoc.SignatureItem, lineWidthPt string) string {
	var sb strings.Builder
	anchorString := c.getAnchorString(sig)

	// Always reserve a fixed slot above the line so all lines in a grid row align.
	// The image is placed out-of-flow (via #place) so its size never affects layout.
	const slotHeightPt = 60.0 // matches editor h-20 (80px × 0.75 pxToPt)
	const gapPt = 6.0         // matches editor mb-2 (8px × 0.75 pxToPt)
	// Gap below #line equals half the Documenso rect (defaultHeight=6% × 792pt ≈ 47pt
	// on Letter → centered on line extends ≈24pt down) plus 1pt clearance.
	const labelGapBelowLinePt = 25.0
	fmt.Fprintf(&sb, "    #v(%.1fpt)\n", slotHeightPt)
	fmt.Fprintf(&sb, "    #v(%.1fpt)\n", gapPt)

	// Build image markup ahead of the line block (only when signed)
	imgMarkup := ""
	if sig.IsSigned() {
		imgFile := c.RegisterRemoteImage(*sig.ImageData)
		scale := 1.0
		if sig.ImageScale != nil && *sig.ImageScale > 0 {
			scale = *sig.ImageScale
		}
		heightPt := slotHeightPt * scale
		imgMarkup = fmt.Sprintf("#image(\"%s\", height: %.1fpt, fit: \"contain\")",
			escapeTypstString(imgFile), heightPt)
		if sig.ImageRotation != nil && *sig.ImageRotation != 0 {
			imgMarkup = fmt.Sprintf("#rotate(%ddeg)[%s]", *sig.ImageRotation, imgMarkup)
		}
	}

	fmt.Fprintf(&sb, "    #block(width: %s)[\n", lineWidthPt)
	// Anchor text (invisible, out-of-flow) placed at dy:0 so pdftotext reports
	// its yMin at exactly the block top — the same Y as the #line below.
	// The conversion formula then uses this anchor Y as the line Y directly.
	fmt.Fprintf(&sb, "      #place(top + center, dy: 0.0pt)[#text(size: 0.1pt, fill: white)[%s]]\n", escapeTypst(anchorString))
	if imgMarkup != "" {
		// Float the image out-of-flow, centered above the line.
		// dy moves the image's top edge up into the reserved slot so it sits over the line.
		fmt.Fprintf(&sb, "      #place(top + center, dy: -%.1fpt)[%s]\n", slotHeightPt+gapPt, imgMarkup)
	}
	sb.WriteString("      #line(length: 100%, stroke: 0.5pt)\n")
	fmt.Fprintf(&sb, "      #v(%.1fpt)\n", labelGapBelowLinePt)
	label := sig.Label
	if label == "" {
		label = "Firma"
	}
	fmt.Fprintf(&sb, "      #align(center)[#text(size: 9pt)[%s]]\n", escapeTypst(label))
	if sig.Subtitle != nil && *sig.Subtitle != "" {
		fmt.Fprintf(&sb, "      #align(center)[#text(size: 8pt, fill: luma(100))[%s]]\n", escapeTypst(*sig.Subtitle))
	}
	sb.WriteString("    ]\n")

	return sb.String()
}

// --- List Injector Nodes ---

func (c *typstConverter) listInjector(node portabledoc.Node) string {
	variableID, _ := node.Attrs["variableId"].(string)
	lang, _ := node.Attrs["lang"].(string)
	if lang == "" {
		lang = "en"
	}

	listData := c.resolveListValue(variableID)
	if listData == nil {
		label, _ := node.Attrs["label"].(string)
		if label == "" {
			label = variableID
		}
		return fmt.Sprintf(
			"#block(fill: rgb(\"%s\"), stroke: (dash: \"dashed\", paint: rgb(\"%s\")), inset: 1em, width: 100%%)[#text(fill: rgb(\"%s\"), style: \"italic\")[\\[List: %s\\]]]\n",
			c.tokens.PlaceholderFillBg, c.tokens.PlaceholderStroke, c.tokens.PlaceholderTextColor, escapeTypst(label),
		)
	}

	// Override symbol from editor attrs
	if sym, ok := node.Attrs["symbol"].(string); ok && sym != "" {
		listData.Symbol = entity.ListSymbol(sym)
	}

	// Override header label from editor attrs
	if label, ok := node.Attrs["label"].(string); ok && label != "" {
		if listData.HeaderLabel == nil {
			listData.HeaderLabel = make(map[string]string)
		}
		listData.HeaderLabel[lang] = label
	}

	// Merge styles: injector data styles as base, node attrs as override
	headerStyles := c.parseListStylesFromAttrs(node.Attrs, "header")
	itemStyles := c.parseListStylesFromAttrs(node.Attrs, "item")

	if listData.HeaderStyles != nil {
		headerStyles = c.mergeListStyles(listData.HeaderStyles, headerStyles)
	}
	if listData.ItemStyles != nil {
		itemStyles = c.mergeListStyles(listData.ItemStyles, itemStyles)
	}

	return c.renderTypstList(listData, lang, headerStyles, itemStyles)
}

func (c *typstConverter) resolveListValue(variableID string) *entity.ListValue {
	if v, ok := c.injectables[variableID]; ok {
		if listVal, ok := v.(*entity.ListValue); ok {
			return listVal
		}
		if mapVal, ok := v.(map[string]any); ok {
			return c.parseListFromMap(mapVal)
		}
	}
	return nil
}

func (c *typstConverter) parseListFromMap(m map[string]any) *entity.ListValue {
	list := entity.NewListValue()

	if symbol, ok := m["symbol"].(string); ok {
		list.WithSymbol(entity.ListSymbol(symbol))
	}
	if headerLabel, ok := m["headerLabel"].(map[string]any); ok {
		labels := make(map[string]string)
		for k, v := range headerLabel {
			if s, ok := v.(string); ok {
				labels[k] = s
			}
		}
		list.WithHeaderLabel(labels)
	}
	if items, ok := m["items"].([]any); ok {
		for _, itemAny := range items {
			if itemMap, ok := itemAny.(map[string]any); ok {
				list.Items = append(list.Items, c.parseListItemFromMap(itemMap))
			}
		}
	}
	return list
}

func (c *typstConverter) parseListItemFromMap(m map[string]any) entity.ListItem {
	item := entity.ListItem{}
	if valueMap, ok := m["value"].(map[string]any); ok {
		cell := c.parseInjectableValue(valueMap)
		item.Value = cell.Value
	} else if strVal, ok := m["value"].(string); ok {
		v := entity.StringValue(strVal)
		item.Value = &v
	}
	if children, ok := m["children"].([]any); ok {
		for _, childAny := range children {
			if childMap, ok := childAny.(map[string]any); ok {
				item.Children = append(item.Children, c.parseListItemFromMap(childMap))
			}
		}
	}
	return item
}

func (c *typstConverter) renderTypstList(listData *entity.ListValue, lang string, headerStyles, itemStyles *entity.ListStyles) string {
	var sb strings.Builder
	sb.WriteString("#block[\n") // content block to scope #set rules

	// Render header label if present
	if len(listData.HeaderLabel) > 0 {
		label := c.getListHeaderLabel(listData.HeaderLabel, lang)
		if label != "" {
			sb.WriteString(c.renderListHeader(label, headerStyles))
		}
	}

	// Emit symbol config
	isEnum, config := typstListConfig(listData.Symbol)
	if config != "" {
		sb.WriteString(config)
	}

	// Apply item styles via #set text if needed
	if itemStyles != nil {
		if rule := c.buildListTextSetRule(itemStyles); rule != "" {
			sb.WriteString(rule)
		}
	}

	// Blank line to separate #set rules from list content
	sb.WriteString("\n")

	// Render items recursively
	for _, item := range listData.Items {
		c.renderListItem(&sb, item, isEnum, 0)
	}
	sb.WriteString("]\n") // close content block

	return sb.String()
}

func (c *typstConverter) renderListHeader(label string, styles *entity.ListStyles) string {
	var sb strings.Builder
	sb.WriteString("#text(")
	parts := c.collectListStyleParts(styles)
	sb.WriteString(strings.Join(parts, ", "))
	sb.WriteString(")[")
	sb.WriteString(escapeTypst(label))
	sb.WriteString("]\n\n")
	return sb.String()
}

func (c *typstConverter) renderListItem(sb *strings.Builder, item entity.ListItem, isEnum bool, depth int) {
	indent := strings.Repeat("  ", depth)
	marker := "- "
	if isEnum {
		marker = "+ "
	}

	value := ""
	if item.Value != nil {
		value = c.formatCellValue(item.Value, "")
	}

	fmt.Fprintf(sb, "%s%s%s\n", indent, marker, strings.TrimSpace(value))

	for _, child := range item.Children {
		c.renderListItem(sb, child, isEnum, depth+1)
	}
}

func (c *typstConverter) getListHeaderLabel(labels map[string]string, lang string) string {
	if label, ok := labels[lang]; ok {
		return label
	}
	if label, ok := labels["en"]; ok {
		return label
	}
	for _, label := range labels {
		return label
	}
	return ""
}

// --- Table Nodes ---

func (c *typstConverter) tableCellData(node portabledoc.Node) string {
	return c.tableCell(node, false)
}

func (c *typstConverter) tableCellHeader(node portabledoc.Node) string {
	return c.tableCell(node, true)
}

func (c *typstConverter) tableInjector(node portabledoc.Node) string {
	variableID, _ := node.Attrs["variableId"].(string)
	lang, _ := node.Attrs["lang"].(string)
	if lang == "" {
		lang = "en"
	}

	tableData := c.resolveTableValue(variableID)
	if tableData == nil {
		label, _ := node.Attrs["label"].(string)
		if label == "" {
			label = variableID
		}
		return fmt.Sprintf(
			"#block(fill: rgb(\"%s\"), stroke: (dash: \"dashed\", paint: rgb(\"%s\")), inset: 1em, width: 100%%)[#text(fill: rgb(\"%s\"), style: \"italic\")[\\[Table: %s\\]]]\n",
			c.tokens.PlaceholderFillBg, c.tokens.PlaceholderStroke, c.tokens.PlaceholderTextColor, escapeTypst(label),
		)
	}

	headerStyles := c.parseTableStylesFromAttrs(node.Attrs, "header")
	bodyStyles := c.parseTableStylesFromAttrs(node.Attrs, "body")

	if tableData.HeaderStyles != nil {
		headerStyles = c.mergeTableStyles(tableData.HeaderStyles, headerStyles)
	}
	if tableData.BodyStyles != nil {
		bodyStyles = c.mergeTableStyles(tableData.BodyStyles, bodyStyles)
	}

	return c.renderTypstTable(tableData, lang, headerStyles, bodyStyles)
}

func (c *typstConverter) resolveTableValue(variableID string) *entity.TableValue {
	if v, ok := c.injectables[variableID]; ok {
		if tableVal, ok := v.(*entity.TableValue); ok {
			return tableVal
		}
		if mapVal, ok := v.(map[string]any); ok {
			return c.parseTableFromMap(mapVal)
		}
	}
	return nil
}

func (c *typstConverter) parseTableFromMap(m map[string]any) *entity.TableValue {
	table := entity.NewTableValue()
	c.parseColumnsFromMap(m, table)
	c.parseRowsFromMap(m, table)
	return table
}

func (c *typstConverter) parseColumnsFromMap(m map[string]any, table *entity.TableValue) {
	cols, ok := m["columns"].([]any)
	if !ok {
		return
	}
	for _, colAny := range cols {
		col, ok := colAny.(map[string]any)
		if !ok {
			continue
		}
		c.addColumnFromMap(col, table)
	}
}

func (c *typstConverter) addColumnFromMap(col map[string]any, table *entity.TableValue) {
	key, _ := col["key"].(string)
	dataTypeStr, _ := col["dataType"].(string)
	labels := c.parseLabelsFromMap(col)
	dataType := c.parseDataType(dataTypeStr)

	if width, ok := col["width"].(string); ok && width != "" {
		table.AddColumnWithWidth(key, labels, dataType, width)
	} else {
		table.AddColumn(key, labels, dataType)
	}
}

func (c *typstConverter) parseLabelsFromMap(col map[string]any) map[string]string {
	labels := make(map[string]string)
	labelsMap, ok := col["labels"].(map[string]any)
	if !ok {
		return labels
	}
	for lang, label := range labelsMap {
		if labelStr, ok := label.(string); ok {
			labels[lang] = labelStr
		}
	}
	return labels
}

func (c *typstConverter) parseRowsFromMap(m map[string]any, table *entity.TableValue) {
	rows, ok := m["rows"].([]any)
	if !ok {
		return
	}
	for _, rowAny := range rows {
		row, ok := rowAny.(map[string]any)
		if !ok {
			continue
		}
		cells := c.parseCellsFromRow(row)
		if len(cells) > 0 {
			table.AddRow(cells...)
		}
	}
}

func (c *typstConverter) parseCellsFromRow(row map[string]any) []entity.TableCell {
	cellsAny, ok := row["cells"].([]any)
	if !ok {
		return nil
	}
	cells := make([]entity.TableCell, 0, len(cellsAny))
	for _, cellAny := range cellsAny {
		if cell, ok := cellAny.(map[string]any); ok {
			cells = append(cells, c.parseCellFromMap(cell))
		}
	}
	return cells
}

func (c *typstConverter) parseCellFromMap(cell map[string]any) entity.TableCell {
	valueMap, ok := cell["value"].(map[string]any)
	if !ok || valueMap == nil {
		return entity.EmptyCell()
	}
	return c.parseInjectableValue(valueMap)
}

func (c *typstConverter) parseInjectableValue(valueMap map[string]any) entity.TableCell {
	typeStr, _ := valueMap["type"].(string)

	switch typeStr {
	case "STRING":
		return c.parseStringCell(valueMap)
	case "NUMBER":
		return c.parseNumberCell(valueMap)
	case "BOOLEAN":
		return c.parseBoolCell(valueMap)
	case "DATE":
		return c.parseDateCell(valueMap)
	default:
		return entity.EmptyCell()
	}
}

func (c *typstConverter) parseStringCell(valueMap map[string]any) entity.TableCell {
	if strVal, ok := valueMap["strVal"].(string); ok {
		return entity.Cell(entity.StringValue(strVal))
	}
	return entity.EmptyCell()
}

func (c *typstConverter) parseNumberCell(valueMap map[string]any) entity.TableCell {
	if numVal, ok := valueMap["numVal"].(float64); ok {
		return entity.Cell(entity.NumberValue(numVal))
	}
	return entity.EmptyCell()
}

func (c *typstConverter) parseBoolCell(valueMap map[string]any) entity.TableCell {
	if boolVal, ok := valueMap["boolVal"].(bool); ok {
		return entity.Cell(entity.BoolValue(boolVal))
	}
	return entity.EmptyCell()
}

func (c *typstConverter) parseDateCell(valueMap map[string]any) entity.TableCell {
	timeStr, ok := valueMap["timeVal"].(string)
	if !ok || timeStr == "" {
		return entity.EmptyCell()
	}

	layouts := []string{"2006-01-02", "2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, timeStr); err == nil {
			return entity.Cell(entity.TimeValue(t))
		}
	}
	return entity.Cell(entity.StringValue(timeStr))
}

func (c *typstConverter) parseDataType(s string) entity.ValueType {
	switch s {
	case "NUMBER", "CURRENCY":
		return entity.ValueTypeNumber
	case "BOOLEAN":
		return entity.ValueTypeBool
	case "DATE":
		return entity.ValueTypeTime
	case "TABLE":
		return entity.ValueTypeTable
	default:
		return entity.ValueTypeString
	}
}

// renderTypstTable generates Typst table markup for a TableValue (tableInjector).
func (c *typstConverter) renderTypstTable(tableData *entity.TableValue, lang string, headerStyles, bodyStyles *entity.TableStyles) string {
	if len(tableData.Columns) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("#block[\n") // content block to scope #show rules
	sb.WriteString("#show table.cell: set par(spacing: 0pt, leading: 0.65em)\n")

	sb.WriteString(c.buildTableStyleRules(headerStyles))
	sb.WriteString(c.buildTableBodyStyleRules(bodyStyles))

	colWidths := c.buildTypstColumnWidths(tableData.Columns)
	headerFill := c.getTableHeaderFillColor(headerStyles)
	fmt.Fprintf(&sb, "#table(\n  columns: (%s),\n  inset: (x: 0pt, y: 0pt),\n  stroke: 0.5pt + %s,\n  fill: (x, y) => if y == 0 { rgb(%q) },\n", colWidths, c.tokens.TableStrokeColor, headerFill)
	sb.WriteString(c.buildTableAlignParam(headerStyles, bodyStyles))
	sb.WriteString(c.renderTypstTableHeader(tableData.Columns, lang))
	sb.WriteString(c.renderTypstTableRows(tableData))
	sb.WriteString(")\n")
	sb.WriteString("]\n") // close content block
	return sb.String()
}

func (c *typstConverter) renderTypstTableHeader(columns []entity.TableColumn, lang string) string {
	var sb strings.Builder
	sb.WriteString("  table.header(")
	for i, col := range columns {
		if i > 0 {
			sb.WriteString(", ")
		}
		fmt.Fprintf(&sb, "table.cell(inset: %s)[%s]", c.tokens.TableHeaderCellInset, escapeTypst(c.getColumnLabel(col, lang)))
	}
	sb.WriteString("),\n")
	return sb.String()
}

func (c *typstConverter) renderTypstTableRows(tableData *entity.TableValue) string {
	var sb strings.Builder
	for _, row := range tableData.Rows {
		for i, cell := range row.Cells {
			if cell.Value == nil && cell.Colspan == 0 && cell.Rowspan == 0 {
				continue
			}
			format := c.getColumnFormat(tableData.Columns, i)
			sb.WriteString(c.renderTypstDataCell(cell, format))
		}
	}
	return sb.String()
}

func (c *typstConverter) getColumnFormat(columns []entity.TableColumn, idx int) string {
	if idx < len(columns) && columns[idx].Format != nil {
		return *columns[idx].Format
	}
	return ""
}

func (c *typstConverter) renderTypstDataCell(cell entity.TableCell, format string) string {
	content := escapeTypst(c.formatCellValue(cell.Value, format))
	if cell.Colspan > 1 || cell.Rowspan > 1 {
		attrs := c.buildTypstCellSpanAttrs(cell.Colspan, cell.Rowspan)
		return fmt.Sprintf("  table.cell(%s, inset: %s)[%s],\n", attrs, c.tokens.TableBodyCellInset, content)
	}
	return fmt.Sprintf("  table.cell(inset: %s)[%s],\n", c.tokens.TableBodyCellInset, content)
}

func (c *typstConverter) buildTypstColumnWidths(columns []entity.TableColumn) string {
	widths := make([]string, len(columns))
	for i, col := range columns {
		widths[i] = c.convertColumnWidth(col.Width)
	}
	return strings.Join(widths, ", ")
}

func (c *typstConverter) convertColumnWidth(width *string) string {
	if width == nil {
		return "1fr"
	}
	w := *width
	switch {
	case strings.HasSuffix(w, "%"):
		return strings.TrimSuffix(w, "%") + "%"
	case strings.HasSuffix(w, "px"):
		px := strings.TrimSuffix(w, "px")
		if f, err := strconv.ParseFloat(px, 64); err == nil {
			return fmt.Sprintf("%.1fpt", f*0.75)
		}
		return "1fr"
	default:
		return "1fr"
	}
}

func (c *typstConverter) buildTypstCellSpanAttrs(colspan, rowspan int) string {
	var parts []string
	if colspan > 1 {
		parts = append(parts, fmt.Sprintf("colspan: %d", colspan))
	}
	if rowspan > 1 {
		parts = append(parts, fmt.Sprintf("rowspan: %d", rowspan))
	}
	return strings.Join(parts, ", ")
}

func (c *typstConverter) getColumnLabel(col entity.TableColumn, lang string) string {
	if label, ok := col.Labels[lang]; ok {
		return label
	}
	if label, ok := col.Labels["en"]; ok {
		return label
	}
	for _, label := range col.Labels {
		return label
	}
	return col.Key
}

func (c *typstConverter) formatCellValue(value *entity.InjectableValue, format string) string {
	if value == nil {
		return ""
	}

	switch value.Type() {
	case entity.ValueTypeString:
		s, _ := value.String()
		return s
	case entity.ValueTypeNumber:
		n, _ := value.Number()
		if format != "" {
			return fmt.Sprintf(format, n)
		}
		if n == float64(int64(n)) {
			return strconv.FormatInt(int64(n), 10)
		}
		return strconv.FormatFloat(n, 'f', 2, 64)
	case entity.ValueTypeBool:
		b, _ := value.Bool()
		if b {
			return "Yes"
		}
		return "No"
	case entity.ValueTypeTime:
		t, _ := value.Time()
		if format != "" {
			return t.Format(format)
		}
		return t.Format("2006-01-02")
	default:
		return ""
	}
}

// table renders a user-created editable table.
func (c *typstConverter) table(node portabledoc.Node) string {
	c.currentTableHeaderStyles = c.parseTableStylesFromAttrs(node.Attrs, "header")
	c.currentTableBodyStyles = c.parseTableStylesFromAttrs(node.Attrs, "body")
	defer func() {
		c.currentTableHeaderStyles = nil
		c.currentTableBodyStyles = nil
	}()

	numCols := c.countTableColumns(node)
	colWidths := c.parseEditableTableColumnWidths(node, numCols)

	var sb strings.Builder

	sb.WriteString("#block[\n") // scope #show rules to this table
	sb.WriteString("#show table.cell: set par(spacing: 0pt, leading: 0.65em)\n")
	sb.WriteString(c.buildTableStyleRules(c.currentTableHeaderStyles))
	sb.WriteString(c.buildTableBodyStyleRules(c.currentTableBodyStyles))

	headerFill := c.getTableHeaderFillColor(c.currentTableHeaderStyles)
	fmt.Fprintf(&sb, "#table(\n  columns: (%s),\n  inset: %s,\n  stroke: 0.5pt + %s,\n  fill: (x, y) => if y == 0 { rgb(%q) },\n", colWidths, c.tokens.TableCellInset, c.tokens.TableStrokeColor, headerFill)
	sb.WriteString(c.buildTableAlignParam(c.currentTableHeaderStyles, c.currentTableBodyStyles))

	isFirstRow := true
	for _, row := range node.Content {
		if row.Type != portabledoc.NodeTypeTableRow {
			continue
		}
		for _, cell := range row.Content {
			sb.WriteString(c.renderEditableTableCell(cell, isFirstRow))
		}
		isFirstRow = false
	}

	sb.WriteString(")\n")
	sb.WriteString("]\n") // close block
	return sb.String()
}

func (c *typstConverter) countTableColumns(node portabledoc.Node) int {
	maxCols := 1
	for _, row := range node.Content {
		cols := 0
		for _, cell := range row.Content {
			cols += getIntAttr(cell.Attrs, "colspan", 1)
		}
		if cols > maxCols {
			maxCols = cols
		}
	}
	return maxCols
}

func (c *typstConverter) renderEditableTableCell(cell portabledoc.Node, _ bool) string {
	content := c.convertNodes(cell.Content)
	// Strip empty-paragraph vertical spacing -- #v() inflates cell height in tables
	content = strings.ReplaceAll(content, fmt.Sprintf("#v(%s)", c.tokens.ParagraphSpacing), "")
	// Preserve paragraph structure, trim line whitespace
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	content = strings.Join(lines, "\n")
	content = strings.TrimSpace(content)
	if content == "" {
		content = "~" // Typst non-breaking space -- has text line height
	}

	colspan := getIntAttr(cell.Attrs, "colspan", 1)
	rowspan := getIntAttr(cell.Attrs, "rowspan", 1)

	if colspan > 1 || rowspan > 1 {
		attrs := c.buildTypstCellSpanAttrs(colspan, rowspan)
		return fmt.Sprintf("  table.cell(%s)[%s],\n", attrs, content)
	}
	return fmt.Sprintf("  [%s],\n", content)
}

// parseEditableTableColumnWidths extracts colwidth from first-row cells and converts to proportional Typst fr units.
// TipTap stores colwidth on each cell node (not the table node) as an array of pixel widths (length = colspan).
// prosemirror-tables only sets colwidth on explicitly resized columns; unresized columns stay nil.
// For nil columns, we compute their width from the content area: missing = contentWidth - sum(known).
func (c *typstConverter) parseEditableTableColumnWidths(node portabledoc.Node, numCols int) string {
	fallback := c.equalColumnWidths(numCols)

	firstRow := c.findFirstTableRow(node)
	if firstRow == nil {
		return fallback
	}

	colwidths, missingIdx, hasAny := c.extractColwidths(firstRow)
	if !hasAny || len(colwidths) != numCols {
		return fallback
	}

	if len(missingIdx) > 0 {
		if !c.fillMissingColwidths(colwidths, missingIdx) {
			return fallback
		}
	}

	return c.colwidthsToFrSpec(colwidths)
}

func (c *typstConverter) equalColumnWidths(numCols int) string {
	specs := make([]string, numCols)
	for i := range specs {
		specs[i] = "1fr"
	}
	return strings.Join(specs, ", ")
}

func (c *typstConverter) findFirstTableRow(node portabledoc.Node) *portabledoc.Node {
	for i := range node.Content {
		if node.Content[i].Type == portabledoc.NodeTypeTableRow {
			return &node.Content[i]
		}
	}
	return nil
}

func (c *typstConverter) extractColwidths(firstRow *portabledoc.Node) ([]float64, []int, bool) {
	var colwidths []float64
	var missingIdx []int
	hasAny := false

	for _, cell := range firstRow.Content {
		colspan := getIntAttr(cell.Attrs, "colspan", 1)
		cwAttr, ok := cell.Attrs["colwidth"]
		if !ok || cwAttr == nil {
			for range colspan {
				missingIdx = append(missingIdx, len(colwidths))
				colwidths = append(colwidths, 0)
			}
			continue
		}

		cellWidths := c.parseColwidthAttr(cwAttr, colspan)
		if cellWidths == nil {
			return nil, nil, false
		}
		colwidths = append(colwidths, cellWidths...)
		hasAny = true
	}

	return colwidths, missingIdx, hasAny
}

func (c *typstConverter) parseColwidthAttr(cwAttr any, colspan int) []float64 {
	var cellWidths []float64
	switch v := cwAttr.(type) {
	case []any:
		for _, val := range v {
			if num, ok := val.(float64); ok && num > 0 {
				cellWidths = append(cellWidths, num)
			} else {
				return nil
			}
		}
	case []float64:
		cellWidths = v
	default:
		return nil
	}

	if len(cellWidths) != colspan {
		return nil
	}
	return cellWidths
}

func (c *typstConverter) fillMissingColwidths(colwidths []float64, missingIdx []int) bool {
	if c.contentWidthPx <= 0 {
		return false
	}
	var knownSum float64
	for _, w := range colwidths {
		knownSum += w
	}
	remaining := c.contentWidthPx - knownSum
	perMissing := remaining / float64(len(missingIdx))
	if perMissing < 1 {
		perMissing = 1
	}
	for _, idx := range missingIdx {
		colwidths[idx] = perMissing
	}
	return true
}

func (c *typstConverter) colwidthsToFrSpec(colwidths []float64) string {
	specs := make([]string, len(colwidths))
	for i, w := range colwidths {
		specs[i] = fmt.Sprintf("%.0ffr", w)
	}
	return strings.Join(specs, ", ")
}

// tableRow is a fallback -- normally handled inline by table().
func (c *typstConverter) tableRow(node portabledoc.Node) string {
	var sb strings.Builder
	for _, child := range node.Content {
		sb.WriteString(c.convertNode(child))
	}
	return sb.String()
}

// tableCell renders a cell -- used as fallback when not inside table().
func (c *typstConverter) tableCell(node portabledoc.Node, _ bool) string {
	content := c.convertNodes(node.Content)
	if content == "" {
		content = " "
	}
	return fmt.Sprintf("[%s], ", strings.TrimSpace(content))
}
