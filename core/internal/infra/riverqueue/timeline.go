package riverqueue

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// recordAttemptTimelineTx records the document-level signing timeline event
// (DOCUMENT_SENT, DOCUMENT_COMPLETED or DOCUMENT_DECLINED) for the active
// attempt moving from old to its current status. Callers only reach it for the
// attempt the document projects (they check ownership under lock first).
func recordAttemptTimelineTx(ctx context.Context, tx pgx.Tx, timelineEvents port.DocumentEventRepository, attempt *entity.SigningAttempt, old entity.SigningAttemptStatus) error {
	if timelineEvents == nil {
		return nil
	}
	eventType := entity.TimelineEventForAttemptStatus(old, attempt.Status)
	if eventType == "" {
		return nil
	}
	event := entity.NewAttemptTimelineEvent(attempt, eventType, entity.ActorSystem, old, attempt.Status, time.Now().UTC())
	return recordTimelineEventTx(ctx, tx, timelineEvents, event)
}

// recordTimelineEventTx writes a timeline event once inside tx, under a
// savepoint: a timeline write that fails is logged and rolled back alone, and
// never fails the signing transition it describes. A nil repository (the
// signing.document_timeline flag off) writes nothing.
func recordTimelineEventTx(ctx context.Context, tx pgx.Tx, timelineEvents port.DocumentEventRepository, event *entity.DocumentEvent) error {
	if timelineEvents == nil || event == nil {
		return nil
	}
	sp, err := tx.Begin(ctx)
	if err != nil {
		return err
	}
	if err := timelineEvents.CreateOnceTx(ctx, sp, event); err != nil {
		_ = sp.Rollback(ctx)
		slog.WarnContext(ctx, "failed to record signing timeline event",
			slog.String("document_id", event.DocumentID),
			slog.String("event_type", event.EventType),
			slog.String("error", err.Error()),
		)
		return nil
	}
	return sp.Commit(ctx)
}
