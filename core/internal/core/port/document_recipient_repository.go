package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// DocumentRecipientRepository defines the interface for document recipient data access.
type DocumentRecipientRepository interface {
	// Create creates a new document recipient.
	Create(ctx context.Context, recipient *entity.DocumentRecipient) (string, error)

	// CreateBatch creates multiple recipients for a document.
	CreateBatch(ctx context.Context, recipients []*entity.DocumentRecipient) error

	// FindByID finds a recipient by ID.
	FindByID(ctx context.Context, id string) (*entity.DocumentRecipient, error)

	// FindByDocumentID finds all recipients for a document.
	FindByDocumentID(ctx context.Context, documentID string) ([]*entity.DocumentRecipient, error)

	// FindByDocumentIDWithRoles finds all recipients for a document with their role information.
	FindByDocumentIDWithRoles(ctx context.Context, documentID string) ([]*entity.DocumentRecipientWithRole, error)

	// FindBySignerRecipientID finds a recipient by the external signing provider's recipient ID.
	FindBySignerRecipientID(ctx context.Context, signerRecipientID string) (*entity.DocumentRecipient, error)

	// FindByDocumentAndRole finds a recipient by document ID and role ID.
	FindByDocumentAndRole(ctx context.Context, documentID, roleID string) (*entity.DocumentRecipient, error)

	// FindByDocumentAndEmail finds a recipient by document ID and email (case-insensitive).
	FindByDocumentAndEmail(ctx context.Context, documentID, email string) (*entity.DocumentRecipient, error)

	// Update updates a document recipient.
	Update(ctx context.Context, recipient *entity.DocumentRecipient) error

	// UpdateStatus updates only the status of a recipient.
	UpdateStatus(ctx context.Context, id string, status entity.RecipientStatus) error

	// UpdateSignerInfo updates the signer provider recipient ID.
	UpdateSignerInfo(ctx context.Context, id, signerRecipientID string) error

	// Delete deletes a document recipient.
	Delete(ctx context.Context, id string) error

	// DeleteByDocumentID deletes all recipients for a document.
	DeleteByDocumentID(ctx context.Context, documentID string) error

	// CountByDocumentAndStatus returns the count of recipients by status for a document.
	CountByDocumentAndStatus(ctx context.Context, documentID string, status entity.RecipientStatus) (int, error)

	// CountByDocument returns the total number of recipients for a document.
	CountByDocument(ctx context.Context, documentID string) (int, error)

	// AllSigned checks if all recipients for a document have signed.
	AllSigned(ctx context.Context, documentID string) (bool, error)

	// AnyDeclined checks if any recipient has declined.
	AnyDeclined(ctx context.Context, documentID string) (bool, error)
}
