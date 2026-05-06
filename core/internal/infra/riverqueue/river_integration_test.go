//go:build integration

package riverqueue_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

	"github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/document_repo"
	"github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/signing_attempt_repo"
	signingmock "github.com/rendis/doc-assembly/core/internal/adapters/secondary/signing/mock"
	"github.com/rendis/doc-assembly/core/internal/adapters/secondary/storage/local"
	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	"github.com/rendis/doc-assembly/core/internal/infra/config"
	"github.com/rendis/doc-assembly/core/internal/infra/riverqueue"
	"github.com/rendis/doc-assembly/core/internal/testing/testhelper"
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
	require.Nil(t, oldAttempt.ProviderSubmitPhase)

	var activeAttemptID string
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT active_attempt_id FROM execution.documents WHERE id=$1`, fx.documentID).Scan(&activeAttemptID))
	require.NotEqual(t, attempt.ID, activeAttemptID)

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
	attempt.ProviderName = &providerName
	attempt.ProviderCorrelationKey = &corr
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
