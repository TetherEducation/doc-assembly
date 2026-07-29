package dto

import (
	"encoding/json"
	"time"

	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
)

// CreateReadOnlyViewLinkResponse is returned when creating a public read-only view link.
type CreateReadOnlyViewLinkResponse struct {
	URL       string    `json:"url"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// NewCreateReadOnlyViewLinkResponse maps a use case result to an HTTP DTO.
func NewCreateReadOnlyViewLinkResponse(result *documentuc.CreateReadOnlyViewLinkResult) CreateReadOnlyViewLinkResponse {
	if result == nil {
		return CreateReadOnlyViewLinkResponse{}
	}
	return CreateReadOnlyViewLinkResponse{
		URL:       result.URL,
		Token:     result.Token,
		ExpiresAt: result.ExpiresAt,
	}
}

// SigningStateRequest asks for the real signing state of a batch of documents.
type SigningStateRequest struct {
	DocumentIDs []string `json:"documentIds" binding:"required"`
}

// SigningStateRecipientResponse reports one recipient's progress on a document.
type SigningStateRecipientResponse struct {
	Name        string `json:"name"`
	Email       string `json:"email"`
	RoleName    string `json:"roleName,omitempty"`
	SignerOrder int    `json:"signerOrder"`
	// Status is a projection: only SIGNED is authoritative. Intermediate provider
	// states are attempt-owned and may lag, so do not branch on SENT/DELIVERED.
	Status   string     `json:"status"`
	Signed   bool       `json:"signed"`
	SignedAt *time.Time `json:"signedAt,omitempty"`
}

// SigningStateDocumentResponse reports whether one document was actually signed.
type SigningStateDocumentResponse struct {
	DocumentID          string                          `json:"documentId"`
	ExternalReferenceID *string                         `json:"externalReferenceId,omitempty"`
	Status              string                          `json:"status"`
	Signed              bool                            `json:"signed"`
	Expired             bool                            `json:"expired"`
	ExpiresAt           *time.Time                      `json:"expiresAt,omitempty"`
	Recipients          []SigningStateRecipientResponse `json:"recipients"`
}

// SigningStateResponse is the batch answer for the requested document IDs.
type SigningStateResponse struct {
	Documents []SigningStateDocumentResponse `json:"documents"`
	// Unavailable merges "does not exist" with "belongs to another workspace" so
	// the endpoint cannot be used to probe for documents outside the caller's
	// workspace. Callers should treat these as unknown, not as unsigned.
	Unavailable []string `json:"unavailable"`
}

// NewSigningStateResponse maps a use case result to an HTTP DTO.
func NewSigningStateResponse(result *documentuc.SigningStateResult) SigningStateResponse {
	if result == nil {
		return SigningStateResponse{
			Documents:   []SigningStateDocumentResponse{},
			Unavailable: []string{},
		}
	}

	documents := make([]SigningStateDocumentResponse, 0, len(result.Documents))
	for _, doc := range result.Documents {
		recipients := make([]SigningStateRecipientResponse, 0, len(doc.Recipients))
		for _, recipient := range doc.Recipients {
			recipients = append(recipients, SigningStateRecipientResponse{
				Name:        recipient.Name,
				Email:       recipient.Email,
				RoleName:    recipient.RoleName,
				SignerOrder: recipient.SignerOrder,
				Status:      string(recipient.Status),
				Signed:      recipient.Signed,
				SignedAt:    recipient.SignedAt,
			})
		}
		documents = append(documents, SigningStateDocumentResponse{
			DocumentID:          doc.DocumentID,
			ExternalReferenceID: doc.ExternalReferenceID,
			Status:              string(doc.Status),
			Signed:              doc.Signed,
			Expired:             doc.Expired,
			ExpiresAt:           doc.ExpiresAt,
			Recipients:          recipients,
		})
	}

	unavailable := result.Unavailable
	if unavailable == nil {
		unavailable = []string{}
	}

	return SigningStateResponse{
		Documents:   documents,
		Unavailable: unavailable,
	}
}

// ReadOnlyViewResponse describes a public read-only document view.
type ReadOnlyViewResponse struct {
	Mode           documentuc.ReadOnlyViewMode `json:"mode"`
	DocumentID     string                      `json:"documentId"`
	DocumentTitle  string                      `json:"documentTitle"`
	DocumentStatus string                      `json:"documentStatus"`
	ExpiresAt      time.Time                   `json:"expiresAt"`
	Content        json.RawMessage             `json:"content,omitempty" swaggertype:"object"`
	PDFURL         *string                     `json:"pdfUrl,omitempty"`
	Reason         *string                     `json:"reason,omitempty"`
}

// NewReadOnlyViewResponse maps a use case response to an HTTP DTO.
func NewReadOnlyViewResponse(resp *documentuc.ReadOnlyViewResponse) ReadOnlyViewResponse {
	if resp == nil {
		return ReadOnlyViewResponse{}
	}
	return ReadOnlyViewResponse{
		Mode:           resp.Mode,
		DocumentID:     resp.DocumentID,
		DocumentTitle:  resp.DocumentTitle,
		DocumentStatus: string(resp.DocumentStatus),
		ExpiresAt:      resp.ExpiresAt,
		Content:        resp.Content,
		PDFURL:         resp.PDFURL,
		Reason:         resp.Reason,
	}
}
