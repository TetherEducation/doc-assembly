//go:build integration

package riverqueue_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/database/postgres/document_event_repo"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/database/postgres/document_recipient_repo"
	signingmock "github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/signing/mock"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/storage/local"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	documentsvc "github.com/TetherEducation/doc-assembly/core/internal/core/service/document"
	"github.com/TetherEducation/doc-assembly/core/internal/infra/config"
	"github.com/TetherEducation/doc-assembly/core/internal/infra/riverqueue"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

// timelineRow is one document_events row as the timeline panel reads it.
type timelineRow struct {
	EventType      string
	RecipientEmail string
	TimeSource     string
	CreatedAt      time.Time
}

func (r timelineRow) key() string {
	if r.RecipientEmail == "" {
		return r.EventType
	}
	return r.EventType + " " + r.RecipientEmail
}

// TestSigningTimeline_TwoSignerFlowRecordsEachEventOnce runs a two-signer
// document through the mock provider with every change reported twice, by a
// webhook and by a status refresh, and checks the timeline has each step once
// and in order.
func TestSigningTimeline_TwoSignerFlowRecordsEachEventOnce(t *testing.T) {
	rows := runTwoSignerTimelineFlow(t, true)

	keys := make([]string, 0, len(rows))
	for _, r := range rows {
		keys = append(keys, r.key())
	}
	require.Equal(t, []string{
		entity.EventDocumentCreated,
		entity.EventDocumentSent,
		entity.EventRecipientDelivered + " signer1@example.com",
		entity.EventRecipientSigned + " signer1@example.com",
		entity.EventRecipientDelivered + " signer2@example.com",
		entity.EventRecipientSigned + " signer2@example.com",
		entity.EventDocumentCompleted,
	}, keys)

	for _, r := range rows {
		if r.EventType == entity.EventRecipientSigned {
			require.Equal(t, entity.TimelineTimeProvider, r.TimeSource, "%s should carry the provider's signing time", r.key())
		}
	}
}

// TestSigningTimeline_FlagOffWritesNothingNew runs the same flow with
// signing.document_timeline off: document_events keeps only what it recorded
// before (here, DOCUMENT_CREATED), which is what admissions sees today.
func TestSigningTimeline_FlagOffWritesNothingNew(t *testing.T) {
	rows := runTwoSignerTimelineFlow(t, false)
	require.Len(t, rows, 1)
	require.Equal(t, entity.EventDocumentCreated, rows[0].EventType)
}

