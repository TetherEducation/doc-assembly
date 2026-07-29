package document

import (
	"context"
	"encoding/json"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// ReadOnlyViewMode identifies how a read-only document view should be rendered.
type ReadOnlyViewMode string

const (
	ReadOnlyViewModeContent     ReadOnlyViewMode = "content"
	ReadOnlyViewModePDF         ReadOnlyViewMode = "pdf"
	ReadOnlyViewModeUnavailable ReadOnlyViewMode = "unavailable"
)

// CreateReadOnlyViewLinkResult contains the expiring public link details for a read-only view.
type CreateReadOnlyViewLinkResult struct {
	URL       string
	Token     string
	ExpiresAt time.Time
}

// ReadOnlyViewResponse contains the public read-only view state for a token.
type ReadOnlyViewResponse struct {
	Mode           ReadOnlyViewMode
	DocumentID     string
	DocumentTitle  string
	DocumentStatus entity.DocumentStatus
	ExpiresAt      time.Time
	Content        json.RawMessage
	PDFURL         *string
	Reason         *string
}

// SigningStateRecipient reports one recipient's progress on a document.
type SigningStateRecipient struct {
	Name        string
	Email       string
	RoleName    string
	SignerOrder int
	// Status is the recipient projection on the document. Only SIGNED is
	// authoritative: intermediate provider states (SENT/DELIVERED) are owned by
	// the signing attempt and are not consistently projected back here.
	Status   entity.RecipientStatus
	Signed   bool
	SignedAt *time.Time
}

// SigningStateDocument reports whether one document has actually been signed.
type SigningStateDocument struct {
	DocumentID          string
	ExternalReferenceID *string
	Status              entity.DocumentStatus
	// Signed is the single question callers are asking: did this document reach
	// COMPLETED. A caller's own workflow state saying "done" is not evidence.
	Signed bool
	// Expired documents cannot mint a signing session (CreateOrGetSession
	// refuses them), so they need regeneration rather than a reminder.
	Expired    bool
	ExpiresAt  *time.Time
	Recipients []SigningStateRecipient
}

// SigningStateResult is the batch answer for a set of requested document IDs.
type SigningStateResult struct {
	Documents []SigningStateDocument
	// Unavailable lists requested IDs that were not resolved. Documents that do
	// not exist and documents belonging to another workspace are deliberately
	// merged here so the endpoint cannot be used to probe for the existence of
	// documents outside the caller's workspace.
	Unavailable []string
}

// ReadOnlyViewUseCase defines the input port for expiring read-only document views.
type ReadOnlyViewUseCase interface {
	CreateReadOnlyViewLink(ctx context.Context, workspaceID, documentID string) (*CreateReadOnlyViewLinkResult, error)
	CreateReadOnlyViewLinkByWorkspaceCode(ctx context.Context, workspaceCode, documentID string) (*CreateReadOnlyViewLinkResult, error)
	GetReadOnlyView(ctx context.Context, token string) (*ReadOnlyViewResponse, error)
	GetReadOnlyViewPDF(ctx context.Context, token string) ([]byte, string, error)
	// GetPrintPDF renders the current unsigned PDF (or a blank template when
	// blank is true) for in-person signing, authorized by internal workspace ID.
	GetPrintPDF(ctx context.Context, workspaceID, documentID string, blank bool) ([]byte, string, error)
	// GetPrintPDFByWorkspaceCode is the external-caller flavor of GetPrintPDF,
	// authorized by workspace business code.
	GetPrintPDFByWorkspaceCode(ctx context.Context, workspaceCode, documentID string, blank bool) ([]byte, string, error)
	// GetSigningStateByWorkspaceCode reports, for each requested document, whether
	// it was actually signed. External callers track their own completion state
	// (e.g. a CRM requirement marked done by staff) which can diverge from whether
	// anyone ever signed; this is the authoritative answer.
	GetSigningStateByWorkspaceCode(ctx context.Context, workspaceCode string, documentIDs []string) (*SigningStateResult, error)
}
