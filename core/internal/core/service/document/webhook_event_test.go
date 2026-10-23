package document

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// webhookAttemptRepoFake returns the attempt as the webhook read it. Saving that copy
// is what stranded production attempts, so any attempt write fails the test.
type webhookAttemptRepoFake struct {
	port.SigningAttemptRepository
	t      *testing.T
	read   *entity.SigningAttempt
	events []*entity.SigningAttemptEvent
}

func (r *webhookAttemptRepoFake) FindByProviderCorrelationKey(context.Context, string, string) (*entity.SigningAttempt, error) {
	cp := *r.read
	return &cp, nil
}

func (r *webhookAttemptRepoFake) Update(context.Context, *entity.SigningAttempt) error {
	r.t.Fatal("HandleWebhookEvent saved the attempt copy it read instead of merging under lock")
	return nil
}

func (r *webhookAttemptRepoFake) InsertEvent(_ context.Context, ev *entity.SigningAttemptEvent) error {
	r.events = append(r.events, ev)
	return nil
}

type webhookUOWFake struct {
	port.SigningExecutionUnitOfWork
	attemptID string
	report    entity.ProviderReport
	calls     int
	result    port.ProviderWebhookResult
}

func (u *webhookUOWFake) ApplyProviderWebhook(_ context.Context, attemptID string, report entity.ProviderReport) (port.ProviderWebhookResult, error) {
	u.calls++
	u.attemptID = attemptID
	u.report = report
	return u.result, nil
}

func TestHandleWebhookEventMergesUnderLockInsteadOfSavingItsCopy(t *testing.T) {
	ctx := context.Background()
	phase := entity.ProviderSubmitPhaseAddRecipients
	attempts := &webhookAttemptRepoFake{t: t, read: &entity.SigningAttempt{
		ID:                  "attempt-1",
		DocumentID:          "doc-1",
		Status:              entity.SigningAttemptStatusSubmittingProvider,
		ProviderSubmitPhase: &phase,
	}}
	uow := &webhookUOWFake{result: port.ProviderWebhookResult{
		OldStatus: entity.SigningAttemptStatusSubmittingProvider,
		NewStatus: entity.SigningAttemptStatusSubmittingProvider,
	}}
	svc := &DocumentService{attemptRepo: attempts, signingUOW: uow}

	signingReady := entity.SigningAttemptStatusSigningReady
	require.NoError(t, svc.HandleWebhookEvent(ctx, &port.WebhookEvent{
		EventType:              "DOCUMENT_CREATED",
		ProviderName:           "documenso",
		ProviderCorrelationKey: "doc-1:attempt-1",
		DocumentStatus:         &signingReady,
	}))

	require.Equal(t, 1, uow.calls)
	assert.Equal(t, "attempt-1", uow.attemptID)
	require.NotNil(t, uow.report.Status)
	assert.Equal(t, entity.SigningAttemptStatusSigningReady, *uow.report.Status)
	assert.Equal(t, "doc-1:attempt-1", uow.report.CorrelationKey)

	require.Len(t, attempts.events, 1)
	assert.Equal(t, "ATTEMPT_WEBHOOK_RECEIVED", attempts.events[0].EventType)
	assert.Equal(t, entity.SigningAttemptStatusSubmittingProvider, *attempts.events[0].NewStatus,
		"the audit event must record the status the attempt kept, not the one the provider echoed")
}
