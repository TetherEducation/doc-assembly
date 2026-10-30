//go:build integration

package riverqueue_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/database/postgres/document_field_response_repo"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/database/postgres/document_recipient_repo"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/database/postgres/document_repo"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/database/postgres/signing_attempt_repo"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/database/postgres/template_version_repo"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/database/postgres/template_version_signer_role_repo"
	signingmock "github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/signing/mock"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/storage/local"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	documentsvc "github.com/TetherEducation/doc-assembly/core/internal/core/service/document"
	"github.com/TetherEducation/doc-assembly/core/internal/infra/config"
	"github.com/TetherEducation/doc-assembly/core/internal/infra/riverqueue"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

func TestSigningAttemptUOW_CreateAttemptEnqueuesRenderAtomically(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	riverSvc, err := riverqueue.New(ctx, fx.pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{DocumentRepo: fx.docRepo, AttemptRepo: fx.attemptRepo})
	require.NoError(t, err)

	attempt, err := riverSvc.SigningExecutionUOW().CreateAttemptAndEnqueueRender(ctx, fx.documentID, fx.recipients(), fx.signerOrders())
	require.NoError(t, err)
	require.NotEmpty(t, attempt.ID)

	var activeAttemptID string
	var status entity.DocumentStatus
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT active_attempt_id, status FROM execution.documents WHERE id=$1`, fx.documentID).Scan(&activeAttemptID, &status))
	require.Equal(t, attempt.ID, activeAttemptID)
	require.Equal(t, entity.DocumentStatusPreparingSignature, status)

	var jobs int
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind = 'render_attempt_pdf' AND args->>'attempt_id' = $1`, attempt.ID).Scan(&jobs))
	require.Equal(t, 1, jobs)
}

// Direct send at creation: the document, its recipients, the attempt and the
// render job commit together.
func TestSigningAttemptUOW_CreateDocumentAndStartSigningCommitsTogether(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	uow := fx.documentCreationUOW(t, ctx)

	doc := fx.newUnsavedDocument("direct-send-commit")
	recipients := fx.newUnsavedRecipients()
	attempt, err := uow.CreateDocumentAndStartSigning(ctx, doc, recipients, fx.signerOrders())
	require.NoError(t, err)
	require.NotEmpty(t, doc.ID)
	require.Equal(t, doc.ID, attempt.DocumentID)
	require.NotEmpty(t, recipients[0].ID)
	require.Equal(t, doc.ID, recipients[0].DocumentID)

	var status entity.DocumentStatus
	var activeAttemptID string
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT status, active_attempt_id FROM execution.documents WHERE id=$1`, doc.ID).Scan(&status, &activeAttemptID))
	require.Equal(t, entity.DocumentStatusPreparingSignature, status)
	require.Equal(t, attempt.ID, activeAttemptID)

	var documentRecipients, linkedAttemptRecipients, renderJobs, createdEvents int
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT count(*) FROM execution.document_recipients WHERE document_id=$1`, doc.ID).Scan(&documentRecipients))
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT count(*) FROM execution.signing_attempt_recipients WHERE attempt_id=$1 AND document_recipient_id=$2`, attempt.ID, recipients[0].ID).Scan(&linkedAttemptRecipients))
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='render_attempt_pdf' AND args->>'attempt_id'=$1`, attempt.ID).Scan(&renderJobs))
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT count(*) FROM execution.signing_attempt_events WHERE attempt_id=$1 AND event_type='ATTEMPT_CREATED'`, attempt.ID).Scan(&createdEvents))
	require.Equal(t, 1, documentRecipients)
	require.Equal(t, 1, linkedAttemptRecipients, "the attempt recipient must link to the persisted document recipient")
	require.Equal(t, 1, renderJobs)
	require.Equal(t, 1, createdEvents)
}

// Direct send's partial-success bug (2026-10-13 review, P2): the River insert
// fails after the document rows are written. Nothing may survive the failure,
// and the caller's retry must end with one document and one active attempt.
func TestSigningAttemptUOW_CreateDocumentAndStartSigningRollsBackWhenEnqueueFails(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	uow := fx.documentCreationUOW(t, ctx)
	const title = "direct-send-rollback"

	fx.failRenderJobInserts(t, ctx)
	doc := fx.newUnsavedDocument(title)
	recipients := fx.newUnsavedRecipients()
	_, err := uow.CreateDocumentAndStartSigning(ctx, doc, recipients, fx.signerOrders())
	require.Error(t, err)
	require.Contains(t, err.Error(), "injected render job insert failure")
	require.Empty(t, doc.ID, "a failed creation leaves the caller's document unchanged")
	require.Empty(t, recipients[0].ID)
	require.Equal(t, 0, fx.countDocumentsTitled(t, ctx, title), "a failed start must not leave the document behind")

	fx.allowRenderJobInserts(t, ctx)
	retryDoc := fx.newUnsavedDocument(title)
	attempt, err := uow.CreateDocumentAndStartSigning(ctx, retryDoc, fx.newUnsavedRecipients(), fx.signerOrders())
	require.NoError(t, err)

	require.Equal(t, 1, fx.countDocumentsTitled(t, ctx, title), "the retry must leave exactly one document")
	var attempts, active int
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT count(*) FROM execution.signing_attempts WHERE document_id=$1`, retryDoc.ID).Scan(&attempts))
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT count(*) FROM execution.documents WHERE id=$1 AND active_attempt_id=$2`, retryDoc.ID, attempt.ID).Scan(&active))
	require.Equal(t, 1, attempts)
	require.Equal(t, 1, active)
}

func TestSigningAttemptUOW_TransitionSubmitPhaseEnqueuesAdvanceProviderSubmission(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	riverSvc, err := riverqueue.New(ctx, fx.pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{DocumentRepo: fx.docRepo, AttemptRepo: fx.attemptRepo})
	require.NoError(t, err)

	attempt, err := riverSvc.SigningExecutionUOW().CreateAttemptAndEnqueueRender(ctx, fx.documentID, fx.recipients(), fx.signerOrders())
	require.NoError(t, err)
	attempt.Status = entity.SigningAttemptStatusReadyToSubmit
	require.NoError(t, riverSvc.SigningExecutionUOW().TransitionAndEnqueue(ctx, attempt, port.SigningJobPhaseSubmitAttemptToProvider, "ATTEMPT_PDF_READY"))

	var advanceJobs, legacyJobs int
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='advance_provider_submission' AND args->>'attempt_id'=$1`, attempt.ID).Scan(&advanceJobs))
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='submit_attempt_to_provider' AND args->>'attempt_id'=$1`, attempt.ID).Scan(&legacyJobs))
	require.Equal(t, 1, advanceJobs)
	require.Equal(t, 0, legacyJobs)
}

func TestSigningAttemptUOW_CreateAttemptIsIdempotentUnderConcurrency(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	riverSvc, err := riverqueue.New(ctx, fx.pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{DocumentRepo: fx.docRepo, AttemptRepo: fx.attemptRepo})
	require.NoError(t, err)

	const callers = 8
	var wg sync.WaitGroup
	ids := make(chan string, callers)
	errs := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			attempt, callErr := riverSvc.SigningExecutionUOW().CreateAttemptAndEnqueueRender(ctx, fx.documentID, fx.recipients(), fx.signerOrders())
			if callErr != nil {
				errs <- callErr
				return
			}
			ids <- attempt.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	var first string
	for id := range ids {
		if first == "" {
			first = id
		}
		require.Equal(t, first, id)
	}
	require.NotEmpty(t, first)

	var attempts, renderJobs int
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT count(*) FROM execution.signing_attempts WHERE document_id=$1`, fx.documentID).Scan(&attempts))
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='render_attempt_pdf' AND args->>'attempt_id'=$1`, first).Scan(&renderJobs))
	require.Equal(t, 1, attempts)
	require.Equal(t, 1, renderJobs)
}

func TestSigningAttemptUOW_SupersedeCreatesNewAttemptAndCleanupJob(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	riverSvc, err := riverqueue.New(ctx, fx.pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{DocumentRepo: fx.docRepo, AttemptRepo: fx.attemptRepo})
	require.NoError(t, err)

	oldAttempt, err := riverSvc.SigningExecutionUOW().CreateAttemptAndEnqueueRender(ctx, fx.documentID, fx.recipients(), fx.signerOrders())
	require.NoError(t, err)
	_, err = fx.pool.Exec(ctx, `
		UPDATE execution.signing_attempts
		SET provider_name='mock', provider_document_id='provider-old'
		WHERE id=$1`, oldAttempt.ID)
	require.NoError(t, err)

	newAttempt, err := riverSvc.SigningExecutionUOW().SupersedeActiveAndCreateAttempt(ctx, fx.documentID, oldAttempt.ID, "regenerate", fx.recipients(), fx.signerOrders())
	require.NoError(t, err)
	require.NotEqual(t, oldAttempt.ID, newAttempt.ID)

	var oldStatus entity.SigningAttemptStatus
	var activeAttemptID string
	var cleanupJobs int
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT status FROM execution.signing_attempts WHERE id=$1`, oldAttempt.ID).Scan(&oldStatus))
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT active_attempt_id FROM execution.documents WHERE id=$1`, fx.documentID).Scan(&activeAttemptID))
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='cleanup_provider_attempt' AND args->>'attempt_id'=$1`, oldAttempt.ID).Scan(&cleanupJobs))
	require.Equal(t, entity.SigningAttemptStatusSuperseded, oldStatus)
	require.Equal(t, newAttempt.ID, activeAttemptID)
	require.Equal(t, 1, cleanupJobs)
}

