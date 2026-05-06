package riverqueue

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

// SigningExecutionUnitOfWork persists signing attempt transitions and River jobs
// in the same PostgreSQL transaction.
type SigningExecutionUnitOfWork struct {
	pool        *pgxpool.Pool
	client      *river.Client[pgx.Tx]
	attemptRepo port.SigningAttemptRepository
}

func NewSigningExecutionUnitOfWork(
	pool *pgxpool.Pool,
	client *river.Client[pgx.Tx],
	attemptRepo port.SigningAttemptRepository,
) *SigningExecutionUnitOfWork {
	return &SigningExecutionUnitOfWork{pool: pool, client: client, attemptRepo: attemptRepo}
}

func (u *SigningExecutionUnitOfWork) CreateAttemptAndEnqueueRender(
	ctx context.Context,
	documentID string,
	recipients []*entity.DocumentRecipient,
	signerOrders map[string]int,
) (*entity.SigningAttempt, error) {
	tx, err := u.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin signing attempt tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var activeAttemptID *string
	if err := tx.QueryRow(ctx, `SELECT active_attempt_id FROM execution.documents WHERE id = $1 FOR UPDATE`, documentID).Scan(&activeAttemptID); err != nil {
		return nil, fmt.Errorf("locking document for attempt creation: %w", err)
	}
	if activeAttemptID != nil && *activeAttemptID != "" {
		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit existing active attempt tx: %w", err)
		}
		return u.attemptRepo.FindByID(ctx, *activeAttemptID)
	}

	attempt, err := u.createAttemptInTx(ctx, tx, documentID, recipients, signerOrders)
	if err != nil {
		return nil, err
	}
	if err := u.setActiveProjectionTx(ctx, tx, attempt.DocumentID, attempt.ID, entity.ProjectDocumentStatusFromAttempt(attempt.Status)); err != nil {
		return nil, err
	}
	if err := u.enqueueTx(ctx, tx, port.SigningJobPhaseRenderAttemptPDF, attempt); err != nil {
		return nil, err
	}
	if err := u.insertEventTx(ctx, tx, attempt, nil, attempt.Status, "ATTEMPT_CREATED"); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit signing attempt tx: %w", err)
	}
	return attempt, nil
}

//nolint:funlen,gocognit,gocyclo,nestif
func (u *SigningExecutionUnitOfWork) SupersedeActiveAndCreateAttempt(
	ctx context.Context,
	documentID, expectedOldAttemptID, reason string,
	recipients []*entity.DocumentRecipient,
	signerOrders map[string]int,
) (*entity.SigningAttempt, error) {
	tx, err := u.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin regeneration tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	var activeAttemptID *string
	if err := tx.QueryRow(ctx, `SELECT active_attempt_id FROM execution.documents WHERE id = $1 FOR UPDATE`, documentID).Scan(&activeAttemptID); err != nil {
		return nil, fmt.Errorf("locking document for regeneration: %w", err)
	}
	if (activeAttemptID == nil || *activeAttemptID == "") && expectedOldAttemptID != "" {
		return nil, entity.ErrInvalidDocumentState
	}
	if expectedOldAttemptID != "" && (activeAttemptID == nil || *activeAttemptID != expectedOldAttemptID) {
		return nil, entity.ErrOptimisticLock
	}

	var oldAttempt *entity.SigningAttempt
	if activeAttemptID != nil && *activeAttemptID != "" {
		oldAttempt, err = u.lockAttemptTx(ctx, tx, *activeAttemptID)
		if err != nil {
			return nil, err
		}
		oldStatus := oldAttempt.Status
		now := time.Now().UTC()
		oldAttempt.Status = entity.SigningAttemptStatusSuperseded
		oldAttempt.InvalidationReason = &reason
		oldAttempt.TerminalAt = &now
		if oldAttempt.ProviderDocumentID != nil {
			cleanupStatus := "PENDING"
			oldAttempt.CleanupStatus = &cleanupStatus
		}
		if err := u.attemptRepo.UpdateTx(ctx, tx, oldAttempt); err != nil {
			return nil, err
		}
		if err := u.insertEventTx(ctx, tx, oldAttempt, &oldStatus, oldAttempt.Status, "ATTEMPT_SUPERSEDED"); err != nil {
			return nil, err
		}
		if oldAttempt.ProviderDocumentID != nil {
			if err := u.enqueueTx(ctx, tx, port.SigningJobPhaseCleanupProviderAttempt, oldAttempt); err != nil {
				return nil, err
			}
		}
	}

	attempt, err := u.createAttemptInTx(ctx, tx, documentID, recipients, signerOrders)
	if err != nil {
		return nil, err
	}
	if err := u.setActiveProjectionTx(ctx, tx, attempt.DocumentID, attempt.ID, entity.ProjectDocumentStatusFromAttempt(attempt.Status)); err != nil {
		return nil, err
	}
	if err := u.enqueueTx(ctx, tx, port.SigningJobPhaseRenderAttemptPDF, attempt); err != nil {
		return nil, err
	}
	if err := u.insertEventTx(ctx, tx, attempt, nil, attempt.Status, "ATTEMPT_CREATED"); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit regeneration tx: %w", err)
	}
	return attempt, nil
}

