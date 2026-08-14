//nolint:staticcheck
package pdfrenderer

import (
	"fmt"
	"strings"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// --- Table style parsing ---

func (c *typstConverter) parseTableStylesFromAttrs(attrs map[string]any, prefix string) *entity.TableStyles {
	styles := &entity.TableStyles{}
	hasStyles := false

	if v, ok := attrs[prefix+"FontFamily"].(string); ok && v != "" {
		styles.FontFamily = &v
		hasStyles = true
	}
	if v, ok := attrs[prefix+"FontSize"].(float64); ok && v > 0 {
		i := int(v)
		styles.FontSize = &i
		hasStyles = true
	}
	if v, ok := attrs[prefix+"FontWeight"].(string); ok && v != "" {
		styles.FontWeight = &v
		hasStyles = true
	}
	if v, ok := attrs[prefix+"TextColor"].(string); ok && v != "" {
		styles.TextColor = &v
		hasStyles = true
	}
	if v, ok := attrs[prefix+"TextAlign"].(string); ok && v != "" {
		styles.TextAlign = &v
		hasStyles = true
	}
	if v, ok := attrs[prefix+"Background"].(string); ok && v != "" {
		styles.Background = &v
		hasStyles = true
	}

	if !hasStyles {
		return nil
	}
	return styles
}

func (c *typstConverter) mergeTableStyles(base, override *entity.TableStyles) *entity.TableStyles {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}

	result := *base
	if override.FontFamily != nil {
		result.FontFamily = override.FontFamily
	}
	if override.FontSize != nil {
		result.FontSize = override.FontSize
	}
	if override.FontWeight != nil {
		result.FontWeight = override.FontWeight
	}
	if override.TextColor != nil {
		result.TextColor = override.TextColor
	}
	if override.TextAlign != nil {
		result.TextAlign = override.TextAlign
	}
	if override.Background != nil {
		result.Background = override.Background
	}
	return &result
}

// --- List style parsing ---

func (c *typstConverter) parseListStylesFromAttrs(attrs map[string]any, prefix string) *entity.ListStyles {
	styles := &entity.ListStyles{}
	hasValue := false

	if v, ok := attrs[prefix+"FontFamily"].(string); ok && v != "" {
		styles.FontFamily = &v
		hasValue = true
	}
	if v, ok := attrs[prefix+"FontSize"].(float64); ok && v > 0 {
		intVal := int(v)
		styles.FontSize = &intVal
		hasValue = true
	}
	if v, ok := attrs[prefix+"FontWeight"].(string); ok && v != "" {
		styles.FontWeight = &v
		hasValue = true
	}
	if v, ok := attrs[prefix+"TextColor"].(string); ok && v != "" {
		styles.TextColor = &v
		hasValue = true
	}
	if v, ok := attrs[prefix+"TextAlign"].(string); ok && v != "" {
		styles.TextAlign = &v
		hasValue = true
	}

	if !hasValue {
		return nil
	}
	return styles
}

func (c *typstConverter) mergeListStyles(base, override *entity.ListStyles) *entity.ListStyles {
	if base == nil {
		return override
	}
	if override == nil {
		return base
	}

	result := *base
	if override.FontFamily != nil {
		result.FontFamily = override.FontFamily
	}
	if override.FontSize != nil {
		result.FontSize = override.FontSize
	}
	if override.FontWeight != nil {
		result.FontWeight = override.FontWeight
	}
	if override.TextColor != nil {
		result.TextColor = override.TextColor
	}
	if override.TextAlign != nil {
		result.TextAlign = override.TextAlign
	}
	return &result
}

// --- Typst style output ---

// collectListStyleParts builds Typst text parameters from list styles.
func (c *typstConverter) collectListStyleParts(styles *entity.ListStyles) []string {
	parts := make([]string, 0)
	if styles == nil {
		return parts
	}
	if styles.FontSize != nil {
		parts = append(parts, fmt.Sprintf("size: %dpt", *styles.FontSize))
	}
	if styles.FontWeight != nil && *styles.FontWeight == "bold" {
		parts = append(parts, "weight: \"bold\"")
	}
	if styles.TextColor != nil {
		parts = append(parts, fmt.Sprintf("fill: rgb(\"%s\")", *styles.TextColor))
	}
	if styles.FontFamily != nil {
		parts = append(parts, fmt.Sprintf("font: %s", fontWithFallbacks(*styles.FontFamily)))
	}
	return parts
}

