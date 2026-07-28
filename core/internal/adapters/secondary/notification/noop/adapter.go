package noop

import (
	"context"
	"log/slog"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// Adapter implements port.NotificationProvider as a no-op (logs only).
type Adapter struct{}

// New creates a new no-op notification adapter.
func New() port.NotificationProvider {
	return &Adapter{}
}

// Send logs the notification but does not actually send it.
func (a *Adapter) Send(ctx context.Context, req *port.NotificationRequest) error {
	slog.InfoContext(ctx, "notification (noop)",
		slog.String("to", req.To),
		slog.String("subject", req.Subject),
	)
	return nil
}
