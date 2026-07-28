package dto

import "github.com/TetherEducation/doc-assembly/core/internal/core/entity"

// ContentValidationErrorDTO represents a single content validation error.
type ContentValidationErrorDTO struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ContentValidationWarningDTO represents a single content validation warning.
type ContentValidationWarningDTO struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ContentValidationResultDTO represents the complete validation result.
type ContentValidationResultDTO struct {
	Valid    bool                          `json:"valid"`
	Errors   []ContentValidationErrorDTO   `json:"errors,omitempty"`
	Warnings []ContentValidationWarningDTO `json:"warnings,omitempty"`
}

// ContentValidationErrorResponse extends ErrorResponse for validation failures.
// Used when publish fails due to content validation errors.
type ContentValidationErrorResponse struct {
	Error      string                      `json:"error"`
	Validation *ContentValidationResultDTO `json:"validation,omitempty"`
}

// NewContentValidationResultDTO creates a new validation result DTO from entity.
func NewContentValidationResultDTO(err *entity.ContentValidationError) *ContentValidationResultDTO {
	if err == nil {
		return &ContentValidationResultDTO{Valid: true}
	}

	result := &ContentValidationResultDTO{
		Valid:    !err.HasErrors(),
		Errors:   make([]ContentValidationErrorDTO, 0, len(err.Errors)),
		Warnings: make([]ContentValidationWarningDTO, 0, len(err.Warnings)),
	}

	for _, e := range err.Errors {
		result.Errors = append(result.Errors, ContentValidationErrorDTO{
			Code:    e.Code,
			Path:    e.Path,
			Message: e.Message,
		})
	}

	for _, w := range err.Warnings {
		result.Warnings = append(result.Warnings, ContentValidationWarningDTO{
			Code:    w.Code,
			Path:    w.Path,
			Message: w.Message,
		})
	}

	return result
}

// NewContentValidationErrorResponse creates a validation error response.
func NewContentValidationErrorResponse(err *entity.ContentValidationError) ContentValidationErrorResponse {
	return ContentValidationErrorResponse{
		Error:      "content validation failed",
		Validation: NewContentValidationResultDTO(err),
	}
}