func runTwoSignerTimelineFlow(t *testing.T, timeline bool) []timelineRow {
	t.Helper()
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	role2ID := testhelper.CreateTestSignerRole(t, fx.pool, fx.versionID, "Signer 2", "__sig_signer_2__", 2)
	eventRepo := documenteventrepo.New(fx.pool)
	var timelineEvents port.DocumentEventRepository
	if timeline {
		timelineEvents = eventRepo
	}

	// Document creation records DOCUMENT_CREATED whatever the flag.
	require.NoError(t, eventRepo.Create(ctx, entity.NewDocumentEvent(fx.documentID, entity.EventDocumentCreated, entity.ActorUser, "")))

	signers := []*entity.DocumentRecipient{
		{DocumentID: fx.documentID, TemplateVersionRoleID: fx.roleID, Email: "signer1@example.com", Name: "Signer One", Status: entity.RecipientStatusPending},
		{DocumentID: fx.documentID, TemplateVersionRoleID: role2ID, Email: "signer2@example.com", Name: "Signer Two", Status: entity.RecipientStatusPending},
	}
	for _, s := range signers {
		require.NoError(t, fx.pool.QueryRow(ctx, `
			INSERT INTO execution.document_recipients (document_id, template_version_role_id, name, email, status, created_at)
			VALUES ($1, $2, $3, $4, $5, now()) RETURNING id`,
			s.DocumentID, s.TemplateVersionRoleID, s.Name, s.Email, s.Status).Scan(&s.ID))
	}

	provider := signingmock.New()
	storage, err := local.New(t.TempDir())
	require.NoError(t, err)
	client, err := river.NewClient(riverpgxv5.New(fx.pool), &river.Config{})
	require.NoError(t, err)
	executor := riverqueue.NewSigningAttemptExecutor(riverqueue.SigningAttemptExecutorConfig{
		Pool:            fx.pool,
		Client:          client,
		DocumentRepo:    fx.docRepo,
		AttemptRepo:     fx.attemptRepo,
		SigningProvider: provider,
		StorageAdapter:  storage,
		StorageEnabled:  true,
		TimelineEvents:  timelineEvents,
	})

	riverSvc, err := riverqueue.New(ctx, fx.pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{
		DocumentRepo:   fx.docRepo,
		AttemptRepo:    fx.attemptRepo,
		TimelineEvents: timelineEvents,
	})
	require.NoError(t, err)
	attempt, err := riverSvc.SigningExecutionUOW().CreateAttemptAndEnqueueRender(ctx, fx.documentID, signers, map[string]int{fx.roleID: 1, role2ID: 2})
	require.NoError(t, err)
	fx.makeProviderReady(t, ctx, attempt, provider, storage, []string{fx.roleID, role2ID})
	fx.drainProviderSubmission(t, ctx, executor, attempt.ID)
	attempt = fx.mustAttempt(t, ctx, attempt.ID)
	require.Equal(t, entity.SigningAttemptStatusSigningReady, attempt.Status)

	svc := documentsvc.NewDocumentService(
		fx.docRepo, documentrecipientrepo.New(fx.pool), fx.attemptRepo, riverSvc.SigningExecutionUOW(),
		nil, nil, nil, nil, nil, nil,
		documentsvc.NewEventEmitter(eventRepo, documentsvc.WithDocumentTimeline(timeline)),
		nil, 0, nil, nil, false,
	)

	attemptRecipients, err := fx.attemptRepo.FindRecipientsByAttemptID(ctx, attempt.ID)
	require.NoError(t, err)
	require.Len(t, attemptRecipients, 2)
	providerIDs := map[string]string{}
	for _, r := range attemptRecipients {
		require.NotNil(t, r.ProviderRecipientID)
		providerIDs[r.Email] = *r.ProviderRecipientID
	}
	s1, s2 := providerIDs["signer1@example.com"], providerIDs["signer2@example.com"]

	webhook := func(eventType, recipientID string, recipientStatus entity.RecipientStatus, docStatus entity.SigningAttemptStatus) {
		t.Helper()
		var recipients []port.WebhookRecipientState
		for _, id := range []string{s1, s2} {
			if r := provider.GetMockRecipient(id); r.Status == "SIGNED" {
				recipients = append(recipients, port.WebhookRecipientState{ProviderRecipientID: id, Signed: true, SignedAt: r.SignedAt})
			}
		}
		require.NoError(t, svc.HandleWebhookEvent(ctx, &port.WebhookEvent{
			EventType:              eventType,
			ProviderName:           provider.ProviderName(),
			ProviderDocumentID:     *attempt.ProviderDocumentID,
			ProviderCorrelationKey: *attempt.ProviderCorrelationKey,
			ProviderRecipientID:    recipientID,
			RecipientStatus:        &recipientStatus,
			DocumentStatus:         &docStatus,
			Recipients:             recipients,
		}))
	}
	refresh := func() {
		t.Helper()
		require.NoError(t, executor.RefreshAttemptProviderStatus(ctx, attempt.ID))
	}

	// Signer 1 opens: webhook first, then the refresh sees the same.
	provider.SimulateView(s1)
	webhook("RECIPIENT_VIEWED", s1, entity.RecipientStatusDelivered, entity.SigningAttemptStatusSigning)
	refresh()
	// Signer 1 signs: refresh first, then the webhook.
	provider.SimulateSign(s1)
	refresh()
	webhook("RECIPIENT_SIGNED", s1, entity.RecipientStatusSigned, entity.SigningAttemptStatusSigning)
	// Signer 2 opens: refresh first, then the webhook.
	provider.SimulateView(s2)
	refresh()
	webhook("RECIPIENT_VIEWED", s2, entity.RecipientStatusDelivered, entity.SigningAttemptStatusSigning)
	// Signer 2 signs, completing the document: webhook first, then refresh.
	provider.SimulateSign(s2)
	webhook("DOCUMENT_COMPLETED", s2, entity.RecipientStatusSigned, entity.SigningAttemptStatusCompleted)
	refresh()
	// A late duplicate of each webhook changes nothing.
	webhook("RECIPIENT_VIEWED", s1, entity.RecipientStatusDelivered, entity.SigningAttemptStatusCompleted)
	webhook("DOCUMENT_COMPLETED", s2, entity.RecipientStatusSigned, entity.SigningAttemptStatusCompleted)

	require.Equal(t, entity.SigningAttemptStatusCompleted, fx.mustAttempt(t, ctx, attempt.ID).Status)
	return fx.timelineRows(t, ctx)
}

