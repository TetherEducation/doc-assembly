package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// NotificationRequest contains the data needed to send a notification.
type NotificationRequest struct {
	To          string // Recipient email
	Subject     string
	HTMLBody    string
	TextBody    string             // Optional plain-text fallback
	ReplyTo     string             // Optional reply-to address
	Environment entity.Environment // dev or prod
	Attachments []NotificationAttachment
}

// NotificationAttachment represents an email attachment.
type NotificationAttachment struct {
	Filename    string
	ContentType string
	Data        []byte
}

// NotificationProvider defines the interface for sending notifications.
type NotificationProvider interface {
	// Send sends a notification.
	Send(ctx context.Context, req *NotificationRequest) error
}
