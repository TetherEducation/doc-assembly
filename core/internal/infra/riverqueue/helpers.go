package riverqueue

import (
	"context"
	"encoding/json"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

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

func buildSignerRoleValues(recipients []*entity.DocumentRecipient, dbSignerRoles []*entity.TemplateVersionSignerRole, portableDocRoles []portabledoc.SignerRole) map[string]port.SignerRoleValue {
	dbRoleToAnchor := make(map[string]string, len(dbSignerRoles))
	for _, role := range dbSignerRoles {
		dbRoleToAnchor[role.ID] = role.AnchorString
	}
	anchorToPortableID := make(map[string]string, len(portableDocRoles))
	for _, role := range portableDocRoles {
		anchorToPortableID[portabledoc.GenerateAnchorString(role.Label)] = role.ID
	}
	values := make(map[string]port.SignerRoleValue, len(recipients))
	for _, r := range recipients {
		if portableID := anchorToPortableID[dbRoleToAnchor[r.TemplateVersionRoleID]]; portableID != "" {
			values[portableID] = port.SignerRoleValue{Name: r.Name, Email: r.Email}
		}
	}
	return values
}

func buildAnchorToRoleIDMap(signerRoles []*entity.TemplateVersionSignerRole) map[string]string {
	m := make(map[string]string, len(signerRoles))
	for _, role := range signerRoles {
		if role.AnchorString != "" {
			m[role.AnchorString] = role.ID
		}
	}
	return m
}

func mapSignatureFieldPositions(fields []port.SignatureField, dbSignerRoles []*entity.TemplateVersionSignerRole, portableDocRoles []portabledoc.SignerRole) []port.SignatureFieldPosition {
	if len(fields) == 0 {
		return nil
	}
	anchorToDBRoleID := buildAnchorToRoleIDMap(dbSignerRoles)
	portableIDToAnchor := make(map[string]string, len(portableDocRoles))
	for _, role := range portableDocRoles {
		portableIDToAnchor[role.ID] = portabledoc.GenerateAnchorString(role.Label)
	}
	positions := make([]port.SignatureFieldPosition, 0, len(fields))
	for _, f := range fields {
		anchor := f.AnchorString
		if a := portableIDToAnchor[f.RoleID]; a != "" {
			anchor = a
		}
		dbRoleID := anchorToDBRoleID[anchor]
		if dbRoleID == "" {
			continue
		}
		posX, posY := port.ConvertFieldToDocumensoPosition(f)
		positions = append(positions, port.SignatureFieldPosition{RoleID: dbRoleID, Page: f.Page, PositionX: posX, PositionY: posY, Width: f.Width, Height: f.Height})
	}
	return positions
}

func buildDefaultSignatureFieldPositions(recipients []*entity.DocumentRecipient) []port.SignatureFieldPosition {
	fields := make([]port.SignatureFieldPosition, 0, len(recipients))
	for i, r := range recipients {
		fields = append(fields, port.SignatureFieldPosition{RoleID: r.TemplateVersionRoleID, Page: 1, PositionX: 10, PositionY: float64(70 + i*12), Width: 30, Height: 5})
	}
	return fields
}

func documentTitle(doc *entity.Document) string {
	if doc != nil && doc.Title != nil {
		return *doc.Title
	}
	if doc != nil {
		return doc.ID
	}
	return "document"
}
