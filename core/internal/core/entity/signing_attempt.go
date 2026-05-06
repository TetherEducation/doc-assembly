package entity

import (
	"encoding/json"
	"time"
)

// SigningAttemptStatus is the technical execution status for one concrete render/provider submission.
type SigningAttemptStatus string

const (
	SigningAttemptStatusCreated              SigningAttemptStatus = "CREATED"
	SigningAttemptStatusRendering            SigningAttemptStatus = "RENDERING"
	SigningAttemptStatusPDFReady             SigningAttemptStatus = "PDF_READY"
	SigningAttemptStatusReadyToSubmit        SigningAttemptStatus = "READY_TO_SUBMIT"
	SigningAttemptStatusSubmittingProvider   SigningAttemptStatus = "SUBMITTING_PROVIDER"
	SigningAttemptStatusProviderRetryWaiting SigningAttemptStatus = "PROVIDER_RETRY_WAITING"
	SigningAttemptStatusSubmissionUnknown    SigningAttemptStatus = "SUBMISSION_UNKNOWN"
	SigningAttemptStatusReconcilingProvider  SigningAttemptStatus = "RECONCILING_PROVIDER"
	SigningAttemptStatusSigningReady         SigningAttemptStatus = "SIGNING_READY"
	SigningAttemptStatusSigning              SigningAttemptStatus = "SIGNING"
	SigningAttemptStatusCompleted            SigningAttemptStatus = "COMPLETED"
	SigningAttemptStatusDeclined             SigningAttemptStatus = "DECLINED"
	SigningAttemptStatusInvalidated          SigningAttemptStatus = "INVALIDATED"
	SigningAttemptStatusSuperseded           SigningAttemptStatus = "SUPERSEDED"
	SigningAttemptStatusCancelled            SigningAttemptStatus = "CANCELLED"
	SigningAttemptStatusRequiresReview       SigningAttemptStatus = "REQUIRES_REVIEW"
	SigningAttemptStatusFailedPermanent      SigningAttemptStatus = "FAILED_PERMANENT"
)

// ProviderSubmitPhase identifies the provider step where a submit failure happened.
type ProviderSubmitPhase string

const (
	ProviderSubmitPhaseBeforeRequest          ProviderSubmitPhase = "BEFORE_REQUEST"
	ProviderSubmitPhaseCreateProviderDocument ProviderSubmitPhase = "CREATE_PROVIDER_DOCUMENT"
	ProviderSubmitPhaseAddRecipients          ProviderSubmitPhase = "ADD_RECIPIENTS"
	ProviderSubmitPhaseCreateFields           ProviderSubmitPhase = "CREATE_FIELDS"
	ProviderSubmitPhaseDistributeDocument     ProviderSubmitPhase = "DISTRIBUTE_DOCUMENT"
	ProviderSubmitPhaseFetchSigningReferences ProviderSubmitPhase = "FETCH_SIGNING_REFERENCES"
)

func (p ProviderSubmitPhase) IsProviderMutationPhase() bool {
	switch p {
	case ProviderSubmitPhaseCreateProviderDocument,
		ProviderSubmitPhaseAddRecipients,
		ProviderSubmitPhaseCreateFields,
		ProviderSubmitPhaseDistributeDocument,
		ProviderSubmitPhaseFetchSigningReferences:
		return true
	default:
		return false
	}
}

func NextProviderSubmitPhase(p ProviderSubmitPhase) ProviderSubmitPhase {
	switch p {
	case ProviderSubmitPhaseCreateProviderDocument:
		return ProviderSubmitPhaseAddRecipients
	case ProviderSubmitPhaseAddRecipients:
		return ProviderSubmitPhaseCreateFields
	case ProviderSubmitPhaseCreateFields:
		return ProviderSubmitPhaseDistributeDocument
	case ProviderSubmitPhaseDistributeDocument:
		return ProviderSubmitPhaseFetchSigningReferences
	default:
		return ""
	}
}

// ProviderErrorClass controls whether a provider failure can be retried, reconciled, or must stop.
type ProviderErrorClass string

const (
	ProviderErrorClassTransient     ProviderErrorClass = "TRANSIENT"
	ProviderErrorClassPermanent     ProviderErrorClass = "PERMANENT"
	ProviderErrorClassAmbiguous     ProviderErrorClass = "AMBIGUOUS"
	ProviderErrorClassConflictStale ProviderErrorClass = "CONFLICT_STALE"
)