func TestSigningAttemptExecutor_CleanupProviderAttemptRecordsHistoricalSupersededAttempt(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	provider := signingmock.New()
	riverSvc, err := riverqueue.New(ctx, fx.pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{DocumentRepo: fx.docRepo, AttemptRepo: fx.attemptRepo})
	require.NoError(t, err)

	oldAttempt, err := riverSvc.SigningExecutionUOW().CreateAttemptAndEnqueueRender(ctx, fx.documentID, fx.recipients(), fx.signerOrders())
	require.NoError(t, err)
	corr := fmt.Sprintf("%s:%s", fx.documentID, oldAttempt.ID)
	providerDoc, err := provider.EnsureProviderDocument(ctx, &port.EnsureProviderDocumentRequest{
		AttemptID:      oldAttempt.ID,
		DocumentID:     fx.documentID,
		CorrelationKey: corr,
		PDF:            []byte("%PDF-1.4 cleanup regression\n"),
		PDFChecksum:    "cleanup",
		Title:          "cleanup regression",
		Environment:    entity.EnvironmentProd,
	})
	require.NoError(t, err)
	providerName := provider.ProviderName()
	oldAttempt.ProviderName = &providerName
	oldAttempt.ProviderCorrelationKey = &corr
	oldAttempt.ProviderDocumentID = &providerDoc.ProviderDocumentID
	require.NoError(t, fx.attemptRepo.Update(ctx, oldAttempt))

	newAttempt, err := riverSvc.SigningExecutionUOW().SupersedeActiveAndCreateAttempt(
		ctx,
		fx.documentID,
		oldAttempt.ID,
		"cleanup regression",
		fx.recipients(),
		fx.signerOrders(),
	)
	require.NoError(t, err)

	executor := riverqueue.NewSigningAttemptExecutor(riverqueue.SigningAttemptExecutorConfig{
		Pool:            fx.pool,
		DocumentRepo:    fx.docRepo,
		AttemptRepo:     fx.attemptRepo,
		SigningProvider: provider,
	})
	require.NoError(t, executor.CleanupProviderAttempt(ctx, oldAttempt.ID))

	cleaned := fx.mustAttempt(t, ctx, oldAttempt.ID)
	require.Equal(t, entity.SigningAttemptStatusSuperseded, cleaned.Status)
	require.NotNil(t, cleaned.CleanupStatus)
	require.Equal(t, "SUCCEEDED", *cleaned.CleanupStatus)
	require.NotNil(t, cleaned.CleanupAction)
	require.Equal(t, "CANCEL", *cleaned.CleanupAction)
	require.Nil(t, cleaned.CleanupError)

	var activeAttemptID string
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT active_attempt_id FROM execution.documents WHERE id=$1`, fx.documentID).Scan(&activeAttemptID))
	require.Equal(t, newAttempt.ID, activeAttemptID)

	var cleanupEvents int
	require.NoError(t, fx.pool.QueryRow(ctx, `
		SELECT count(*) FROM execution.signing_attempt_events
		WHERE attempt_id=$1 AND event_type='ATTEMPT_PROVIDER_CLEANUP_FINISHED'`, oldAttempt.ID).Scan(&cleanupEvents))
	require.Equal(t, 1, cleanupEvents)
}

func TestSigningAttemptConstraints_ActiveAttemptMustBelongToDocument(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	riverSvc, err := riverqueue.New(ctx, fx.pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{DocumentRepo: fx.docRepo, AttemptRepo: fx.attemptRepo})
	require.NoError(t, err)

	attempt, err := riverSvc.SigningExecutionUOW().CreateAttemptAndEnqueueRender(ctx, fx.documentID, fx.recipients(), fx.signerOrders())
	require.NoError(t, err)
	otherDocID := fx.createDocument(t, ctx, "other-doc")

	_, err = fx.pool.Exec(ctx, `UPDATE execution.documents SET active_attempt_id=$1 WHERE id=$2`, attempt.ID, otherDocID)
	require.Error(t, err)
}

func TestSigningAttemptExecutor_CompletedEventCarriesDocumentTypeCode(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	riverSvc, err := riverqueue.New(ctx, fx.pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{DocumentRepo: fx.docRepo, AttemptRepo: fx.attemptRepo})
	require.NoError(t, err)

	attempt, err := riverSvc.SigningExecutionUOW().CreateAttemptAndEnqueueRender(ctx, fx.documentID, fx.recipients(), fx.signerOrders())
	require.NoError(t, err)
	_, err = fx.pool.Exec(ctx, `UPDATE execution.signing_attempts SET status=$1 WHERE id=$2`, entity.SigningAttemptStatusCompleted, attempt.ID)
	require.NoError(t, err)

	var captured port.DocumentCompletedEvent
	var calls atomic.Int32
	executor := riverqueue.NewSigningAttemptExecutor(riverqueue.SigningAttemptExecutorConfig{
		Pool:         fx.pool,
		DocumentRepo: fx.docRepo,
		AttemptRepo:  fx.attemptRepo,
		CompletionHandler: port.DocumentCompletedHandler(func(_ context.Context, ev port.DocumentCompletedEvent) error {
			captured = ev
			calls.Add(1)
			return nil
		}),
	})
	require.NoError(t, executor.DispatchAttemptCompletion(ctx, attempt.ID))
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, fx.documentID, captured.DocumentID)
	require.NotNil(t, captured.DocumentTypeCode)
	require.Equal(t, fx.documentTypeCode, *captured.DocumentTypeCode)
	require.Len(t, captured.Recipients, 1)
}

func TestSigningAttemptExecutor_StaleCompletionDispatchIsNoop(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	riverSvc, err := riverqueue.New(ctx, fx.pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{DocumentRepo: fx.docRepo, AttemptRepo: fx.attemptRepo})
	require.NoError(t, err)

	oldAttempt, err := riverSvc.SigningExecutionUOW().CreateAttemptAndEnqueueRender(ctx, fx.documentID, fx.recipients(), fx.signerOrders())
	require.NoError(t, err)
	newAttempt, err := riverSvc.SigningExecutionUOW().SupersedeActiveAndCreateAttempt(ctx, fx.documentID, oldAttempt.ID, "regenerate", fx.recipients(), fx.signerOrders())
	require.NoError(t, err)
	require.NotEqual(t, oldAttempt.ID, newAttempt.ID)

	var calls atomic.Int32
	executor := riverqueue.NewSigningAttemptExecutor(riverqueue.SigningAttemptExecutorConfig{
		Pool:         fx.pool,
		DocumentRepo: fx.docRepo,
		AttemptRepo:  fx.attemptRepo,
		CompletionHandler: port.DocumentCompletedHandler(func(context.Context, port.DocumentCompletedEvent) error {
			calls.Add(1)
			return nil
		}),
	})
	require.NoError(t, executor.DispatchAttemptCompletion(ctx, oldAttempt.ID))
	require.Equal(t, int32(0), calls.Load())
}

func TestSigningExecutionUnitOfWork_TransitionAndEnqueueNoopsForTerminalAttempt(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	riverSvc, err := riverqueue.New(ctx, fx.pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{DocumentRepo: fx.docRepo, AttemptRepo: fx.attemptRepo})
	require.NoError(t, err)
	attempt, err := riverSvc.SigningExecutionUOW().CreateAttemptAndEnqueueRender(ctx, fx.documentID, fx.recipients(), fx.signerOrders())
	require.NoError(t, err)

	stale := *attempt
	providerDocumentID := "provider-terminal"
	_, err = fx.pool.Exec(ctx, `
		UPDATE execution.signing_attempts
		SET status=$2, provider_document_id=$3, terminal_at=now()
		WHERE id=$1`, attempt.ID, entity.SigningAttemptStatusCompleted, providerDocumentID)
	require.NoError(t, err)

	stale.Status = entity.SigningAttemptStatusSigningReady
	stale.ProviderDocumentID = &providerDocumentID
	require.NoError(t, riverSvc.SigningExecutionUOW().TransitionAndEnqueue(ctx, &stale, port.SigningJobPhaseRefreshProviderStatus, "STALE_PUBLIC_REFRESH"))

	current := fx.mustAttempt(t, ctx, attempt.ID)
	require.Equal(t, entity.SigningAttemptStatusCompleted, current.Status)

	var refreshJobs int
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='refresh_attempt_provider_status' AND args->>'attempt_id'=$1`, attempt.ID).Scan(&refreshJobs))
	require.Equal(t, 0, refreshJobs)
}

