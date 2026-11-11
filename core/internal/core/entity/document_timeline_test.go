package entity

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimelineEventForAttemptStatus(t *testing.T) {
	cases := []struct {
		old, new SigningAttemptStatus
		want     string
	}{
		{SigningAttemptStatusSubmittingProvider, SigningAttemptStatusSigningReady, EventDocumentSent},
		{SigningAttemptStatusReconcilingProvider, SigningAttemptStatusSigning, EventDocumentSent},
		{SigningAttemptStatusSigningReady, SigningAttemptStatusSigning, ""},
		{SigningAttemptStatusSigning, SigningAttemptStatusSigning, ""},
		{SigningAttemptStatusSigning, SigningAttemptStatusCompleted, EventDocumentCompleted},
		{SigningAttemptStatusSigningReady, SigningAttemptStatusCompleted, EventDocumentCompleted},
		{SigningAttemptStatusSigning, SigningAttemptStatusDeclined, EventDocumentDeclined},
		{SigningAttemptStatusCompleted, SigningAttemptStatusCompleted, ""},
		{SigningAttemptStatusCreated, SigningAttemptStatusRendering, ""},
		{SigningAttemptStatusSigning, SigningAttemptStatusCancelled, ""},
		{SigningAttemptStatusSigning, SigningAttemptStatusInvalidated, ""},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, TimelineEventForAttemptStatus(tc.old, tc.new), "%s -> %s", tc.old, tc.new)
	}
}

func TestTimelineEventForRecipientStatus(t *testing.T) {
	cases := []struct {
		old, new RecipientStatus
		want     string
	}{
		{RecipientStatusPending, RecipientStatusDelivered, EventRecipientDelivered},
		{RecipientStatusSent, RecipientStatusDelivered, EventRecipientDelivered},
		{RecipientStatusDelivered, RecipientStatusDelivered, ""},
		{RecipientStatusSigned, RecipientStatusDelivered, ""},
		{RecipientStatusDeclined, RecipientStatusDelivered, ""},
		{RecipientStatusDelivered, RecipientStatusSigned, EventRecipientSigned},
		{RecipientStatusPending, RecipientStatusSigned, EventRecipientSigned},
		{RecipientStatusSigned, RecipientStatusSigned, ""},
		{RecipientStatusDelivered, RecipientStatusDeclined, EventRecipientDeclined},
		{RecipientStatusDelivered, RecipientStatusRejected, EventRecipientDeclined},
		{RecipientStatusRejected, RecipientStatusDeclined, ""},
		{RecipientStatusPending, RecipientStatusSent, ""},
		{RecipientStatusWaiting, RecipientStatusPending, ""},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, TimelineEventForRecipientStatus(tc.old, tc.new), "%s -> %s", tc.old, tc.new)
	}
}

func TestNewRecipientTimelineEvent(t *testing.T) {
	docRecipientID := "11111111-1111-1111-1111-111111111111"
	at := time.Date(2026, 11, 2, 15, 4, 5, 0, time.FixedZone("CLT", -3*3600))
	rec := &SigningAttemptRecipient{
		AttemptID:           "attempt-1",
		DocumentRecipientID: &docRecipientID,
		Name:                "Ana Signer",
		Email:               "ana@example.com",
		Status:              RecipientStatusSigned,
	}

	event := NewRecipientTimelineEvent("doc-1", rec, EventRecipientSigned, ActorWebhook, RecipientStatusDelivered, at, TimelineTimeProvider)

	assert.Equal(t, "doc-1", event.DocumentID)
	assert.Equal(t, docRecipientID, event.RecipientID)
	assert.Equal(t, EventRecipientSigned, event.EventType)
	assert.Equal(t, "DELIVERED", event.OldStatus)
	assert.Equal(t, "SIGNED", event.NewStatus)
	assert.True(t, event.CreatedAt.Equal(at))
	assert.Equal(t, time.UTC, event.CreatedAt.Location())
	var meta TimelineMetadata
	require.NoError(t, json.Unmarshal(event.Metadata, &meta))
	assert.Equal(t, TimelineMetadata{AttemptID: "attempt-1", RecipientName: "Ana Signer", RecipientEmail: "ana@example.com", TimeSource: TimelineTimeProvider}, meta)
}

func TestNewAttemptTimelineEvent(t *testing.T) {
	attempt := &SigningAttempt{ID: "attempt-1", DocumentID: "doc-1"}
	event := NewAttemptTimelineEvent(attempt, EventDocumentCompleted, ActorSystem, SigningAttemptStatusSigning, SigningAttemptStatusCompleted, time.Now())

	assert.Equal(t, "doc-1", event.DocumentID)
	assert.Empty(t, event.RecipientID)
	assert.Equal(t, string(ProjectDocumentStatusFromAttempt(SigningAttemptStatusCompleted)), event.NewStatus)
	var meta TimelineMetadata
	require.NoError(t, json.Unmarshal(event.Metadata, &meta))
	assert.Equal(t, "attempt-1", meta.AttemptID)
}
