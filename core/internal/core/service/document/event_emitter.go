package document

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// EventEmitter handles creating document audit events.
type EventEmitter struct {
	eventRepo port.DocumentEventRepository
}

// NewEventEmitter creates a new event emitter.
func NewEventEmitter(repo port.DocumentEventRepository) *EventEmitter {
	return &EventEmitter{eventRepo: repo}
}

// EmitDocumentEvent creates a document-level event.
func (e *EventEmitter) EmitDocumentEvent(
	ctx context.Context,
	documentID, eventType, actorType, actorID, oldStatus, newStatus string,
	metadata json.RawMessage,
) {
	event := entity.NewDocumentEvent(documentID, eventType, actorType, actorID)
	event.OldStatus = oldStatus
	event.NewStatus = newStatus
	event.Metadata = metadata

	if err := e.eventRepo.Create(ctx, event); err != nil {
		slog.WarnContext(ctx, "failed to emit document event",
			slog.String("document_id", documentID),
			slog.String("event_type", eventType),
			slog.String("error", err.Error()),
		)
	}
}

// EmitRecipientEvent creates a recipient-level event.
func (e *EventEmitter) EmitRecipientEvent(
	ctx context.Context,
	documentID, recipientID, eventType, actorType, actorID, oldStatus, newStatus string,
) {
	event := entity.NewDocumentEvent(documentID, eventType, actorType, actorID)
	event.RecipientID = recipientID
	event.OldStatus = oldStatus
	event.NewStatus = newStatus

	if err := e.eventRepo.Create(ctx, event); err != nil {
		slog.WarnContext(ctx, "failed to emit recipient event",
			slog.String("document_id", documentID),
			slog.String("recipient_id", recipientID),
			slog.String("event_type", eventType),
			slog.String("error", err.Error()),
		)
	}
}

// GetDocumentEvents retrieves events for a document.
func (e *EventEmitter) GetDocumentEvents(ctx context.Context, documentID string, limit, offset int) ([]*entity.DocumentEvent, error) {
	return e.eventRepo.FindByDocumentID(ctx, documentID, limit, offset)
}