func TestSigningAttemptExecutor_ReconcileProviderPartialEnvelopeResumesWithoutUnknownLoop(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	provider := signingmock.New()
	executor, storage, _ := newProviderSubmissionExecutor(t, ctx, fx, provider, nil)
	attempt := fx.createProviderReadyAttempt(t, ctx, provider, storage)
	corr := *attempt.ProviderCorrelationKey

	envelope, err := provider.EnsureProviderDocument(ctx, &port.EnsureProviderDocumentRequest{
		AttemptID:      attempt.ID,
		DocumentID:     fx.documentID,
		CorrelationKey: corr,
		PDF:            []byte("%PDF-1.4 partial envelope\n"),
		PDFChecksum:    "partial",
		Title:          "Partial envelope",
		Environment:    entity.EnvironmentProd,
	})
	require.NoError(t, err)
	_, err = fx.pool.Exec(ctx, `
		UPDATE execution.signing_attempts
		SET status=$2, provider_document_id=$3, provider_submit_phase=NULL
		WHERE id=$1`,
		attempt.ID,
		entity.SigningAttemptStatusSubmissionUnknown,
		envelope.ProviderDocumentID,
	)
	require.NoError(t, err)

	require.NoError(t, executor.ReconcileProviderSubmission(ctx, attempt.ID))
	reconciled := fx.mustAttempt(t, ctx, attempt.ID)
	require.Equal(t, entity.SigningAttemptStatusSubmissionUnknown, reconciled.Status)
	require.NotNil(t, reconciled.ProviderSubmitPhase)
	require.Equal(t, entity.ProviderSubmitPhaseAddRecipients, *reconciled.ProviderSubmitPhase)

	fx.drainProviderSubmission(t, ctx, executor, attempt.ID)
	finalAttempt := fx.mustAttempt(t, ctx, attempt.ID)
	require.Equal(t, entity.SigningAttemptStatusSigningReady, finalAttempt.Status)
	require.Nil(t, finalAttempt.ProviderSubmitPhase)
	require.NotNil(t, finalAttempt.ProviderDocumentID)
	recipients, err := fx.attemptRepo.FindRecipientsByAttemptID(ctx, attempt.ID)
	require.NoError(t, err)
	require.Len(t, recipients, 1)
	require.NotNil(t, recipients[0].ProviderRecipientID)
	require.NotNil(t, recipients[0].SigningURL)
}

