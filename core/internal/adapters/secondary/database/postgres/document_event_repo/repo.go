package documenteventrepo

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
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

// CreateOnce records a timeline event once (see port.DocumentEventRepository).
func (r *Repository) CreateOnce(ctx context.Context, event *entity.DocumentEvent) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin document event tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if err := r.CreateOnceTx(ctx, tx, event); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit document event tx: %w", err)
	}
	return nil
}

// CreateOnceTx records a timeline event once, inside tx. The per-document
// advisory lock is held until tx ends.
func (r *Repository) CreateOnceTx(ctx context.Context, tx pgx.Tx, event *entity.DocumentEvent) error {
	if _, err := tx.Exec(ctx, queryLockDocumentEvents, event.DocumentID); err != nil {
		return fmt.Errorf("locking document events: %w", err)
	}
	args := []any{
		event.DocumentID,
		event.EventType,
		event.ActorType,
		nilIfEmpty(event.ActorID),
		nilIfEmpty(event.OldStatus),
		nilIfEmpty(event.NewStatus),
		nilIfEmpty(event.RecipientID),
		event.Metadata,
		event.CreatedAt,
	}
	tag, err := tx.Exec(ctx, queryCreateOnce, args...)
	if err != nil {
		return fmt.Errorf("creating document event once: %w", err)
	}
	if tag.RowsAffected() > 0 || !providerSignedTime(event) {
		return nil
	}
	if _, err := tx.Exec(ctx, queryUpgradeSignedTime,
		event.DocumentID, event.EventType, nilIfEmpty(event.RecipientID),
		event.Metadata, event.CreatedAt, event.ActorType,
	); err != nil {
		return fmt.Errorf("updating signed event time: %w", err)
	}
	return nil
}

// providerSignedTime reports whether event is a RECIPIENT_SIGNED event timed
// by the provider, which replaces an engine-guessed time.
func providerSignedTime(event *entity.DocumentEvent) bool {
	if event.EventType != entity.EventRecipientSigned || len(event.Metadata) == 0 {
		return false
	}
	var meta struct {
		TimeSource string `json:"timeSource"`
	}
	if err := json.Unmarshal(event.Metadata, &meta); err != nil {
		return false
	}
	return meta.TimeSource == entity.TimelineTimeProvider
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
