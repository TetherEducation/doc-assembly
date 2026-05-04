//go:build integration

package riverqueue_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/document_repo"
	"github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/signing_attempt_repo"
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