func TestSigningAttemptExecutor_RefreshProviderStatusPropagatesSignedRecipients(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	provider := signingmock.New()
	executor, storage, _ := newProviderSubmissionExecutor(t, ctx, fx, provider, nil)
	attempt := fx.createProviderReadyAttempt(t, ctx, provider, storage)
	fx.drainProviderSubmission(t, ctx, executor, attempt.ID)

	ready := fx.mustAttempt(t, ctx, attempt.ID)
	require.Equal(t, entity.SigningAttemptStatusSigningReady, ready.Status)
	require.NotNil(t, ready.ProviderDocumentID)

	attemptRecipients, err := fx.attemptRepo.FindRecipientsByAttemptID(ctx, attempt.ID)
	require.NoError(t, err)
	require.Len(t, attemptRecipients, 1)
	require.NotNil(t, attemptRecipients[0].ProviderRecipientID)

	documentRecipientID := fx.createDocumentRecipient(t, ctx)
	_, err = fx.pool.Exec(ctx, `
		UPDATE execution.signing_attempt_recipients
		SET document_recipient_id=$1
		WHERE id=$2`, documentRecipientID, attemptRecipients[0].ID)
	require.NoError(t, err)

	provider.SimulateComplete(*ready.ProviderDocumentID)
	require.NoError(t, executor.RefreshAttemptProviderStatus(ctx, attempt.ID))

	refreshed := fx.mustAttempt(t, ctx, attempt.ID)
	require.Equal(t, entity.SigningAttemptStatusCompleted, refreshed.Status)

	attemptRecipients, err = fx.attemptRepo.FindRecipientsByAttemptID(ctx, attempt.ID)
	require.NoError(t, err)
	require.Len(t, attemptRecipients, 1)
	require.Equal(t, entity.RecipientStatusSigned, attemptRecipients[0].Status)
	require.NotNil(t, attemptRecipients[0].SignedAt)

	var documentStatus entity.DocumentStatus
	var documentRecipientStatus entity.RecipientStatus
	var documentRecipientSignedAt *time.Time
	require.NoError(t, fx.pool.QueryRow(ctx, `
		SELECT d.status, dr.status, dr.signed_at
		FROM execution.documents d
		JOIN execution.document_recipients dr ON dr.document_id=d.id
		WHERE d.id=$1 AND dr.id=$2`, fx.documentID, documentRecipientID).
		Scan(&documentStatus, &documentRecipientStatus, &documentRecipientSignedAt))
	require.Equal(t, entity.DocumentStatusCompleted, documentStatus)
	require.Equal(t, entity.RecipientStatusSigned, documentRecipientStatus)
	require.NotNil(t, documentRecipientSignedAt)

	var dispatchJobs int
	require.NoError(t, fx.pool.QueryRow(ctx, `
		SELECT count(*)
		FROM river_job
		WHERE kind='dispatch_attempt_completion' AND args->>'attempt_id'=$1`, attempt.ID).Scan(&dispatchJobs))
	require.Equal(t, 1, dispatchJobs)
}

func TestSigningAttemptExecutor_TransientProviderErrorReturnsForRiverRetryAndResumes(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	baseProvider := signingmock.New()
	provider := &transientRecipientsProvider{SigningProvider: baseProvider}
	executor, storage, _ := newProviderSubmissionExecutor(t, ctx, fx, provider, nil)
	attempt := fx.createProviderReadyAttempt(t, ctx, provider, storage)

	require.NoError(t, executor.AdvanceProviderSubmission(ctx, attempt.ID))
	err := executor.AdvanceProviderSubmission(ctx, attempt.ID)
	require.Error(t, err)
	require.Contains(t, err.Error(), "temporary recipient failure")

	retrying := fx.mustAttempt(t, ctx, attempt.ID)
	require.Equal(t, entity.SigningAttemptStatusProviderRetryWaiting, retrying.Status)
	require.NotNil(t, retrying.ProviderSubmitPhase)
	require.Equal(t, entity.ProviderSubmitPhaseAddRecipients, *retrying.ProviderSubmitPhase)

	fx.drainProviderSubmission(t, ctx, executor, attempt.ID)
	finalAttempt := fx.mustAttempt(t, ctx, attempt.ID)
	require.Equal(t, entity.SigningAttemptStatusSigningReady, finalAttempt.Status)
	require.Nil(t, finalAttempt.ProviderSubmitPhase)
}

func TestSigningAttemptExecutor_ProviderTransitionNoopsWhenAttemptSupersededAfterProviderCall(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	baseProvider := signingmock.New()
	var oldAttemptID string
	provider := &afterDocumentProvider{SigningProvider: baseProvider}
	riverSvc, err := riverqueue.New(ctx, fx.pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{DocumentRepo: fx.docRepo, AttemptRepo: fx.attemptRepo})
	require.NoError(t, err)
	provider.after = func() error {
		old := fx.mustAttempt(t, ctx, oldAttemptID)
		_, supersedeErr := riverSvc.SigningExecutionUOW().SupersedeActiveAndCreateAttempt(
			ctx,
			fx.documentID,
			old.ID,
			"provider race regression",
			fx.recipients(),
			fx.signerOrders(),
		)
		return supersedeErr
	}
	executor, storage, _ := newProviderSubmissionExecutor(t, ctx, fx, provider, nil)
	attempt := fx.createProviderReadyAttempt(t, ctx, provider, storage)
	oldAttemptID = attempt.ID

	require.NoError(t, executor.AdvanceProviderSubmission(ctx, attempt.ID))

	oldAttempt := fx.mustAttempt(t, ctx, attempt.ID)
	require.Equal(t, entity.SigningAttemptStatusSuperseded, oldAttempt.Status)
	require.Nil(t, oldAttempt.ProviderDocumentID)
	require.NotNil(t, oldAttempt.ProviderSubmitPhase)
	require.Equal(t, entity.ProviderSubmitPhaseCreateProviderDocument, *oldAttempt.ProviderSubmitPhase)

	var activeAttemptID string
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT active_attempt_id FROM execution.documents WHERE id=$1`, fx.documentID).Scan(&activeAttemptID))
	require.NotEqual(t, attempt.ID, activeAttemptID)

	var staleAdvanceJobs int
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='advance_provider_submission' AND args->>'attempt_id'=$1`, attempt.ID).Scan(&staleAdvanceJobs))
	require.Equal(t, 0, staleAdvanceJobs)
}

