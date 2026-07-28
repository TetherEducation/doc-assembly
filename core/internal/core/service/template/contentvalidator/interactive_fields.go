package contentvalidator

import (
	"fmt"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
)

// validateInteractiveFields validates all interactive field nodes in the document.
func (s *Service) validateInteractiveFields(vctx *validationContext) {
	doc := vctx.doc

	// Track seen field IDs for duplicate detection
	seenFieldIDs := make(portabledoc.Set[string])

	fieldCount := 0
	for i, node := range doc.NodesOfType(portabledoc.NodeTypeInteractiveField) {
		path := fmt.Sprintf("content.interactiveField[%d]", i)
		validateInteractiveFieldNode(vctx, node, path, seenFieldIDs)
		fieldCount++
	}

	// Cross-validate: if interactive fields exist, warn if not exactly 1 unsigned role
	if fieldCount > 0 {
		validateUnsignedRoleForInteractiveFields(vctx)
	}
}

// validateInteractiveFieldNode validates a single interactive field node.
func validateInteractiveFieldNode(
	vctx *validationContext,
	node portabledoc.Node,
	path string,
	seenFieldIDs portabledoc.Set[string],
) {
	attrs, err := portabledoc.ParseInteractiveFieldAttrs(node.Attrs)
	if err != nil {
		vctx.addErrorf(ErrCodeInvalidInteractiveAttrs, path+".attrs",
			"Invalid interactive field attributes: %s", err.Error())
		return
	}

	// Validate unique field ID
	if attrs.ID != "" {
		if seenFieldIDs.Contains(attrs.ID) {
			vctx.addErrorf(ErrCodeDuplicateInteractiveField, path+".attrs.id",
				"Duplicate interactive field ID: %s", attrs.ID)
		} else {
			seenFieldIDs.Add(attrs.ID)
		}
	}

	// Validate fieldType
	if !portabledoc.ValidInteractiveFieldTypes.Contains(attrs.FieldType) {
		vctx.addErrorf(ErrCodeInvalidInteractiveType, path+".attrs.fieldType",
			"Invalid interactive field type '%s'. Must be 'checkbox', 'radio', or 'text'", attrs.FieldType)
	}

	// Validate label
	if attrs.Label == "" {
		vctx.addError(ErrCodeEmptyInteractiveLabel, path+".attrs.label",
			"Interactive field label is required")
	}

	// Validate roleId
	validateInteractiveFieldRole(vctx, attrs, path)

	// Validate type-specific rules
	switch attrs.FieldType {
	case portabledoc.InteractiveFieldTypeCheckbox:
		validateCheckboxOptions(vctx, attrs.Options, path, 1)
	case portabledoc.InteractiveFieldTypeRadio:
		validateCheckboxOptions(vctx, attrs.Options, path, 2)
	case portabledoc.InteractiveFieldTypeText:
		validateTextFieldMaxLength(vctx, attrs.MaxLength, path)
	}
}

// validateInteractiveFieldRole validates the roleId of an interactive field.
func validateInteractiveFieldRole(
	vctx *validationContext,
	attrs *portabledoc.InteractiveFieldAttrs,
	path string,
) {
	if attrs.RoleID == "" {
		vctx.addError(ErrCodeEmptyInteractiveRoleID, path+".attrs.roleId",
			"Interactive field must have a role assigned")
		return
	}

	if !vctx.roleIDSet.Contains(attrs.RoleID) {
		vctx.addErrorf(ErrCodeInvalidInteractiveRoleRef, path+".attrs.roleId",
			"Interactive field references unknown role: %s", attrs.RoleID)
	}
}

// validateCheckboxOptions validates options for checkbox or radio fields.
// minOptions is 1 for checkbox, 2 for radio.
func validateCheckboxOptions(
	vctx *validationContext,
	options []portabledoc.InteractiveOption,
	path string,
	minOptions int,
) {
	fieldType := "checkbox"
	if minOptions == 2 {
		fieldType = "radio"
	}

	if len(options) < minOptions {
		vctx.addErrorf(ErrCodeInsufficientOptions, path+".attrs.options",
			"%s field requires at least %d option(s), got: %d", fieldType, minOptions, len(options))
		return
	}

	seenOptionIDs := make(portabledoc.Set[string])
	for i, opt := range options {
		optPath := fmt.Sprintf("%s.attrs.options[%d]", path, i)

		if opt.ID == "" {
			vctx.addError(ErrCodeEmptyOptionID, optPath+".id",
				"Option ID is required")
		} else if seenOptionIDs.Contains(opt.ID) {
			vctx.addErrorf(ErrCodeDuplicateOptionID, optPath+".id",
				"Duplicate option ID: %s", opt.ID)
		} else {
			seenOptionIDs.Add(opt.ID)
		}

		if opt.Label == "" {
			vctx.addError(ErrCodeEmptyOptionLabel, optPath+".label",
				"Option label is required")
		}
	}
}

// validateTextFieldMaxLength validates the maxLength for text fields.
func validateTextFieldMaxLength(
	vctx *validationContext,
	maxLength int,
	path string,
) {
	if maxLength < 0 {
		vctx.addErrorf(ErrCodeInvalidMaxLength, path+".attrs.maxLength",
			"Text field maxLength must be >= 0, got: %d", maxLength)
	}
}

// validateUnsignedRoleForInteractiveFields checks that exactly 1 signer role
// has no image signature when interactive fields are present.
// This is a WARNING, not a hard error.
func validateUnsignedRoleForInteractiveFields(vctx *validationContext) {
	doc := vctx.doc

	// Collect all signature items across all signature nodes
	unsignedRoleCount := 0
	for _, node := range doc.NodesOfType(portabledoc.NodeTypeSignature) {
		attrs, err := portabledoc.ParseSignatureAttrs(node.Attrs)
		if err != nil {
			continue
		}

		for _, sig := range attrs.Signatures {
			if sig.HasRole() && !sig.IsSigned() {
				unsignedRoleCount++
			}
		}
	}

	if unsignedRoleCount != 1 {
		vctx.addWarningf(WarnCodeInteractiveFieldsNoUnsignedRole, "content",
			"Interactive fields are present but found %d unsigned signer role(s) (expected exactly 1). "+
				"The signing flow may not work correctly", unsignedRoleCount)
	}
}
