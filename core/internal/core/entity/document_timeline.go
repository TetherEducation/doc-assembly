package entity

import (
	"encoding/json"
	"time"
)

// Signing timeline (signing.document_timeline): besides DOCUMENT_CREATED and the
// user actions, the engine records in document_events what happens at the
// provider: the document going out, each signer viewing, signing or declining,
// and the document completing or being declined. These helpers decide which
// status changes are timeline events; they are written with
// DocumentEventRepository.CreateOnce, so a webhook and a status refresh that
// both see a change record it once.

// sentAttemptStatus reports whether the provider has the document out for
// signing.
func sentAttemptStatus(s SigningAttemptStatus) bool {
	return s == SigningAttemptStatusSigningReady || s == SigningAttemptStatusSigning
}

// TimelineEventForAttemptStatus returns the document-level timeline event for
// an attempt moving from oldStatus to newStatus, or "" when the change is not
// one: SIGNING_READY/SIGNING → DOCUMENT_SENT, COMPLETED → DOCUMENT_COMPLETED,
// DECLINED → DOCUMENT_DECLINED.
func TimelineEventForAttemptStatus(oldStatus, newStatus SigningAttemptStatus) string {
	if oldStatus == newStatus {
		return ""
	}
	switch {
	case sentAttemptStatus(newStatus) && !sentAttemptStatus(oldStatus):
		return EventDocumentSent
	case newStatus == SigningAttemptStatusCompleted:
		return EventDocumentCompleted
	case newStatus == SigningAttemptStatusDeclined:
		return EventDocumentDeclined
	}
	return ""
}

// TimelineEventForRecipientStatus returns the signer timeline event for a
// signer moving from oldStatus to newStatus, or "" when the change is not one.
// DELIVERED is the provider's "opened" (viewed); a signer who has already
// signed or declined is not reported as viewing again.
func TimelineEventForRecipientStatus(oldStatus, newStatus RecipientStatus) string {
	oldStatus, newStatus = oldStatus.Normalize(), newStatus.Normalize()
	if oldStatus == newStatus {
		return ""
	}
	switch newStatus {
	case RecipientStatusDelivered:
		if oldStatus == RecipientStatusSigned || oldStatus == RecipientStatusDeclined {
			return ""
		}
		return EventRecipientDelivered
	case RecipientStatusSigned:
		return EventRecipientSigned
	case RecipientStatusDeclined:
		return EventRecipientDeclined
	}
	return ""
}

// NewAttemptTimelineEvent builds a document-level timeline event for attempt.
func NewAttemptTimelineEvent(attempt *SigningAttempt, eventType, actorType string, oldStatus, newStatus SigningAttemptStatus, at time.Time) *DocumentEvent {
	event := NewDocumentEvent(attempt.DocumentID, eventType, actorType, "")
	event.OldStatus = string(ProjectDocumentStatusFromAttempt(oldStatus))
	event.NewStatus = string(ProjectDocumentStatusFromAttempt(newStatus))
	event.Metadata = timelineMetadata(TimelineMetadata{AttemptID: attempt.ID})
	event.CreatedAt = at.UTC()
	return event
}

// NewRecipientTimelineEvent builds a signer timeline event for an attempt
// recipient on documentID. Its recipient is the document recipient the panel
// lists; its metadata names the signer, since the panel shows no recipient IDs.
func NewRecipientTimelineEvent(documentID string, rec *SigningAttemptRecipient, eventType, actorType string, oldStatus RecipientStatus, at time.Time, timeSource string) *DocumentEvent {
	event := NewDocumentEvent(documentID, eventType, actorType, "")
	if rec.DocumentRecipientID != nil {
		event.RecipientID = *rec.DocumentRecipientID
	}
	event.OldStatus = string(oldStatus.Normalize())
	event.NewStatus = string(rec.Status.Normalize())
	event.Metadata = timelineMetadata(TimelineMetadata{
		AttemptID:      rec.AttemptID,
		RecipientName:  rec.Name,
		RecipientEmail: rec.Email,
		TimeSource:     timeSource,
	})
	event.CreatedAt = at.UTC()
	return event
}

func timelineMetadata(meta TimelineMetadata) json.RawMessage {
	raw, err := json.Marshal(meta)
	if err != nil {
		return nil
	}
	return raw
}
