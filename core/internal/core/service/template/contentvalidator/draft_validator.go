package contentvalidator

import (
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// validateDraft performs minimal validation for draft mode.
// Only checks that content is valid JSON.
// Empty content is considered valid for drafts.
func validateDraft(content []byte) *port.ContentValidationResult {
	result := port.NewValidationResult()

	// Empty content is valid for drafts
	if len(content) == 0 {
		return result
	}

	// Only check JSON parseability
	_, err := portabledoc.Parse(content)
	if err != nil {
		result.AddError(ErrCodeInvalidJSON, "", sanitizeJSONError(err))
	}

	return result
}
