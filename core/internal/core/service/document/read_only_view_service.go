package document

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
)

// ReadOnlyViewService implements expiring public read-only document views.
type ReadOnlyViewService struct {
	documentRepo      port.DocumentRepository
	accessTokenRepo   port.DocumentAccessTokenRepository
	recipientRepo     port.DocumentRecipientRepository
	versionRepo       port.TemplateVersionRepository
	signerRoleRepo    port.TemplateVersionSignerRoleRepository
	fieldResponseRepo port.DocumentFieldResponseRepository
	pdfRenderer       port.PDFRenderer
	storageAdapter    port.StorageAdapter
	storageEnabled    bool
	tokenTTLHours     int
	publicURL         string
}

var _ documentuc.ReadOnlyViewUseCase = (*ReadOnlyViewService)(nil)

// NewReadOnlyViewService creates a new ReadOnlyViewService.
func NewReadOnlyViewService(
	documentRepo port.DocumentRepository,
	accessTokenRepo port.DocumentAccessTokenRepository,
	recipientRepo port.DocumentRecipientRepository,
	versionRepo port.TemplateVersionRepository,
	signerRoleRepo port.TemplateVersionSignerRoleRepository,
	fieldResponseRepo port.DocumentFieldResponseRepository,
	pdfRenderer port.PDFRenderer,
	storageAdapter port.StorageAdapter,
	storageEnabled bool,
	tokenTTLHours int,
	publicURL string,
) *ReadOnlyViewService {
	return &ReadOnlyViewService{
		documentRepo:      documentRepo,
		accessTokenRepo:   accessTokenRepo,
		recipientRepo:     recipientRepo,
		versionRepo:       versionRepo,
		signerRoleRepo:    signerRoleRepo,
		fieldResponseRepo: fieldResponseRepo,
		pdfRenderer:       pdfRenderer,
		storageAdapter:    storageAdapter,
		storageEnabled:    storageEnabled,
		tokenTTLHours:     tokenTTLHours,
		publicURL:         publicURL,
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
func (s *ReadOnlyViewService) GetReadOnlyView(ctx context.Context, token string) (*documentuc.ReadOnlyViewResponse, error) {
	accessToken, err := s.validateReadOnlyViewToken(ctx, token)
	if err != nil {
		return nil, err
	}

	doc, err := s.documentRepo.FindByID(ctx, accessToken.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("find document: %w", err)
	}
	if doc == nil {
		return nil, entity.ErrDocumentNotFound
	}

	response := &documentuc.ReadOnlyViewResponse{
		Mode:           readOnlyViewMode(doc.Status),
		DocumentID:     doc.ID,
		DocumentTitle:  documentTitle(doc),
		DocumentStatus: doc.Status,
		ExpiresAt:      accessToken.ExpiresAt,
	}

	switch response.Mode {
	case documentuc.ReadOnlyViewModeContent:
		content, err := s.readOnlyViewContent(ctx, doc)
		if err != nil {
			return nil, err
		}
		response.Content = content
	case documentuc.ReadOnlyViewModePDF:
		pdfURL := "/public/view/" + token + "/pdf"
		response.PDFURL = &pdfURL
	case documentuc.ReadOnlyViewModeUnavailable:
		reason := "document_unavailable"
		response.Reason = &reason
	}

	return response, nil
}

// GetReadOnlyViewPDF returns the read-only PDF bytes for a public token.
func (s *ReadOnlyViewService) GetReadOnlyViewPDF(ctx context.Context, token string) ([]byte, string, error) {
	accessToken, err := s.validateReadOnlyViewToken(ctx, token)
	if err != nil {
		return nil, "", err
	}

	doc, err := s.documentRepo.FindByID(ctx, accessToken.DocumentID)
	if err != nil {
		return nil, "", fmt.Errorf("find document: %w", err)
	}
	if doc == nil {
		return nil, "", entity.ErrDocumentNotFound
	}

	switch doc.Status {
	case entity.DocumentStatusCompleted:
		return s.getCompletedReadOnlyViewPDF(ctx, doc)
	case entity.DocumentStatusPreparingSignature,
		entity.DocumentStatusReadyToSign,
		entity.DocumentStatusSigning:
		return s.renderReadOnlyViewPreviewPDF(ctx, doc)
	default:
		return nil, "", errors.New("PDF is not available")
	}
}

func (s *ReadOnlyViewService) buildReadOnlyViewURL(token string) string {
	path := "/public/view/" + token
	if s.publicURL == "" {
		return path
	}
	return strings.TrimRight(s.publicURL, "/") + path
}

func (s *ReadOnlyViewService) validateReadOnlyViewToken(ctx context.Context, token string) (*entity.DocumentAccessToken, error) {
	if token == "" {
		return nil, entity.ErrMissingToken
	}

	accessToken, err := s.accessTokenRepo.FindByToken(ctx, token)
	if err != nil || accessToken == nil {
		return nil, entity.ErrInvalidToken
	}
	if !accessToken.IsViewOnly() {
		return nil, entity.ErrInvalidToken
	}
	if time.Now().UTC().After(accessToken.ExpiresAt) {
		return nil, entity.ErrTokenExpired
	}

	return accessToken, nil
}

func readOnlyViewMode(status entity.DocumentStatus) documentuc.ReadOnlyViewMode {
	switch status {
	case entity.DocumentStatusDraft, entity.DocumentStatusAwaitingInput:
		return documentuc.ReadOnlyViewModeContent
	case entity.DocumentStatusPreparingSignature,
		entity.DocumentStatusReadyToSign,
		entity.DocumentStatusSigning,
		entity.DocumentStatusCompleted:
		return documentuc.ReadOnlyViewModePDF
	default:
		return documentuc.ReadOnlyViewModeUnavailable
	}
}

func (s *ReadOnlyViewService) readOnlyViewContent(ctx context.Context, doc *entity.Document) (json.RawMessage, error) {
	version, err := s.versionRepo.FindByID(ctx, doc.TemplateVersionID)
	if err != nil {
		return nil, fmt.Errorf("find template version: %w", err)
	}
	if version == nil {
		return nil, fmt.Errorf("find template version: %w", entity.ErrVersionNotFound)
	}

	portableDoc, err := parsePortableDocument(version.ContentStructure)
	if err != nil {
		return nil, fmt.Errorf("parse portable document: %w", err)
	}
	if portableDoc == nil || portableDoc.Content == nil {
		return nil, errors.New("read-only view content is empty")
	}

	content, err := json.Marshal(portableDoc.Content)
	if err != nil {
		return nil, fmt.Errorf("marshal read-only view content: %w", err)
	}
	if len(content) == 0 {
		return nil, errors.New("read-only view content is empty")
	}

	return content, nil
}

func (s *ReadOnlyViewService) getCompletedReadOnlyViewPDF(ctx context.Context, doc *entity.Document) ([]byte, string, error) {
	if !s.storageEnabled || s.storageAdapter == nil {
		return nil, "", errors.New("signed PDF storage is disabled")
	}

	storageKey := completedPDFStorageKey(doc.CompletedPDFURL)
	if storageKey == "" {
		return nil, "", errors.New("signed PDF not available for this document")
	}

	pdfData, err := s.storageAdapter.Download(ctx, &port.StorageRequest{Key: storageKey})
	if err != nil {
		return nil, "", fmt.Errorf("download signed PDF: %w", err)
	}

	return pdfData, signedDocumentFilename(doc), nil
}

func (s *ReadOnlyViewService) renderReadOnlyViewPreviewPDF(ctx context.Context, doc *entity.Document) ([]byte, string, error) {
	if s.pdfRenderer == nil {
		return nil, "", errors.New("PDF is not available")
	}

	version, err := s.versionRepo.FindByID(ctx, doc.TemplateVersionID)
	if err != nil {
		return nil, "", fmt.Errorf("find template version: %w", err)
	}
	if version == nil {
		return nil, "", fmt.Errorf("find template version: %w", entity.ErrVersionNotFound)
	}

	portableDoc, err := parsePortableDocument(version.ContentStructure)
	if err != nil {
		return nil, "", fmt.Errorf("parse portable document: %w", err)
	}
	if portableDoc == nil {
		return nil, "", errors.New("document has no content")
	}

	recipients, err := s.recipientRepo.FindByDocumentID(ctx, doc.ID)
	if err != nil {
		return nil, "", fmt.Errorf("load recipients: %w", err)
	}

	signerRoles, err := s.signerRoleRepo.FindByVersionID(ctx, doc.TemplateVersionID)
	if err != nil {
		return nil, "", fmt.Errorf("load signer roles: %w", err)
	}

	var injectables map[string]any
	if doc.InjectedValuesSnapshot != nil {
		_ = json.Unmarshal(doc.InjectedValuesSnapshot, &injectables)
	}

	var fieldResponses map[string]json.RawMessage
	if s.fieldResponseRepo != nil {
		fieldResponses = loadFieldResponseMap(ctx, s.fieldResponseRepo, doc.ID)
	}

	renderResult, err := s.pdfRenderer.RenderPreview(ctx, &port.RenderPreviewRequest{
		Document:         portableDoc,
		Injectables:      injectables,
		SignerRoleValues: buildSignerRoleValues(recipients, signerRoles, portableDoc.SignerRoles),
		FieldResponses:   fieldResponses,
	})
	if err != nil {
		return nil, "", fmt.Errorf("render preview PDF: %w", err)
	}

	filename := signedDocumentFilename(doc)
	if strings.TrimSpace(renderResult.Filename) != "" {
		filename = renderResult.Filename
	}

	return renderResult.PDF, filename, nil
}