func TestSigningAttemptExecutor_ProviderPhaseTransitionNoopsWhenPhaseAdvancedAfterProviderCall(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	baseProvider := signingmock.New()
	var attemptID string
	provider := &afterDocumentProvider{SigningProvider: baseProvider}
	provider.after = func() error {
		phase := entity.ProviderSubmitPhaseCreateFields
		_, err := fx.pool.Exec(ctx, `
			UPDATE execution.signing_attempts
			SET status=$2, provider_document_id=$3, provider_submit_phase=$4
			WHERE id=$1`,
			attemptID,
			entity.SigningAttemptStatusSubmittingProvider,
			"provider-doc-current",
			phase,
		)
		return err
	}
	executor, storage, _ := newProviderSubmissionExecutor(t, ctx, fx, provider, nil)
	attempt := fx.createProviderReadyAttempt(t, ctx, provider, storage)
	attemptID = attempt.ID

	require.NoError(t, executor.AdvanceProviderSubmissionForPhase(ctx, attempt.ID, entity.ProviderSubmitPhaseCreateProviderDocument))

	current := fx.mustAttempt(t, ctx, attempt.ID)
	require.Equal(t, entity.SigningAttemptStatusSubmittingProvider, current.Status)
	require.NotNil(t, current.ProviderSubmitPhase)
	require.Equal(t, entity.ProviderSubmitPhaseCreateFields, *current.ProviderSubmitPhase)
	require.NotNil(t, current.ProviderDocumentID)
	require.Equal(t, "provider-doc-current", *current.ProviderDocumentID)

	var staleAdvanceJobs int
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='advance_provider_submission' AND args->>'attempt_id'=$1`, attempt.ID).Scan(&staleAdvanceJobs))
	require.Equal(t, 0, staleAdvanceJobs)
}

func TestSigningAttemptExecutor_AdvanceProviderSubmissionResumesAfterProviderStepFailpoints(t *testing.T) {
	ctx := context.Background()
	failpoints := []string{
		"submit_after_envelope_before_recipients",
		"submit_after_recipients_before_fields",
		"submit_after_fields_before_distribute",
		"submit_after_distribute_before_refs",
	}
	for _, failpoint := range failpoints {
		t.Run(failpoint, func(t *testing.T) {
			fx := newAttemptFixture(t, ctx)
			provider := signingmock.New()
			failingExecutor, storage, _ := newProviderSubmissionExecutor(t, ctx, fx, provider, riverqueue.AttemptFailpoints{failpoint: true})
			attempt := fx.createProviderReadyAttempt(t, ctx, provider, storage)

			var failErr error
			for range 5 {
				failErr = failingExecutor.AdvanceProviderSubmission(ctx, attempt.ID)
				if failErr != nil {
					break
				}
			}
			require.Error(t, failErr)
			require.True(t, strings.Contains(failErr.Error(), failpoint), "error %q did not include failpoint %q", failErr.Error(), failpoint)

			resumeExecutor := newProviderSubmissionExecutorWithStorage(t, fx, provider, storage, nil)
			fx.drainProviderSubmission(t, ctx, resumeExecutor, attempt.ID)
			finalAttempt := fx.mustAttempt(t, ctx, attempt.ID)
			require.Equal(t, entity.SigningAttemptStatusSigningReady, finalAttempt.Status)
			require.Nil(t, finalAttempt.ProviderSubmitPhase)
		})
	}
}

// The template's language must reach the provider: the render phase stores it
// with the attempt, and the create phase passes it on so Documenso sends the
// envelope's emails and signing certificate in the document's language.
func TestSigningAttemptExecutor_TemplateLanguageReachesProviderDocument(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	_, err := fx.pool.Exec(ctx, `
		UPDATE content.template_versions
		SET content_structure = jsonb_set(content_structure, '{meta,language}', '"es"')
		WHERE id = $1`, fx.versionID)
	require.NoError(t, err)

	provider := &languageCapturingProvider{SigningProvider: signingmock.New()}
	storage, err := local.New(t.TempDir())
	require.NoError(t, err)
	client, err := river.NewClient(riverpgxv5.New(fx.pool), &river.Config{})
	require.NoError(t, err)
	executor := riverqueue.NewSigningAttemptExecutor(riverqueue.SigningAttemptExecutorConfig{
		Pool:              fx.pool,
		Client:            client,
		DocumentRepo:      fx.docRepo,
		RecipientRepo:     documentrecipientrepo.New(fx.pool),
		AttemptRepo:       fx.attemptRepo,
		VersionRepo:       templateversionrepo.New(fx.pool),
		SignerRoleRepo:    templateversionsignerrolerepo.New(fx.pool),
		FieldResponseRepo: documentfieldresponserepo.New(fx.pool),
		PDFRenderer:       staticPDFRenderer{},
		SigningProvider:   provider,
		StorageAdapter:    storage,
		StorageEnabled:    true,
	})
	riverSvc, err := riverqueue.New(ctx, fx.pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{DocumentRepo: fx.docRepo, AttemptRepo: fx.attemptRepo})
	require.NoError(t, err)
	attempt, err := riverSvc.SigningExecutionUOW().CreateAttemptAndEnqueueRender(ctx, fx.documentID, fx.recipients(), fx.signerOrders())
	require.NoError(t, err)

	require.NoError(t, executor.RenderAttemptPDF(ctx, attempt.ID))
	rendered := fx.mustAttempt(t, ctx, attempt.ID)
	require.Equal(t, entity.SigningAttemptStatusReadyToSubmit, rendered.Status)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(rendered.ProviderUploadPayload, &payload))
	require.Equal(t, "es", payload["language"])

	require.NoError(t, executor.AdvanceProviderSubmissionForPhase(ctx, attempt.ID, entity.ProviderSubmitPhaseCreateProviderDocument))
	require.Equal(t, []string{"es"}, provider.languages)
}

// Documenso sends DOCUMENT_CREATED as soon as the envelope exists, while the worker
// is still adding recipients and fields. In production the webhook read the attempt,
// the worker committed the next phase, and the webhook then saved its copy back as
// SIGNING_READY: the next phase job found a status it does not own, exited as a
// no-op, and the signer waited on "preparing document" forever.
func TestProviderWebhook_DocumentCreatedDuringSubmissionDoesNotStrandAttempt(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	provider := signingmock.New()
	executor, storage, _ := newProviderSubmissionExecutor(t, ctx, fx, provider, nil)
	attempt := fx.createProviderReadyAttempt(t, ctx, provider, storage)

	// The worker creates the envelope and commits ADD_RECIPIENTS.
	require.NoError(t, executor.AdvanceProviderSubmissionForPhase(ctx, attempt.ID, entity.ProviderSubmitPhaseCreateProviderDocument))

	// The webhook reads the attempt, the worker adds recipients and commits
	// CREATE_FIELDS, and only then does the webhook persist.
	attempts := &advanceAfterWebhookReadRepo{SigningAttemptRepository: fx.attemptRepo}
	attempts.afterRead = func() {
		require.True(t, fx.runNextQueuedAdvanceJob(t, ctx, executor, attempt.ID), "expected the ADD_RECIPIENTS job to be queued")
	}
	svc := fx.newWebhookService(t, ctx, attempts)
	signingReady := entity.SigningAttemptStatusSigningReady
	require.NoError(t, svc.HandleWebhookEvent(ctx, &port.WebhookEvent{
		EventType:              "DOCUMENT_CREATED",
		ProviderName:           provider.ProviderName(),
		ProviderCorrelationKey: *attempt.ProviderCorrelationKey,
		DocumentStatus:         &signingReady,
	}))
	require.True(t, attempts.read, "webhook did not look the attempt up by correlation key")

	for fx.runNextQueuedAdvanceJob(t, ctx, executor, attempt.ID) {
	}

	final := fx.mustAttempt(t, ctx, attempt.ID)
	if final.ProviderSubmitPhase != nil {
		t.Fatalf("provider submission stranded at phase %s with status %s", *final.ProviderSubmitPhase, final.Status)
	}
	require.Equal(t, entity.SigningAttemptStatusSigningReady, final.Status)
	require.NotNil(t, final.ProviderDocumentID)
	recipients, err := fx.attemptRepo.FindRecipientsByAttemptID(ctx, attempt.ID)
	require.NoError(t, err)
	require.Len(t, recipients, 1)
	require.NotNil(t, recipients[0].ProviderRecipientID)
}

func TestProviderWebhook_SigningAndCompletionStillApplyAfterSubmission(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	provider := signingmock.New()
	executor, storage, _ := newProviderSubmissionExecutor(t, ctx, fx, provider, nil)
	attempt := fx.createProviderReadyAttempt(t, ctx, provider, storage)
	require.NoError(t, executor.AdvanceProviderSubmissionForPhase(ctx, attempt.ID, entity.ProviderSubmitPhaseCreateProviderDocument))
	for fx.runNextQueuedAdvanceJob(t, ctx, executor, attempt.ID) {
	}
	require.Equal(t, entity.SigningAttemptStatusSigningReady, fx.mustAttempt(t, ctx, attempt.ID).Status)

	svc := fx.newWebhookService(t, ctx, fx.attemptRepo)
	deliver := func(eventType string, status entity.SigningAttemptStatus) {
		t.Helper()
		require.NoError(t, svc.HandleWebhookEvent(ctx, &port.WebhookEvent{
			EventType:              eventType,
			ProviderName:           provider.ProviderName(),
			ProviderCorrelationKey: *attempt.ProviderCorrelationKey,
			DocumentStatus:         &status,
		}))
	}

	deliver("DOCUMENT_OPENED", entity.SigningAttemptStatusSigning)
	require.Equal(t, entity.SigningAttemptStatusSigning, fx.mustAttempt(t, ctx, attempt.ID).Status)

	deliver("DOCUMENT_COMPLETED", entity.SigningAttemptStatusCompleted)
	deliver("DOCUMENT_COMPLETED", entity.SigningAttemptStatusCompleted)
	final := fx.mustAttempt(t, ctx, attempt.ID)
	require.Equal(t, entity.SigningAttemptStatusCompleted, final.Status)
	require.NotNil(t, final.TerminalAt)

	var docStatus entity.DocumentStatus
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT status FROM execution.documents WHERE id=$1`, fx.documentID).Scan(&docStatus))
	require.Equal(t, entity.DocumentStatusCompleted, docStatus)
	var dispatchJobs int
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT count(*) FROM river_job WHERE kind='dispatch_attempt_completion' AND args->>'attempt_id'=$1`, attempt.ID).Scan(&dispatchJobs))
	require.Equal(t, 1, dispatchJobs, "a repeated DOCUMENT_COMPLETED must not dispatch completion twice")
}

// advanceAfterWebhookReadRepo lets the worker advance between the webhook's read of
// the attempt and its write, reproducing the production interleaving.
type advanceAfterWebhookReadRepo struct {
	port.SigningAttemptRepository
	afterRead func()
	read      bool
}

func (r *advanceAfterWebhookReadRepo) FindByProviderCorrelationKey(ctx context.Context, providerName, correlationKey string) (*entity.SigningAttempt, error) {
	attempt, err := r.SigningAttemptRepository.FindByProviderCorrelationKey(ctx, providerName, correlationKey)
	if err == nil && !r.read {
		r.read = true
		if r.afterRead != nil {
			r.afterRead()
		}
	}
	return attempt, err
}

func (f *attemptFixture) newWebhookService(t *testing.T, ctx context.Context, attempts port.SigningAttemptRepository) *documentsvc.DocumentService {
	t.Helper()
	riverSvc, err := riverqueue.New(ctx, f.pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{DocumentRepo: f.docRepo, AttemptRepo: f.attemptRepo})
	require.NoError(t, err)
	return documentsvc.NewDocumentService(f.docRepo, nil, attempts, riverSvc.SigningExecutionUOW(), nil, nil, nil, nil, nil, nil, nil, nil, 0, nil, nil, false)
}

// runNextQueuedAdvanceJob runs the oldest available advance_provider_submission job
// for the attempt with the phase it was enqueued for, as River would, and reports
// whether there was one. A phase job that finds the attempt in a state it does not
// own completes without enqueuing anything, so the chain stops here too.
func (f *attemptFixture) runNextQueuedAdvanceJob(t *testing.T, ctx context.Context, executor *riverqueue.SigningAttemptExecutor, attemptID string) bool {
	t.Helper()
	var jobID int64
	var phase string
	err := f.pool.QueryRow(ctx, `
		SELECT id, COALESCE(args->>'provider_submit_phase', '')
		FROM river_job
		WHERE kind='advance_provider_submission' AND args->>'attempt_id'=$1 AND state='available'
		ORDER BY id LIMIT 1`, attemptID).Scan(&jobID, &phase)
	if errors.Is(err, pgx.ErrNoRows) {
		return false
	}
	require.NoError(t, err)
	require.NoError(t, executor.AdvanceProviderSubmissionForPhase(ctx, attemptID, entity.ProviderSubmitPhase(phase)))
	_, err = f.pool.Exec(ctx, `UPDATE river_job SET state='completed', finalized_at=now() WHERE id=$1`, jobID)
	require.NoError(t, err)
	return true
}

type transientRecipientsProvider struct {
	port.SigningProvider
	calls atomic.Int32
}

func (p *transientRecipientsProvider) EnsureProviderRecipients(
	ctx context.Context,
	req *port.EnsureProviderRecipientsRequest,
) (*port.EnsureProviderRecipientsResult, error) {
	if p.calls.Add(1) == 1 {
		return nil, &port.ProviderError{
			Class:        entity.ProviderErrorClassTransient,
			Phase:        entity.ProviderSubmitPhaseAddRecipients,
			ProviderName: p.ProviderName(),
			Retryable:    true,
			Message:      "temporary recipient failure",
		}
	}
	return p.SigningProvider.EnsureProviderRecipients(ctx, req)
}

type afterDocumentProvider struct {
	port.SigningProvider
	after func() error
	calls atomic.Int32
}

func (p *afterDocumentProvider) EnsureProviderDocument(
	ctx context.Context,
	req *port.EnsureProviderDocumentRequest,
) (*port.EnsureProviderDocumentResult, error) {
	result, err := p.SigningProvider.EnsureProviderDocument(ctx, req)
	if err != nil {
		return nil, err
	}
	if p.after != nil && p.calls.Add(1) == 1 {
		if hookErr := p.after(); hookErr != nil {
			return nil, hookErr
		}
	}
	return result, nil
}

// languageCapturingProvider records the language of each provider document it creates.
type languageCapturingProvider struct {
	port.SigningProvider
	languages []string
}

func (p *languageCapturingProvider) EnsureProviderDocument(
	ctx context.Context,
	req *port.EnsureProviderDocumentRequest,
) (*port.EnsureProviderDocumentResult, error) {
	p.languages = append(p.languages, req.Language)
	return p.SigningProvider.EnsureProviderDocument(ctx, req)
}

// staticPDFRenderer stands in for the Typst renderer: the render phase only
// needs bytes to store, and with no signature fields the executor falls back
// to default positions.
type staticPDFRenderer struct{ port.PDFRenderer }

func (staticPDFRenderer) RenderPreview(context.Context, *port.RenderPreviewRequest) (*port.RenderPreviewResult, error) {
	return &port.RenderPreviewResult{PDF: []byte("%PDF-1.4 language test\n")}, nil
}

type attemptFixture struct {
	pool             *pgxpool.Pool
	docRepo          port.DocumentRepository
	attemptRepo      port.SigningAttemptRepository
	tenantID         string
	workspaceID      string
	versionID        string
	documentTypeID   string
	documentTypeCode string
	roleID           string
	documentID       string
}

func newAttemptFixture(t *testing.T, ctx context.Context) *attemptFixture {
	t.Helper()
	pool := testhelper.GetTestPool(t)
	suffix := time.Now().UnixNano() % 1_000_000_000
	tenantID := testhelper.CreateTestTenant(t, pool, "River Attempts", fmt.Sprintf("RA%08d", suffix%100_000_000))
	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "WS", entity.WorkspaceTypeClient)
	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Template", nil)
	versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "v1", entity.VersionStatusPublished)
	roleID := testhelper.CreateTestSignerRole(t, pool, versionID, "Signer", "__sig_signer__", 1)
	documentTypeCode := fmt.Sprintf("RIV_DOC_%d", suffix)
	documentTypeID := testhelper.CreateTestDocumentType(t, pool, tenantID, documentTypeCode, "Document")
	testhelper.SetTemplateDocumentType(t, pool, templateID, documentTypeID)
	t.Cleanup(func() {
		testhelper.CleanupWorkspace(t, pool, workspaceID)
		testhelper.CleanupTenant(t, pool, tenantID)
	})

	fx := &attemptFixture{
		pool:             pool,
		docRepo:          documentrepo.New(pool),
		attemptRepo:      signingattemptrepo.New(pool),
		tenantID:         tenantID,
		workspaceID:      workspaceID,
		versionID:        versionID,
		documentTypeID:   documentTypeID,
		documentTypeCode: documentTypeCode,
		roleID:           roleID,
	}
	fx.documentID = fx.createDocument(t, ctx, "attempt-doc")
	return fx
}

func (f *attemptFixture) createDocument(t *testing.T, ctx context.Context, txn string) string {
	t.Helper()
	doc := entity.NewDocument(f.workspaceID, f.versionID)
	doc.DocumentTypeID = f.documentTypeID
	doc.Status = entity.DocumentStatusAwaitingInput
	doc.SetTitle("Attempt UOW")
	doc.SetTransactionalID(fmt.Sprintf("%s-%d", txn, time.Now().UnixNano()))
	docID, err := f.docRepo.Create(ctx, doc)
	require.NoError(t, err)
	return docID
}

func (f *attemptFixture) recipients() []*entity.DocumentRecipient {
	return []*entity.DocumentRecipient{{
		DocumentID:            f.documentID,
		TemplateVersionRoleID: f.roleID,
		Email:                 "signer@example.com",
		Name:                  "Signer",
		Status:                entity.RecipientStatusPending,
	}}
}

// documentCreationUOW returns a unit of work that can create documents
// (direct send at creation), wired as the API does.
func (f *attemptFixture) documentCreationUOW(t *testing.T, ctx context.Context) port.SigningExecutionUnitOfWork {
	t.Helper()
	riverSvc, err := riverqueue.New(ctx, f.pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{
		DocumentRepo:     f.docRepo,
		AttemptRepo:      f.attemptRepo,
		DocumentCreator:  documentrepo.NewConcrete(f.pool),
		RecipientCreator: documentrecipientrepo.NewConcrete(f.pool),
	})
	require.NoError(t, err)
	return riverSvc.SigningExecutionUOW()
}

// newUnsavedDocument builds a document the way creation hands it to the unit of
// work: in AWAITING_INPUT, not yet persisted.
func (f *attemptFixture) newUnsavedDocument(title string) *entity.Document {
	doc := entity.NewDocument(f.workspaceID, f.versionID)
	doc.DocumentTypeID = f.documentTypeID
	doc.Status = entity.DocumentStatusAwaitingInput
	doc.SetTitle(title)
	doc.SetTransactionalID(fmt.Sprintf("%s-%d", title, time.Now().UnixNano()))
	return doc
}

func (f *attemptFixture) newUnsavedRecipients() []*entity.DocumentRecipient {
	return []*entity.DocumentRecipient{entity.NewDocumentRecipient("", f.roleID, "Signer", "signer@example.com")}
}

func (f *attemptFixture) countDocumentsTitled(t *testing.T, ctx context.Context, title string) int {
	t.Helper()
	var n int
	require.NoError(t, f.pool.QueryRow(ctx, `SELECT count(*) FROM execution.documents WHERE workspace_id=$1 AND title=$2`, f.workspaceID, title).Scan(&n))
	return n
}

// failRenderJobInserts makes the database reject render_attempt_pdf jobs for
// this fixture's documents, the way a failed River insert aborts the enqueue
// after the document rows are written. Jobs for other workspaces are untouched.
func (f *attemptFixture) failRenderJobInserts(t *testing.T, ctx context.Context) {
	t.Helper()
	name := f.renderFailTriggerName()
	f.allowRenderJobInserts(t, ctx)
	_, err := f.pool.Exec(ctx, fmt.Sprintf(`
		CREATE FUNCTION %s() RETURNS trigger AS $$
		BEGIN
			IF NEW.kind = 'render_attempt_pdf' AND EXISTS (
				SELECT 1
				FROM execution.signing_attempts a
				JOIN execution.documents d ON d.id = a.document_id
				WHERE a.id::text = NEW.args->>'attempt_id' AND d.workspace_id::text = '%s'
			) THEN
				RAISE EXCEPTION 'injected render job insert failure';
			END IF;
			RETURN NEW;
		END
		$$ LANGUAGE plpgsql`, name, f.workspaceID))
	require.NoError(t, err)
	_, err = f.pool.Exec(ctx, fmt.Sprintf(`CREATE TRIGGER %[1]s BEFORE INSERT ON river_job FOR EACH ROW EXECUTE FUNCTION %[1]s()`, name))
	require.NoError(t, err)
	t.Cleanup(func() { f.allowRenderJobInserts(t, context.Background()) })
}

func (f *attemptFixture) allowRenderJobInserts(t *testing.T, ctx context.Context) {
	t.Helper()
	name := f.renderFailTriggerName()
	_, err := f.pool.Exec(ctx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON river_job`, name))
	require.NoError(t, err)
	_, err = f.pool.Exec(ctx, fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, name))
	require.NoError(t, err)
}

