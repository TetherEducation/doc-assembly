package entity

import "time"

// Token type constants distinguish the entry point for public signing flows.
const (
	// TokenTypePreSigning is used for documents with interactive fields (Path B).
	// The signer first fills a form, then transitions to embedded signing.
	TokenTypePreSigning = "PRE_SIGNING"

	// TokenTypeSigning is used for documents without interactive fields (Path A).
	// The signer sees a PDF preview, then proceeds to embedded signing.
	TokenTypeSigning = "SIGNING"

	// TokenTypeViewOnly is used for expiring read-only contract views and never
	// grants signing or pre-signing permissions.
	TokenTypeViewOnly = "VIEW_ONLY"
)

// DocumentAccessToken represents a time-limited access token for public signing access.
type DocumentAccessToken struct {
	ID          string     `json:"id"`
	DocumentID  string     `json:"documentId"`
	RecipientID string     `json:"recipientId"`
	AttemptID   *string    `json:"attemptId,omitempty"`
	Token       string     `json:"token"`
	TokenType   string     `json:"tokenType"`
	ExpiresAt   time.Time  `json:"expiresAt"`
	UsedAt      *time.Time `json:"usedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

// IsPreSigning returns true if this is a pre-signing token (Path B, interactive fields).
func (t *DocumentAccessToken) IsPreSigning() bool {
	return t.TokenType == TokenTypePreSigning
}

// IsSigning returns true if this is a signing token (Path A, direct signing).
func (t *DocumentAccessToken) IsSigning() bool {
	return t.TokenType == TokenTypeSigning
}

// IsViewOnly returns true if this is a read-only view token.
func (t *DocumentAccessToken) IsViewOnly() bool {
	return t.TokenType == TokenTypeViewOnly
}

// IsUsed returns true if the token has already been consumed.
func (t *DocumentAccessToken) IsUsed() bool {
	return t.UsedAt != nil
}
