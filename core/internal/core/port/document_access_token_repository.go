package port

import (
	"context"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// DocumentAccessTokenRepository defines the interface for document access token data access.
type DocumentAccessTokenRepository interface {
	// Create creates a new document access token.
	Create(ctx context.Context, token *entity.DocumentAccessToken) error

	// FindByToken finds an access token by its token string.
	FindByToken(ctx context.Context, token string) (*entity.DocumentAccessToken, error)

	// FindActiveByRecipientAndType finds an active (unused, non-expired) token
	// for a recipient with a specific type (PRE_SIGNING or SIGNING).
	FindActiveByRecipientAndType(ctx context.Context, recipientID, tokenType string) (*entity.DocumentAccessToken, error)

	// FindActiveByDocumentAndRecipientAndType finds an active (unused,
	// non-expired) token for a document+recipient and type.
	FindActiveByDocumentAndRecipientAndType(ctx context.Context, documentID, recipientID, tokenType string) (*entity.DocumentAccessToken, error)

	// MarkAsUsed marks an access token as used by setting its used_at timestamp.
	MarkAsUsed(ctx context.Context, tokenID string) error

	// InvalidateByDocumentID invalidates all access tokens for a document
	// by setting their used_at timestamp (if not already used).
	InvalidateByDocumentID(ctx context.Context, documentID string) error

	// CountRecentByDocumentAndRecipient counts tokens created for a document+recipient since a given time.
	CountRecentByDocumentAndRecipient(ctx context.Context, documentID, recipientID string, since time.Time) (int, error)
}
