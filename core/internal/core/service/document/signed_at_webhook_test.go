package document

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// Fakes for tools-team-signing#35: two signers, each on the attempt (with the
// provider's recipient ID) and on the document (what the panel shows).

type signedAtAttemptRepoFake struct {
	port.SigningAttemptRepository
	attempt    *entity.SigningAttempt
	recipients []*entity.SigningAttemptRecipient
}

func (f *signedAtAttemptRepoFake) FindByProviderDocumentID(context.Context, string, string) (*entity.SigningAttempt, error) {
	return f.attempt, nil
}

func (f *signedAtAttemptRepoFake) FindRecipientsByAttemptID(context.Context, string) ([]*entity.SigningAttemptRecipient, error) {
	return f.recipients, nil
}

func (f *signedAtAttemptRepoFake) UpdateRecipient(context.Context, *entity.SigningAttemptRecipient) error {
	return nil // the fake hands out pointers, so the service's changes are already in place
}

func (f *signedAtAttemptRepoFake) InsertEvent(context.Context, *entity.SigningAttemptEvent) error {
	return nil
}

type signedAtRecipientRepoFake struct {
	port.DocumentRecipientRepository
	byID map[string]*entity.DocumentRecipient
}

func (f *signedAtRecipientRepoFake) FindByID(_ context.Context, id string) (*entity.DocumentRecipient, error) {
	return f.byID[id], nil
}

func (f *signedAtRecipientRepoFake) Update(_ context.Context, r *entity.DocumentRecipient) error {
	f.byID[r.ID] = r
	return nil
}

type signedAtUOWFake struct{ port.SigningExecutionUnitOfWork }

func (signedAtUOWFake) ApplyProviderWebhook(context.Context, string, entity.ProviderReport) (port.ProviderWebhookResult, error) {
	return port.ProviderWebhookResult{}, nil
}

var (
	signer1Signed = time.Date(2026, 10, 30, 17, 13, 42, 0, time.UTC)
	signer2Signed = time.Date(2026, 10, 30, 17, 14, 9, 0, time.UTC)
)

func signedAtFixture() (*DocumentService, *signedAtAttemptRepoFake, *signedAtRecipientRepoFake) {
	ptr := func(s string) *string { return &s }
	attempts := &signedAtAttemptRepoFake{
		attempt: &entity.SigningAttempt{ID: "attempt-1", DocumentID: "doc-1"},
		recipients: []*entity.SigningAttemptRecipient{
			{ID: "ar-1", DocumentRecipientID: ptr("rec-1"), ProviderRecipientID: ptr("11"), Status: entity.RecipientStatusSent},
			{ID: "ar-2", DocumentRecipientID: ptr("rec-2"), ProviderRecipientID: ptr("12"), Status: entity.RecipientStatusPending},
		},
	}
	recipients := &signedAtRecipientRepoFake{byID: map[string]*entity.DocumentRecipient{
		"rec-1": {ID: "rec-1", Status: entity.RecipientStatusSent},
		"rec-2": {ID: "rec-2", Status: entity.RecipientStatusPending},
	}}
	return &DocumentService{attemptRepo: attempts, recipientRepo: recipients, signingUOW: signedAtUOWFake{}}, attempts, recipients
}

func signedWebhook(providerRecipientID string, states ...port.WebhookRecipientState) *port.WebhookEvent {
	signed := entity.RecipientStatusSigned
	return &port.WebhookEvent{
		EventType:           "DOCUMENT_SIGNED",
		ProviderName:        "documenso",
		ProviderDocumentID:  "7",
		ProviderRecipientID: providerRecipientID,
		RecipientStatus:     &signed,
		Recipients:          states,
	}
}

func signedState(id string, at time.Time) port.WebhookRecipientState {
	return port.WebhookRecipientState{ProviderRecipientID: id, Signed: true, SignedAt: &at}
}

func notSignedState(id string) port.WebhookRecipientState {
	return port.WebhookRecipientState{ProviderRecipientID: id}
}

func assertSignedAt(t *testing.T, r *entity.DocumentRecipient, want time.Time) {
	t.Helper()
	require.Equal(t, entity.RecipientStatusSigned, r.Status, "recipient %s", r.ID)
	require.NotNil(t, r.SignedAt, "recipient %s", r.ID)
	assert.True(t, r.SignedAt.Equal(want), "recipient %s signed at %v, want %v", r.ID, r.SignedAt, want)
}

