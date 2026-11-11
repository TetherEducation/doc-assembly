package port

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// DocumentEventRepository defines the interface for document event data access.
type DocumentEventRepository interface {
	// Create creates a new document event.
	Create(ctx context.Context, event *entity.DocumentEvent) error

	// CreateOnce records a timeline event unless the same event is already
	// recorded. Same event = same document, event type, recipient (both may be
	// empty) and metadata attemptId. For RECIPIENT_SIGNED, an event whose
	// metadata timeSource is "provider" replaces the time (and metadata) of an
	// existing one the engine guessed. Concurrent calls for one document are
	// serialized, so each event is written once.
	CreateOnce(ctx context.Context, event *entity.DocumentEvent) error

	// CreateOnceTx is CreateOnce inside a caller-owned transaction.
	CreateOnceTx(ctx context.Context, tx pgx.Tx, event *entity.DocumentEvent) error

	// FindByDocumentID returns events for a document, ordered by created_at DESC.
	FindByDocumentID(ctx context.Context, documentID string, limit, offset int) ([]*entity.DocumentEvent, error)
}
