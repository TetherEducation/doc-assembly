package documenteventrepo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new document event repository.
func New(pool *pgxpool.Pool) port.DocumentEventRepository {
	return &Repository{pool: pool}
}

// Repository implements port.DocumentEventRepository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// Create creates a new document event.
func (r *Repository) Create(ctx context.Context, event *entity.DocumentEvent) error {
	_, err := r.pool.Exec(ctx, queryCreate,
		event.DocumentID,
		event.EventType,
		event.ActorType,
		nilIfEmpty(event.ActorID),
		nilIfEmpty(event.OldStatus),
		nilIfEmpty(event.NewStatus),
		nilIfEmpty(event.RecipientID),
		event.Metadata,
		event.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("creating document event: %w", err)
	}

	return nil
}

// FindByDocumentID returns events for a document, ordered by created_at DESC.
func (r *Repository) FindByDocumentID(ctx context.Context, documentID string, limit, offset int) ([]*entity.DocumentEvent, error) {
	rows, err := r.pool.Query(ctx, queryFindByDocumentID, documentID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("querying document events: %w", err)
	}
	defer rows.Close()

	var events []*entity.DocumentEvent
	for rows.Next() {
		e := &entity.DocumentEvent{}
		var actorID, oldStatus, newStatus, recipientID *string
		if err := rows.Scan(
			&e.ID,
			&e.DocumentID,
			&e.EventType,
			&e.ActorType,
			&actorID,
			&oldStatus,
			&newStatus,
			&recipientID,
			&e.Metadata,
			&e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning document event: %w", err)
		}
		if actorID != nil {
			e.ActorID = *actorID
		}
		if oldStatus != nil {
			e.OldStatus = *oldStatus
		}
		if newStatus != nil {
			e.NewStatus = *newStatus
		}
		if recipientID != nil {
			e.RecipientID = *recipientID
		}
		events = append(events, e)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating document events: %w", err)
	}

	return events, nil
}

// nilIfEmpty returns nil if the string is empty, otherwise returns a pointer to it.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
