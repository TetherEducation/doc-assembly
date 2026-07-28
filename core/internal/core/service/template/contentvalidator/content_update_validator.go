package contentvalidator

import (
	"context"
	"log/slog"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// validateContentUpdate validates format and injectable accessibility for saving draft content.
// Validates: JSON format, document structure (version, meta, pageConfig), and injectables.
// Does NOT validate: signer roles, signatures, interactive fields, conditionals, workflow.
func (s *Service) validateContentUpdate(
	ctx context.Context,
	workspaceID, versionID string,
	content []byte,
) *port.ContentValidationResult {
	result := port.NewValidationResult()

	// Empty content is valid for drafts
	if len(content) == 0 {
		return result
	}

	slog.DebugContext(ctx, "starting content update validation",
		slog.String("workspace_id", workspaceID),
		slog.String("version_id", versionID),
	)

	doc, ok := parseDocument(content, result)
	if !ok {
		return result
	}

	vctx := &validationContext{
		ctx:         ctx,
		workspaceID: workspaceID,
		versionID:   versionID,
		doc:         doc,
		result:      result,
		service:     s,
		roleIDSet:   buildRoleIDSet(doc.SignerRoles),
		variableSet: buildVariableSet(doc.VariableIDs, doc.SignerRoles),
	}

	if err := s.loadAccessibleInjectables(vctx); err != nil {
		slog.WarnContext(ctx, "failed to load accessible injectables for content update",
			slog.String("error", err.Error()))
	}

	validators := []func(*validationContext){
		s.validateStructure,
		s.validatePageConfig,
		s.validateVariables,
	}
	for _, validate := range validators {
		if vctx.checkCancelled() {
			break
		}
		validate(vctx)
	}

	if result.Valid {
		// Extract injectables (signer roles are not validated yet — publish-only concern)
		result.ExtractedInjectables = extractInjectables(vctx)
	}

	return result
}
