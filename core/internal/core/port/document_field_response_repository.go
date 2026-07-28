package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// DocumentFieldResponseRepository defines the interface for document field response data access.
type DocumentFieldResponseRepository interface {
	// Create creates a new document field response.
	Create(ctx context.Context, response *entity.DocumentFieldResponse) error

	// FindByDocumentID finds all field responses for a document.
	FindByDocumentID(ctx context.Context, documentID string) ([]entity.DocumentFieldResponse, error)

	// DeleteByDocumentID deletes all field responses for a document.
	DeleteByDocumentID(ctx context.Context, documentID string) error
}
