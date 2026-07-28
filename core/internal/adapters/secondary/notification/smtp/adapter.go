package smtp

import (
	"context"
	"fmt"
	"log/slog"
	"net/smtp"
	"strings"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// Config holds SMTP connection configuration.
type Config struct {
	Host     string
	Port     int
	Username string
	Password string //nolint:gosec
	From     string
}

// Adapter implements port.NotificationProvider using SMTP.
type Adapter struct {
	cfg Config
}

// New creates a new SMTP notification adapter.
func New(cfg *Config) port.NotificationProvider {
	return &Adapter{cfg: *cfg}
}

// Send sends an email via SMTP.
func (a *Adapter) Send(ctx context.Context, req *port.NotificationRequest) error {
	addr := fmt.Sprintf("%s:%d", a.cfg.Host, a.cfg.Port)

	msg := buildMessage(a.cfg.From, req)

	var auth smtp.Auth
	if a.cfg.Username != "" {
		auth = smtp.PlainAuth("", a.cfg.Username, a.cfg.Password, a.cfg.Host)
	}

	if err := smtp.SendMail(addr, auth, a.cfg.From, []string{req.To}, []byte(msg)); err != nil {
		return fmt.Errorf("sending email via SMTP: %w", err)
	}

	slog.InfoContext(ctx, "notification sent via SMTP",
		slog.String("to", req.To),
		slog.String("subject", req.Subject),
	)

	return nil
}

// buildMessage constructs the raw email message.
func buildMessage(from string, req *port.NotificationRequest) string {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + req.To + "\r\n")
	b.WriteString("Subject: " + req.Subject + "\r\n")
	if req.ReplyTo != "" {
		b.WriteString("Reply-To: " + req.ReplyTo + "\r\n")
	}
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: text/html; charset=\"UTF-8\"\r\n")
	b.WriteString("\r\n")
	b.WriteString(req.HTMLBody)
	return b.String()
}
