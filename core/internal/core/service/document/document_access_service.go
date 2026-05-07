package document

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
)

// DocumentAccessService handles email-verified access to public documents.
type DocumentAccessService struct {
	documentRepo    port.DocumentRepository
	recipientRepo   port.DocumentRecipientRepository
	versionRepo     port.TemplateVersionRepository
	accessTokenRepo port.DocumentAccessTokenRepository
	notificationSvc *NotificationService
	publicURL       string
	rateLimitMax    int
	rateLimitWindow time.Duration
	tokenTTLHours   int
}

// NewDocumentAccessService creates a new document access service.
func NewDocumentAccessService(
	documentRepo port.DocumentRepository,
	recipientRepo port.DocumentRecipientRepository,
	versionRepo port.TemplateVersionRepository,
	accessTokenRepo port.DocumentAccessTokenRepository,
	notificationSvc *NotificationService,
	publicURL string,
	rateLimitMax int,
	rateLimitWindowMin int,
	tokenTTLHours int,
) *DocumentAccessService {
	return &DocumentAccessService{
		documentRepo:    documentRepo,
		recipientRepo:   recipientRepo,
		versionRepo:     versionRepo,
		accessTokenRepo: accessTokenRepo,
		notificationSvc: notificationSvc,
		publicURL:       publicURL,
		rateLimitMax:    rateLimitMax,
		rateLimitWindow: time.Duration(rateLimitWindowMin) * time.Minute,
		tokenTTLHours:   tokenTTLHours,
	}
}

// GetPublicDocumentInfo returns minimal public info about a document.
func (s *DocumentAccessService) GetPublicDocumentInfo(ctx context.Context, documentID string) (*documentuc.PublicDocumentInfoResponse, error) {
	doc, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		if errors.Is(err, entity.ErrDocumentNotFound) || errors.Is(err, entity.ErrRecordNotFound) {
			return nil, entity.ErrDocumentNotFound
		}
		return nil, err
	}

	return &documentuc.PublicDocumentInfoResponse{
		DocumentID:    doc.ID,
		DocumentTitle: documentTitle(doc),
		Status:        mapPublicStatus(doc),
	}, nil
}

// RequestAccess validates the email against document recipients and sends an access link.
// Always returns nil to prevent email enumeration.
func (s *DocumentAccessService) RequestAccess(ctx context.Context, documentID, email, language string) error {
	doc, recipient, ok := s.validateAccessRequest(ctx, documentID, email)
	if !ok {
		return nil
	}

	if err := s.generateAndSendToken(ctx, doc, recipient, language); err != nil {
		slog.ErrorContext(ctx, "failed to send access link",
			slog.String("document_id", documentID),
			slog.String("error", err.Error()),
		)
	}

	return nil
}

// RequestAccessByToken requests a new access link using an existing token as the
// entrypoint (expired-link recovery). Always returns nil to prevent enumeration.
func (s *DocumentAccessService) RequestAccessByToken(ctx context.Context, token, email, language string) error {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}

	accessToken, _ := s.accessTokenRepo.FindByToken(ctx, token)
	if accessToken == nil || (!accessToken.IsSigning() && !accessToken.IsPreSigning()) {
		return nil
	}

	return s.RequestAccess(ctx, accessToken.DocumentID, email, language)
}

// RequestDirectAccess generates a tokenized signing URL for an authenticated
// recipient (custom middleware path). It does not send email.
func (s *DocumentAccessService) RequestDirectAccess(ctx context.Context, documentID, email string) (string, error) {
	doc, recipient, ok := s.validateAccessRequest(ctx, documentID, email)
	if !ok {
		return "", fmt.Errorf("direct access denied")
	}

	tokenStr, err := s.createAccessToken(ctx, doc, recipient)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("/public/sign/%s", tokenStr), nil
}