func TestWebhook_FirstSignerShowsTheirOwnTimeBeforeTheSecondSigns(t *testing.T) {
	svc, attempts, recipients := signedAtFixture()

	require.NoError(t, svc.HandleWebhookEvent(context.Background(),
		signedWebhook("11", signedState("11", signer1Signed), notSignedState("12"))))

	assertSignedAt(t, recipients.byID["rec-1"], signer1Signed)
	assert.Equal(t, entity.RecipientStatusPending, recipients.byID["rec-2"].Status)
	assert.Nil(t, recipients.byID["rec-2"].SignedAt)
	assert.True(t, attempts.recipients[0].SignedAt.Equal(signer1Signed))
}

// The case #35 calls out: signer 2's webhook lists both signers as SIGNED.
func TestWebhook_SecondSignersWebhookKeepsEachSignersTime(t *testing.T) {
	svc, _, recipients := signedAtFixture()
	ctx := context.Background()
	require.NoError(t, svc.HandleWebhookEvent(ctx, signedWebhook("11", signedState("11", signer1Signed), notSignedState("12"))))

	require.NoError(t, svc.HandleWebhookEvent(ctx,
		signedWebhook("12", signedState("11", signer1Signed), signedState("12", signer2Signed))))

	assertSignedAt(t, recipients.byID["rec-1"], signer1Signed)
	assertSignedAt(t, recipients.byID["rec-2"], signer2Signed)
}

func TestWebhook_CompletionUsesEachSignersProviderTime(t *testing.T) {
	svc, _, recipients := signedAtFixture()
	completed := entity.SigningAttemptStatusCompleted

	require.NoError(t, svc.HandleWebhookEvent(context.Background(), &port.WebhookEvent{
		EventType:          "DOCUMENT_COMPLETED",
		ProviderName:       "documenso",
		ProviderDocumentID: "7",
		DocumentStatus:     &completed,
		Recipients:         []port.WebhookRecipientState{signedState("11", signer1Signed), signedState("12", signer2Signed)},
	}))

	assertSignedAt(t, recipients.byID["rec-1"], signer1Signed)
	assertSignedAt(t, recipients.byID["rec-2"], signer2Signed)
}

// A time the engine guessed earlier (the moment a webhook arrived, as before
// this fix) is replaced once the provider's own time is known.
func TestWebhook_ProviderTimeReplacesAnEarlierGuess(t *testing.T) {
	svc, attempts, recipients := signedAtFixture()
	guessed := signer2Signed.Add(4 * time.Second)
	recipients.byID["rec-1"].Status = entity.RecipientStatusSigned
	recipients.byID["rec-1"].SignedAt = &guessed
	attempts.recipients[0].Status = entity.RecipientStatusSigned
	attempts.recipients[0].SignedAt = &guessed

	require.NoError(t, svc.HandleWebhookEvent(context.Background(),
		signedWebhook("12", signedState("11", signer1Signed), signedState("12", signer2Signed))))

	assertSignedAt(t, recipients.byID["rec-1"], signer1Signed)
	assert.True(t, attempts.recipients[0].SignedAt.Equal(signer1Signed))
}

// Without provider times (another provider, or an older payload), completion
// keeps a time already recorded and stamps only the rest.
func TestWebhook_CompletionWithoutProviderTimesFillsOnlyMissingOnes(t *testing.T) {
	svc, attempts, recipients := signedAtFixture()
	attempts.recipients[0].Status = entity.RecipientStatusSigned
	attempts.recipients[0].SignedAt = &signer1Signed
	recipients.byID["rec-1"].Status = entity.RecipientStatusSigned
	recipients.byID["rec-1"].SignedAt = &signer1Signed
	completed := entity.SigningAttemptStatusCompleted
	before := time.Now().UTC()

	require.NoError(t, svc.HandleWebhookEvent(context.Background(), &port.WebhookEvent{
		EventType:          "DOCUMENT_COMPLETED",
		ProviderName:       "documenso",
		ProviderDocumentID: "7",
		DocumentStatus:     &completed,
	}))

	assertSignedAt(t, recipients.byID["rec-1"], signer1Signed)
	rec2 := recipients.byID["rec-2"]
	require.Equal(t, entity.RecipientStatusSigned, rec2.Status)
	require.NotNil(t, rec2.SignedAt)
	assert.False(t, rec2.SignedAt.Before(before), "with no provider time, the completion time is used")
}
