package contentvalidator

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	injectableuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/injectable"
)

// Service implements the ContentValidator interface.
type Service struct {
	injectableUC    injectableuc.InjectableUseCase
	maxNestingDepth int
	strictMode      bool
}

// Option configures the validator service.
type Option func(*Service)

// WithStrictMode enables strict validation mode.
// In strict mode, warnings are treated as errors.
func WithStrictMode() Option {
	return func(s *Service) {
		s.strictMode = true
	}
}

// WithMaxNestingDepth sets the maximum conditional nesting depth.
// Default is 3.
func WithMaxNestingDepth(depth int) Option {
	return func(s *Service) {
		if depth > 0 {
			s.maxNestingDepth = depth
		}
	}
}

// New creates a new content validator service.
func New(injectableUC injectableuc.InjectableUseCase, opts ...Option) *Service {
	s := &Service{
		injectableUC:    injectableUC,
		maxNestingDepth: 3, // default
		strictMode:      false,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// ValidateForDraft performs minimal validation for draft mode.
func (s *Service) ValidateForDraft(ctx context.Context, content []byte) *port.ContentValidationResult {
	return validateDraft(content)
}

// ValidateForContentUpdate validates format and injectables for saving draft content.
// It checks: JSON parseability, document structure, and injectable accessibility.
// Signer roles, signatures, and workflow are NOT validated (publish-only concerns).
func (s *Service) ValidateForContentUpdate(
	ctx context.Context,
	workspaceID, versionID string,
	content []byte,
) *port.ContentValidationResult {
	return s.validateContentUpdate(ctx, workspaceID, versionID, content)
}

// ValidateForPublish performs complete validation for publish mode.
func (s *Service) ValidateForPublish(
	ctx context.Context,
	workspaceID, versionID string,
	content []byte,
) *port.ContentValidationResult {
	return s.validatePublish(ctx, workspaceID, versionID, content)
}

// Ensure Service implements ContentValidator interface.
var _ port.ContentValidator = (*Service)(nil)
