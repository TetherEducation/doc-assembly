package document

import (
	"bytes"
	"context"
	"embed"
	"fmt"
	"html/template"
	"log/slog"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

//go:embed templates/*.html
var templateFS embed.FS

var emailTemplates = template.Must(template.ParseFS(templateFS, "templates/*.html"))

// templateData holds the data passed to all email templates.
type templateData struct {
	RecipientName string
	DocumentTitle string
	ActionURL     string
	CompanyName   string
}

const defaultCompanyName = "Doc Engine"

// NotificationService handles sending document-related notifications.
type NotificationService struct {
	provider        port.NotificationProvider
	recipientRepo   port.DocumentRecipientRepository
	documentRepo    port.DocumentRepository
	accessTokenRepo port.DocumentAccessTokenRepository
	publicURL       string
}

// NewNotificationService creates a new notification service.
func NewNotificationService(
	provider port.NotificationProvider,
	recipientRepo port.DocumentRecipientRepository,
	documentRepo port.DocumentRepository,
	accessTokenRepo port.DocumentAccessTokenRepository,
	publicURL string,
) *NotificationService {
	return &NotificationService{
		provider:        provider,
		recipientRepo:   recipientRepo,
		documentRepo:    documentRepo,
		accessTokenRepo: accessTokenRepo,
		publicURL:       publicURL,
	}
}

// buildSigningURL builds a public signing URL for a recipient.
// Tries to find an active token for the recipient; falls back to public document URL.
func (s *NotificationService) buildSigningURL(ctx context.Context, recipient *entity.DocumentRecipient) string {
	if s.publicURL != "" {
		// Try SIGNING token first (Path A), then PRE_SIGNING (Path B).
		for _, tokenType := range []string{entity.TokenTypeSigning, entity.TokenTypePreSigning} {
			token, err := s.accessTokenRepo.FindActiveByRecipientAndType(ctx, recipient.ID, tokenType)
			if err == nil && token != nil {
				return fmt.Sprintf("%s/public/sign/%s", s.publicURL, token.Token)
			}
		}

		// Fallback: public document URL (email-verification gate).
		return fmt.Sprintf("%s/public/doc/%s", s.publicURL, recipient.DocumentID)
	}

	// Fallback to direct signing URL from provider.
	if recipient.SigningURL != nil && *recipient.SigningURL != "" {
		return *recipient.SigningURL
	}

	return ""
}

// SendReminder sends a reminder notification to pending recipients of a document.
func (s *NotificationService) SendReminder(ctx context.Context, documentID string) error {
	doc, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		return fmt.Errorf("finding document: %w", err)
	}

	if doc.IsTerminal() {
		return fmt.Errorf("cannot send reminder for document in terminal state: %s", doc.Status)
	}

	recipients, err := s.recipientRepo.FindByDocumentID(ctx, documentID)
	if err != nil {
		return fmt.Errorf("finding recipients: %w", err)
	}

	title := "Document"
	if doc.Title != nil {
		title = *doc.Title
	}

	sent := 0
	for _, recipient := range recipients {
		if recipient.IsSigned() || recipient.IsDeclined() {
			continue
		}
		if s.sendReminderToRecipient(ctx, documentID, recipient, title) {
			sent++
		}
	}

	slog.InfoContext(ctx, "reminders sent",
		slog.String("document_id", documentID),
		slog.Int("sent", sent),
		slog.Int("total_recipients", len(recipients)),
	)

	return nil
}

// sendReminderToRecipient sends a single reminder email. Returns true on success.
func (s *NotificationService) sendReminderToRecipient(
	ctx context.Context,
	documentID string,
	recipient *entity.DocumentRecipient,
	title string,
) bool {
	actionURL := s.buildSigningURL(ctx, recipient)

	body, renderErr := renderTemplate("signing_reminder.html", templateData{
		RecipientName: recipient.Name,
		DocumentTitle: title,
		ActionURL:     actionURL,
		CompanyName:   defaultCompanyName,
	})
	if renderErr != nil {
		slog.ErrorContext(ctx, "failed to render reminder template", slog.String("error", renderErr.Error()))
		return false
	}

	req := &port.NotificationRequest{
		To:       recipient.Email,
		Subject:  fmt.Sprintf("Reminder: Please sign \"%s\"", title),
		HTMLBody: body,
	}

	if err := s.provider.Send(ctx, req); err != nil {
		slog.WarnContext(ctx, "failed to send reminder",
			slog.String("document_id", documentID),
			slog.String("recipient_id", recipient.ID),
			slog.String("email", recipient.Email),
			slog.String("error", err.Error()),
		)
		return false
	}
	return true
}

