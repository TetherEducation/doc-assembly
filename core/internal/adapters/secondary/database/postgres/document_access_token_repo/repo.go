package documentaccesstokenrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new document access token repository.
func New(pool *pgxpool.Pool) port.DocumentAccessTokenRepository {
	return &Repository{pool: pool}
}

// Repository implements port.DocumentAccessTokenRepository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// Create creates a new document access token.
func (r *Repository) Create(ctx context.Context, token *entity.DocumentAccessToken) error {
	_, err := r.pool.Exec(ctx, queryCreate,
		token.DocumentID,
		token.RecipientID,
		token.AttemptID,
		token.Token,
		token.TokenType,
		token.ExpiresAt,
		token.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("creating document access token: %w", err)
	}
	return nil
}

// FindByToken finds an access token by its token string.
func (r *Repository) FindByToken(ctx context.Context, token string) (*entity.DocumentAccessToken, error) {
	row := r.pool.QueryRow(ctx, queryFindByAccessValue, token)
	return scanToken(row)
}

// FindActiveByRecipientAndType finds an active (unused, non-expired) token for a recipient with a specific type.
func (r *Repository) FindActiveByRecipientAndType(ctx context.Context, recipientID, tokenType string) (*entity.DocumentAccessToken, error) {
	row := r.pool.QueryRow(ctx, queryFindActiveByRecipientAndType, recipientID, tokenType)
	return scanToken(row)
}

// FindActiveByDocumentAndRecipientAndType finds an active (unused,
// non-expired) token for a document+recipient and type.
func (r *Repository) FindActiveByDocumentAndRecipientAndType(
	ctx context.Context,
	documentID,
	recipientID,
	tokenType string,
) (*entity.DocumentAccessToken, error) {
	row := r.pool.QueryRow(ctx, queryFindActiveByDocumentRecipientAndType, documentID, recipientID, tokenType)
	return scanToken(row)
}

// MarkAsUsed marks an access token as used by setting its used_at timestamp.
func (r *Repository) MarkAsUsed(ctx context.Context, tokenID string) error {
	_, err := r.pool.Exec(ctx, queryMarkAsUsed, tokenID)
	if err != nil {
		return fmt.Errorf("marking document access token as used: %w", err)
	}
	return nil
}

// InvalidateByDocumentID invalidates all access tokens for a document.
func (r *Repository) InvalidateByDocumentID(ctx context.Context, documentID string) error {
	_, err := r.pool.Exec(ctx, queryInvalidateByDocumentID, documentID)
	if err != nil {
		return fmt.Errorf("invalidating document access tokens: %w", err)
	}
	return nil
}

// CountRecentByDocumentAndRecipient counts tokens created for a document+recipient since a given time.
func (r *Repository) CountRecentByDocumentAndRecipient(ctx context.Context, documentID, recipientID string, since time.Time) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, queryCountRecentByDocAndRecipient, documentID, recipientID, since).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting recent tokens for document %s recipient %s: %w", documentID, recipientID, err)
	}
	return count, nil
}

// scanToken scans a single token row into a DocumentAccessToken entity.
func scanToken(row pgx.Row) (*entity.DocumentAccessToken, error) {
	t := &entity.DocumentAccessToken{}
	err := row.Scan(
		&t.ID,
		&t.DocumentID,
		&t.RecipientID,
		&t.AttemptID,
		&t.Token,
		&t.TokenType,
		&t.ExpiresAt,
		&t.UsedAt,
		&t.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrRecordNotFound
		}
		return nil, fmt.Errorf("scanning document access token: %w", err)
	}
	return t, nil
}
