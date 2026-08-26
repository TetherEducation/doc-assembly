package document

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
)

// ReadOnlyViewService implements expiring public read-only document views.
type ReadOnlyViewService struct {
	documentRepo      port.DocumentRepository
	workspaceRepo     port.WorkspaceRepository
	documentTypeRepo  port.DocumentTypeRepository
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

// SetDocumentTypeRepository enables resolving a document's business type code in the
// signing-state report. Optional: without it the code is simply omitted.
func (s *ReadOnlyViewService) SetDocumentTypeRepository(documentTypeRepo port.DocumentTypeRepository) *ReadOnlyViewService {
	s.documentTypeRepo = documentTypeRepo
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

// GetSigningStateByWorkspaceCode reports the real signing state of each requested
// document, authorized by workspace business code.
//
// Unlike the read-only link and print-PDF flows, this deliberately does NOT reject
// invalidated, cancelled or expired documents: reporting that a document is in a
// dead state is the useful answer, not an error. Callers use it to tell "nobody
// ever signed this" apart from "signed", and to spot documents that need
// regenerating rather than a reminder.
func (s *ReadOnlyViewService) GetSigningStateByWorkspaceCode(
	ctx context.Context,
	workspaceCode string,
	documentIDs []string,
) (*documentuc.SigningStateResult, error) {
	result := &documentuc.SigningStateResult{
		Documents:   make([]documentuc.SigningStateDocument, 0, len(documentIDs)),
		Unavailable: make([]string, 0),
	}

	seen := make(map[string]struct{}, len(documentIDs))
	// A batch spans only a handful of document types, so resolve each code once
	// rather than per document.
	typeCodes := make(map[string]string)
	for _, rawID := range documentIDs {
		documentID := strings.TrimSpace(rawID)
		if documentID == "" {
			continue
		}
		// Callers batch by admission and can legitimately repeat an ID; answer once.
		if _, duplicate := seen[documentID]; duplicate {
			continue
		}
		seen[documentID] = struct{}{}

		state, err := s.resolveSigningStateDocument(ctx, workspaceCode, documentID, typeCodes)
		if err != nil {
			return nil, err
		}
		if state == nil {
			result.Unavailable = append(result.Unavailable, documentID)
			continue
		}
		result.Documents = append(result.Documents, *state)
	}

	return result, nil
}

// resolveSigningStateDocument returns nil when the document does not exist or is
// not visible to workspaceCode. Both cases collapse to nil on purpose so the
// caller cannot report them differently — see SigningStateResult.Unavailable.
//
// A missing document is a per-item outcome rather than a batch failure: one stale
// reference in a caller's set must not blank the whole answer.
func (s *ReadOnlyViewService) resolveSigningStateDocument(
	ctx context.Context,
	workspaceCode string,
	documentID string,
	typeCodes map[string]string,
) (*documentuc.SigningStateDocument, error) {
	doc, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		if errors.Is(err, entity.ErrDocumentNotFound) || errors.Is(err, entity.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find document: %w", err)
	}
	if doc == nil {
		return nil, nil
	}

	matches, err := s.matchesDocumentWorkspaceCode(ctx, doc, workspaceCode)
	if err != nil {
		return nil, err
	}
	if !matches {
		return nil, nil
	}

	recipients, err := s.signingStateRecipients(ctx, doc.ID)
	if err != nil {
		return nil, err
	}

	return &documentuc.SigningStateDocument{
		DocumentID:          doc.ID,
		ExternalReferenceID: doc.ClientExternalReferenceID,
		DocumentTypeCode:    s.signingStateDocumentTypeCode(ctx, doc.DocumentTypeID, typeCodes),
		Status:              doc.Status,
		Signed:              doc.Status == entity.DocumentStatusCompleted,
		Expired:             doc.IsExpired(),
		ExpiresAt:           doc.ExpiresAt,
		Recipients:          recipients,
	}, nil
}

// signingStateDocumentTypeCode resolves the document's business type code, memoized
// per batch. Best-effort: the code is a convenience for callers joining a document
// back to their own per-type record, so a lookup failure returns "" rather than
// failing a report about signatures. An empty code is itself informative — a document
// with no type never publishes a completion event (see the completion handler).
func (s *ReadOnlyViewService) signingStateDocumentTypeCode(
	ctx context.Context,
	documentTypeID string,
	typeCodes map[string]string,
) string {
	documentTypeID = strings.TrimSpace(documentTypeID)
	if documentTypeID == "" || s.documentTypeRepo == nil {
		return ""
	}
	if code, cached := typeCodes[documentTypeID]; cached {
		return code
	}

	code := ""
	docType, err := s.documentTypeRepo.FindByID(ctx, documentTypeID)
	switch {
	case err != nil:
		slog.WarnContext(ctx, "signing state: could not resolve document type code",
			slog.String("document_type_id", documentTypeID),
			slog.Any("error", err),
		)
	case docType != nil:
		code = docType.Code
	}

	typeCodes[documentTypeID] = code
	return code
}

func (s *ReadOnlyViewService) signingStateRecipients(
	ctx context.Context,
	documentID string,
) ([]documentuc.SigningStateRecipient, error) {
	recipients, err := s.recipientRepo.FindByDocumentIDWithRoles(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("find signing state recipients: %w", err)
	}

	out := make([]documentuc.SigningStateRecipient, 0, len(recipients))
	for _, recipient := range recipients {
		if recipient == nil {
			continue
		}
		out = append(out, documentuc.SigningStateRecipient{
			Name:        recipient.Name,
			Email:       recipient.Email,
			RoleName:    recipient.RoleName,
			SignerOrder: recipient.SignerOrder,
			Status:      recipient.Status,
			Signed:      recipient.Status == entity.RecipientStatusSigned,
			SignedAt:    recipient.SignedAt,
		})
	}
	return out, nil
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
	if storageKey != "" {
		return s.downloadCompletedReadOnlyViewPDFFromStorage(ctx, storageKey, doc)
	}

	return s.getCompletedReadOnlyViewPDFFromProvider(ctx, doc)
}

func (s *ReadOnlyViewService) getCompletedReadOnlyViewPDFFromProvider(ctx context.Context, doc *entity.Document) ([]byte, string, error) {
	// The signing provider records CompletedPDFURL as a URL (not a storage key) and
	// no step persists the sealed PDF to storage, so completedPDFStorageKey is empty
	// for provider-completed documents. Fall back to downloading the signed PDF from
	// the provider — the same path the public /download endpoint uses — instead of
	// failing with a 500. (Persisting the sealed PDF to storage on completion would
	// let this serve from GCS and avoid a provider round-trip per view.)
	attempt, err := s.completedSigningAttempt(ctx, doc)
	if err != nil {
		return nil, "", err
	}
	if attempt == nil {
		return nil, "", errors.New("signed PDF not available for this document")
	}

	storageKey := completedPDFStorageKey(stringValueFromJSON(attempt.ProviderUploadPayload, "completedPdfUrl"))
	if storageKey != "" {
		return s.downloadCompletedReadOnlyViewPDFFromStorage(ctx, storageKey, doc)
	}

	result, err := s.downloadCompletedPDFFromProvider(ctx, attempt)
	if err != nil {
		return nil, "", err
	}
	return result.PDF, providerCompletedPDFFilename(result.Filename, doc), nil
}

func (s *ReadOnlyViewService) downloadCompletedReadOnlyViewPDFFromStorage(ctx context.Context, storageKey string, doc *entity.Document) ([]byte, string, error) {
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
	if result == nil || len(result.PDF) == 0 {
		return nil, errors.New("signed PDF not available for this document")
	}
	return result, nil
}

// GetPrintPDF renders the current unsigned PDF (filled or blank) for printing,
// authorized by the internal workspace ID from authenticated panel context.
func (s *ReadOnlyViewService) GetPrintPDF(ctx context.Context, workspaceID, documentID string, blank bool) ([]byte, string, error) {
	return s.getPrintPDF(ctx, documentID, blank, func(doc *entity.Document) (bool, error) {
		return doc.WorkspaceID == strings.TrimSpace(workspaceID), nil
	})
}

// GetPrintPDFByWorkspaceCode renders the current unsigned PDF (filled or blank)
// for printing, authorized by the workspace business code from external callers.
func (s *ReadOnlyViewService) GetPrintPDFByWorkspaceCode(ctx context.Context, workspaceCode, documentID string, blank bool) ([]byte, string, error) {
	return s.getPrintPDF(ctx, documentID, blank, func(doc *entity.Document) (bool, error) {
		return s.matchesDocumentWorkspaceCode(ctx, doc, workspaceCode)
	})
}

// getPrintPDF serves the in-person signing flow: campuses print the unsigned
// document (or a blank template) and collect signatures on paper. Unlike the
// token-based read-only view, document expiry does not block printing — paper
// signing is precisely the fallback once the digital flow has stalled.
func (s *ReadOnlyViewService) getPrintPDF(
	ctx context.Context,
	documentID string,
	blank bool,
	matchesWorkspace func(*entity.Document) (bool, error),
) ([]byte, string, error) {
	doc, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		return nil, "", fmt.Errorf("find document: %w", err)
	}
	if doc == nil {
		return nil, "", entity.ErrDocumentNotFound
	}
	matches, err := matchesWorkspace(doc)
	if err != nil {
		return nil, "", err
	}
	if !matches {
		return nil, "", entity.ErrForbidden
	}
	if doc.Status == entity.DocumentStatusInvalidated || doc.Status == entity.DocumentStatusCancelled {
		return nil, "", entity.ErrInvalidDocumentState
	}
	// A completed or declined document must not be reprinted with values: the
	// signed artifact (or the decline record) is canonical. Blank reprints of
	// the underlying template remain allowed.
	if !blank && (doc.IsCompleted() || doc.IsDeclined()) {
		return nil, "", entity.ErrInvalidDocumentState
	}

	if blank {
		return s.renderBlankPrintPDF(ctx, doc)
	}

	pdf, _, err := s.renderReadOnlyViewPreviewPDF(ctx, doc)
	if err != nil {
		return nil, "", err
	}
	return pdf, printPDFFilename(doc, false), nil
}

// renderBlankPrintPDF renders the document's template version with no injected
// values, signer values, or field responses: an empty form for hand completion.
func (s *ReadOnlyViewService) renderBlankPrintPDF(ctx context.Context, doc *entity.Document) ([]byte, string, error) {
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

	renderResult, err := s.pdfRenderer.RenderPreview(ctx, &port.RenderPreviewRequest{
		Document: portableDoc,
	})
	if err != nil {
		return nil, "", fmt.Errorf("render blank PDF: %w", err)
	}
	if renderResult == nil || len(renderResult.PDF) == 0 {
		return nil, "", errors.New("PDF is not available")
	}

	return renderResult.PDF, printPDFFilename(doc, true), nil
}

func printPDFFilename(doc *entity.Document, blank bool) string {
	suffix := "print"
	if blank {
		suffix = "blank"
	}
	if doc != nil && doc.Title != nil && strings.TrimSpace(*doc.Title) != "" {
		return fmt.Sprintf("%s-%s.pdf", *doc.Title, suffix)
	}
	if doc != nil {
		return fmt.Sprintf("document-%s-%s.pdf", doc.ID, suffix)
	}
	return fmt.Sprintf("document-%s.pdf", suffix)
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
