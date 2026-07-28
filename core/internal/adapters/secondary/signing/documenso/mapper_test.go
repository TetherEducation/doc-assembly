package documenso

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

func TestMapEnvelopeStatus(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect entity.SigningAttemptStatus
	}{
		{"CREATED -> DRAFT", "CREATED", entity.SigningAttemptStatusSigningReady},
		{"PENDING -> PENDING", "PENDING", entity.SigningAttemptStatusSigningReady},
		{"SENT -> PENDING", "SENT", entity.SigningAttemptStatusSigningReady},
		{"OPENED -> IN_PROGRESS", "OPENED", entity.SigningAttemptStatusSigning},
		{"SIGNED -> COMPLETED", "SIGNED", entity.SigningAttemptStatusCompleted},
		{"COMPLETED -> COMPLETED", "COMPLETED", entity.SigningAttemptStatusCompleted},
		{"REJECTED -> DECLINED", "REJECTED", entity.SigningAttemptStatusDeclined},
		{"CANCELLED -> VOIDED", "CANCELLED", entity.SigningAttemptStatusCancelled},
		{"unknown -> ERROR", "UNKNOWN_STATUS", entity.SigningAttemptStatusRequiresReview},
		{"empty -> ERROR", "", entity.SigningAttemptStatusRequiresReview},
		{"lowercase created -> DRAFT", "created", entity.SigningAttemptStatusSigningReady},
		{"mixed case Sent -> PENDING", "Sent", entity.SigningAttemptStatusSigningReady},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, MapEnvelopeStatus(tt.input))
		})
	}
}

func TestMapRecipientStatus(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect entity.RecipientStatus
	}{
		{"PENDING -> PENDING", "PENDING", entity.RecipientStatusPending},
		{"SENT -> SENT", "SENT", entity.RecipientStatusSent},
		{"OPENED -> DELIVERED", "OPENED", entity.RecipientStatusDelivered},
		{"SIGNED -> SIGNED", "SIGNED", entity.RecipientStatusSigned},
		{"COMPLETED -> SIGNED", "COMPLETED", entity.RecipientStatusSigned},
		{"REJECTED -> DECLINED", "REJECTED", entity.RecipientStatusDeclined},
		{"unknown -> PENDING", "UNKNOWN", entity.RecipientStatusPending},
		{"empty -> PENDING", "", entity.RecipientStatusPending},
		{"lowercase sent -> SENT", "sent", entity.RecipientStatusSent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, MapRecipientStatus(tt.input))
		})
	}
}

func TestProcessRecipientsTreatsSignedAtAsSigned(t *testing.T) {
	signedAt := time.Date(2026, time.April, 21, 23, 6, 42, 0, time.UTC).Format(time.RFC3339)

	results, allSigned, anyDeclined := processRecipients([]recipientResponse{
		{
			ID:       123,
			Status:   "PENDING",
			SignedAt: signedAt,
		},
	})

	assert.False(t, anyDeclined)
	assert.True(t, allSigned)
	assert.Len(t, results, 1)
	assert.Equal(t, "123", results[0].ProviderRecipientID)
	assert.Equal(t, entity.RecipientStatusSigned, results[0].Status)
	assert.NotNil(t, results[0].SignedAt)
	assert.Equal(t, "PENDING", results[0].ProviderStatus)
}

