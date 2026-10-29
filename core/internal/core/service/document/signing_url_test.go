package document

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

type signingURLDocumentRepoFake struct {
	port.DocumentRepository
	doc *entity.DocumentWithRecipients
}

func (f *signingURLDocumentRepoFake) FindByIDWithRecipients(context.Context, string) (*entity.DocumentWithRecipients, error) {
	return f.doc, nil
}

func (f *signingURLDocumentRepoFake) FindByID(context.Context, string) (*entity.Document, error) {
	return &f.doc.Document, nil
}

type signingURLAttemptRepoFake struct {
	port.SigningAttemptRepository
	recipients []*entity.SigningAttemptRecipient
	askedFor   string
}

func (f *signingURLAttemptRepoFake) FindRecipientsByAttemptID(_ context.Context, attemptID string) ([]*entity.SigningAttemptRecipient, error) {
	f.askedFor = attemptID
	return f.recipients, nil
}

func signingURLFixture(activeAttemptID *string, attemptRecipients ...*entity.SigningAttemptRecipient) (*DocumentService, *signingURLAttemptRepoFake) {
	doc := &entity.DocumentWithRecipients{
		Document: entity.Document{ID: "doc-1", Status: entity.DocumentStatusSigning, ActiveAttemptID: activeAttemptID},
		Recipients: []*entity.DocumentRecipient{
			{ID: "rec-1", Name: "Ana"},
			{ID: "rec-2", Name: "Ben"},
		},
	}
	attempts := &signingURLAttemptRepoFake{recipients: attemptRecipients}
	// signingProvider stays nil: reading a stored link must not call the provider.
	return &DocumentService{documentRepo: &signingURLDocumentRepoFake{doc: doc}, attemptRepo: attempts}, attempts
}

func strPtr(s string) *string { return &s }

func TestGetSigningURL_ReturnsTheStoredProviderLink(t *testing.T) {
	svc, attempts := signingURLFixture(strPtr("attempt-1"),
		&entity.SigningAttemptRecipient{DocumentRecipientID: strPtr("rec-1"), SigningURL: strPtr("https://sign.example.test/sign/tok-ana")},
		&entity.SigningAttemptRecipient{DocumentRecipientID: strPtr("rec-2"), SigningURL: strPtr("https://sign.example.test/sign/tok-ben")},
	)

	url, err := svc.GetSigningURL(context.Background(), "doc-1", "rec-2")

	require.NoError(t, err)
	assert.Equal(t, "https://sign.example.test/sign/tok-ben", url, "the plain /sign/ link, not the embed URL")
	assert.Equal(t, "attempt-1", attempts.askedFor, "links come from the document's active attempt")
}

func TestGetSigningURL_Errors(t *testing.T) {
	ana := func(link *string) *entity.SigningAttemptRecipient {
		return &entity.SigningAttemptRecipient{DocumentRecipientID: strPtr("rec-1"), SigningURL: link}
	}

	tests := []struct {
		name        string
		activeID    *string
		attempt     []*entity.SigningAttemptRecipient
		recipientID string
		want        error
	}{
		{"recipient not on the document", strPtr("attempt-1"), []*entity.SigningAttemptRecipient{ana(strPtr("https://x/sign/t"))}, "rec-9", entity.ErrDocumentRecipientNotFound},
		{"no active attempt yet", nil, nil, "rec-1", entity.ErrSigningAttemptNotFound},
		{"link not stored yet", strPtr("attempt-1"), []*entity.SigningAttemptRecipient{ana(nil)}, "rec-1", entity.ErrSigningURLNotReady},
		{"blank link", strPtr("attempt-1"), []*entity.SigningAttemptRecipient{ana(strPtr("  "))}, "rec-1", entity.ErrSigningURLNotReady},
		{"recipient not on the attempt yet", strPtr("attempt-1"), []*entity.SigningAttemptRecipient{ana(strPtr("https://x/sign/t"))}, "rec-2", entity.ErrSigningURLNotReady},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _ := signingURLFixture(tt.activeID, tt.attempt...)

			url, err := svc.GetSigningURL(context.Background(), "doc-1", tt.recipientID)

			require.ErrorIs(t, err, tt.want)
			assert.Empty(t, url)
		})
	}
}

func TestGetDocumentPDF_RefusesDocumentsThatNeverCompleted(t *testing.T) {
	for _, status := range []entity.DocumentStatus{
		entity.DocumentStatusDraft,
		entity.DocumentStatusAwaitingInput,
		entity.DocumentStatusPreparingSignature,
		entity.DocumentStatusReadyToSign,
		entity.DocumentStatusSigning,
		entity.DocumentStatusDeclined,
		entity.DocumentStatusCancelled,
		entity.DocumentStatusError,
	} {
		t.Run(string(status), func(t *testing.T) {
			doc := &entity.DocumentWithRecipients{Document: entity.Document{ID: "doc-1", Status: status, ActiveAttemptID: strPtr("attempt-1")}}
			// Storage, provider and attempt repository stay nil: the state check
			// must answer before any of them is used.
			svc := &DocumentService{documentRepo: &signingURLDocumentRepoFake{doc: doc}, storageEnabled: true}

			pdf, _, err := svc.GetDocumentPDF(context.Background(), "doc-1")

			require.ErrorIs(t, err, entity.ErrInvalidDocumentState)
			assert.Nil(t, pdf)
		})
	}
}

func TestGetDocumentPDF_CompletedWithoutAnyPDF_NotAvailable(t *testing.T) {
	for _, status := range []entity.DocumentStatus{entity.DocumentStatusCompleted, entity.DocumentStatusInvalidated} {
		t.Run(string(status), func(t *testing.T) {
			doc := &entity.DocumentWithRecipients{Document: entity.Document{ID: "doc-1", Status: status}}
			svc := &DocumentService{documentRepo: &signingURLDocumentRepoFake{doc: doc}, storageEnabled: true}

			_, _, err := svc.GetDocumentPDF(context.Background(), "doc-1")

			require.ErrorIs(t, err, entity.ErrSignedPDFNotAvailable)
		})
	}
}
