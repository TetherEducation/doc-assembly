package documenso

import (
	"strings"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

type DocumensoEnvelopeStatus string

const (
	EnvelopeStatusCreated   DocumensoEnvelopeStatus = "CREATED"
	EnvelopeStatusPending   DocumensoEnvelopeStatus = "PENDING"
	EnvelopeStatusSent      DocumensoEnvelopeStatus = "SENT"
	EnvelopeStatusOpened    DocumensoEnvelopeStatus = "OPENED"
	EnvelopeStatusSigned    DocumensoEnvelopeStatus = "SIGNED"
	EnvelopeStatusCompleted DocumensoEnvelopeStatus = "COMPLETED"
	EnvelopeStatusRejected  DocumensoEnvelopeStatus = "REJECTED"
	EnvelopeStatusCancelled DocumensoEnvelopeStatus = "CANCELLED"
)

type DocumensoRecipientStatus string

const (
	RecipientStatusPending   DocumensoRecipientStatus = "PENDING"
	RecipientStatusSent      DocumensoRecipientStatus = "SENT"
	RecipientStatusOpened    DocumensoRecipientStatus = "OPENED"
	RecipientStatusSigned    DocumensoRecipientStatus = "SIGNED"
	RecipientStatusCompleted DocumensoRecipientStatus = "COMPLETED"
	RecipientStatusRejected  DocumensoRecipientStatus = "REJECTED"
)

func MapEnvelopeStatus(status string) entity.SigningAttemptStatus {
	switch DocumensoEnvelopeStatus(strings.ToUpper(status)) {
	case EnvelopeStatusCreated, EnvelopeStatusPending, EnvelopeStatusSent:
		return entity.SigningAttemptStatusSigningReady
	case EnvelopeStatusOpened:
		return entity.SigningAttemptStatusSigning
	case EnvelopeStatusSigned, EnvelopeStatusCompleted:
		return entity.SigningAttemptStatusCompleted
	case EnvelopeStatusRejected:
		return entity.SigningAttemptStatusDeclined
	case EnvelopeStatusCancelled:
		return entity.SigningAttemptStatusCancelled
	default:
		return entity.SigningAttemptStatusRequiresReview
	}
}

func MapRecipientStatus(status string) entity.RecipientStatus {
	switch DocumensoRecipientStatus(strings.ToUpper(status)) {
	case RecipientStatusPending:
		return entity.RecipientStatusPending
	case RecipientStatusSent:
		return entity.RecipientStatusSent
	case RecipientStatusOpened:
		return entity.RecipientStatusDelivered
	case RecipientStatusSigned, RecipientStatusCompleted:
		return entity.RecipientStatusSigned
	case RecipientStatusRejected:
		return entity.RecipientStatusDeclined
	default:
		return entity.RecipientStatusPending
	}
}

type WebhookEventMapping struct {
	DocumentStatus  *entity.SigningAttemptStatus
	RecipientStatus *entity.RecipientStatus
}

func MapWebhookEvent(eventType string) WebhookEventMapping {
	mapping := WebhookEventMapping{}
	normalized := strings.ToLower(eventType)
	normalized = strings.ReplaceAll(normalized, "_", ".")

	switch normalized {
	case "document.created":
		status := entity.SigningAttemptStatusSigningReady
		mapping.DocumentStatus = &status
	case "document.sent":
		status := entity.SigningAttemptStatusSigningReady
		recipientStatus := entity.RecipientStatusSent
		mapping.DocumentStatus = &status
		mapping.RecipientStatus = &recipientStatus
	case "document.opened":
		status := entity.SigningAttemptStatusSigning
		recipientStatus := entity.RecipientStatusDelivered
		mapping.DocumentStatus = &status
		mapping.RecipientStatus = &recipientStatus
	case "document.signed":
		recipientStatus := entity.RecipientStatusSigned
		mapping.RecipientStatus = &recipientStatus
	case "document.completed":
		status := entity.SigningAttemptStatusCompleted
		mapping.DocumentStatus = &status
	case "document.rejected":
		status := entity.SigningAttemptStatusDeclined
		recipientStatus := entity.RecipientStatusDeclined
		mapping.DocumentStatus = &status
		mapping.RecipientStatus = &recipientStatus
	case "document.cancelled":
		status := entity.SigningAttemptStatusCancelled
		mapping.DocumentStatus = &status
	}
	return mapping
}

func InternalToDocumensoStatus(status entity.SigningAttemptStatus) DocumensoEnvelopeStatus {
	switch status {
	case entity.SigningAttemptStatusCreated:
		return EnvelopeStatusCreated
	case entity.SigningAttemptStatusSigningReady:
		return EnvelopeStatusSent
	case entity.SigningAttemptStatusSigning:
		return EnvelopeStatusOpened
	case entity.SigningAttemptStatusCompleted:
		return EnvelopeStatusCompleted
	case entity.SigningAttemptStatusDeclined:
		return EnvelopeStatusRejected
	case entity.SigningAttemptStatusCancelled:
		return EnvelopeStatusCancelled
	default:
		return ""
	}
}
