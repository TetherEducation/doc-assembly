package document

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
)

var errReadOnlyViewNotImplemented = errors.New("read-only view retrieval is not implemented")

// ReadOnlyViewService implements expiring public read-only document views.
type ReadOnlyViewService struct {
	documentRepo    port.DocumentRepository
	accessTokenRepo port.DocumentAccessTokenRepository
	recipientRepo   port.DocumentRecipientRepository
	tokenTTLHours   int
	publicURL       string
}

var _ documentuc.ReadOnlyViewUseCase = (*ReadOnlyViewService)(nil)

// NewReadOnlyViewService creates a new ReadOnlyViewService.
func NewReadOnlyViewService(
	documentRepo port.DocumentRepository,
	accessTokenRepo port.DocumentAccessTokenRepository,
	recipientRepo port.DocumentRecipientRepository,
	tokenTTLHours int,
	publicURL string,
) *ReadOnlyViewService {
	return &ReadOnlyViewService{
		documentRepo:    documentRepo,
		accessTokenRepo: accessTokenRepo,
		recipientRepo:   recipientRepo,
		tokenTTLHours:   tokenTTLHours,
		publicURL:       publicURL,
	}
}

// CreateReadOnlyViewLink creates a fresh expiring token for a public read-only view.
func (s *ReadOnlyViewService) CreateReadOnlyViewLink(ctx context.Context, documentID string) (*documentuc.CreateReadOnlyViewLinkResult, error) {
	doc, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("find document: %w", err)
	}
	if doc == nil {
		return nil, entity.ErrDocumentNotFound
	}
	if doc.Status == entity.DocumentStatusInvalidated || doc.Status == entity.DocumentStatusCancelled || doc.IsExpired() {
		return nil, entity.ErrInvalidDocumentState
	}

	recipients, err := s.recipientRepo.FindByDocumentID(ctx, doc.ID)
	if err != nil {
		return nil, fmt.Errorf("find read-only view anchor recipient: %w", err)
	}
	if len(recipients) == 0 {
		return nil, fmt.Errorf("find read-only view anchor recipient: no recipients for document %s", doc.ID)
	}

	tokenStr, err := generateAccessToken()
	if err != nil {
		return nil, fmt.Errorf("generate read-only view token: %w", err)
	}

	now := time.Now().UTC()
	accessToken := &entity.DocumentAccessToken{
		DocumentID: doc.ID,
		// VIEW_ONLY tokens are technically anchored to an existing recipient only
		// to satisfy the current persistence schema; read-only semantics must not
		// treat this recipient as granting signing permission.
		RecipientID: recipients[0].ID,
		Token:       tokenStr,
		TokenType:   entity.TokenTypeViewOnly,
		ExpiresAt:   now.Add(time.Duration(s.tokenTTLHours) * time.Hour),
		CreatedAt:   now,
	}
	if err := s.accessTokenRepo.Create(ctx, accessToken); err != nil {
		return nil, fmt.Errorf("create read-only view token: %w", err)
	}

	return &documentuc.CreateReadOnlyViewLinkResult{
		URL:       s.buildReadOnlyViewURL(tokenStr),
		Token:     tokenStr,
		ExpiresAt: accessToken.ExpiresAt,
	}, nil
}

// GetReadOnlyView returns read-only metadata/content for a public token.
func (s *ReadOnlyViewService) GetReadOnlyView(context.Context, string) (*documentuc.ReadOnlyViewResponse, error) {
	return nil, errReadOnlyViewNotImplemented
}

// GetReadOnlyViewPDF returns the read-only PDF bytes for a public token.
func (s *ReadOnlyViewService) GetReadOnlyViewPDF(context.Context, string) ([]byte, string, error) {
	return nil, "", errReadOnlyViewNotImplemented
}

func (s *ReadOnlyViewService) buildReadOnlyViewURL(token string) string {
	path := "/public/view/" + token
	if s.publicURL == "" {
		return path
	}
	return strings.TrimRight(s.publicURL, "/") + path
}
