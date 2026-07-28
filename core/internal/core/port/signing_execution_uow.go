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
	TransitionAndEnqueue(ctx context.Context, attempt *entity.SigningAttempt, nextPhase SigningJobPhase, eventType string) error
	Transition(ctx context.Context, attempt *entity.SigningAttempt, eventType string) error
	TerminateActiveAttempt(ctx context.Context, attempt *entity.SigningAttempt, status entity.SigningAttemptStatus, reason, eventType string) error
	SupersedeActiveAndCreateAttempt(ctx context.Context, documentID, expectedOldAttemptID, reason string, recipients []*entity.DocumentRecipient, signerOrders map[string]int) (*entity.SigningAttempt, error)
}
