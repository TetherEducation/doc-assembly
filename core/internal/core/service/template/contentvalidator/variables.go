package contentvalidator

import (
	"fmt"
	"strings"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
)

// validateVariables validates all variables and injectors in the document.
func (s *Service) validateVariables(vctx *validationContext) {
	// Validate that declared variables are accessible
	validateDeclaredVariables(vctx)

	// Validate injector nodes in content
	validateInjectorNodes(vctx)

	// Validate image injector bindings in content and header
	validateImageBindings(vctx)
}

// validateDeclaredVariables validates that all declared variableIds are accessible.
func validateDeclaredVariables(vctx *validationContext) {
	if vctx.accessibleInjectables.Len() == 0 {
		// Skip if we couldn't load accessible injectables
		return
	}

	for i, varID := range vctx.doc.VariableIDs {
		path := fmt.Sprintf("variableIds[%d]", i)

		// Skip role variables (they're generated, not from backend)
		if strings.HasPrefix(varID, portabledoc.RoleVariablePrefix) {
			continue
		}

		// Check if variable is accessible to workspace
		if !vctx.accessibleInjectables.Contains(varID) {
			vctx.addErrorf(ErrCodeInaccessibleVariable, path,
				"Variable '%s' is not accessible to this workspace", varID)
		}
	}
}

// validateInjectorNodes validates all injector nodes in the document content.
func validateInjectorNodes(vctx *validationContext) {
	doc := vctx.doc

	// Collect all injector nodes
	for i, node := range doc.NodesOfType(portabledoc.NodeTypeInjector) {
		path := fmt.Sprintf("content.injector[%d]", i)
		validateInjectorNode(vctx, node, path)
	}
}

func validateImageBindings(vctx *validationContext) {
	doc := vctx.doc

	for i, node := range doc.NodesOfType(portabledoc.NodeTypeImage) {
		validateImageBinding(vctx, node.Attrs, fmt.Sprintf("content.image[%d].attrs.injectableId", i))
	}

	for i, node := range doc.NodesOfType(portabledoc.NodeTypeCustomImage) {
		validateImageBinding(vctx, node.Attrs, fmt.Sprintf("content.customImage[%d].attrs.injectableId", i))
	}

	if doc.Header != nil && doc.Header.ImageInjectableID != nil && *doc.Header.ImageInjectableID != "" {
		validateVariableReference(vctx, *doc.Header.ImageInjectableID, "header.imageInjectableId", true)
	}
}

func validateImageBinding(vctx *validationContext, attrs map[string]any, path string) {
	if attrs == nil {
		return
	}

	injectableID, _ := attrs["injectableId"].(string)
	if injectableID == "" {
		return
	}

	validateVariableReference(vctx, injectableID, path, true)
}

func validateVariableReference(vctx *validationContext, variableID, path string, requireAccess bool) {
	if !vctx.variableSet.Contains(variableID) {
		vctx.addErrorf(ErrCodeUnknownVariable, path,
			"Variable '%s' not found in document variableIds", variableID)
		return
	}

	if requireAccess && vctx.accessibleInjectables.Len() > 0 && !vctx.accessibleInjectables.Contains(variableID) {
		vctx.addErrorf(ErrCodeInaccessibleVariable, path,
			"Variable '%s' is not accessible to this workspace", variableID)
	}
}

// validateInjectorNode validates a single injector node.
func validateInjectorNode(vctx *validationContext, node portabledoc.Node, path string) {
	attrs, err := portabledoc.ParseInjectorAttrs(node.Attrs)
	if err != nil {
		vctx.addErrorf(ErrCodeInvalidInjectorType, path+".attrs",
			"Invalid injector attributes: %s", err.Error())
		return
	}

	// Validate injector type
	if !portabledoc.ValidInjectorTypes.Contains(attrs.Type) {
		vctx.addErrorf(ErrCodeInvalidInjectorType, path+".attrs.type",
			"Invalid injector type: %s", attrs.Type)
	}

	// Validate variableId
	if attrs.VariableID == "" {
		vctx.addError(ErrCodeUnknownVariable, path+".attrs.variableId",
			"Injector variableId is required")
		return
	}

	// Handle role variables differently
	if attrs.IsRoleVar() {
		validateRoleVariable(vctx, attrs, path)
		return
	}

	// Regular variable: must be in variableIds and in variableSet
	validateVariableReference(vctx, attrs.VariableID, path+".attrs.variableId", false)
}

// validateRoleVariable validates a role variable (ROLE.{label}.{property}).
func validateRoleVariable(vctx *validationContext, attrs *portabledoc.InjectorAttrs, path string) {
	// Type must be ROLE_TEXT
	if attrs.Type != portabledoc.InjectorTypeRoleText {
		vctx.addErrorf(ErrCodeInvalidRoleVariable, path+".attrs.type",
			"Role variable must have type ROLE_TEXT, got: %s", attrs.Type)
	}

	// PropertyKey must be valid
	if attrs.PropertyKey != nil && !portabledoc.ValidRoleProperties.Contains(*attrs.PropertyKey) {
		vctx.addErrorf(ErrCodeInvalidRoleProperty, path+".attrs.propertyKey",
			"Invalid role property: %s. Must be 'name' or 'email'", *attrs.PropertyKey)
	}

	// RoleLabel must reference an existing role
	if attrs.RoleLabel != nil && *attrs.RoleLabel != "" {
		found := false
		for _, role := range vctx.doc.SignerRoles {
			if role.Label == *attrs.RoleLabel {
				found = true
				break
			}
		}
		if !found {
			vctx.addErrorf(ErrCodeInvalidRoleVariable, path+".attrs.roleLabel",
				"Role variable references unknown role: %s", *attrs.RoleLabel)
		}
	}

	// VariableId format validation (ROLE.{label}.{property})
	if !strings.HasPrefix(attrs.VariableID, portabledoc.RoleVariablePrefix) {
		vctx.addErrorf(ErrCodeInvalidRoleVariable, path+".attrs.variableId",
			"Role variable ID must start with '%s', got: %s", portabledoc.RoleVariablePrefix, attrs.VariableID)
	}
}
