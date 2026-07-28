package contentvalidator

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/validation"
)

// validateSignerRoles validates all signer roles in the document.
func (s *Service) validateSignerRoles(vctx *validationContext) {
	roles := vctx.doc.SignerRoles

	// Warn if no signer roles defined
	if len(roles) == 0 {
		vctx.addWarning(WarnCodeNoSignerRoles, "signerRoles", "No signer roles defined in document")
		return
	}

	// Track seen IDs and orders for duplicate detection
	seenIDs := make(portabledoc.Set[string])
	seenOrders := make(portabledoc.Set[int])

	for i, role := range roles {
		path := fmt.Sprintf("signerRoles[%d]", i)
		validateSignerRole(vctx, role, path, seenIDs, seenOrders)
	}
}

// validateSignerRole validates a single signer role.
func validateSignerRole(
	vctx *validationContext,
	role portabledoc.SignerRole,
	path string,
	seenIDs portabledoc.Set[string],
	seenOrders portabledoc.Set[int],
) {
	// ID is required and must be unique
	if role.ID == "" {
		vctx.addError(ErrCodeEmptyRoleID, path+".id", "Signer role ID is required")
	} else if seenIDs.Contains(role.ID) {
		vctx.addErrorf(ErrCodeDuplicateRoleID, path+".id",
			"Duplicate signer role ID: %s", role.ID)
	} else {
		seenIDs.Add(role.ID)
	}

	// Label is required
	if role.Label == "" {
		vctx.addError(ErrCodeEmptyRoleLabel, path+".label", "Signer role label is required")
	}

	// Order must be positive and unique
	if role.Order < 1 {
		vctx.addErrorf(ErrCodeInvalidRoleOrder, path+".order",
			"Signer order must be >= 1, got: %d", role.Order)
	} else if seenOrders.Contains(role.Order) {
		vctx.addErrorf(ErrCodeDuplicateRoleOrder, path+".order",
			"Duplicate signer order: %d", role.Order)
	} else {
		seenOrders.Add(role.Order)
	}

	// Validate name field
	validateFieldValue(vctx, role.Name, path+".name", "name", role.Label, vctx.doc.VariableIDs, vctx.accessibleInjectables)

	// Validate email field
	validateFieldValue(vctx, role.Email, path+".email", "email", role.Label, vctx.doc.VariableIDs, vctx.accessibleInjectables)
}

// validateFieldValue validates a SignerRoleFieldValue.
func validateFieldValue(
	vctx *validationContext,
	field portabledoc.FieldValue,
	path string,
	fieldName string,
	roleLabel string,
	variableIDs []string,
	accessibleInjectables portabledoc.Set[string],
) {
	// Type must be valid
	if !portabledoc.ValidFieldTypes.Contains(field.Type) {
		vctx.addErrorf(ErrCodeInvalidRoleField, path+".type",
			"Role '%s': invalid %s field type '%s' (must be 'text' or 'injectable')", roleLabel, fieldName, field.Type)
		return
	}

	// Validate based on type
	if field.IsText() {
		validateTextField(vctx, field, path, fieldName, roleLabel)
	} else if field.IsInjectable() {
		validateInjectableField(vctx, field, path, fieldName, roleLabel, variableIDs, accessibleInjectables)
	}
}

// validateTextField validates a text-type field value.
func validateTextField(vctx *validationContext, field portabledoc.FieldValue, path, fieldName, roleLabel string) {
	if field.IsEmpty() {
		vctx.addErrorf(ErrCodeEmptyRoleFieldValue, path+".value",
			"Role '%s': %s is required", roleLabel, fieldName)
		return
	}

	// Validate email format for hardcoded email values
	if fieldName == "email" && !validation.IsValidEmail(field.Value) {
		vctx.addErrorf(ErrCodeInvalidEmailFormat, path+".value",
			"Role '%s': invalid email format '%s'", roleLabel, field.Value)
	}
}

// validateInjectableField validates an injectable-type field value.
func validateInjectableField(
	vctx *validationContext,
	field portabledoc.FieldValue,
	path, fieldName, roleLabel string,
	variableIDs []string,
	accessibleInjectables portabledoc.Set[string],
) {
	refs := field.InjectableRefs()

	// Injectable reference must not be empty
	if len(refs) == 0 {
		vctx.addErrorf(ErrCodeEmptyRoleFieldValue, path+".value",
			"Role '%s': %s variable reference is required", roleLabel, fieldName)
		return
	}

	// Email field supports a single injectable only.
	if fieldName == "email" && len(refs) > 1 {
		vctx.addErrorf(ErrCodeInvalidRoleField, path+".values",
			"Role '%s': email supports a single injectable reference", roleLabel)
		return
	}

	// Multi-reference name fields currently support only space separator.
	if fieldName == "name" && len(refs) > 1 &&
		field.Separator != "" && field.Separator != portabledoc.FieldSeparatorSpace {
		vctx.addErrorf(ErrCodeInvalidRoleField, path+".separator",
			"Role '%s': unsupported name separator '%s' (must be 'space')", roleLabel, field.Separator)
		return
	}

	// Fallback to variableIds check only when accessible injectables could not be loaded.
	variableSet := portabledoc.NewSet(variableIDs)
	for idx, ref := range refs {
		refPath := path + ".value"
		if len(refs) > 1 {
			refPath = path + fmt.Sprintf(".values[%d]", idx)
		}

		// Primary check: workspace-accessible injectables.
		if accessibleInjectables.Len() > 0 {
			if !accessibleInjectables.Contains(ref) {
				vctx.addErrorf(ErrCodeInaccessibleInjectable, refPath,
					"Role '%s': %s references inaccessible variable '%s'", roleLabel, fieldName, ref)
			}
			continue
		}

		// Fallback check (when access list could not be loaded): document variableIds.
		if !variableSet.Contains(ref) {
			vctx.addErrorf(ErrCodeRoleInjectableNotFound, refPath,
				"Role '%s': %s references unknown variable '%s'", roleLabel, fieldName, ref)
		}
	}
}

