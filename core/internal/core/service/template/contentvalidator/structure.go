package contentvalidator

import (
	"regexp"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
)

// versionRegex validates semantic version format.
var versionRegex = regexp.MustCompile(`^\d+\.\d+\.\d+$`)

// validateStructure validates document structure (version, meta).
func (s *Service) validateStructure(vctx *validationContext) {
	doc := vctx.doc

	// Validate version format
	if doc.Version == "" {
		vctx.addError(ErrCodeInvalidVersion, "version", "Version is required")
	} else if !versionRegex.MatchString(doc.Version) {
		vctx.addErrorf(ErrCodeInvalidVersion, "version",
			"Version must be in semantic format (x.y.z), got: %s", doc.Version)
	}

	// Validate meta
	validateMeta(vctx)
}

// validateMeta validates document metadata.
func validateMeta(vctx *validationContext) {
	meta := vctx.doc.Meta

	// Title is required
	if meta.Title == "" {
		vctx.addError(ErrCodeMissingMetaTitle, "meta.title", "Document title is required")
	}

	// Language must be valid if provided
	if meta.Language != "" && !portabledoc.ValidLanguages.Contains(meta.Language) {
		vctx.addErrorf(ErrCodeInvalidLanguage, "meta.language",
			"Invalid language code: %s. Must be 'en' or 'es'", meta.Language)
	}
}

// validatePageConfig validates page configuration.
func (s *Service) validatePageConfig(vctx *validationContext) {
	pc := vctx.doc.PageConfig

	// FormatID must be valid
	if pc.FormatID != "" && !portabledoc.ValidPageFormats.Contains(pc.FormatID) {
		vctx.addErrorf(ErrCodeInvalidPageFormat, "pageConfig.formatId",
			"Invalid page format: %s. Must be A4, LETTER, LEGAL, or CUSTOM", pc.FormatID)
	}

	// Width and height must be positive
	if pc.Width <= 0 {
		vctx.addError(ErrCodeInvalidPageSize, "pageConfig.width", "Page width must be positive")
	}
	if pc.Height <= 0 {
		vctx.addError(ErrCodeInvalidPageSize, "pageConfig.height", "Page height must be positive")
	}

	// Validate margins
	validateMargins(vctx, pc.Margins)
}

// validateMargins validates page margins.
func validateMargins(vctx *validationContext, margins portabledoc.Margins) {
	if margins.Top < 0 {
		vctx.addError(ErrCodeInvalidMargins, "pageConfig.margins.top", "Top margin cannot be negative")
	}
	if margins.Bottom < 0 {
		vctx.addError(ErrCodeInvalidMargins, "pageConfig.margins.bottom", "Bottom margin cannot be negative")
	}
	if margins.Left < 0 {
		vctx.addError(ErrCodeInvalidMargins, "pageConfig.margins.left", "Left margin cannot be negative")
	}
	if margins.Right < 0 {
		vctx.addError(ErrCodeInvalidMargins, "pageConfig.margins.right", "Right margin cannot be negative")
	}
}
