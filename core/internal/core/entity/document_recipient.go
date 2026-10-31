package entity

import (
	"time"
)

// DocumentRecipient represents a signer/recipient for a document.
// It tracks individual recipient status through the signing workflow.
type DocumentRecipient struct {
	ID                    string          `json:"id"`
	DocumentID            string          `json:"documentId"`
	TemplateVersionRoleID string          `json:"templateVersionRoleId"`
	Name                  string          `json:"name"`
	Email                 string          `json:"email"`
	SignerRecipientID     *string         `json:"signerRecipientId,omitempty"`
	SigningURL            *string         `json:"signingUrl,omitempty"`
	Status                RecipientStatus `json:"status"`
	SignedAt              *time.Time      `json:"signedAt,omitempty"`
	CreatedAt             time.Time       `json:"createdAt"`
	UpdatedAt             *time.Time      `json:"updatedAt,omitempty"`
}

// NewDocumentRecipient creates a new document recipient in PENDING status.
func NewDocumentRecipient(documentID, roleID, name, email string) *DocumentRecipient {
	return &DocumentRecipient{
		DocumentID:            documentID,
		TemplateVersionRoleID: roleID,
		Name:                  name,
		Email:                 email,
		Status:                RecipientStatusPending,
		CreatedAt:             time.Now().UTC(),
	}
}

// SetSignerRecipientID sets the recipient ID from the signing provider.
func (r *DocumentRecipient) SetSignerRecipientID(id string) {
	r.SignerRecipientID = &id
	r.touch()
}

// SetSigningURL sets the URL where this recipient can sign the document.
func (r *DocumentRecipient) SetSigningURL(url string) {
	r.SigningURL = &url
	r.touch()
}

// MarkAsSent transitions the recipient to SENT status (email sent).
func (r *DocumentRecipient) MarkAsSent() error {
	if r.Status != RecipientStatusPending {
		return ErrInvalidRecipientStatusTransition
	}
	r.Status = RecipientStatusSent
	r.touch()
	return nil
}

// MarkAsDelivered transitions the recipient to DELIVERED status (viewed document).
func (r *DocumentRecipient) MarkAsDelivered() error {
	if r.Status != RecipientStatusPending && r.Status != RecipientStatusSent {
		return ErrInvalidRecipientStatusTransition
	}
	r.Status = RecipientStatusDelivered
	r.touch()
	return nil
}

// MarkAsSigned transitions the recipient to SIGNED status (completed signing).
func (r *DocumentRecipient) MarkAsSigned() error {
	return r.MarkAsSignedAt(time.Now().UTC())
}

// MarkAsSignedAt marks the recipient as signed at the given time, normally the
// signing provider's own time for that signer.
func (r *DocumentRecipient) MarkAsSignedAt(signedAt time.Time) error {
	if r.Status == RecipientStatusSigned || r.Status == RecipientStatusDeclined {
		return ErrInvalidRecipientStatusTransition
	}
	now := time.Now().UTC()
	at := signedAt.UTC()
	r.Status = RecipientStatusSigned
	r.SignedAt = &at
	r.UpdatedAt = &now
	return nil
}

// MarkAsDeclined transitions the recipient to DECLINED status (rejected signing).
func (r *DocumentRecipient) MarkAsDeclined() error {
	if r.Status == RecipientStatusSigned || r.Status == RecipientStatusDeclined {
		return ErrInvalidRecipientStatusTransition
	}
	r.Status = RecipientStatusDeclined
	r.touch()
	return nil
}

// UpdateStatus updates the recipient status from provider status.
func (r *DocumentRecipient) UpdateStatus(newStatus RecipientStatus) error {
	if !newStatus.IsValid() {
		return ErrInvalidRecipientStatus
	}

	// Track signed timestamp
	if newStatus == RecipientStatusSigned && r.SignedAt == nil {
		now := time.Now().UTC()
		r.SignedAt = &now
	}

	r.Status = newStatus
	r.touch()
	return nil
}

// IsPending returns true if the recipient is in pending status.
func (r *DocumentRecipient) IsPending() bool {
	return r.Status == RecipientStatusPending
}

// IsSent returns true if the email has been sent to the recipient.
func (r *DocumentRecipient) IsSent() bool {
	return r.Status == RecipientStatusSent
}

// IsDelivered returns true if the recipient has viewed the document.
func (r *DocumentRecipient) IsDelivered() bool {
	return r.Status == RecipientStatusDelivered
}

// IsSigned returns true if the recipient has signed.
func (r *DocumentRecipient) IsSigned() bool {
	return r.Status == RecipientStatusSigned
}

// IsDeclined returns true if the recipient has declined.
func (r *DocumentRecipient) IsDeclined() bool {
	return r.Status == RecipientStatusDeclined
}

// IsTerminal returns true if the recipient is in a terminal state.
func (r *DocumentRecipient) IsTerminal() bool {
	return r.Status == RecipientStatusSigned || r.Status == RecipientStatusDeclined
}

// HasSignerInfo returns true if the recipient has been registered with the signing provider.
func (r *DocumentRecipient) HasSignerInfo() bool {
	return r.SignerRecipientID != nil
}

// Validate checks if the document recipient data is valid.
func (r *DocumentRecipient) Validate() error {
	if r.DocumentID == "" {
		return ErrRequiredField
	}
	if r.TemplateVersionRoleID == "" {
		return ErrRequiredField
	}
	if r.Name == "" {
		return ErrRequiredField
	}
	if len(r.Name) > 255 {
		return ErrFieldTooLong
	}
	if r.Email == "" {
		return ErrRequiredField
	}
	if len(r.Email) > 255 {
		return ErrFieldTooLong
	}
	if !r.Status.IsValid() {
		return ErrInvalidRecipientStatus
	}
	return nil
}

// touch updates the UpdatedAt timestamp.
func (r *DocumentRecipient) touch() {
	now := time.Now().UTC()
	r.UpdatedAt = &now
}

// DocumentRecipientWithRole represents a document recipient with its associated role.
type DocumentRecipientWithRole struct {
	DocumentRecipient
	RoleName    string `json:"roleName"`
	SignerOrder int    `json:"signerOrder"`
}
