package document

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// loadFieldResponseMap loads saved field responses as a map for PDF rendering.
func loadFieldResponseMap(ctx context.Context, repo port.DocumentFieldResponseRepository, documentID string) map[string]json.RawMessage {
	responses, err := repo.FindByDocumentID(ctx, documentID)
	if err != nil || len(responses) == 0 {
		return nil
	}
	m := make(map[string]json.RawMessage, len(responses))
	for _, resp := range responses {
		m[resp.FieldID] = resp.Response
	}
	return m
}

// buildSignerRoleValues maps recipient data to portable doc role IDs for PDF rendering.
func buildSignerRoleValues(
	recipients []*entity.DocumentRecipient,
	dbSignerRoles []*entity.TemplateVersionSignerRole,
	portableDocRoles []portabledoc.SignerRole,
) map[string]port.SignerRoleValue {
	dbRoleToAnchor := make(map[string]string, len(dbSignerRoles))
	for _, role := range dbSignerRoles {
		dbRoleToAnchor[role.ID] = role.AnchorString
	}

	anchorToPortableID := make(map[string]string, len(portableDocRoles))
	for _, role := range portableDocRoles {
		anchor := portabledoc.GenerateAnchorString(role.Label)
		anchorToPortableID[anchor] = role.ID
	}

	values := make(map[string]port.SignerRoleValue, len(recipients))
	for _, r := range recipients {
		anchor := dbRoleToAnchor[r.TemplateVersionRoleID]
		portableDocRoleID := anchorToPortableID[anchor]
		if portableDocRoleID != "" {
			values[portableDocRoleID] = port.SignerRoleValue{
				Name:  r.Name,
				Email: r.Email,
			}
		}
	}
	return values
}

// documentTitle returns the document title, falling back to the document ID.
func documentTitle(doc *entity.Document) string {
	if doc.Title != nil {
		return *doc.Title
	}
	return doc.ID
}

func signedDocumentFilename(doc *entity.Document) string {
	if doc != nil && doc.Title != nil {
		return fmt.Sprintf("%s-signed.pdf", *doc.Title)
	}
	if doc != nil {
		return fmt.Sprintf("document-%s-signed.pdf", doc.ID)
	}
	return "document-signed.pdf"
}
