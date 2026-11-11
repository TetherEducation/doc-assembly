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
	timeline  bool
}

// EventEmitterOption configures an EventEmitter.
type EventEmitterOption func(*EventEmitter)

// WithDocumentTimeline turns on the signing timeline events
// (signing.document_timeline): each signer's viewed/signed/declined. Off,
// EmitTimelineEvent writes nothing.
func WithDocumentTimeline(enabled bool) EventEmitterOption {
	return func(e *EventEmitter) { e.timeline = enabled }
}

// NewEventEmitter creates a new event emitter.
func NewEventEmitter(repo port.DocumentEventRepository, opts ...EventEmitterOption) *EventEmitter {
	e := &EventEmitter{eventRepo: repo}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// TimelineEnabled reports whether signing timeline events are written.
func (e *EventEmitter) TimelineEnabled() bool {
	return e != nil && e.timeline && e.eventRepo != nil
}

// EmitTimelineEvent records a signing timeline event once (CreateOnce), when
// the timeline is enabled. A failure is logged, not returned: the timeline
// must not fail the webhook that carried the change.
func (e *EventEmitter) EmitTimelineEvent(ctx context.Context, event *entity.DocumentEvent) {
	if !e.TimelineEnabled() || event == nil {
		return
	}
	if err := e.eventRepo.CreateOnce(ctx, event); err != nil {
		slog.WarnContext(ctx, "failed to record signing timeline event",
			slog.String("document_id", event.DocumentID),
			slog.String("recipient_id", event.RecipientID),
			slog.String("event_type", event.EventType),
			slog.String("error", err.Error()),
		)
	}
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