// buildListTextSetRule generates a #set text(...) rule from list styles.
func (c *typstConverter) buildListTextSetRule(styles *entity.ListStyles) string {
	parts := c.collectListStyleParts(styles)
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("#set text(%s)\n", strings.Join(parts, ", "))
}

// buildTableStyleRules generates Typst show rules for header text styling.
func (c *typstConverter) buildTableStyleRules(headerStyles *entity.TableStyles) string {
	if headerStyles == nil {
		return ""
	}

	var sb strings.Builder

	if headerStyles.FontWeight != nil {
		//nolint:staticcheck
		sb.WriteString(fmt.Sprintf("#show table.cell.where(y: 0): set text(weight: \"%s\")\n", *headerStyles.FontWeight))
	}
	if headerStyles.TextColor != nil {
		//nolint:staticcheck
		sb.WriteString(fmt.Sprintf("#show table.cell.where(y: 0): set text(fill: rgb(\"%s\"))\n", *headerStyles.TextColor))
	}
	if headerStyles.FontSize != nil {
		//nolint:staticcheck
		sb.WriteString(fmt.Sprintf("#show table.cell.where(y: 0): set text(size: %dpt)\n", *headerStyles.FontSize))
	}
	if headerStyles.FontFamily != nil {
		sb.WriteString(fmt.Sprintf("#show table.cell.where(y: 0): set text(font: %s)\n", fontWithFallbacks(*headerStyles.FontFamily)))
	}

	return sb.String()
}

// buildTableBodyStyleRules generates Typst show rules for body cell styling.
//
// Body rows are every row but the header, which `where()` cannot express: its `y` selector
// takes a single integer, so the previous `where(y: range(1, none))` was rejected outright
// with "expected integer, found none" and failed the whole compile. Any template holding a
// table was therefore unrenderable whenever body styles were set. A predicate show rule
// says the same thing in a form Typst accepts.
func (c *typstConverter) buildTableBodyStyleRules(bodyStyles *entity.TableStyles) string {
	if bodyStyles == nil {
		return ""
	}

	var sets []string
	if bodyStyles.FontWeight != nil {
		sets = append(sets, fmt.Sprintf("set text(weight: %q)", *bodyStyles.FontWeight))
	}
	if bodyStyles.TextColor != nil {
		sets = append(sets, fmt.Sprintf("set text(fill: rgb(%q))", *bodyStyles.TextColor))
	}
	if bodyStyles.FontSize != nil {
		sets = append(sets, fmt.Sprintf("set text(size: %dpt)", *bodyStyles.FontSize))
	}
	if bodyStyles.FontFamily != nil {
		sets = append(sets, fmt.Sprintf("set text(font: %s)", fontWithFallbacks(*bodyStyles.FontFamily)))
	}
	if len(sets) == 0 {
		return ""
	}

	// Header cells (y == 0) are left to buildTableStyleRules; everything below is body.
	return fmt.Sprintf("#show table.cell: it => { if it.y > 0 { %s; it } else { it } }\n",
		strings.Join(sets, "; "))
}

// buildTableAlignParam generates the align parameter for a Typst table.
func (c *typstConverter) buildTableAlignParam(headerStyles, bodyStyles *entity.TableStyles) string {
	headerAlign := getTypstAlignment(headerStyles)
	bodyAlign := getTypstAlignment(bodyStyles)

	if headerAlign != "" && bodyAlign != "" {
		return fmt.Sprintf("  align: (x, y) => if y == 0 { %s } else { %s },\n", headerAlign, bodyAlign)
	}
	if headerAlign != "" {
		return fmt.Sprintf("  align: (x, y) => if y == 0 { %s } else { auto },\n", headerAlign)
	}
	if bodyAlign != "" {
		return fmt.Sprintf("  align: %s,\n", bodyAlign)
	}
	return ""
}

func (c *typstConverter) getTableHeaderFillColor(styles *entity.TableStyles) string {
	if styles != nil && styles.Background != nil {
		return *styles.Background
	}
	return c.tokens.TableHeaderFillDefault
}

// getTypstAlignment converts a CSS text-align value to Typst alignment.
func getTypstAlignment(styles *entity.TableStyles) string {
	if styles == nil || styles.TextAlign == nil {
		return ""
	}
	switch *styles.TextAlign {
	case "left":
		return "left"
	case "center":
		return "center"
	case "right":
		return "right"
	default:
		return ""
	}
}