func (f *attemptFixture) renderFailTriggerName() string {
	return "test_fail_render_" + strings.ReplaceAll(f.workspaceID, "-", "_")
}

func (f *attemptFixture) createDocumentRecipient(t *testing.T, ctx context.Context) string {
	t.Helper()
	var id string
	require.NoError(t, f.pool.QueryRow(ctx, `
		INSERT INTO execution.document_recipients (
			document_id, template_version_role_id, name, email, status, created_at
		) VALUES ($1, $2, $3, $4, $5, now())
		RETURNING id`,
		f.documentID,
		f.roleID,
		"Signer",
		"signer@example.com",
		entity.RecipientStatusPending,
	).Scan(&id))
	return id
}

func (f *attemptFixture) signerOrders() map[string]int {
	return map[string]int{f.roleID: 1}
}

func newProviderSubmissionExecutor(
	t *testing.T,
	ctx context.Context,
	fx *attemptFixture,
	provider port.SigningProvider,
	failpoints riverqueue.AttemptFailpoints,
) (*riverqueue.SigningAttemptExecutor, port.StorageAdapter, *river.Client[pgx.Tx]) {
	t.Helper()
	_, err := riverqueue.New(ctx, fx.pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{DocumentRepo: fx.docRepo, AttemptRepo: fx.attemptRepo})
	require.NoError(t, err)
	storage, err := local.New(t.TempDir())
	require.NoError(t, err)
	client, err := river.NewClient(riverpgxv5.New(fx.pool), &river.Config{})
	require.NoError(t, err)
	return newProviderSubmissionExecutorWithClient(t, fx, provider, storage, client, failpoints), storage, client
}

