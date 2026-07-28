package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// DocumentEventRepository defines the interface for document event data access.
type DocumentEventRepository interface {
	// Create creates a new document event.
	Create(ctx context.Context, event *entity.DocumentEvent) error

	// FindByDocumentID returns events for a document, ordered by created_at DESC.
	FindByDocumentID(ctx context.Context, documentID string, limit, offset int) ([]*entity.DocumentEvent, error)
}
