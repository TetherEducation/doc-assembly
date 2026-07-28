package contentvalidator

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	injectableuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/injectable"
)

// validationContext holds shared state during validation.
type validationContext struct {
	ctx         context.Context
	workspaceID string
	versionID   string
	doc         *portabledoc.Document
	result      *port.ContentValidationResult
	service     *Service

	// Computed sets for validation
	roleIDSet   portabledoc.Set[string]
	variableSet portabledoc.Set[string] // includes role variables

	// Accessible injectables cache (loaded from DB)
	accessibleInjectables    portabledoc.Set[string]
	accessibleInjectableList []*entity.InjectableDefinition // Full list for extraction
}

// addError adds a validation error.
func (vc *validationContext) addError(code, path, message string) {
	vc.result.AddError(code, path, message)
}

// addErrorf adds a formatted validation error.
func (vc *validationContext) addErrorf(code, path, format string, args ...any) {
	vc.result.AddError(code, path, fmt.Sprintf(format, args...))
}

// addWarning adds a validation warning.
func (vc *validationContext) addWarning(code, path, message string) {
	vc.result.AddWarning(code, path, message)
}

// addWarningf adds a formatted validation warning.
func (vc *validationContext) addWarningf(code, path, format string, args ...any) {
	vc.result.AddWarning(code, path, fmt.Sprintf(format, args...))
}

// checkCancelled checks if context was cancelled.
func (vc *validationContext) checkCancelled() bool {
	select {
	case <-vc.ctx.Done():
		vc.addError(ErrCodeValidationCancelled, "", "Validation was cancelled")
		return true
	default:
		return false
	}
}

// parseDocument parses content and adds error to result if parsing fails.
// Returns the parsed document and true if successful, nil and false otherwise.
func parseDocument(content []byte, result *port.ContentValidationResult) (*portabledoc.Document, bool) {
	doc, err := portabledoc.Parse(content)
	if err != nil {
		result.AddError(ErrCodeInvalidJSON, "", sanitizeJSONError(err))
		return nil, false
	}
	if doc == nil {
		result.AddError(ErrCodeEmptyContent, "", "Content structure is required for publishing")
		return nil, false
	}
	return doc, true
}

// finalizeValidation extracts signer roles and injectables on success and logs the outcome.
func finalizeValidation(vctx *validationContext) {
	if vctx.result.Valid {
		vctx.result.ExtractedSignerRoles = extractSignerRoles(vctx.versionID, vctx.doc)
		vctx.result.ExtractedInjectables = extractInjectables(vctx)
		slog.DebugContext(vctx.ctx, "content validation successful",
			slog.Int("signer_roles", len(vctx.result.ExtractedSignerRoles)),
			slog.Int("injectables", len(vctx.result.ExtractedInjectables)),
		)
		return
	}
	slog.WarnContext(vctx.ctx, "content validation failed",
		slog.Int("error_count", vctx.result.ErrorCount()),
		slog.Int("warning_count", vctx.result.WarningCount()),
	)
}

// validatePublish orchestrates all publish-time validations.
func (s *Service) validatePublish(
	ctx context.Context,
	workspaceID, versionID string,
	content []byte,
) *port.ContentValidationResult {
	result := port.NewValidationResult()
	slog.DebugContext(ctx, "starting content validation",
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
		slog.WarnContext(ctx, "failed to load accessible injectables", slog.String("error", err.Error()))
	}

	validators := []func(*validationContext){
		s.validateStructure,
		s.validatePageConfig,
		s.validateSignerRoles,
		s.validateVariables,
		s.validateSignatures,
		s.validateInteractiveFields,
		s.validateConditionals,
		s.validateWorkflow,
	}
	for _, validate := range validators {
		if vctx.checkCancelled() {
			break
		}
		validate(vctx)
	}

	finalizeValidation(vctx)
	return result
}

// buildRoleIDSet creates a set of signer role IDs from the document.
func buildRoleIDSet(roles []portabledoc.SignerRole) portabledoc.Set[string] {
	set := make(portabledoc.Set[string], len(roles))
	for _, role := range roles {
		if role.ID != "" {
			set.Add(role.ID)
		}
	}
	return set
}

// buildVariableSet creates a set of valid variable IDs.
// Includes both document variables and role-generated variables.
func buildVariableSet(variableIDs []string, roles []portabledoc.SignerRole) portabledoc.Set[string] {
	set := portabledoc.NewSet(variableIDs)

	// Add role-generated variables (ROLE.{label}.name and ROLE.{label}.email)
	for _, role := range roles {
		if role.Label != "" {
			set.Add(portabledoc.BuildRoleVariableID(role.Label, portabledoc.RolePropertyName))
			set.Add(portabledoc.BuildRoleVariableID(role.Label, portabledoc.RolePropertyEmail))
		}
	}

	return set
}

// loadAccessibleInjectables loads the set of injectable keys accessible to the workspace.
// This includes both DB injectables and system injectables.
func (s *Service) loadAccessibleInjectables(vctx *validationContext) error {
	if s.injectableUC == nil {
		vctx.accessibleInjectables = make(portabledoc.Set[string])
		return nil
	}

	result, err := s.injectableUC.ListInjectables(vctx.ctx, &injectableuc.ListInjectablesRequest{
		WorkspaceID: vctx.workspaceID,
	})
	if err != nil {
		return err
	}

	vctx.accessibleInjectables = make(portabledoc.Set[string], len(result.Injectables))
	for _, inj := range result.Injectables {
		vctx.accessibleInjectables.Add(inj.Key)
	}

	// Store full list for extraction phase
	vctx.accessibleInjectableList = result.Injectables

	return nil
}
