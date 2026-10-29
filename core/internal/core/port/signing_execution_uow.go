package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// SigningJobPhase identifies attempt-scoped River work.
type SigningJobPhase string

const (
	SigningJobPhaseRenderAttemptPDF          SigningJobPhase = "render_attempt_pdf"
	SigningJobPhaseSubmitAttemptToProvider   SigningJobPhase = "submit_attempt_to_provider"
	SigningJobPhaseAdvanceProviderSubmission SigningJobPhase = "advance_provider_submission"
	SigningJobPhaseReconcileProvider         SigningJobPhase = "reconcile_provider_submission"
	SigningJobPhaseRefreshProviderStatus     SigningJobPhase = "refresh_attempt_provider_status"
	SigningJobPhaseCleanupProviderAttempt    SigningJobPhase = "cleanup_provider_attempt"
	SigningJobPhaseDispatchCompletion        SigningJobPhase = "dispatch_attempt_completion"
)

// SigningExecutionUnitOfWork performs attempt state transitions and River enqueue atomically.
type SigningExecutionUnitOfWork interface {
	CreateAttemptAndEnqueueRender(ctx context.Context, documentID string, recipients []*entity.DocumentRecipient, signerOrders map[string]int) (*entity.SigningAttempt, error)
	// CreateDocumentAndStartSigning persists a new document, its recipients and its
	// first signing attempt in one transaction, so either all of them exist
	// afterwards or none does. On success document.ID and every recipient's ID and
	// DocumentID hold the persisted values; on error the arguments are unchanged.
	CreateDocumentAndStartSigning(ctx context.Context, document *entity.Document, recipients []*entity.DocumentRecipient, signerOrders map[string]int) (*entity.SigningAttempt, error)
	TransitionAndEnqueue(ctx context.Context, attempt *entity.SigningAttempt, nextPhase SigningJobPhase, eventType string) error
	Transition(ctx context.Context, attempt *entity.SigningAttempt, eventType string) error
	TerminateActiveAttempt(ctx context.Context, attempt *entity.SigningAttempt, status entity.SigningAttemptStatus, reason, eventType string) error
	SupersedeActiveAndCreateAttempt(ctx context.Context, documentID, expectedOldAttemptID, reason string, recipients []*entity.DocumentRecipient, signerOrders map[string]int) (*entity.SigningAttempt, error)
	// ApplyProviderWebhook merges a provider report into the attempt row read under
	// lock, never into a copy the caller loaded earlier.
	ApplyProviderWebhook(ctx context.Context, attemptID string, report entity.ProviderReport) (ProviderWebhookResult, error)
}

// ProviderWebhookResult describes what applying a provider webhook did to an attempt.
type ProviderWebhookResult struct {
	OldStatus     entity.SigningAttemptStatus
	NewStatus     entity.SigningAttemptStatus
	StatusChanged bool
}
