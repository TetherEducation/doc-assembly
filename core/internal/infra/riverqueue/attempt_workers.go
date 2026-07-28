package riverqueue

import (
	"context"
	"time"

	"github.com/riverqueue/river"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

type RenderAttemptPDFWorker struct {
	river.WorkerDefaults[RenderAttemptPDFArgs]
	executor *SigningAttemptExecutor
}

func (w *RenderAttemptPDFWorker) Work(ctx context.Context, job *river.Job[RenderAttemptPDFArgs]) error {
	return w.executor.RenderAttemptPDF(ctx, job.Args.AttemptID)
}
func (w *RenderAttemptPDFWorker) Timeout(_ *river.Job[RenderAttemptPDFArgs]) time.Duration {
	return 2 * time.Minute
}

type SubmitAttemptToProviderWorker struct {
	river.WorkerDefaults[SubmitAttemptToProviderArgs]
	executor *SigningAttemptExecutor
}

func (w *SubmitAttemptToProviderWorker) Work(ctx context.Context, job *river.Job[SubmitAttemptToProviderArgs]) error {
	return w.executor.SubmitAttemptToProvider(ctx, job.Args.AttemptID)
}
func (w *SubmitAttemptToProviderWorker) Timeout(_ *river.Job[SubmitAttemptToProviderArgs]) time.Duration {
	return 2 * time.Minute
}

type AdvanceProviderSubmissionWorker struct {
	river.WorkerDefaults[AdvanceProviderSubmissionArgs]
	executor *SigningAttemptExecutor
}

func (w *AdvanceProviderSubmissionWorker) Work(ctx context.Context, job *river.Job[AdvanceProviderSubmissionArgs]) error {
	return w.executor.AdvanceProviderSubmissionForPhase(ctx, job.Args.AttemptID, entity.ProviderSubmitPhase(job.Args.ProviderSubmitPhase))
}
func (w *AdvanceProviderSubmissionWorker) Timeout(_ *river.Job[AdvanceProviderSubmissionArgs]) time.Duration {
	return 2 * time.Minute
}

type ReconcileProviderSubmissionWorker struct {
	river.WorkerDefaults[ReconcileProviderSubmissionArgs]
	executor *SigningAttemptExecutor
}

func (w *ReconcileProviderSubmissionWorker) Work(ctx context.Context, job *river.Job[ReconcileProviderSubmissionArgs]) error {
	return w.executor.ReconcileProviderSubmission(ctx, job.Args.AttemptID)
}
func (w *ReconcileProviderSubmissionWorker) Timeout(_ *river.Job[ReconcileProviderSubmissionArgs]) time.Duration {
	return time.Minute
}

type RefreshAttemptProviderStatusWorker struct {
	river.WorkerDefaults[RefreshAttemptProviderStatusArgs]
	executor *SigningAttemptExecutor
}

func (w *RefreshAttemptProviderStatusWorker) Work(ctx context.Context, job *river.Job[RefreshAttemptProviderStatusArgs]) error {
	return w.executor.RefreshAttemptProviderStatus(ctx, job.Args.AttemptID)
}
func (w *RefreshAttemptProviderStatusWorker) Timeout(_ *river.Job[RefreshAttemptProviderStatusArgs]) time.Duration {
	return time.Minute
}

type CleanupProviderAttemptWorker struct {
	river.WorkerDefaults[CleanupProviderAttemptArgs]
	executor *SigningAttemptExecutor
}

func (w *CleanupProviderAttemptWorker) Work(ctx context.Context, job *river.Job[CleanupProviderAttemptArgs]) error {
	return w.executor.CleanupProviderAttempt(ctx, job.Args.AttemptID)
}
func (w *CleanupProviderAttemptWorker) Timeout(_ *river.Job[CleanupProviderAttemptArgs]) time.Duration {
	return time.Minute
}

type DispatchAttemptCompletionWorker struct {
	river.WorkerDefaults[DispatchAttemptCompletionArgs]
	executor *SigningAttemptExecutor
}

func (w *DispatchAttemptCompletionWorker) Work(ctx context.Context, job *river.Job[DispatchAttemptCompletionArgs]) error {
	return w.executor.DispatchAttemptCompletion(ctx, job.Args.AttemptID)
}
func (w *DispatchAttemptCompletionWorker) Timeout(_ *river.Job[DispatchAttemptCompletionArgs]) time.Duration {
	return 30 * time.Second
}

var _ river.Worker[RenderAttemptPDFArgs] = (*RenderAttemptPDFWorker)(nil)
var _ river.Worker[SubmitAttemptToProviderArgs] = (*SubmitAttemptToProviderWorker)(nil)
var _ river.Worker[AdvanceProviderSubmissionArgs] = (*AdvanceProviderSubmissionWorker)(nil)
var _ river.Worker[ReconcileProviderSubmissionArgs] = (*ReconcileProviderSubmissionWorker)(nil)
var _ river.Worker[RefreshAttemptProviderStatusArgs] = (*RefreshAttemptProviderStatusWorker)(nil)
var _ river.Worker[CleanupProviderAttemptArgs] = (*CleanupProviderAttemptWorker)(nil)
var _ river.Worker[DispatchAttemptCompletionArgs] = (*DispatchAttemptCompletionWorker)(nil)
