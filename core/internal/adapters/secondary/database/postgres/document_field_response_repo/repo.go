package documentfieldresponserepo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new document field response repository.
func New(pool *pgxpool.Pool) port.DocumentFieldResponseRepository {
	return &Repository{pool: pool}
}

// Repository implements port.DocumentFieldResponseRepository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// Create creates a new document field response.
func (r *Repository) Create(ctx context.Context, response *entity.DocumentFieldResponse) error {
	_, err := r.pool.Exec(ctx, queryCreate,
		response.DocumentID,
		response.RecipientID,
		response.FieldID,
		response.FieldType,
		response.Response,
		response.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("creating document field response: %w", err)
	}
	return nil
}

// FindByDocumentID finds all field responses for a document.
func (r *Repository) FindByDocumentID(ctx context.Context, documentID string) ([]entity.DocumentFieldResponse, error) {
	rows, err := r.pool.Query(ctx, queryFindByDocumentID, documentID)
	if err != nil {
		return nil, fmt.Errorf("querying document field responses: %w", err)
	}
	defer rows.Close()

	var responses []entity.DocumentFieldResponse
	for rows.Next() {
		var resp entity.DocumentFieldResponse
		if err := rows.Scan(
			&resp.ID,
			&resp.DocumentID,
			&resp.RecipientID,
			&resp.FieldID,
			&resp.FieldType,
			&resp.Response,
			&resp.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning document field response: %w", err)
		}
		responses = append(responses, resp)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating document field responses: %w", err)
	}

	return responses, nil
}

// DeleteByDocumentID deletes all field responses for a document.
func (r *Repository) DeleteByDocumentID(ctx context.Context, documentID string) error {
	_, err := r.pool.Exec(ctx, queryDeleteByDocumentID, documentID)
	if err != nil {
		return fmt.Errorf("deleting document field responses: %w", err)
	}
	return nil
}
