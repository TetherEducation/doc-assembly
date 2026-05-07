package document

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
)

const (
	defaultSigningSessionTokenTTLHours = 48
	defaultSigningSessionRateWindow    = time.Minute
	defaultSigningSessionRateMax       = 60
)

type signingSessionRateEntry struct {
	windowStart time.Time
	count       int
}

// SigningSessionService handles authenticated signing session creation for
// embedded CRM flows.
type SigningSessionService struct {
	documentRepo      port.DocumentRepository
	recipientRepo     port.DocumentRecipientRepository
	versionRepo       port.TemplateVersionRepository
	accessTokenRepo   port.DocumentAccessTokenRepository
	preSigningUC      documentuc.PreSigningUseCase
	storageAdapter    port.StorageAdapter
	storageEnabled    bool
	tokenTTLHours     int
	rateLimitWindow   time.Duration
	rateLimitMax      int
	rateLimitMu       sync.Mutex
	rateLimitByTarget map[string]signingSessionRateEntry
}

// NewSigningSessionService creates a new signing session service.
func NewSigningSessionService(
	documentRepo port.DocumentRepository,
	recipientRepo port.DocumentRecipientRepository,
	versionRepo port.TemplateVersionRepository,
	accessTokenRepo port.DocumentAccessTokenRepository,
	preSigningUC documentuc.PreSigningUseCase,
	storageAdapter port.StorageAdapter,
	tokenTTLHours int,
	storageEnabled bool,
) *SigningSessionService {
	if tokenTTLHours <= 0 {
		tokenTTLHours = defaultSigningSessionTokenTTLHours
	}

	return &SigningSessionService{
		documentRepo:      documentRepo,
		recipientRepo:     recipientRepo,
		versionRepo:       versionRepo,
		accessTokenRepo:   accessTokenRepo,
		preSigningUC:      preSigningUC,
		storageAdapter:    storageAdapter,
		storageEnabled:    storageEnabled,
		tokenTTLHours:     tokenTTLHours,
		rateLimitWindow:   defaultSigningSessionRateWindow,
		rateLimitMax:      defaultSigningSessionRateMax,
		rateLimitByTarget: make(map[string]signingSessionRateEntry),
	}
}

// CreateOrGetSession returns a reusable tokenized signing session URL and
// summarized flow state for an authenticated principal.
//
//nolint:funlen,gocognit,gocyclo,nestif
func (s *SigningSessionService) CreateOrGetSession(
	ctx context.Context,
	documentID string,
	principal *documentuc.SigningSessionPrincipal,
) (*documentuc.SigningSessionResponse, error) {
	documentID = strings.TrimSpace(documentID)
	if documentID == "" || principal == nil {
		return nil, entity.ErrUnauthorized
	}

	email := s.resolvePrincipalEmail(principal)
	if email == "" {
		return nil, entity.ErrForbidden
	}

	doc, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		if errors.Is(err, entity.ErrDocumentNotFound) || errors.Is(err, entity.ErrRecordNotFound) {
			return nil, entity.ErrForbidden
		}
		return nil, err
	}

	if doc.Status == entity.DocumentStatusCancelled || doc.Status == entity.DocumentStatusInvalidated || doc.IsExpired() {
		return nil, entity.ErrInvalidDocumentState
	}

	recipient, err := s.recipientRepo.FindByDocumentAndEmail(ctx, documentID, email)
	if err != nil {
		if errors.Is(err, entity.ErrDocumentRecipientNotFound) || errors.Is(err, entity.ErrRecordNotFound) {
			return nil, entity.ErrForbidden
		}
		return nil, err
	}

	if !s.allowRequest(documentID, recipient.ID) {
		return nil, entity.ErrTooManyRequests
	}

	tokenType := s.resolveTokenType(ctx, doc.TemplateVersionID)
	tokenStr, reused, err := s.getOrCreateToken(ctx, doc.ID, recipient.ID, tokenType)
	if err != nil {
		return nil, err
	}

	publicPage, err := s.preSigningUC.GetPublicSigningPage(ctx, tokenStr)
	if err != nil {
		return nil, normalizeSigningSessionError(err)
	}
	if tokenType == entity.TokenTypeSigning && publicPage.Step == documentuc.StepPreview {
		publicPage, err = s.preSigningUC.ProceedToSigning(ctx, tokenStr)
		if err != nil {
			return nil, normalizeSigningSessionError(err)
		}
	}

	slog.InfoContext(ctx, "signing session resolved",
		slog.String("document_id", doc.ID),
		slog.String("recipient_id", recipient.ID),
		slog.String("step", publicPage.Step),
		slog.Bool("token_reused", reused),
	)

	return &documentuc.SigningSessionResponse{
		SessionURL:  appendSigningURLLanguage(fmt.Sprintf("/public/sign/%s", tokenStr), principal.Language),
		Step:        publicPage.Step,
		CanSign:     publicPage.CanSign,
		CanDownload: publicPage.CanDownload,
		DownloadURL: publicPage.DownloadURL,
	}, nil
}

