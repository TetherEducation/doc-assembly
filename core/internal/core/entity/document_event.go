package entity

import (
	"encoding/json"
	"time"
)

// Event type constants.
const (
	EventDocumentCreated    = "DOCUMENT_CREATED"
	EventDocumentSent       = "DOCUMENT_SENT"
	EventDocumentCancelled  = "DOCUMENT_CANCELLED"
	EventDocumentCompleted  = "DOCUMENT_COMPLETED"
	EventDocumentDeprecated = "DOCUMENT_DEPRECATED"
	EventDocumentExpired    = "DOCUMENT_EXPIRED"
	EventDocumentError      = "DOCUMENT_ERROR"
	EventRecipientSent      = "RECIPIENT_SENT"
	EventRecipientDelivered = "RECIPIENT_DELIVERED"
	EventRecipientSigned    = "RECIPIENT_SIGNED"
	EventRecipientDeclined  = "RECIPIENT_DECLINED"
	EventWebhookReceived    = "WEBHOOK_RECEIVED"
	EventStatusRefreshed    = "STATUS_REFRESHED"
)

// Actor type constants.
const (
	ActorSystem    = "SYSTEM"
	ActorUser      = "USER"
	ActorWebhook   = "WEBHOOK"
	ActorScheduler = "SCHEDULER"
)

// DocumentEvent represents an audit event for a document lifecycle transition.
type DocumentEvent struct {
	ID          string          `json:"id"`
	DocumentID  string          `json:"documentId"`
	EventType   string          `json:"eventType"`
	ActorType   string          `json:"actorType"`
	ActorID     string          `json:"actorId,omitempty"`
	OldStatus   string          `json:"oldStatus,omitempty"`
	NewStatus   string          `json:"newStatus,omitempty"`
	RecipientID string          `json:"recipientId,omitempty"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	CreatedAt   time.Time       `json:"createdAt"`
}

// NewDocumentEvent creates a new document event.
func NewDocumentEvent(documentID, eventType, actorType, actorID string) *DocumentEvent {
	return &DocumentEvent{
		DocumentID: documentID,
		EventType:  eventType,
		ActorType:  actorType,
		ActorID:    actorID,
		CreatedAt:  time.Now().UTC(),
	}
}
