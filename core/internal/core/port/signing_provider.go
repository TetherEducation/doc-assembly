package port

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
)

// ErrEmbeddingNotSupported is returned when a provider does not support embedded signing URLs.
var ErrEmbeddingNotSupported = errors.New("provider does not support embedded signing")

// SigningProvider defines attempt-aware operations for external signing services.
type SigningProvider interface {
	SubmitAttemptDocument(ctx context.Context, req *SubmitAttemptDocumentRequest) (*SubmitAttemptDocumentResult, error)
	FindProviderDocumentByCorrelationKey(ctx context.Context, req *FindProviderDocumentRequest) (*ProviderDocumentResult, error)
	InspectProviderSubmission(ctx context.Context, req *FindProviderDocumentRequest) (*ProviderSubmissionSnapshot, error)
	EnsureProviderDocument(ctx context.Context, req *EnsureProviderDocumentRequest) (*EnsureProviderDocumentResult, error)
	EnsureProviderRecipients(ctx context.Context, req *EnsureProviderRecipientsRequest) (*EnsureProviderRecipientsResult, error)
	EnsureProviderFields(ctx context.Context, req *EnsureProviderFieldsRequest) (*EnsureProviderFieldsResult, error)
	EnsureProviderDistributed(ctx context.Context, req *EnsureProviderDistributedRequest) (*EnsureProviderDistributedResult, error)
	FetchProviderSigningReferences(ctx context.Context, req *FetchProviderSigningReferencesRequest) (*FetchProviderSigningReferencesResult, error)
	GetProviderDocumentStatus(ctx context.Context, req *GetProviderDocumentStatusRequest) (*ProviderDocumentStatusResult, error)
	GetAttemptRecipientEmbeddedURL(ctx context.Context, req *GetAttemptRecipientEmbeddedURLRequest) (*GetAttemptRecipientEmbeddedURLResult, error)
	DownloadCompletedPDF(ctx context.Context, req *DownloadCompletedPDFRequest) (*DownloadCompletedPDFResult, error)
	CleanupProviderDocument(ctx context.Context, req *CleanupProviderDocumentRequest) (*CleanupProviderDocumentResult, error)
	ProviderCapabilities() ProviderCapabilities
	ProviderName() string
}

// ProviderCapabilities advertises provider behavior that affects safe retry/reconciliation.
type ProviderCapabilities struct {
	CanFindByCorrelationKey bool
	CanCancel               bool
	CanVoid                 bool
	CanDelete               bool
	CanEmbedSigning         bool
	CanDownloadCompletedPDF bool
	WebhookIncludesIDs      bool
}

// ProviderError is a typed provider-boundary error.
type ProviderError struct {
	Class              entity.ProviderErrorClass
	Phase              entity.ProviderSubmitPhase
	ProviderName       string
	ProviderDocumentID *string
	Retryable          bool
	SafeToResubmit     bool
	Message            string
	Cause              error
}

