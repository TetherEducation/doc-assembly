package contentvalidator

import (
	"fmt"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
)

// validateSignatures validates all signature blocks in the document.
func (s *Service) validateSignatures(vctx *validationContext) {
	doc := vctx.doc

	// Track assigned role IDs across all signature blocks
	assignedRoleIDs := make(portabledoc.Set[string])

	// Collect and validate all signature nodes
	signatureCount := 0
	for i, node := range doc.NodesOfType(portabledoc.NodeTypeSignature) {
		path := fmt.Sprintf("content.signature[%d]", i)
		validateSignatureNode(vctx, node, path, assignedRoleIDs)
		signatureCount++
	}

	// Error if no signatures defined - at least one is required for signing
	if signatureCount == 0 {
		vctx.addError(ErrCodeNoSignatures, "content", "At least one signature block is required")
	}

	// Validate that all roles have at least one signature
	s.validateRolesHaveSignatures(vctx, assignedRoleIDs)
}

// validateRolesHaveSignatures checks that each signer role has at least one signature assigned.
func (s *Service) validateRolesHaveSignatures(vctx *validationContext, assignedRoleIDs portabledoc.Set[string]) {
	for _, role := range vctx.doc.SignerRoles {
		if !assignedRoleIDs.Contains(role.ID) {
			vctx.addErrorf(ErrCodeOrphanedRole, "signerRoles",
				"Role '%s' has no signature assigned", role.Label)
		}
	}
}

// validateSignatureNode validates a single signature block node.
func validateSignatureNode(
	vctx *validationContext,
	node portabledoc.Node,
	path string,
	assignedRoleIDs portabledoc.Set[string],
) {
	attrs, err := portabledoc.ParseSignatureAttrs(node.Attrs)
	if err != nil {
		vctx.addErrorf(ErrCodeInvalidSignatureCount, path+".attrs",
			"Invalid signature attributes: %s", err.Error())
		return
	}

	// Validate count
	if attrs.Count < portabledoc.MinSignatureCount || attrs.Count > portabledoc.MaxSignatureCount {
		vctx.addErrorf(ErrCodeInvalidSignatureCount, path+".attrs.count",
			"Signature count must be between %d and %d, got: %d",
			portabledoc.MinSignatureCount, portabledoc.MaxSignatureCount, attrs.Count)
	}

	// Validate layout for count
	if validLayouts, ok := portabledoc.ValidLayoutsForCount[attrs.Count]; ok {
		if !validLayouts.Contains(attrs.Layout) {
			vctx.addErrorf(ErrCodeInvalidSignatureLayout, path+".attrs.layout",
				"Invalid layout '%s' for count %d", attrs.Layout, attrs.Count)
		}
	}

	// Validate line width
	if attrs.LineWidth != "" && !portabledoc.ValidLineWidths.Contains(attrs.LineWidth) {
		vctx.addErrorf(ErrCodeInvalidLineWidth, path+".attrs.lineWidth",
			"Invalid line width: %s. Must be 'sm', 'md', or 'lg'", attrs.LineWidth)
	}

	// Validate each signature item
	for i, sig := range attrs.Signatures {
		sigPath := fmt.Sprintf("%s.attrs.signatures[%d]", path, i)
		validateSignatureItem(vctx, sig, sigPath, assignedRoleIDs)
	}

	// Warn if signatures count doesn't match declared count
	if len(attrs.Signatures) != attrs.Count {
		vctx.addWarningf(WarnCodeNoSignatures, path,
			"Signature count (%d) doesn't match actual signatures (%d)",
			attrs.Count, len(attrs.Signatures))
	}
}

// validateSignatureItem validates a single signature item.
func validateSignatureItem(
	vctx *validationContext,
	sig portabledoc.SignatureItem,
	path string,
	assignedRoleIDs portabledoc.Set[string],
) {
	if !sig.HasRole() {
		label := sig.Label
		if label == "" {
			label = "(unnamed)"
		}
		vctx.addErrorf(ErrCodeMissingSignatureRole, path+".roleId",
			"Signature '%s' must have a role assigned", label)
		return
	}

	roleID := sig.GetRoleID()

	// Check if role exists in document
	if !vctx.roleIDSet.Contains(roleID) {
		vctx.addErrorf(ErrCodeInvalidSignatureRoleRef, path+".roleId",
			"Signature references unknown role: %s", roleID)
		return
	}

	// Check for duplicate role assignment
	if assignedRoleIDs.Contains(roleID) {
		vctx.addErrorf(ErrCodeDuplicateSignatureRole, path+".roleId",
			"Role '%s' is already assigned to another signature", roleID)
		return
	}

	assignedRoleIDs.Add(roleID)
}