func (u *SigningExecutionUnitOfWork) TransitionAndEnqueue(ctx context.Context, attempt *entity.SigningAttempt, nextPhase port.SigningJobPhase, eventType string) error {
	if nextPhase == port.SigningJobPhaseSubmitAttemptToProvider && attempt.ProviderSubmitPhase == nil {
		phase := entity.ProviderSubmitPhaseCreateProviderDocument
		attempt.ProviderSubmitPhase = &phase
	}
	return u.transition(ctx, attempt, eventType, &nextPhase)
}

func (u *SigningExecutionUnitOfWork) Transition(ctx context.Context, attempt *entity.SigningAttempt, eventType string) error {
	return u.transition(ctx, attempt, eventType, nil)
}

func (u *SigningExecutionUnitOfWork) TerminateActiveAttempt(
	ctx context.Context,
	attempt *entity.SigningAttempt,
	status entity.SigningAttemptStatus,
	reason, eventType string,
) error {
	tx, err := u.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin attempt termination tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	current, err := u.lockAttemptTx(ctx, tx, attempt.ID)
	if err != nil {
		return err
	}
	oldStatus := current.Status
	now := time.Now().UTC()
	attempt.Status = status
	attempt.TerminalAt = &now
	attempt.InvalidationReason = &reason
	if attempt.ProviderDocumentID != nil {
		cleanupStatus := "PENDING"
		attempt.CleanupStatus = &cleanupStatus
	}
	if err := u.attemptRepo.UpdateTx(ctx, tx, attempt); err != nil {
		return err
	}
	if err := u.setDocumentProjectionIfActiveTx(ctx, tx, attempt); err != nil {
		return err
	}
	if eventType != "" {
		if err := u.insertEventTx(ctx, tx, attempt, &oldStatus, attempt.Status, eventType); err != nil {
			return err
		}
	}
	if attempt.ProviderDocumentID != nil {
		if err := u.enqueueTx(ctx, tx, port.SigningJobPhaseCleanupProviderAttempt, attempt); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit attempt termination tx: %w", err)
	}
	return nil
}

func (u *SigningExecutionUnitOfWork) transition(ctx context.Context, attempt *entity.SigningAttempt, eventType string, phase *port.SigningJobPhase) error {
	tx, err := u.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin attempt transition tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	current, err := u.lockAttemptTx(ctx, tx, attempt.ID)
	if err != nil {
		return err
	}
	oldStatus := current.Status
	if err := u.attemptRepo.UpdateTx(ctx, tx, attempt); err != nil {
		return err
	}
	if err := u.setDocumentProjectionIfActiveTx(ctx, tx, attempt); err != nil {
		return err
	}
	if eventType != "" {
		if err := u.insertEventTx(ctx, tx, attempt, &oldStatus, attempt.Status, eventType); err != nil {
			return err
		}
	}
	if phase != nil {
		if err := u.enqueueTx(ctx, tx, *phase, attempt); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit attempt transition tx: %w", err)
	}
	return nil
}

func (u *SigningExecutionUnitOfWork) createAttemptInTx(ctx context.Context, tx pgx.Tx, documentID string, recipients []*entity.DocumentRecipient, signerOrders map[string]int) (*entity.SigningAttempt, error) {
	seq, err := u.attemptRepo.NextSequenceTx(ctx, tx, documentID)
	if err != nil {
		return nil, err
	}
	attempt := &entity.SigningAttempt{DocumentID: documentID, Sequence: seq, Status: entity.SigningAttemptStatusCreated}
	if _, err := u.attemptRepo.CreateTx(ctx, tx, attempt); err != nil {
		return nil, err
	}
	for i, r := range recipients {
		order := signerOrders[r.TemplateVersionRoleID]
		if order == 0 {
			order = i + 1
		}
		var documentRecipientID *string
		if r.ID != "" {
			id := r.ID
			documentRecipientID = &id
		}
		rec := &entity.SigningAttemptRecipient{
			AttemptID:             attempt.ID,
			DocumentRecipientID:   documentRecipientID,
			TemplateVersionRoleID: r.TemplateVersionRoleID,
			SignerOrder:           order,
			Email:                 r.Email,
			Name:                  r.Name,
			Status:                entity.RecipientStatusPending,
		}
		if _, err := u.attemptRepo.CreateRecipientTx(ctx, tx, rec); err != nil {
			return nil, err
		}
	}
	return attempt, nil
}