func (e *ProviderError) Error() string {
	if e == nil {
		return "provider error"
	}
	if e.Cause != nil {
		return fmt.Sprintf("%s %s: %s: %v", e.ProviderName, e.Phase, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s %s: %s", e.ProviderName, e.Phase, e.Message)
}

func (e *ProviderError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// SubmitAttemptDocumentRequest contains all persisted attempt data needed for provider submission.
type SubmitAttemptDocumentRequest struct {
	AttemptID       string
	DocumentID      string
	CorrelationKey  string
	PDF             []byte
	PDFChecksum     string
	Title           string
	Recipients      []SigningRecipient
	WebhookURL      string
	Metadata        map[string]string
	SignatureFields []SignatureFieldPosition
	Environment     entity.Environment
}

// SubmitAttemptDocumentResult contains provider IDs and signing references for an attempt.
type SubmitAttemptDocumentResult struct {
	ProviderDocumentID string
	ProviderName       string
	CorrelationKey     string
	Recipients         []RecipientResult
	InitialStatus      entity.SigningAttemptStatus
}

type ProviderSubmissionSnapshot struct {
	ProviderDocumentID string
	ProviderName       string
	CorrelationKey     string
	RawStatus          string
	HasDocument        bool
	HasRecipients      bool
	HasFields          bool
	IsDistributed      bool
	Recipients         []RecipientResult
	Reason             string
}

type EnsureProviderDocumentRequest struct {
	AttemptID      string
	DocumentID     string
	CorrelationKey string
	PDF            []byte
	PDFChecksum    string
	Title          string
	Environment    entity.Environment
}

type EnsureProviderDocumentResult struct {
	ProviderDocumentID string
	ProviderName       string
	CorrelationKey     string
	RawStatus          string
}

type EnsureProviderRecipientsRequest struct {
	ProviderDocumentID string
	CorrelationKey     string
	Recipients         []SigningRecipient
	Environment        entity.Environment
}

type EnsureProviderRecipientsResult struct {
	Recipients []RecipientResult
}

type EnsureProviderFieldsRequest struct {
	ProviderDocumentID string
	CorrelationKey     string
	Recipients         []RecipientResult
	SignatureFields    []SignatureFieldPosition
	Environment        entity.Environment
}

type EnsureProviderFieldsResult struct {
	FieldCount int
}

type EnsureProviderDistributedRequest struct {
	ProviderDocumentID string
	CorrelationKey     string
	Environment        entity.Environment
}

type EnsureProviderDistributedResult struct {
	RawStatus string
}

type FetchProviderSigningReferencesRequest struct {
	ProviderDocumentID string
	CorrelationKey     string
	Environment        entity.Environment
}

type FetchProviderSigningReferencesResult struct {
	Recipients []RecipientResult
	Status     entity.SigningAttemptStatus
	RawStatus  string
}

// FindProviderDocumentRequest finds existing provider state for an attempt correlation key.
type FindProviderDocumentRequest struct {
	ProviderName   string
	CorrelationKey string
	Environment    entity.Environment
}

// ProviderDocumentResult describes a provider document discovered during reconciliation.
type ProviderDocumentResult struct {
	Found              bool
	Usable             bool
	ProviderDocumentID string
	ProviderName       string
	CorrelationKey     string
	Recipients         []RecipientResult
	Status             entity.SigningAttemptStatus
	RawStatus          string
	Reason             string
}

// GetProviderDocumentStatusRequest retrieves current provider state for an attempt.
type GetProviderDocumentStatusRequest struct {
	ProviderDocumentID string
	Environment        entity.Environment
}

// ProviderDocumentStatusResult contains current provider status and recipients.
type ProviderDocumentStatusResult struct {
	Status              entity.SigningAttemptStatus
	Recipients          []RecipientStatusResult
	CompletedPDFURL     *string
	ProviderStatus      string
	ProviderDocumentID  string
	ProviderCorrelation *string
}

// GetAttemptRecipientEmbeddedURLRequest gets the current embedded URL for one attempt recipient.
type GetAttemptRecipientEmbeddedURLRequest struct {
	ProviderDocumentID  string
	ProviderRecipientID string
	CallbackURL         string
	Environment         entity.Environment
}

// GetAttemptRecipientEmbeddedURLResult contains the embedded URL and CSP data.
type GetAttemptRecipientEmbeddedURLResult struct {
	EmbeddedURL    string
	FrameSrcDomain string
	ExpiresAt      *time.Time
}

// DownloadCompletedPDFRequest downloads the completed/signed PDF from the provider.
type DownloadCompletedPDFRequest struct {
	ProviderDocumentID string
	Environment        entity.Environment
}

// DownloadCompletedPDFResult contains the completed/signed PDF bytes.
type DownloadCompletedPDFResult struct {
	PDF         []byte
	Filename    string
	ContentType string
}

// CleanupProviderDocumentRequest asks the provider to cancel/void/delete a historical attempt.
type CleanupProviderDocumentRequest struct {
	ProviderDocumentID string
	Environment        entity.Environment
}

// CleanupProviderDocumentResult records best-effort cleanup outcome.
type CleanupProviderDocumentResult struct {
	Action string
	Status string
	Reason string
}

// SignatureFieldPosition contains the position and size of a signature field.
type SignatureFieldPosition struct {
	RoleID    string
	Page      int
	PositionX float64
	PositionY float64
	Width     float64
	Height    float64
}

// SigningRecipient represents a person who needs to sign the document.
type SigningRecipient struct {
	Email       string
	Name        string
	RoleID      string
	SignerOrder int
}

// RecipientResult contains provider references for one recipient.
type RecipientResult struct {
	RoleID               string
	ProviderRecipientID  string
	ProviderSigningToken string
	SigningURL           string
	Status               entity.RecipientStatus
}

// RecipientStatusResult contains current provider state for one recipient.
type RecipientStatusResult struct {
	ProviderRecipientID string
	Status              entity.RecipientStatus
	SignedAt            *time.Time
	ProviderStatus      string
}

// ParseWebhookRequest contains the data needed to parse a webhook event.
type ParseWebhookRequest struct {
	Body        []byte
	Signature   string
	Environment entity.Environment
}

// WebhookEvent represents an incoming webhook event from a signing provider.
type WebhookEvent struct {
	EventType              string
	ProviderName           string
	ProviderDocumentID     string
	ProviderCorrelationKey string
	ProviderRecipientID    string
	DocumentStatus         *entity.SigningAttemptStatus
	RecipientStatus        *entity.RecipientStatus
	Timestamp              time.Time
	RawPayload             []byte
}

// WebhookHandler defines the interface for processing webhook events.
type WebhookHandler interface {
	ParseWebhook(ctx context.Context, req *ParseWebhookRequest) (*WebhookEvent, error)
}