// TestDocumentEventRepo_CreateOnceDedupes checks CreateOnce writes an event
// once, keeps signers and attempts apart, lets a provider time replace an
// engine-guessed signing time (never the reverse), and holds under concurrency.
func TestDocumentEventRepo_CreateOnceDedupes(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	repo := documenteventrepo.New(fx.pool)
	docRecipientID := fx.createDocumentRecipient(t, ctx)

	attemptRec := func(attemptID, email string) *entity.SigningAttemptRecipient {
		return &entity.SigningAttemptRecipient{
			AttemptID: attemptID, DocumentRecipientID: &docRecipientID,
			Email: email, Name: "Signer", Status: entity.RecipientStatusSigned,
		}
	}
	signed := func(rec *entity.SigningAttemptRecipient, at time.Time, source string) *entity.DocumentEvent {
		return entity.NewRecipientTimelineEvent(fx.documentID, rec, entity.EventRecipientSigned, entity.ActorWebhook, entity.RecipientStatusDelivered, at, source)
	}

	attemptA := "11111111-1111-1111-1111-111111111111"
	engineTime := time.Now().UTC().Truncate(time.Microsecond)
	providerTime := engineTime.Add(-90 * time.Second)

	require.NoError(t, repo.CreateOnce(ctx, signed(attemptRec(attemptA, "signer@example.com"), engineTime, entity.TimelineTimeEngine)))
	require.NoError(t, repo.CreateOnce(ctx, signed(attemptRec(attemptA, "signer@example.com"), engineTime.Add(time.Second), entity.TimelineTimeEngine)))
	rows := fx.timelineRows(t, ctx)
	require.Len(t, rows, 1, "a repeated event is written once")
	require.True(t, rows[0].CreatedAt.Equal(engineTime))
	require.Equal(t, entity.TimelineTimeEngine, rows[0].TimeSource)

	require.NoError(t, repo.CreateOnce(ctx, signed(attemptRec(attemptA, "signer@example.com"), providerTime, entity.TimelineTimeProvider)))
	rows = fx.timelineRows(t, ctx)
	require.Len(t, rows, 1, "the provider's time replaces the engine's on the same event")
	require.True(t, rows[0].CreatedAt.Equal(providerTime), "got %s, want %s", rows[0].CreatedAt, providerTime)
	require.Equal(t, entity.TimelineTimeProvider, rows[0].TimeSource)

	require.NoError(t, repo.CreateOnce(ctx, signed(attemptRec(attemptA, "signer@example.com"), engineTime.Add(time.Minute), entity.TimelineTimeEngine)))
	require.NoError(t, repo.CreateOnce(ctx, signed(attemptRec(attemptA, "signer@example.com"), providerTime.Add(time.Minute), entity.TimelineTimeProvider)))
	rows = fx.timelineRows(t, ctx)
	require.Len(t, rows, 1)
	require.True(t, rows[0].CreatedAt.Equal(providerTime), "a provider time is not replaced")

	// Another attempt, or another signer on an unlinked attempt recipient, is a new event.
	require.NoError(t, repo.CreateOnce(ctx, signed(attemptRec("22222222-2222-2222-2222-222222222222", "signer@example.com"), engineTime, entity.TimelineTimeEngine)))
	unlinked := func(email string) *entity.DocumentEvent {
		rec := attemptRec(attemptA, email)
		rec.DocumentRecipientID = nil
		return signed(rec, engineTime, entity.TimelineTimeEngine)
	}
	require.NoError(t, repo.CreateOnce(ctx, unlinked("a@example.com")))
	require.NoError(t, repo.CreateOnce(ctx, unlinked("b@example.com")))
	require.NoError(t, repo.CreateOnce(ctx, unlinked("b@example.com")))
	require.Len(t, fx.timelineRows(t, ctx), 4)

	// Concurrent writers of one event write it once.
	sent := entity.NewAttemptTimelineEvent(&entity.SigningAttempt{ID: attemptA, DocumentID: fx.documentID},
		entity.EventDocumentSent, entity.ActorSystem, entity.SigningAttemptStatusSubmittingProvider, entity.SigningAttemptStatusSigningReady, engineTime)
	var wg sync.WaitGroup
	errs := make(chan error, 10)
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ev := *sent
			errs <- repo.CreateOnce(ctx, &ev)
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var sentCount int
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT count(*) FROM execution.document_events WHERE document_id=$1 AND event_type=$2`,
		fx.documentID, entity.EventDocumentSent).Scan(&sentCount))
	require.Equal(t, 1, sentCount)
}

// makeProviderReady fills in what rendering would (PDF, signature fields per
// role, provider payload) so the attempt can be submitted to provider.
func (f *attemptFixture) makeProviderReady(t *testing.T, ctx context.Context, attempt *entity.SigningAttempt, provider port.SigningProvider, storage port.StorageAdapter, roleIDs []string) {
	t.Helper()
	pdf := []byte("%PDF-1.4 signing timeline test\n")
	storagePath := fmt.Sprintf("tests/%s/pre-signed.pdf", attempt.ID)
	require.NoError(t, storage.Upload(ctx, &port.StorageUploadRequest{Key: storagePath, Data: pdf, ContentType: "application/pdf", Environment: entity.EnvironmentProd}))
	fields := make([]port.SignatureFieldPosition, 0, len(roleIDs))
	for i, roleID := range roleIDs {
		fields = append(fields, port.SignatureFieldPosition{RoleID: roleID, Page: 1, PositionX: 10, PositionY: float64(60 + 10*i), Width: 30, Height: 5})
	}
	fieldsJSON, err := json.Marshal(fields)
	require.NoError(t, err)
	sum := sha256.Sum256(pdf)
	checksum := hex.EncodeToString(sum[:])
	corr := fmt.Sprintf("%s:%s", f.documentID, attempt.ID)
	payload, err := json.Marshal(map[string]any{"title": "Attempt UOW", "correlationKey": corr, "pdfStoragePath": storagePath, "pdfChecksum": checksum})
	require.NoError(t, err)
	providerName := provider.ProviderName()
	algo := "sha256"
	phase := entity.ProviderSubmitPhaseCreateProviderDocument
	attempt.Status = entity.SigningAttemptStatusReadyToSubmit
	attempt.PDFStoragePath = &storagePath
	attempt.PDFChecksum = &checksum
	attempt.PDFChecksumAlgorithm = &algo
	attempt.SignatureFieldSnapshot = fieldsJSON
	attempt.ProviderUploadPayload = payload
	attempt.ProviderName = &providerName
	attempt.ProviderCorrelationKey = &corr
	attempt.ProviderSubmitPhase = &phase
	require.NoError(t, f.attemptRepo.Update(ctx, attempt))
}

// timelineRows returns the fixture document's events, oldest first.
func (f *attemptFixture) timelineRows(t *testing.T, ctx context.Context) []timelineRow {
	t.Helper()
	rows, err := f.pool.Query(ctx, `
		SELECT event_type, COALESCE(metadata->>'recipientEmail', ''), COALESCE(metadata->>'timeSource', ''), created_at
		FROM execution.document_events WHERE document_id=$1 ORDER BY created_at, id`, f.documentID)
	require.NoError(t, err)
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (timelineRow, error) {
		var r timelineRow
		err := row.Scan(&r.EventType, &r.RecipientEmail, &r.TimeSource, &r.CreatedAt)
		return r, err
	})
	require.NoError(t, err)
	return out
}
