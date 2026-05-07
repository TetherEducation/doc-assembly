package port

import (
	"context"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
)

// DocumentFilters contains optional filters for document queries.
type DocumentFilters struct {
	Status                    *entity.DocumentStatus
	SignerProvider            *string
	ClientExternalReferenceID *string
	TemplateVersionID         *string
	Search                    string
	Limit                     int
	Offset                    int
}

// InternalCreateRequest contains the data required for the internal create/replay transaction.
type InternalCreateRequest struct {
	WorkspaceID     string
	DocumentTypeID  string
	ExternalID      string
	TransactionalID string
	ForceCreate     bool
	SupersedeReason *string
	Document        *entity.Document
	Recipients      []*entity.DocumentRecipient
}

// InternalCreateResult represents the outcome of internal create/replay processing.
type InternalCreateResult struct {
	DocumentID                   string
	Document                     *entity.DocumentWithRecipients
	IdempotentReplay             bool
	SupersededPreviousDocumentID *string
}

// DocumentRepository defines the interface for document data access.
type DocumentRepository interface {
	// Create creates a new document.
	Create(ctx context.Context, document *entity.Document) (string, error)

	// FindByID finds a document by ID.
	FindByID(ctx context.Context, id string) (*entity.Document, error)

	// FindByIDWithRecipients finds a document by ID with all recipients.
	FindByIDWithRecipients(ctx context.Context, id string) (*entity.DocumentWithRecipients, error)

	// FindByWorkspace lists all documents in a workspace with optional filters.
	FindByWorkspace(ctx context.Context, workspaceID string, filters DocumentFilters) ([]*entity.DocumentListItem, error)

	// FindByClientExternalRef finds documents by the client's external reference ID.
	FindByClientExternalRef(ctx context.Context, workspaceID, clientExternalRef string) ([]*entity.Document, error)

	// FindByTemplateVersion finds all documents generated from a specific template version.
	FindByTemplateVersion(ctx context.Context, templateVersionID string) ([]*entity.DocumentListItem, error)

	// FindExpired finds documents that have passed their expiration time and are still active.
	FindExpired(ctx context.Context, limit int) ([]*entity.Document, error)

	// Update updates a document.
	Update(ctx context.Context, document *entity.Document) error

	// UpdateStatus updates only the status of a document.
	UpdateStatus(ctx context.Context, id string, status entity.DocumentStatus) error

	// Delete deletes a document and all its recipients (cascade).
	Delete(ctx context.Context, id string) error

	// CountByWorkspace returns the total number of documents in a workspace.
	CountByWorkspace(ctx context.Context, workspaceID string) (int, error)

	// CountByStatus returns the count of documents by status in a workspace.
	CountByStatus(ctx context.Context, workspaceID string, status entity.DocumentStatus) (int, error)

	// FindInternalCreateReplay finds an already-created document for an internal-create request before expensive preparation.
	FindInternalCreateReplay(ctx context.Context, workspaceID, documentTypeID, externalID, transactionalID string) (string, bool, error)

	// FindActiveByLogicalKey finds the active logical document for workspace + document type + external ID.
	FindActiveByLogicalKey(ctx context.Context, workspaceID, documentTypeID, externalID string) (string, bool, error)

	// InternalCreateOrReplay executes transactional create/replay/supersede logic for internal create endpoint.
	InternalCreateOrReplay(ctx context.Context, req *InternalCreateRequest) (*InternalCreateResult, error)
}