// extractSignerRoles converts document signer roles to database entities.
func extractSignerRoles(versionID string, doc *portabledoc.Document) []*entity.TemplateVersionSignerRole {
	if len(doc.SignerRoles) == 0 {
		return nil
	}

	roles := make([]*entity.TemplateVersionSignerRole, 0, len(doc.SignerRoles))
	for _, sr := range doc.SignerRoles {
		anchorString := portabledoc.GenerateAnchorString(sr.Label)
		role := entity.NewTemplateVersionSignerRole(
			versionID,
			sr.Label,
			anchorString,
			sr.Order,
		)
		roles = append(roles, role)
	}

	return roles
}

// extractInjectables converts injectable references used by the document to
// template version injectables.
// Sources:
//   - document variableIds
//   - signer role name/email injectable references
//
// It filters out role variables (ROLE.*) and creates the appropriate injectable
// based on whether it's a system injectable or workspace injectable.
func extractInjectables(vctx *validationContext) []*entity.TemplateVersionInjectable {
	roleRefs := collectInjectableRefsFromSignerRoles(vctx.doc.SignerRoles)
	imageRefs := collectInjectableRefsFromImages(vctx.doc)
	if len(vctx.doc.VariableIDs) == 0 && len(roleRefs) == 0 && len(imageRefs) == 0 {
		return nil
	}

	injectableMap := buildInjectableMap(vctx.accessibleInjectableList)
	allRefs := make([]string, 0, len(vctx.doc.VariableIDs)+len(roleRefs)+len(imageRefs))
	allRefs = append(allRefs, vctx.doc.VariableIDs...)
	allRefs = append(allRefs, roleRefs...)
	allRefs = append(allRefs, imageRefs...)
	injectables := make([]*entity.TemplateVersionInjectable, 0, len(allRefs))
	seen := make(map[string]struct{}, len(allRefs))

	for _, varID := range allRefs {
		if _, exists := seen[varID]; exists {
			continue
		}
		seen[varID] = struct{}{}

		if strings.HasPrefix(varID, portabledoc.RoleVariablePrefix) {
			continue
		}

		inj, exists := injectableMap[varID]
		if !exists {
			continue
		}

		injectables = append(injectables, buildInjectable(vctx.versionID, varID, inj))
	}
	return injectables
}

func collectInjectableRefsFromSignerRoles(roles []portabledoc.SignerRole) []string {
	if len(roles) == 0 {
		return nil
	}

	refs := make([]string, 0, len(roles)*2)
	for _, role := range roles {
		if role.Name.IsInjectable() {
			refs = append(refs, role.Name.InjectableRefs()...)
		}
		if role.Email.IsInjectable() {
			refs = append(refs, role.Email.InjectableRefs()...)
		}
	}
	return refs
}

func collectInjectableRefsFromImages(doc *portabledoc.Document) []string {
	if doc == nil {
		return nil
	}

	refs := make([]string, 0)
	for node := range doc.AllNodes() {
		if node.Type != portabledoc.NodeTypeImage && node.Type != portabledoc.NodeTypeCustomImage {
			continue
		}

		injectableID, _ := node.Attrs["injectableId"].(string)
		if injectableID != "" {
			refs = append(refs, injectableID)
		}
	}

	if doc.Header != nil && doc.Header.ImageInjectableID != nil && *doc.Header.ImageInjectableID != "" {
		refs = append(refs, *doc.Header.ImageInjectableID)
	}

	return refs
}

// buildInjectableMap creates a key -> definition lookup map.
func buildInjectableMap(list []*entity.InjectableDefinition) map[string]*entity.InjectableDefinition {
	m := make(map[string]*entity.InjectableDefinition, len(list))
	for _, inj := range list {
		m[inj.Key] = inj
	}
	return m
}

// buildInjectable creates the appropriate injectable entity based on source type.
// For EXTERNAL injectables or system registry injectables (non-UUID ID), it uses
// the system injectable key. For DB-backed INTERNAL injectables (UUID ID), it
// references the injectable definition directly.
func buildInjectable(versionID, varID string, def *entity.InjectableDefinition) *entity.TemplateVersionInjectable {
	if def.SourceType == entity.InjectableSourceTypeExternal {
		return entity.NewTemplateVersionInjectableFromSystemKey(versionID, varID)
	}
	// System registry injectables have non-UUID IDs (their code/key).
	// Only DB-backed injectables have valid UUID IDs for the FK reference.
	if _, err := uuid.Parse(def.ID); err != nil {
		return entity.NewTemplateVersionInjectableFromSystemKey(versionID, varID)
	}
	return entity.NewTemplateVersionInjectable(versionID, def.ID, false, nil)
}
