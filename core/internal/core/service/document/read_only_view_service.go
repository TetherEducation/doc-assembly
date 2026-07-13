package document

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
)

// ReadOnlyViewService implements expiring public read-only document views.
type ReadOnlyViewService struct {
	documentRepo      port.DocumentRepository
	workspaceRepo     port.WorkspaceRepository
	accessTokenRepo   port.DocumentAccessTokenRepository
	recipientRepo     port.DocumentRecipientRepository
	versionRepo       port.TemplateVersionRepository
	signerRoleRepo    port.TemplateVersionSignerRoleRepository
	fieldResponseRepo port.DocumentFieldResponseRepository
	pdfRenderer       port.PDFRenderer
	storageAdapter    port.StorageAdapter
	signingProvider   port.SigningProvider
	attemptRepo       port.SigningAttemptRepository
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

// SetWorkspaceRepository enables workspace-code validation for external read-only link creation.
func (s *ReadOnlyViewService) SetWorkspaceRepository(workspaceRepo port.WorkspaceRepository) *ReadOnlyViewService {
	s.workspaceRepo = workspaceRepo
	return s
}

// SetCompletedPDFProvider wires the signing provider + attempt repository so a
// completed document whose signed PDF was not persisted to storage (the provider
// recorded a URL instead of a storage key) can still be served by downloading it
// from the provider, mirroring the public download endpoint.
func (s *ReadOnlyViewService) SetCompletedPDFProvider(signingProvider port.SigningProvider, attemptRepo port.SigningAttemptRepository) *ReadOnlyViewService {
	s.signingProvider = signingProvider
	s.attemptRepo = attemptRepo
	return s
}

// CreateReadOnlyViewLink creates a fresh expiring token for a public read-only
// view using the internal workspace ID from authenticated panel context.
func (s *ReadOnlyViewService) CreateReadOnlyViewLink(ctx context.Context, workspaceID, documentID string) (*documentuc.CreateReadOnlyViewLinkResult, error) {
	return s.createReadOnlyViewLink(ctx, documentID, func(doc *entity.Document) (bool, error) {
		return doc.WorkspaceID == strings.TrimSpace(workspaceID), nil
	})
}

// CreateReadOnlyViewLinkByWorkspaceCode creates a fresh expiring token for a
// public read-only view using the workspace business code from external callers.
func (s *ReadOnlyViewService) CreateReadOnlyViewLinkByWorkspaceCode(ctx context.Context, workspaceCode, documentID string) (*documentuc.CreateReadOnlyViewLinkResult, error) {
	return s.createReadOnlyViewLink(ctx, documentID, func(doc *entity.Document) (bool, error) {
		return s.matchesDocumentWorkspaceCode(ctx, doc, workspaceCode)
	})
}

func (s *ReadOnlyViewService) createReadOnlyViewLink(
	ctx context.Context,
	documentID string,
	matchesWorkspace func(*entity.Document) (bool, error),
) (*documentuc.CreateReadOnlyViewLinkResult, error) {
	doc, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("find document: %w", err)
	}
	if doc == nil {
		return nil, entity.ErrDocumentNotFound
	}
	matches, err := matchesWorkspace(doc)
	if err != nil {
		return nil, err
	}
	if !matches {
		return nil, entity.ErrForbidden
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

func (s *ReadOnlyViewService) matchesDocumentWorkspaceCode(
	ctx context.Context,
	doc *entity.Document,
	workspaceCode string,
) (bool, error) {
	workspaceCode = strings.TrimSpace(workspaceCode)
	if workspaceCode == "" || s.workspaceRepo == nil {
		return false, nil
	}

	workspace, err := s.workspaceRepo.FindByID(ctx, doc.WorkspaceID)
	if err != nil {
		if errors.Is(err, entity.ErrWorkspaceNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("find document workspace: %w", err)
	}
	if workspace == nil {
		return false, nil
	}
	if strings.EqualFold(workspace.Code, workspaceCode) {
		return true, nil
	}
	if workspace.SandboxOfID == nil {
		return false, nil
	}

	parent, err := s.workspaceRepo.FindByID(ctx, *workspace.SandboxOfID)
	if err != nil {
		if errors.Is(err, entity.ErrWorkspaceNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("find parent workspace: %w", err)
	}
	return parent != nil && strings.EqualFold(parent.Code, workspaceCode), nil
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
		pdfURL := s.buildReadOnlyViewURL(token) + "/pdf"
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
	if err := s.validateReadOnlyViewContentDependencies(); err != nil {
		return nil, err
	}

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

	resolvedContent, err := s.resolveReadOnlyViewContent(ctx, doc, portableDoc)
	if err != nil {
		return nil, err
	}

	content, err := json.Marshal(resolvedContent)
	if err != nil {
		return nil, fmt.Errorf("marshal read-only view content: %w", err)
	}
	if len(content) == 0 {
		return nil, errors.New("read-only view content is empty")
	}

	return content, nil
}

func (s *ReadOnlyViewService) resolveReadOnlyViewContent(
	ctx context.Context,
	doc *entity.Document,
	portableDoc *portabledoc.Document,
) (*portabledoc.ProseMirrorDoc, error) {
	recipients, err := s.recipientRepo.FindByDocumentID(ctx, doc.ID)
	if err != nil {
		return nil, fmt.Errorf("load recipients: %w", err)
	}

	signerRoles, err := s.signerRoleRepo.FindByVersionID(ctx, doc.TemplateVersionID)
	if err != nil {
		return nil, fmt.Errorf("load signer roles: %w", err)
	}

	var injectables map[string]any
	if doc.InjectedValuesSnapshot != nil {
		if err := json.Unmarshal(doc.InjectedValuesSnapshot, &injectables); err != nil {
			return nil, fmt.Errorf("parse injected values snapshot: %w", err)
		}
	}

	return &portabledoc.ProseMirrorDoc{
		Type:    portableDoc.Content.Type,
		Content: resolvePreviewNodes(portableDoc.Content.Content, injectables, buildSignerRoleValues(recipients, signerRoles, portableDoc.SignerRoles)),
	}, nil
}

func (s *ReadOnlyViewService) getCompletedReadOnlyViewPDF(ctx context.Context, doc *entity.Document) ([]byte, string, error) {
	// A persisted (GCS) copy is preferred when CompletedPDFURL holds a storage key.
	storageKey := completedPDFStorageKey(doc.CompletedPDFURL)

	// The signing provider records CompletedPDFURL as a URL (not a storage key) and
	// no step persists the sealed PDF to storage, so completedPDFStorageKey is empty
	// for provider-completed documents. Fall back to downloading the signed PDF from
	// the provider — the same path the public /download endpoint uses — instead of
	// failing with a 500. (Persisting the sealed PDF to storage on completion would
	// let this serve from GCS and avoid a provider round-trip per view.)
	if storageKey == "" {
		attempt, err := s.completedSigningAttempt(ctx, doc)
		if err != nil {
			return nil, "", err
		}
		if attempt != nil {
			storageKey = completedPDFStorageKey(stringValueFromJSON(attempt.ProviderUploadPayload, "completedPdfUrl"))
			if storageKey == "" {
				result, err := s.downloadCompletedPDFFromProvider(ctx, attempt)
				if err != nil {
					return nil, "", err
				}
				return result.PDF, providerCompletedPDFFilename(result.Filename, doc), nil
			}
		}
	}

	if storageKey == "" {
		return nil, "", errors.New("signed PDF not available for this document")
	}

	if !s.storageEnabled || s.storageAdapter == nil {
		return nil, "", errors.New("signed PDF storage is disabled")
	}

	pdfData, err := s.storageAdapter.Download(ctx, &port.StorageRequest{Key: storageKey})
	if err != nil {
		return nil, "", fmt.Errorf("download signed PDF: %w", err)
	}

	return pdfData, signedDocumentFilename(doc), nil
}

// completedSigningAttempt loads the document's active signing attempt when the
// provider-download fallback is wired; returns (nil, nil) when it is not, so the
// caller degrades to the existing "not available" behavior instead of panicking.
func (s *ReadOnlyViewService) completedSigningAttempt(ctx context.Context, doc *entity.Document) (*entity.SigningAttempt, error) {
	if s.attemptRepo == nil || doc.ActiveAttemptID == nil || strings.TrimSpace(*doc.ActiveAttemptID) == "" {
		return nil, nil
	}
	attempt, err := s.attemptRepo.FindByID(ctx, *doc.ActiveAttemptID)
	if err != nil {
		return nil, fmt.Errorf("finding completed signing attempt: %w", err)
	}
	return attempt, nil
}

// downloadCompletedPDFFromProvider fetches the sealed PDF bytes from the signing
// provider, mirroring PreSigningService.downloadCompletedPDFFromProvider.
func (s *ReadOnlyViewService) downloadCompletedPDFFromProvider(ctx context.Context, attempt *entity.SigningAttempt) (*port.DownloadCompletedPDFResult, error) {
	if s.signingProvider == nil || !s.signingProvider.ProviderCapabilities().CanDownloadCompletedPDF {
		return nil, errors.New("signed PDF not available for this document")
	}
	if attempt == nil || attempt.ProviderDocumentID == nil || strings.TrimSpace(*attempt.ProviderDocumentID) == "" {
		return nil, errors.New("signed PDF not available for this document")
	}
	result, err := s.signingProvider.DownloadCompletedPDF(ctx, &port.DownloadCompletedPDFRequest{
		ProviderDocumentID: *attempt.ProviderDocumentID,
		Environment:        entity.EnvironmentProd,
	})
	if err != nil {
		return nil, fmt.Errorf("downloading completed PDF from provider: %w", err)
	}
	if len(result.PDF) == 0 {
		return nil, errors.New("signed PDF not available for this document")
	}
	return result, nil
}

func (s *ReadOnlyViewService) renderReadOnlyViewPreviewPDF(ctx context.Context, doc *entity.Document) ([]byte, string, error) {
	if err := s.validateReadOnlyViewPreviewPDFDependencies(); err != nil {
		return nil, "", err
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
		if err := json.Unmarshal(doc.InjectedValuesSnapshot, &injectables); err != nil {
			return nil, "", fmt.Errorf("parse injected values snapshot: %w", err)
		}
	}

	fieldResponses, err := loadReadOnlyViewFieldResponseMap(ctx, s.fieldResponseRepo, doc.ID)
	if err != nil {
		return nil, "", fmt.Errorf("load field responses: %w", err)
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
	if renderResult == nil || len(renderResult.PDF) == 0 {
		return nil, "", errors.New("PDF is not available")
	}

	return renderResult.PDF, readOnlyViewPDFFilename(doc, renderResult.Filename), nil
}

func readOnlyViewPDFFilename(doc *entity.Document, rendererFilename string) string {
	if strings.TrimSpace(rendererFilename) != "" {
		return rendererFilename
	}
	return signedDocumentFilename(doc)
}

func (s *ReadOnlyViewService) validateReadOnlyViewPreviewPDFDependencies() error {
	if s.pdfRenderer == nil || s.versionRepo == nil || s.recipientRepo == nil || s.signerRoleRepo == nil || s.fieldResponseRepo == nil {
		return errors.New("PDF is not available: preview PDF renderer is not configured")
	}
	return nil
}

func (s *ReadOnlyViewService) validateReadOnlyViewContentDependencies() error {
	if s.versionRepo == nil || s.recipientRepo == nil || s.signerRoleRepo == nil {
		return errors.New("read-only view content is not available: content dependencies are not configured")
	}
	return nil
}

func loadReadOnlyViewFieldResponseMap(ctx context.Context, repo port.DocumentFieldResponseRepository, documentID string) (map[string]json.RawMessage, error) {
	responses, err := repo.FindByDocumentID(ctx, documentID)
	if err != nil {
		return nil, err
	}
	if len(responses) == 0 {
		return nil, nil
	}

	m := make(map[string]json.RawMessage, len(responses))
	for _, resp := range responses {
		m[resp.FieldID] = resp.Response
	}
	return m, nil
}