func (s *SigningSessionService) resolvePrincipalEmail(principal *documentuc.SigningSessionPrincipal) string {
	if principal == nil {
		return ""
	}

	email := strings.TrimSpace(principal.Email)
	if email != "" {
		return email
	}

	subject := strings.TrimSpace(principal.Subject)
	if strings.Contains(subject, "@") {
		return subject
	}

	return ""
}

func (s *SigningSessionService) resolveTokenType(ctx context.Context, templateVersionID string) string {
	version, err := s.versionRepo.FindByID(ctx, templateVersionID)
	if err != nil || version == nil || version.ContentStructure == nil {
		return entity.TokenTypeSigning
	}

	doc, err := parsePortableDocument(version.ContentStructure)
	if err != nil {
		return entity.TokenTypeSigning
	}

	if doc.HasNodeOfType("interactiveField") {
		return entity.TokenTypePreSigning
	}

	return entity.TokenTypeSigning
}

func (s *SigningSessionService) getOrCreateToken(
	ctx context.Context,
	documentID,
	recipientID,
	tokenType string,
) (string, bool, error) {
	activeToken, err := s.accessTokenRepo.FindActiveByDocumentAndRecipientAndType(ctx, documentID, recipientID, tokenType)
	if err == nil && activeToken != nil {
		return activeToken.Token, true, nil
	}
	if err != nil && !errors.Is(err, entity.ErrRecordNotFound) {
		return "", false, err
	}

	tokenStr, err := generateAccessToken()
	if err != nil {
		return "", false, err
	}

	now := time.Now().UTC()
	accessToken := &entity.DocumentAccessToken{
		DocumentID:  documentID,
		RecipientID: recipientID,
		Token:       tokenStr,
		TokenType:   tokenType,
		ExpiresAt:   now.Add(time.Duration(s.tokenTTLHours) * time.Hour),
		CreatedAt:   now,
	}

	if err := s.accessTokenRepo.Create(ctx, accessToken); err != nil {
		return "", false, err
	}

	return tokenStr, false, nil
}

func (s *SigningSessionService) allowRequest(documentID, recipientID string) bool {
	key := documentID + ":" + recipientID
	now := time.Now().UTC()

	s.rateLimitMu.Lock()
	defer s.rateLimitMu.Unlock()

	entry := s.rateLimitByTarget[key]
	if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= s.rateLimitWindow {
		s.rateLimitByTarget[key] = signingSessionRateEntry{
			windowStart: now,
			count:       1,
		}
		return true
	}

	if entry.count >= s.rateLimitMax {
		return false
	}

	entry.count++
	s.rateLimitByTarget[key] = entry
	return true
}

// Verify SigningSessionService implements SigningSessionUseCase.
var _ documentuc.SigningSessionUseCase = (*SigningSessionService)(nil)

func normalizeSigningSessionError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	case errors.Is(err, entity.ErrUnauthorized),
		errors.Is(err, entity.ErrForbidden),
		errors.Is(err, entity.ErrTooManyRequests),
		errors.Is(err, entity.ErrInvalidDocumentState):
		return err
	}

	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	if strings.Contains(msg, "valid state for signing") ||
		strings.Contains(msg, "not pending signing") {
		return entity.ErrInvalidDocumentState
	}

	return err
}
