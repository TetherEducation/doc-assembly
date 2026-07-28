package document

import (
	"context"
	"encoding/json"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
)

// PreSigningUseCase defines the input port for public signing operations.
// These operations are used by the public signing page (no auth required, token-validated).
type PreSigningUseCase interface {
	// GetPublicSigningPage returns the current signing page state for a given access token.
	// The response step depends on the document status and token type.
	GetPublicSigningPage(ctx context.Context, token string) (*PublicSigningResponse, error)

	// SubmitPreSigningForm submits field responses and returns the updated preview state.
	// Provider submission happens only after ProceedToSigning creates/reuses an attempt and River processes it.
	SubmitPreSigningForm(ctx context.Context, token string, responses []FieldResponseInput) (*PublicSigningResponse, error)

	// ProceedToSigning creates or reuses the active signing attempt and enqueues River work.
	// Accepts both SIGNING (Path A) and PRE_SIGNING (Path B) tokens; callers usually receive processing until River prepares signing refs.
	ProceedToSigning(ctx context.Context, token string) (*PublicSigningResponse, error)

	// RenderPreviewPDF renders the document PDF on-demand for preview (no storage).
	RenderPreviewPDF(ctx context.Context, token string) ([]byte, error)

	// CompleteEmbeddedSigning marks the token as used after signing is completed.
	CompleteEmbeddedSigning(ctx context.Context, token string) error

	// RefreshEmbeddedURL refreshes an expired embedded signing URL.
	RefreshEmbeddedURL(ctx context.Context, token string) (*PublicSigningResponse, error)

	// DownloadCompletedPDF downloads the signed PDF for a completed document when
	// the token holder is authorized.
	DownloadCompletedPDF(ctx context.Context, token string) ([]byte, string, error)

	// InvalidateTokens invalidates all active tokens for a document.
	// Requires the document to be in AWAITING_INPUT status.
	InvalidateTokens(ctx context.Context, documentID string) error
}

// PublicSigningResponse contains the data for rendering the public signing page.
// The Step field determines which UI to show.
type PublicSigningResponse struct {
	// Step indicates the current signing flow step.
	// "preview" = show form (Path B) or PDF preview (Path A)
	// "processing" = River is preparing/retrying/reconciling the active attempt
	// "signing" = show embedded signing iframe
	// "waiting" = waiting for previous signers
	// "completed" = signing completed
	// "declined" = document was declined
	// "document_updated" = token points at a superseded/invalidated attempt
	// "unavailable" = active attempt failed or requires review
	Step string `json:"step"`

	// Form contains the pre-signing form data (step=preview, Path B).
	Form *PreSigningFormDTO `json:"form,omitempty"`

	// PdfURL is the URL to download the rendered PDF for preview (step=preview, Path A).
	PdfURL string `json:"pdfUrl,omitempty"`

	// EmbeddedSigningURL is the URL to load in an iframe (step=signing).
	EmbeddedSigningURL string `json:"embeddedSigningUrl,omitempty"`

	// DocumentTitle is the document title.
	DocumentTitle string `json:"documentTitle"`

	// RecipientName is the signer's display name.
	RecipientName string `json:"recipientName"`

	// WaitingForPrevious is true when waiting for earlier signers (step=waiting).
	WaitingForPrevious bool `json:"waitingForPrevious,omitempty"`

	// SigningPosition is the signer's position in signing order (step=waiting).
	SigningPosition int `json:"signingPosition,omitempty"`

	// TotalSigners is the total number of signers (step=waiting).
	TotalSigners int `json:"totalSigners,omitempty"`

	// FallbackURL is a direct signing URL when embedding is not supported.
	FallbackURL string `json:"fallbackUrl,omitempty"`

	// DocumentStatus is the canonical internal document status.
	DocumentStatus string `json:"documentStatus,omitempty"`

	// ProcessingReason explains public processing copy without exposing internal errors.
	ProcessingReason string `json:"processingReason,omitempty"`

	// RetryAfterSeconds hints how soon the public page should poll again.
	RetryAfterSeconds int `json:"retryAfterSeconds,omitempty"`

	// SupportCode is a safe identifier support can use for long-running processing.
	SupportCode string `json:"supportCode,omitempty"`

	// HasCurrentUserSigned indicates whether the token recipient has signed.
	HasCurrentUserSigned bool `json:"hasCurrentUserSigned"`

	// CanSign indicates whether this recipient can still proceed in signing flow.
	CanSign bool `json:"canSign"`

	// CanDownload indicates whether this recipient can download the completed PDF.
	CanDownload bool `json:"canDownload"`

	// DownloadURL is the endpoint for downloading the completed signed PDF.
	DownloadURL string `json:"downloadUrl,omitempty"`
}

// Signing step constants.
const (
	StepPreview         = "preview"
	StepSigning         = "signing"
	StepWaiting         = "waiting"
	StepCompleted       = "completed"
	StepDeclined        = "declined"
	StepProcessing      = "processing"
	StepDocumentUpdated = "document_updated"
	StepUnavailable     = "unavailable"
)

// FieldResponseInput represents a single field response from the signer.
type FieldResponseInput struct {
	FieldID  string          `json:"fieldId"`
	Response json.RawMessage `json:"response"` // {"selectedOptionIds":[...]} or {"text":"..."}
}

// PreSigningFormDTO contains all data needed to render the pre-signing form.
type PreSigningFormDTO struct {
	DocumentTitle  string                `json:"documentTitle"`
	DocumentStatus string                `json:"documentStatus"`
	RecipientName  string                `json:"recipientName"`
	RecipientEmail string                `json:"recipientEmail"`
	RoleID         string                `json:"roleId"`
	Content        json.RawMessage       `json:"content"` // full portabledoc ProseMirror content
	Fields         []InteractiveFieldDTO `json:"fields"`  // fields for this role
}

// InteractiveFieldDTO describes a single interactive field for the pre-signing form.
type InteractiveFieldDTO struct {
	ID          string                          `json:"id"`
	FieldType   string                          `json:"fieldType"`
	Label       string                          `json:"label"`
	Required    bool                            `json:"required"`
	Options     []portabledoc.InteractiveOption `json:"options,omitempty"`
	Placeholder string                          `json:"placeholder,omitempty"`
	MaxLength   int                             `json:"maxLength,omitempty"`
}