// validateAccessRequest checks the document exists, is active, the email matches a recipient,
// and the recipient is not rate-limited. Returns false if any check fails.
func (s *DocumentAccessService) validateAccessRequest(
	ctx context.Context, documentID, email string,
) (*entity.Document, *entity.DocumentRecipient, bool) {
	doc, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		slog.InfoContext(ctx, "access request for unknown document", slog.String("document_id", documentID))
		return nil, nil, false
	}

	// Public access is allowed for non-terminal docs and COMPLETED docs.
	// DECLINED/VOIDED/EXPIRED remain unavailable.
	if doc.IsDeclined() || doc.Status == entity.DocumentStatusCancelled || doc.IsExpired() || doc.Status == entity.DocumentStatusInvalidated {
		slog.InfoContext(ctx, "access request for terminal document",
			slog.String("document_id", documentID), slog.String("status", string(doc.Status)))
		return nil, nil, false
	}

	recipient, err := s.recipientRepo.FindByDocumentAndEmail(ctx, documentID, email)
	if err != nil {
		slog.InfoContext(ctx, "access request for non-matching email", slog.String("document_id", documentID))
		return nil, nil, false
	}

	since := time.Now().UTC().Add(-s.rateLimitWindow)
	count, err := s.accessTokenRepo.CountRecentByDocumentAndRecipient(ctx, documentID, recipient.ID, since)
	if err != nil {
		slog.WarnContext(ctx, "failed to check rate limit",
			slog.String("document_id", documentID), slog.String("error", err.Error()))
		return nil, nil, false
	}
	if count >= s.rateLimitMax {
		slog.InfoContext(ctx, "access request rate limited",
			slog.String("document_id", documentID), slog.String("recipient_id", recipient.ID))
		return nil, nil, false
	}

	return doc, recipient, true
}

// generateAndSendToken creates an access token and sends it to the recipient.
func (s *DocumentAccessService) generateAndSendToken(
	ctx context.Context, doc *entity.Document, recipient *entity.DocumentRecipient, language string,
) error {
	tokenStr, err := s.createAccessToken(ctx, doc, recipient)
	if err != nil {
		return err
	}

	s.notificationSvc.SendAccessLink(ctx, recipient, doc, tokenStr, language)

	slog.InfoContext(ctx, "access link sent",
		slog.String("document_id", doc.ID),
		slog.String("recipient_id", recipient.ID),
	)

	return nil
}

// createAccessToken creates and stores a new public access token.
func (s *DocumentAccessService) createAccessToken(
	ctx context.Context,
	doc *entity.Document,
	recipient *entity.DocumentRecipient,
) (string, error) {
	tokenType := s.resolveTokenType(ctx, doc.TemplateVersionID)

	tokenStr, err := generateAccessToken()
	if err != nil {
		return "", err
	}

	now := time.Now().UTC()
	accessToken := &entity.DocumentAccessToken{
		DocumentID:  doc.ID,
		RecipientID: recipient.ID,
		Token:       tokenStr,
		TokenType:   tokenType,
		ExpiresAt:   now.Add(time.Duration(s.tokenTTLHours) * time.Hour),
		CreatedAt:   now,
	}

	if err := s.accessTokenRepo.Create(ctx, accessToken); err != nil {
		return "", err
	}

	slog.InfoContext(ctx, "access token created",
		slog.String("document_id", doc.ID),
		slog.String("recipient_id", recipient.ID),
		slog.String("token_type", tokenType),
	)

	return tokenStr, nil
}

// resolveTokenType determines whether the document needs PRE_SIGNING or SIGNING token.
func (s *DocumentAccessService) resolveTokenType(ctx context.Context, templateVersionID string) string {
	version, err := s.versionRepo.FindByID(ctx, templateVersionID)
	if err != nil || version == nil || version.ContentStructure == nil {
		return entity.TokenTypeSigning
	}

	doc, err := parsePortableDocument(version.ContentStructure)
	if err != nil {
		return entity.TokenTypeSigning
	}

	if doc.HasNodeOfType(portabledoc.NodeTypeInteractiveField) {
		return entity.TokenTypePreSigning
	}

	return entity.TokenTypeSigning
}

// mapPublicStatus maps internal document status to public-facing status.
func mapPublicStatus(doc *entity.Document) string {
	switch {
	case doc.IsCompleted():
		return "completed"
	case doc.IsExpired() || doc.Status == entity.DocumentStatusInvalidated:
		return "expired"
	case doc.IsTerminal():
		return "expired" // declined/voided shown as expired to public
	default:
		return "active"
	}
}

// Verify DocumentAccessService implements DocumentAccessUseCase.
var _ documentuc.DocumentAccessUseCase = (*DocumentAccessService)(nil)