func newProviderSubmissionExecutorWithStorage(
	t *testing.T,
	fx *attemptFixture,
	provider port.SigningProvider,
	storage port.StorageAdapter,
	failpoints riverqueue.AttemptFailpoints,
) *riverqueue.SigningAttemptExecutor {
	t.Helper()
	client, err := river.NewClient(riverpgxv5.New(fx.pool), &river.Config{})
	require.NoError(t, err)
	return newProviderSubmissionExecutorWithClient(t, fx, provider, storage, client, failpoints)
}

func newProviderSubmissionExecutorWithClient(
	t *testing.T,
	fx *attemptFixture,
	provider port.SigningProvider,
	storage port.StorageAdapter,
	client *river.Client[pgx.Tx],
	failpoints riverqueue.AttemptFailpoints,
) *riverqueue.SigningAttemptExecutor {
	t.Helper()
	return riverqueue.NewSigningAttemptExecutor(riverqueue.SigningAttemptExecutorConfig{
		Pool:            fx.pool,
		Client:          client,
		DocumentRepo:    fx.docRepo,
		AttemptRepo:     fx.attemptRepo,
		SigningProvider: provider,
		StorageAdapter:  storage,
		StorageEnabled:  true,
		Failpoints:      failpoints,
	})
}

func (f *attemptFixture) createProviderReadyAttempt(t *testing.T, ctx context.Context, provider port.SigningProvider, storage port.StorageAdapter) *entity.SigningAttempt {
	t.Helper()
	riverSvc, err := riverqueue.New(ctx, f.pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{DocumentRepo: f.docRepo, AttemptRepo: f.attemptRepo})
	require.NoError(t, err)
	attempt, err := riverSvc.SigningExecutionUOW().CreateAttemptAndEnqueueRender(ctx, f.documentID, f.recipients(), f.signerOrders())
	require.NoError(t, err)
	pdf := []byte("%PDF-1.4 durable provider submission test\n")
	checksumBytes := sha256.Sum256(pdf)
	checksum := hex.EncodeToString(checksumBytes[:])
	storagePath := fmt.Sprintf("tests/%s/pre-signed.pdf", attempt.ID)
	algo := "sha256"
	require.NoError(t, storage.Upload(ctx, &port.StorageUploadRequest{
		Key:         storagePath,
		Data:        pdf,
		ContentType: "application/pdf",
		Environment: entity.EnvironmentProd,
	}))
	fields, err := json.Marshal([]port.SignatureFieldPosition{{
		RoleID:    f.roleID,
		Page:      1,
		PositionX: 10,
		PositionY: 70,
		Width:     30,
		Height:    5,
	}})
	require.NoError(t, err)
	payload, err := json.Marshal(map[string]any{
		"title":          "Attempt UOW",
		"correlationKey": fmt.Sprintf("%s:%s", f.documentID, attempt.ID),
		"pdfStoragePath": storagePath,
		"pdfChecksum":    checksum,
	})
	require.NoError(t, err)
	providerName := provider.ProviderName()
	corr := fmt.Sprintf("%s:%s", f.documentID, attempt.ID)
	attempt.Status = entity.SigningAttemptStatusReadyToSubmit
	attempt.PDFStoragePath = &storagePath
	attempt.PDFChecksum = &checksum
	attempt.PDFChecksumAlgorithm = &algo
	attempt.SignatureFieldSnapshot = fields
	attempt.ProviderUploadPayload = payload
	providerSubmitPhase := entity.ProviderSubmitPhaseCreateProviderDocument
	attempt.ProviderName = &providerName
	attempt.ProviderCorrelationKey = &corr
	attempt.ProviderSubmitPhase = &providerSubmitPhase
	require.NoError(t, f.attemptRepo.Update(ctx, attempt))
	return attempt
}

func (f *attemptFixture) drainProviderSubmission(t *testing.T, ctx context.Context, executor *riverqueue.SigningAttemptExecutor, attemptID string) {
	t.Helper()
	for range 8 {
		attempt := f.mustAttempt(t, ctx, attemptID)
		if attempt.Status == entity.SigningAttemptStatusSigningReady {
			return
		}
		require.False(t, attempt.Status.IsTerminal(), "attempt reached terminal status %s before signing ready", attempt.Status)
		require.NoError(t, executor.AdvanceProviderSubmission(ctx, attemptID))
	}
	attempt := f.mustAttempt(t, ctx, attemptID)
	require.Equal(t, entity.SigningAttemptStatusSigningReady, attempt.Status)
}

func (f *attemptFixture) mustAttempt(t *testing.T, ctx context.Context, attemptID string) *entity.SigningAttempt {
	t.Helper()
	attempt, err := f.attemptRepo.FindByID(ctx, attemptID)
	require.NoError(t, err)
	return attempt
}