// NotifyDocumentCreated sends notifications to all recipients of a newly created document.
func (s *NotificationService) NotifyDocumentCreated(ctx context.Context, documentID string) {
	doc, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		slog.WarnContext(ctx, "failed to find document for notification", slog.String("error", err.Error()))
		return
	}

	recipients, err := s.recipientRepo.FindByDocumentID(ctx, documentID)
	if err != nil {
		slog.WarnContext(ctx, "failed to find recipients for notification", slog.String("error", err.Error()))
		return
	}

	title := "Document"
	if doc.Title != nil {
		title = *doc.Title
	}

	for _, recipient := range recipients {
		actionURL := s.buildSigningURL(ctx, recipient)

		body, renderErr := renderTemplate("signing_request.html", templateData{
			RecipientName: recipient.Name,
			DocumentTitle: title,
			ActionURL:     actionURL,
			CompanyName:   defaultCompanyName,
		})
		if renderErr != nil {
			slog.ErrorContext(ctx, "failed to render signing request template", slog.String("error", renderErr.Error()))
			continue
		}

		req := &port.NotificationRequest{
			To:       recipient.Email,
			Subject:  fmt.Sprintf("Document ready for signature: \"%s\"", title),
			HTMLBody: body,
		}

		if err := s.provider.Send(ctx, req); err != nil {
			slog.WarnContext(ctx, "failed to send document created notification",
				slog.String("document_id", documentID),
				slog.String("email", recipient.Email),
				slog.String("error", err.Error()),
			)
		}
	}
}

// SendAccessLink sends an email with a signing access link to a recipient.
func (s *NotificationService) SendAccessLink(ctx context.Context, recipient *entity.DocumentRecipient, doc *entity.Document, tokenStr string, language string) {
	title := "Document"
	if doc.Title != nil {
		title = *doc.Title
	}

	actionURL := appendSigningURLLanguage(fmt.Sprintf("%s/public/sign/%s", s.publicURL, tokenStr), language)

	body, renderErr := renderTemplate("access_link.html", templateData{
		RecipientName: recipient.Name,
		DocumentTitle: title,
		ActionURL:     actionURL,
		CompanyName:   defaultCompanyName,
	})
	if renderErr != nil {
		slog.ErrorContext(ctx, "failed to render access link template", slog.String("error", renderErr.Error()))
		return
	}

	req := &port.NotificationRequest{
		To:       recipient.Email,
		Subject:  fmt.Sprintf("Your signing link for \"%s\"", title),
		HTMLBody: body,
	}

	if err := s.provider.Send(ctx, req); err != nil {
		slog.WarnContext(ctx, "failed to send access link email",
			slog.String("document_id", doc.ID),
			slog.String("email", recipient.Email),
			slog.String("error", err.Error()),
		)
	}
}

// NotifyDocumentCompleted sends a notification that the document is fully signed.
func (s *NotificationService) NotifyDocumentCompleted(ctx context.Context, documentID string) {
	s.notifyDocumentStatusChange(
		ctx,
		documentID,
		"signing_completed.html",
		"Document signed: \"%s\"",
		"failed to find document for completion notification",
		"failed to find recipients for completion notification",
		"failed to render completed template",
		"failed to send completion notification",
	)
}

// NotifyDocumentDeclined sends a notification that a signer has declined.
func (s *NotificationService) NotifyDocumentDeclined(ctx context.Context, documentID string) {
	s.notifyDocumentStatusChange(
		ctx,
		documentID,
		"signing_declined.html",
		"Signing declined: \"%s\"",
		"failed to find document for declined notification",
		"failed to find recipients for declined notification",
		"failed to render declined template",
		"failed to send declined notification",
	)
}

// NotifyDocumentExpired sends a notification that the signing period has expired.
func (s *NotificationService) NotifyDocumentExpired(ctx context.Context, documentID string) {
	s.notifyDocumentStatusChange(
		ctx,
		documentID,
		"document_expired.html",
		"Document expired: \"%s\"",
		"failed to find document for expiration notification",
		"failed to find recipients for expiration notification",
		"failed to render expired template",
		"failed to send expiration notification",
	)
}

func (s *NotificationService) notifyDocumentStatusChange(
	ctx context.Context,
	documentID string,
	templateName string,
	subjectFmt string,
	findDocumentErrMsg string,
	findRecipientsErrMsg string,
	renderErrMsg string,
	sendErrMsg string,
) {
	doc, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		slog.WarnContext(ctx, findDocumentErrMsg, slog.String("error", err.Error()))
		return
	}

	recipients, err := s.recipientRepo.FindByDocumentID(ctx, documentID)
	if err != nil {
		slog.WarnContext(ctx, findRecipientsErrMsg, slog.String("error", err.Error()))
		return
	}

	title := "Document"
	if doc.Title != nil {
		title = *doc.Title
	}

	for _, recipient := range recipients {
		body, renderErr := renderTemplate(templateName, templateData{
			RecipientName: recipient.Name,
			DocumentTitle: title,
			CompanyName:   defaultCompanyName,
		})
		if renderErr != nil {
			slog.ErrorContext(ctx, renderErrMsg, slog.String("error", renderErr.Error()))
			continue
		}

		req := &port.NotificationRequest{
			To:       recipient.Email,
			Subject:  fmt.Sprintf(subjectFmt, title),
			HTMLBody: body,
		}

		if err := s.provider.Send(ctx, req); err != nil {
			slog.WarnContext(ctx, sendErrMsg,
				slog.String("document_id", documentID),
				slog.String("email", recipient.Email),
				slog.String("error", err.Error()),
			)
		}
	}
}

// renderTemplate executes the named template with the given data and returns the rendered HTML.
func renderTemplate(name string, data templateData) (string, error) {
	var buf bytes.Buffer
	if err := emailTemplates.ExecuteTemplate(&buf, name, data); err != nil {
		return "", fmt.Errorf("executing template %s: %w", name, err)
	}
	return buf.String(), nil
}
