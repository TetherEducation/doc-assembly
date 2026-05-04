// Document completion ports for the River worker queue.
// See docs/backend/worker-queue-guide.md for the full completion flow.
package port

import (
	"context"
	"time"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
)

// DocumentCompletedHandler is the user-provided callback invoked when a
// document reaches COMPLETED status. Returning an error causes River to
// retry the job automatically.
type DocumentCompletedHandler func(ctx context.Context, event DocumentCompletedEvent) error

// DocumentCompletedEvent carries all relevant data about a completed document.
type DocumentCompletedEvent struct {
	DocumentID       string
	ExternalID       *string
	Title            *string
	Status           entity.DocumentStatus
	WorkspaceCode    string
	DocumentTypeCode *string
	TenantCode       string
	Environment      entity.Environment
	CreatedAt        time.Time
	UpdatedAt        *time.Time
	ExpiresAt        *time.Time
	Metadata         map[string]string
	Recipients       []CompletedRecipient
}

// CompletedRecipient holds signer information within a completed document event.
type CompletedRecipient struct {
	RoleName    string
	SignerOrder int
	Name        string
	Email       string
	Status      entity.RecipientStatus
	SignedAt    *time.Time
}
