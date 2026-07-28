package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// ContentValidationResult holds the result of content validation.
type ContentValidationResult struct {
	Valid                bool
	Errors               []ValidationError
	Warnings             []ValidationWarning
	ExtractedSignerRoles []*entity.TemplateVersionSignerRole // Populated only on successful publish validation
	ExtractedInjectables []*entity.TemplateVersionInjectable // Populated only on successful publish validation
}

// ValidationError represents a validation error.
type ValidationError struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationWarning represents a validation warning (non-blocking).
type ValidationWarning struct {
	Code       string  `json:"code"`
	Path       string  `json:"path"`
	Message    string  `json:"message"`
	Suggestion *string `json:"suggestion,omitempty"`
}

// ContentValidator defines the content validation interface.
type ContentValidator interface {
	// ValidateForDraft performs minimal validation (JSON parseability only).
	// Empty content is considered valid for drafts.
	ValidateForDraft(ctx context.Context, content []byte) *ContentValidationResult

	// ValidateForContentUpdate performs format and injectable validation for saving draft content.
	// It validates JSON parseability, document structure (version, meta, pageConfig),
	// and injectable accessibility — but does NOT require signer roles or signatures.
	// Empty content is considered valid.
	ValidateForContentUpdate(ctx context.Context, workspaceID, versionID string, content []byte) *ContentValidationResult

	// ValidateForPublish performs complete business logic validation.
	// This includes:
	// - Document structure validation
	// - Signer roles validation (values or injectable references)
	// - Signature blocks validation (role assignments)
	// - Variable/injectable access validation
	// - Conditional expression validation
	// - Signing workflow validation
	ValidateForPublish(ctx context.Context, workspaceID, versionID string, content []byte) *ContentValidationResult
}

// NewValidationResult creates a new validation result.
func NewValidationResult() *ContentValidationResult {
	return &ContentValidationResult{
		Valid:    true,
		Errors:   make([]ValidationError, 0),
		Warnings: make([]ValidationWarning, 0),
	}
}

// AddError adds a validation error and marks the result as invalid.
func (r *ContentValidationResult) AddError(code, path, message string) {
	r.Valid = false
	r.Errors = append(r.Errors, ValidationError{
		Code:    code,
		Path:    path,
		Message: message,
	})
}

// AddWarning adds a validation warning (does not affect validity).
func (r *ContentValidationResult) AddWarning(code, path, message string) {
	r.Warnings = append(r.Warnings, ValidationWarning{
		Code:    code,
		Path:    path,
		Message: message,
	})
}

// AddWarningWithSuggestion adds a validation warning with a suggestion.
func (r *ContentValidationResult) AddWarningWithSuggestion(code, path, message, suggestion string) {
	r.Warnings = append(r.Warnings, ValidationWarning{
		Code:       code,
		Path:       path,
		Message:    message,
		Suggestion: &suggestion,
	})
}

// HasErrors returns true if there are any validation errors.
func (r *ContentValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// HasWarnings returns true if there are any validation warnings.
func (r *ContentValidationResult) HasWarnings() bool {
	return len(r.Warnings) > 0
}

// ErrorCount returns the number of validation errors.
func (r *ContentValidationResult) ErrorCount() int {
	return len(r.Errors)
}

// WarningCount returns the number of validation warnings.
func (r *ContentValidationResult) WarningCount() int {
	return len(r.Warnings)
}

// Merge combines another validation result into this one.
func (r *ContentValidationResult) Merge(other *ContentValidationResult) {
	if other == nil {
		return
	}
	if !other.Valid {
		r.Valid = false
	}
	r.Errors = append(r.Errors, other.Errors...)
	r.Warnings = append(r.Warnings, other.Warnings...)
}