// SigningAttempt represents one concrete signing execution for a document.
type SigningAttempt struct {
	ID                       string               `json:"id"`
	DocumentID               string               `json:"documentId"`
	Sequence                 int                  `json:"sequence"`
	Status                   SigningAttemptStatus `json:"status"`
	RenderStartedAt          *time.Time           `json:"renderStartedAt,omitempty"`
	PDFStoragePath           *string              `json:"pdfStoragePath,omitempty"`
	PDFChecksum              *string              `json:"pdfChecksum,omitempty"`
	PDFChecksumAlgorithm     *string              `json:"pdfChecksumAlgorithm,omitempty"`
	RenderMetadata           json.RawMessage      `json:"renderMetadata,omitempty"`
	SignatureFieldSnapshot   json.RawMessage      `json:"signatureFieldSnapshot,omitempty"`
	ProviderUploadPayload    json.RawMessage      `json:"providerUploadPayload,omitempty"`
	ProviderName             *string              `json:"providerName,omitempty"`
	ProviderCorrelationKey   *string              `json:"providerCorrelationKey,omitempty"`
	ProviderDocumentID       *string              `json:"providerDocumentId,omitempty"`
	ProviderSubmitPhase      *ProviderSubmitPhase `json:"providerSubmitPhase,omitempty"`
	RetryCount               int                  `json:"retryCount"`
	NextRetryAt              *time.Time           `json:"nextRetryAt,omitempty"`
	LastErrorClass           *ProviderErrorClass  `json:"lastErrorClass,omitempty"`
	LastErrorMessage         *string              `json:"lastErrorMessage,omitempty"`
	ReconciliationCount      int                  `json:"reconciliationCount"`
	NextReconciliationAt     *time.Time           `json:"nextReconciliationAt,omitempty"`
	CleanupStatus            *string              `json:"cleanupStatus,omitempty"`
	CleanupAction            *string              `json:"cleanupAction,omitempty"`
	CleanupError             *string              `json:"cleanupError,omitempty"`
	ProcessingLeaseOwner     *string              `json:"processingLeaseOwner,omitempty"`
	ProcessingLeaseExpiresAt *time.Time           `json:"processingLeaseExpiresAt,omitempty"`
	InvalidationReason       *string              `json:"invalidationReason,omitempty"`
	CreatedAt                time.Time            `json:"createdAt"`
	UpdatedAt                *time.Time           `json:"updatedAt,omitempty"`
	TerminalAt               *time.Time           `json:"terminalAt,omitempty"`
}

// IsTerminal returns true when this attempt status is terminal.
func (s SigningAttemptStatus) IsTerminal() bool {
	switch s {
	case SigningAttemptStatusCompleted,
		SigningAttemptStatusDeclined,
		SigningAttemptStatusInvalidated,
		SigningAttemptStatusSuperseded,
		SigningAttemptStatusCancelled,
		SigningAttemptStatusRequiresReview,
		SigningAttemptStatusFailedPermanent:
		return true
	default:
		return false
	}
}

// IsTerminal returns true when no automatic worker may continue this attempt.
func (a *SigningAttempt) IsTerminal() bool {
	if a == nil {
		return false
	}
	return a.Status.IsTerminal()
}

// CanRetryAutomatically returns true for non-terminal worker-owned retry/reconciliation states.
func (a *SigningAttempt) CanRetryAutomatically() bool {
	if a == nil || a.IsTerminal() {
		return false
	}
	switch a.Status {
	case SigningAttemptStatusProviderRetryWaiting,
		SigningAttemptStatusSubmissionUnknown,
		SigningAttemptStatusReconcilingProvider,
		SigningAttemptStatusReadyToSubmit,
		SigningAttemptStatusPDFReady:
		return true
	default:
		return false
	}
}

// ProjectDocumentStatusFromAttempt maps a technical attempt status to the document business projection.
func ProjectDocumentStatusFromAttempt(status SigningAttemptStatus) DocumentStatus {
	switch status {
	case SigningAttemptStatusSigningReady:
		return DocumentStatusReadyToSign
	case SigningAttemptStatusSigning:
		return DocumentStatusSigning
	case SigningAttemptStatusCompleted:
		return DocumentStatusCompleted
	case SigningAttemptStatusDeclined:
		return DocumentStatusDeclined
	case SigningAttemptStatusCancelled:
		return DocumentStatusCancelled
	case SigningAttemptStatusInvalidated, SigningAttemptStatusSuperseded:
		return DocumentStatusInvalidated
	case SigningAttemptStatusFailedPermanent, SigningAttemptStatusRequiresReview:
		return DocumentStatusError
	default:
		return DocumentStatusPreparingSignature
	}
}

// AttemptStatusForProviderError maps provider failure classes to attempt states.
func AttemptStatusForProviderError(class ProviderErrorClass) SigningAttemptStatus {
	switch class {
	case ProviderErrorClassTransient:
		return SigningAttemptStatusProviderRetryWaiting
	case ProviderErrorClassPermanent:
		return SigningAttemptStatusFailedPermanent
	case ProviderErrorClassAmbiguous:
		return SigningAttemptStatusSubmissionUnknown
	case ProviderErrorClassConflictStale:
		return SigningAttemptStatusRequiresReview
	default:
		return SigningAttemptStatusFailedPermanent
	}
}