func (u *SigningExecutionUnitOfWork) enqueueTx(ctx context.Context, tx pgx.Tx, phase port.SigningJobPhase, attempt *entity.SigningAttempt) error {
	var err error
	switch phase {
	case port.SigningJobPhaseRenderAttemptPDF:
		_, err = u.client.InsertTx(ctx, tx, RenderAttemptPDFArgs{AttemptID: attempt.ID}, nil)
	case port.SigningJobPhaseSubmitAttemptToProvider, port.SigningJobPhaseAdvanceProviderSubmission:
		_, err = u.client.InsertTx(ctx, tx, AdvanceProviderSubmissionArgs{
			AttemptID:           attempt.ID,
			ProviderSubmitPhase: providerSubmitPhaseArg(attempt.ProviderSubmitPhase),
		}, nil)
	case port.SigningJobPhaseReconcileProvider:
		_, err = u.client.InsertTx(ctx, tx, ReconcileProviderSubmissionArgs{AttemptID: attempt.ID}, nil)
	case port.SigningJobPhaseRefreshProviderStatus:
		_, err = u.client.InsertTx(ctx, tx, RefreshAttemptProviderStatusArgs{AttemptID: attempt.ID}, nil)
	case port.SigningJobPhaseCleanupProviderAttempt:
		_, err = u.client.InsertTx(ctx, tx, CleanupProviderAttemptArgs{AttemptID: attempt.ID}, nil)
	case port.SigningJobPhaseDispatchCompletion:
		_, err = u.client.InsertTx(ctx, tx, DispatchAttemptCompletionArgs{AttemptID: attempt.ID}, nil)
	default:
		return fmt.Errorf("unknown signing job phase %q", phase)
	}
	if err != nil {
		return fmt.Errorf("enqueue %s for attempt %s: %w", phase, attempt.ID, err)
	}
	return nil
}

func (u *SigningExecutionUnitOfWork) lockAttemptTx(ctx context.Context, tx pgx.Tx, attemptID string) (*entity.SigningAttempt, error) {
	row := tx.QueryRow(ctx, `
		SELECT id, document_id, sequence, status, render_started_at, pdf_storage_path, pdf_checksum,
		       pdf_checksum_algorithm, render_metadata, signature_field_snapshot, provider_upload_payload,
		       provider_name, provider_correlation_key, provider_document_id, provider_submit_phase,
		       retry_count, next_retry_at, last_error_class, last_error_message,
		       reconciliation_count, next_reconciliation_at, cleanup_status, cleanup_action, cleanup_error,
		       processing_lease_owner, processing_lease_expires_at, invalidation_reason,
		       created_at, updated_at, terminal_at
		FROM execution.signing_attempts WHERE id = $1 FOR UPDATE`, attemptID)
	return scanAttemptRow(row)
}

func (u *SigningExecutionUnitOfWork) setActiveProjectionTx(ctx context.Context, tx pgx.Tx, documentID, attemptID string, status entity.DocumentStatus) error {
	_, err := tx.Exec(ctx, `
		UPDATE execution.documents
		SET active_attempt_id = $2, status = $3, updated_at = now()
		WHERE id = $1`, documentID, attemptID, status)
	if err != nil {
		return fmt.Errorf("setting active attempt projection: %w", err)
	}
	return nil
}

func (u *SigningExecutionUnitOfWork) setDocumentProjectionIfActiveTx(ctx context.Context, tx pgx.Tx, attempt *entity.SigningAttempt) error {
	status := entity.ProjectDocumentStatusFromAttempt(attempt.Status)
	_, err := tx.Exec(ctx, `
		UPDATE execution.documents
		SET status = $3, completed_pdf_url = COALESCE($4, completed_pdf_url), updated_at = now()
		WHERE id = $1 AND active_attempt_id = $2`, attempt.DocumentID, attempt.ID, status, nil)
	if err != nil {
		return fmt.Errorf("updating active document projection: %w", err)
	}
	return nil
}

func (u *SigningExecutionUnitOfWork) insertEventTx(ctx context.Context, tx pgx.Tx, attempt *entity.SigningAttempt, oldStatus *entity.SigningAttemptStatus, newStatus entity.SigningAttemptStatus, eventType string) error {
	ev := &entity.SigningAttemptEvent{
		AttemptID:          attempt.ID,
		DocumentID:         attempt.DocumentID,
		EventType:          eventType,
		OldStatus:          oldStatus,
		NewStatus:          &newStatus,
		ProviderName:       attempt.ProviderName,
		ProviderDocumentID: attempt.ProviderDocumentID,
		CorrelationKey:     attempt.ProviderCorrelationKey,
		ErrorClass:         attempt.LastErrorClass,
	}
	return u.attemptRepo.InsertEventTx(ctx, tx, ev)
}

var _ port.SigningExecutionUnitOfWork = (*SigningExecutionUnitOfWork)(nil)
