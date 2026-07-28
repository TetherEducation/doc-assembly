package sdk

import "github.com/TetherEducation/doc-assembly/core/internal/core/service/rendering/pdfrenderer"

// TypstDesignTokens controls fonts, colors, spacing, and heading styles in Typst PDF output.
type TypstDesignTokens = pdfrenderer.TypstDesignTokens

// DefaultDesignTokens returns the default design tokens for PDF rendering.
var DefaultDesignTokens = pdfrenderer.DefaultDesignTokens