func TestMapWebhookEvent(t *testing.T) {
	tests := []struct {
		name            string
		eventType       string
		wantDocStatus   *entity.SigningAttemptStatus
		wantRecipStatus *entity.RecipientStatus
	}{
		{
			name:          "document.created",
			eventType:     "document.created",
			wantDocStatus: docStatusPtr(entity.SigningAttemptStatusSigningReady),
		},
		{
			name:            "document.sent",
			eventType:       "document.sent",
			wantDocStatus:   docStatusPtr(entity.SigningAttemptStatusSigningReady),
			wantRecipStatus: recipStatusPtr(entity.RecipientStatusSent),
		},
		{
			name:            "document.opened",
			eventType:       "document.opened",
			wantDocStatus:   docStatusPtr(entity.SigningAttemptStatusSigning),
			wantRecipStatus: recipStatusPtr(entity.RecipientStatusDelivered),
		},
		{
			name:            "document.signed",
			eventType:       "document.signed",
			wantDocStatus:   nil, // determined by service layer
			wantRecipStatus: recipStatusPtr(entity.RecipientStatusSigned),
		},
		{
			name:          "document.completed",
			eventType:     "document.completed",
			wantDocStatus: docStatusPtr(entity.SigningAttemptStatusCompleted),
		},
		{
			name:            "document.rejected",
			eventType:       "document.rejected",
			wantDocStatus:   docStatusPtr(entity.SigningAttemptStatusDeclined),
			wantRecipStatus: recipStatusPtr(entity.RecipientStatusDeclined),
		},
		{
			name:          "document.cancelled",
			eventType:     "document.cancelled",
			wantDocStatus: docStatusPtr(entity.SigningAttemptStatusCancelled),
		},
		{
			name:          "DOCUMENT_CREATED (underscore format)",
			eventType:     "DOCUMENT_CREATED",
			wantDocStatus: docStatusPtr(entity.SigningAttemptStatusSigningReady),
		},
		{
			name:            "DOCUMENT_SENT (underscore format)",
			eventType:       "DOCUMENT_SENT",
			wantDocStatus:   docStatusPtr(entity.SigningAttemptStatusSigningReady),
			wantRecipStatus: recipStatusPtr(entity.RecipientStatusSent),
		},
		{
			name:            "DOCUMENT_OPENED (underscore format)",
			eventType:       "DOCUMENT_OPENED",
			wantDocStatus:   docStatusPtr(entity.SigningAttemptStatusSigning),
			wantRecipStatus: recipStatusPtr(entity.RecipientStatusDelivered),
		},
		{
			name:            "DOCUMENT_SIGNED (underscore format)",
			eventType:       "DOCUMENT_SIGNED",
			wantDocStatus:   nil,
			wantRecipStatus: recipStatusPtr(entity.RecipientStatusSigned),
		},
		{
			name:          "DOCUMENT_COMPLETED (underscore format)",
			eventType:     "DOCUMENT_COMPLETED",
			wantDocStatus: docStatusPtr(entity.SigningAttemptStatusCompleted),
		},
		{
			name:      "unknown event returns empty mapping",
			eventType: "unknown.event",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapping := MapWebhookEvent(tt.eventType)

			if tt.wantDocStatus != nil {
				assert.NotNil(t, mapping.DocumentStatus)
				assert.Equal(t, *tt.wantDocStatus, *mapping.DocumentStatus)
			} else {
				assert.Nil(t, mapping.DocumentStatus)
			}

			if tt.wantRecipStatus != nil {
				assert.NotNil(t, mapping.RecipientStatus)
				assert.Equal(t, *tt.wantRecipStatus, *mapping.RecipientStatus)
			} else {
				assert.Nil(t, mapping.RecipientStatus)
			}
		})
	}
}

func TestInternalToDocumensoStatus(t *testing.T) {
	tests := []struct {
		name   string
		input  entity.SigningAttemptStatus
		expect DocumensoEnvelopeStatus
	}{
		{"CREATED -> CREATED", entity.SigningAttemptStatusCreated, EnvelopeStatusCreated},
		{"PENDING -> SENT", entity.SigningAttemptStatusSigningReady, EnvelopeStatusSent},
		{"IN_PROGRESS -> OPENED", entity.SigningAttemptStatusSigning, EnvelopeStatusOpened},
		{"COMPLETED -> COMPLETED", entity.SigningAttemptStatusCompleted, EnvelopeStatusCompleted},
		{"DECLINED -> REJECTED", entity.SigningAttemptStatusDeclined, EnvelopeStatusRejected},
		{"VOIDED -> CANCELLED", entity.SigningAttemptStatusCancelled, EnvelopeStatusCancelled},
		{"FAILED_PERMANENT -> empty", entity.SigningAttemptStatusFailedPermanent, DocumensoEnvelopeStatus("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expect, InternalToDocumensoStatus(tt.input))
		})
	}
}

func docStatusPtr(s entity.SigningAttemptStatus) *entity.SigningAttemptStatus {
	return &s
}

func recipStatusPtr(s entity.RecipientStatus) *entity.RecipientStatus {
	return &s
}
