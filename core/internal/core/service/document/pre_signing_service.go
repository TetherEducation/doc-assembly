package document

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
)

const publicProcessingSoftLimit = 10 * time.Minute

// PreSigningService implements the public signing use case.
type PreSigningService struct {
	accessTokenRepo   port.DocumentAccessTokenRepository
	fieldResponseRepo port.DocumentFieldResponseRepository
	documentRepo      port.DocumentRepository
	recipientRepo     port.DocumentRecipientRepository
	attemptRepo       port.SigningAttemptRepository
	signingUOW        port.SigningExecutionUnitOfWork
	versionRepo       port.TemplateVersionRepository
	signerRoleRepo    port.TemplateVersionSignerRoleRepository
	pdfRenderer       port.PDFRenderer
	signingProvider   port.SigningProvider
	storageAdapter    port.StorageAdapter
	storageEnabled    bool
	eventEmitter      *EventEmitter
	publicURL         string
}

// NewPreSigningService creates a new PreSigningService.
func NewPreSigningService(
	accessTokenRepo port.DocumentAccessTokenRepository,
	fieldResponseRepo port.DocumentFieldResponseRepository,
	documentRepo port.DocumentRepository,
	recipientRepo port.DocumentRecipientRepository,
	attemptRepo port.SigningAttemptRepository,
	signingUOW port.SigningExecutionUnitOfWork,
	versionRepo port.TemplateVersionRepository,
	signerRoleRepo port.TemplateVersionSignerRoleRepository,
	pdfRenderer port.PDFRenderer,
	signingProvider port.SigningProvider,
	storageAdapter port.StorageAdapter,
	storageEnabled bool,
	eventEmitter *EventEmitter,
	publicURL string,
) documentuc.PreSigningUseCase {
	return &PreSigningService{
		accessTokenRepo:   accessTokenRepo,
		fieldResponseRepo: fieldResponseRepo,
		documentRepo:      documentRepo,
		recipientRepo:     recipientRepo,
		attemptRepo:       attemptRepo,
		signingUOW:        signingUOW,
		versionRepo:       versionRepo,
		signerRoleRepo:    signerRoleRepo,
		pdfRenderer:       pdfRenderer,
		signingProvider:   signingProvider,
		storageAdapter:    storageAdapter,
		storageEnabled:    storageEnabled,
		eventEmitter:      eventEmitter,
		publicURL:         publicURL,
	}
}

// GetPublicSigningPage returns the current signing page state based on document status and token type.
// GetPublicSigningPage returns the current signing page state based on document status and active attempt.
//
//nolint:gocognit,gocyclo // Token/document/attempt resolution is intentionally explicit.
func (s *PreSigningService) GetPublicSigningPage(ctx context.Context, token string) (*documentuc.PublicSigningResponse, error) {
	accessToken, wasUsed, err := s.validateTokenAllowUsed(ctx, token)
	if err != nil {
		return nil, err
	}

	doc, err := s.documentRepo.FindByID(ctx, accessToken.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("finding document: %w", err)
	}
	recipient, err := s.recipientRepo.FindByID(ctx, accessToken.RecipientID)
	if err != nil {
		return nil, fmt.Errorf("finding recipient: %w", err)
	}
	title := documentTitle(doc)

	if accessToken.AttemptID != nil && *accessToken.AttemptID != "" {
		return s.buildAttemptSigningResponse(ctx, doc, recipient, accessToken, *accessToken.AttemptID, title)
	}
	if doc.ActiveAttemptID != nil && *doc.ActiveAttemptID != "" {
		return s.buildAttemptSigningResponse(ctx, doc, recipient, accessToken, *doc.ActiveAttemptID, title)
	}
	if doc.IsCompleted() {
		resp := &documentuc.PublicSigningResponse{Step: documentuc.StepCompleted, DocumentTitle: title, RecipientName: recipient.Name}
		s.applyAccessFlags(resp, doc, recipient, accessToken.Token)
		return resp, nil
	}
	if doc.IsDeclined() {
		resp := &documentuc.PublicSigningResponse{Step: documentuc.StepDeclined, DocumentTitle: title, RecipientName: recipient.Name}
		s.applyAccessFlags(resp, doc, recipient, accessToken.Token)
		return resp, nil
	}
	if wasUsed {
		return nil, fmt.Errorf("access token has already been used")
	}
	if doc.IsAwaitingInput() && accessToken.IsPreSigning() {
		if s.hasFieldResponses(ctx, doc.ID) {
			return s.buildPreviewPDFResponse(doc, recipient, title, accessToken.Token)
		}
		return s.buildPreviewFormResponse(ctx, doc, recipient, title, accessToken.Token)
	}
	if doc.IsAwaitingInput() && accessToken.IsSigning() {
		return s.buildPreviewPDFResponse(doc, recipient, title, accessToken.Token)
	}
	if doc.Status == entity.DocumentStatusPreparingSignature {
		return s.buildProcessingResponse(doc, recipient, accessToken.Token), nil
	}
	return nil, fmt.Errorf("document is not in a valid state for signing")
}

// SubmitPreSigningForm validates responses, saves them, renders PDF, sends to provider,
// and returns the signing page state with embedded URL.
func (s *PreSigningService) SubmitPreSigningForm(
	ctx context.Context,
	token string,
	responses []documentuc.FieldResponseInput,
) (*documentuc.PublicSigningResponse, error) {
	accessToken, err := s.validateToken(ctx, token)
	if err != nil {
		return nil, err
	}

	doc, recipient, version, portableDoc, err := s.loadSubmissionContext(ctx, accessToken)
	if err != nil {
		return nil, err
	}

	// Map DB role to portable doc role for field extraction.
	portableRoleID, err := s.resolvePortableRoleID(ctx, version, portableDoc, recipient.TemplateVersionRoleID)
	if err != nil {
		return nil, err
	}
	fieldDefs := extractInteractiveFieldsForRole(portableDoc, portableRoleID)

	// Validate responses against field definitions.
	if err := s.validateResponses(responses, fieldDefs); err != nil {
		return nil, err
	}

	// Save responses (no render/upload yet — deferred to ProceedToSigning).
	if err := s.saveFieldResponses(ctx, doc.ID, recipient.ID, responses, fieldDefs); err != nil {
		return nil, fmt.Errorf("saving field responses: %w", err)
	}

	slog.InfoContext(ctx, "pre-signing form submitted, responses saved",
		slog.String("document_id", doc.ID),
		slog.String("recipient_id", recipient.ID),
	)

	// Return PDF preview step so the signer can review before proceeding.
	title := documentTitle(doc)
	return s.buildPreviewPDFResponse(doc, recipient, title, accessToken.Token)
}

// ProceedToSigning renders the PDF, uploads to the signing provider, and returns the
// embedded signing URL. Accepts both SIGNING (Path A) and PRE_SIGNING (Path B) tokens.
//
//nolint:gocognit,nestif // Flow is sequential by token/document state to keep behavior explicit.
func (s *PreSigningService) ProceedToSigning(ctx context.Context, token string) (*documentuc.PublicSigningResponse, error) {
	accessToken, err := s.validateToken(ctx, token)
	if err != nil {
		return nil, err
	}
	doc, err := s.documentRepo.FindByID(ctx, accessToken.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("finding document: %w", err)
	}
	recipient, err := s.recipientRepo.FindByID(ctx, accessToken.RecipientID)
	if err != nil {
		return nil, fmt.Errorf("finding recipient: %w", err)
	}
	title := documentTitle(doc)

	if accessToken.AttemptID != nil && *accessToken.AttemptID != "" {
		return s.buildAttemptSigningResponse(ctx, doc, recipient, accessToken, *accessToken.AttemptID, title)
	}

	attemptID := ""
	if doc.ActiveAttemptID != nil && *doc.ActiveAttemptID != "" {
		attemptID = *doc.ActiveAttemptID
	} else {
		if !doc.IsAwaitingInput() && doc.Status != entity.DocumentStatusPreparingSignature {
			return nil, fmt.Errorf("document is not pending signing")
		}
		recipients, err := s.recipientRepo.FindByDocumentID(ctx, doc.ID)
		if err != nil {
			return nil, fmt.Errorf("loading recipients: %w", err)
		}
		signerOrders, err := s.signerOrderMap(ctx, doc.TemplateVersionID)
		if err != nil {
			return nil, err
		}
		attempt, err := s.signingUOW.CreateAttemptAndEnqueueRender(ctx, doc.ID, recipients, signerOrders)
		if err != nil {
			return nil, fmt.Errorf("creating signing attempt: %w", err)
		}
		attemptID = attempt.ID
	}
	if err := s.attemptRepo.BindTokenToAttempt(ctx, accessToken.ID, attemptID); err != nil {
		return nil, err
	}
	accessToken.AttemptID = &attemptID
	doc, _ = s.documentRepo.FindByID(ctx, doc.ID)
	return s.buildAttemptSigningResponse(ctx, doc, recipient, accessToken, attemptID, title)
}

// CompleteEmbeddedSigning marks the token as used after embedded signing is completed.
func (s *PreSigningService) CompleteEmbeddedSigning(ctx context.Context, token string) error {
	accessToken, err := s.validateToken(ctx, token)
	if err != nil {
		return err
	}

	if err := s.accessTokenRepo.MarkAsUsed(ctx, accessToken.ID); err != nil {
		return fmt.Errorf("marking token as used: %w", err)
	}

	slog.InfoContext(ctx, "embedded signing completed, token marked as used",
		slog.String("document_id", accessToken.DocumentID),
		slog.String("recipient_id", accessToken.RecipientID),
	)

	return nil
}

// RefreshEmbeddedURL refreshes an expired embedded signing URL.
func (s *PreSigningService) RefreshEmbeddedURL(ctx context.Context, token string) (*documentuc.PublicSigningResponse, error) {
	accessToken, _, err := s.validateTokenAllowUsed(ctx, token)
	if err != nil {
		return nil, err
	}
	doc, err := s.documentRepo.FindByID(ctx, accessToken.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("finding document: %w", err)
	}
	recipient, err := s.recipientRepo.FindByID(ctx, accessToken.RecipientID)
	if err != nil {
		return nil, fmt.Errorf("finding recipient: %w", err)
	}
	title := documentTitle(doc)
	attemptID := ""
	if accessToken.AttemptID != nil {
		attemptID = *accessToken.AttemptID
	} else if doc.ActiveAttemptID != nil {
		attemptID = *doc.ActiveAttemptID
	}
	if attemptID == "" {
		return s.buildProcessingResponse(doc, recipient, accessToken.Token), nil
	}
	return s.buildAttemptSigningResponse(ctx, doc, recipient, accessToken, attemptID, title)
}

// InvalidateTokens invalidates all active tokens for a document in AWAITING_INPUT status.
func (s *PreSigningService) InvalidateTokens(ctx context.Context, documentID string) error {
	doc, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		return fmt.Errorf("finding document: %w", err)
	}
	if !doc.IsAwaitingInput() {
		return fmt.Errorf("document is not in AWAITING_INPUT status")
	}

	if err := s.accessTokenRepo.InvalidateByDocumentID(ctx, documentID); err != nil {
		return fmt.Errorf("invalidating existing tokens: %w", err)
	}

	slog.InfoContext(ctx, "access tokens invalidated",
		slog.String("document_id", documentID),
	)

	return nil
}

// RenderPreviewPDF renders the document PDF on-demand for preview without storing it.
func (s *PreSigningService) RenderPreviewPDF(ctx context.Context, token string) ([]byte, error) {
	accessToken, wasUsed, err := s.validateTokenAllowUsed(ctx, token)
	if err != nil {
		return nil, err
	}

	doc, err := s.documentRepo.FindByID(ctx, accessToken.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("finding document: %w", err)
	}
	if wasUsed && !doc.IsCompleted() && !doc.IsDeclined() {
		return nil, fmt.Errorf("access token has already been used")
	}

	version, err := s.versionRepo.FindByID(ctx, doc.TemplateVersionID)
	if err != nil {
		return nil, fmt.Errorf("finding template version: %w", err)
	}

	portableDoc, err := parsePortableDocument(version.ContentStructure)
	if err != nil {
		return nil, fmt.Errorf("parsing document content: %w", err)
	}
	if portableDoc == nil {
		return nil, fmt.Errorf("document has no content")
	}

	recipients, err := s.recipientRepo.FindByDocumentID(ctx, doc.ID)
	if err != nil {
		return nil, fmt.Errorf("loading recipients: %w", err)
	}

	signerRoles, err := s.signerRoleRepo.FindByVersionID(ctx, doc.TemplateVersionID)
	if err != nil {
		return nil, fmt.Errorf("loading signer roles: %w", err)
	}

	signerRoleValues := buildSignerRoleValues(recipients, signerRoles, portableDoc.SignerRoles)

	var injectables map[string]any
	if doc.InjectedValuesSnapshot != nil {
		_ = json.Unmarshal(doc.InjectedValuesSnapshot, &injectables)
	}

	fieldResponses := loadFieldResponseMap(ctx, s.fieldResponseRepo, doc.ID)

	renderResult, err := s.pdfRenderer.RenderPreview(ctx, &port.RenderPreviewRequest{
		Document:         portableDoc,
		Injectables:      injectables,
		SignerRoleValues: signerRoleValues,
		FieldResponses:   fieldResponses,
	})
	if err != nil {
		return nil, fmt.Errorf("rendering preview PDF: %w", err)
	}

	return renderResult.PDF, nil
}

// DownloadCompletedPDF returns the signed PDF for completed documents when the
// token recipient is authorized.
//
//nolint:funlen,gocognit,gocyclo,nestif // Explicit guard flow preserves security/state checks.
func (s *PreSigningService) DownloadCompletedPDF(ctx context.Context, token string) ([]byte, string, error) {
	accessToken, _, err := s.validateTokenAllowUsed(ctx, token)
	if err != nil {
		return nil, "", err
	}

	doc, err := s.documentRepo.FindByID(ctx, accessToken.DocumentID)
	if err != nil {
		return nil, "", fmt.Errorf("finding document: %w", err)
	}

	if !doc.IsCompleted() {
		return nil, "", fmt.Errorf("document is not completed")
	}

	attemptID := ""
	if accessToken.AttemptID != nil {
		attemptID = *accessToken.AttemptID
	}
	if attemptID == "" && doc.ActiveAttemptID != nil {
		attemptID = *doc.ActiveAttemptID
	}
	if attemptID == "" {
		return nil, "", fmt.Errorf("completed PDF is not available")
	}

	recipients, err := s.attemptRepo.FindRecipientsByAttemptID(ctx, attemptID)
	if err != nil {
		return nil, "", fmt.Errorf("finding attempt recipients: %w", err)
	}
	authorized := false
	for _, r := range recipients {
		if r.DocumentRecipientID != nil && *r.DocumentRecipientID == accessToken.RecipientID && r.Status == entity.RecipientStatusSigned {
			authorized = true
			break
		}
	}
	if !authorized {
		return nil, "", fmt.Errorf("completed PDF is not available for this recipient")
	}

	if !s.storageEnabled {
		return nil, "", fmt.Errorf("completed PDF storage is disabled")
	}

	storageKey := completedPDFStorageKey(doc.CompletedPDFURL)
	var completedAttempt *entity.SigningAttempt
	if storageKey == "" {
		attempt, err := s.attemptRepo.FindByID(ctx, attemptID)
		if err != nil {
			return nil, "", fmt.Errorf("finding active completed attempt: %w", err)
		}
		completedAttempt = attempt
		storageKey = completedPDFStorageKey(stringValueFromJSON(attempt.ProviderUploadPayload, "completedPdfUrl"))
	}
	if storageKey == "" {
		if completedAttempt == nil {
			attempt, err := s.attemptRepo.FindByID(ctx, attemptID)
			if err != nil {
				return nil, "", fmt.Errorf("finding active completed attempt: %w", err)
			}
			completedAttempt = attempt
		}
		result, err := s.downloadCompletedPDFFromProvider(ctx, completedAttempt)
		if err != nil {
			return nil, "", err
		}
		return result.PDF, providerCompletedPDFFilename(result.Filename, doc), nil
	}

	pdfData, err := s.storageAdapter.Download(ctx, &port.StorageRequest{Key: storageKey})
	if err != nil {
		return nil, "", fmt.Errorf("downloading completed PDF: %w", err)
	}

	return pdfData, signedDocumentFilename(doc), nil
}

func (s *PreSigningService) downloadCompletedPDFFromProvider(ctx context.Context, attempt *entity.SigningAttempt) (*port.DownloadCompletedPDFResult, error) {
	if s.signingProvider == nil || !s.signingProvider.ProviderCapabilities().CanDownloadCompletedPDF {
		return nil, fmt.Errorf("completed PDF is not available")
	}
	if attempt.ProviderDocumentID == nil || strings.TrimSpace(*attempt.ProviderDocumentID) == "" {
		return nil, fmt.Errorf("completed PDF is not available")
	}
	result, err := s.signingProvider.DownloadCompletedPDF(ctx, &port.DownloadCompletedPDFRequest{
		ProviderDocumentID: *attempt.ProviderDocumentID,
		Environment:        entity.EnvironmentProd,
	})
	if err != nil {
		return nil, fmt.Errorf("downloading completed PDF from provider: %w", err)
	}
	if len(result.PDF) == 0 {
		return nil, fmt.Errorf("completed PDF is not available")
	}
	return result, nil
}

func providerCompletedPDFFilename(providerFilename string, doc *entity.Document) string {
	if strings.TrimSpace(providerFilename) != "" {
		return providerFilename
	}
	return signedDocumentFilename(doc)
}

func completedPDFStorageKey(ref *string) string {
	if ref == nil {
		return ""
	}
	value := strings.TrimSpace(*ref)
	if value == "" || strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return ""
	}
	return value
}

func stringValueFromJSON(raw json.RawMessage, key string) *string {
	if len(raw) == 0 {
		return nil
	}
	var values map[string]any
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil
	}
	value, ok := values[key].(string)
	if !ok || strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

// hasFieldResponses checks whether field responses have been saved for a document.
func (s *PreSigningService) hasFieldResponses(ctx context.Context, documentID string) bool {
	responses, err := s.fieldResponseRepo.FindByDocumentID(ctx, documentID)
	if err != nil {
		return false
	}
	return len(responses) > 0
}

// --- Response builders ---

// buildPreviewFormResponse builds a preview response with the pre-signing form (Path B).
func (s *PreSigningService) buildPreviewFormResponse(
	ctx context.Context,
	doc *entity.Document,
	recipient *entity.DocumentRecipient,
	title string,
	token string,
) (*documentuc.PublicSigningResponse, error) {
	version, err := s.versionRepo.FindByID(ctx, doc.TemplateVersionID)
	if err != nil {
		return nil, fmt.Errorf("finding template version: %w", err)
	}

	portableDoc, err := parsePortableDocument(version.ContentStructure)
	if err != nil {
		return nil, fmt.Errorf("parsing document content: %w", err)
	}
	if portableDoc == nil {
		return nil, fmt.Errorf("document has no content")
	}

	content, err := s.resolveContent(ctx, portableDoc, doc)
	if err != nil {
		return nil, fmt.Errorf("resolving content: %w", err)
	}

	portableRoleID, err := s.resolvePortableRoleID(ctx, version, portableDoc, recipient.TemplateVersionRoleID)
	if err != nil {
		return nil, err
	}
	fields := extractInteractiveFieldsForRole(portableDoc, portableRoleID)

	resp := &documentuc.PublicSigningResponse{
		Step:          documentuc.StepPreview,
		DocumentTitle: title,
		RecipientName: recipient.Name,
		Form: &documentuc.PreSigningFormDTO{
			DocumentTitle:  title,
			DocumentStatus: string(doc.Status),
			RecipientName:  recipient.Name,
			RecipientEmail: recipient.Email,
			RoleID:         portableRoleID,
			Content:        content,
			Fields:         fields,
		},
	}
	s.applyAccessFlags(resp, doc, recipient, token)
	return resp, nil
}

// buildProcessingResponse returns a "processing" step when the document is being
// rendered/uploaded and signer info is not yet available.
func (s *PreSigningService) buildProcessingResponse(
	doc *entity.Document,
	recipient *entity.DocumentRecipient,
	token string,
) *documentuc.PublicSigningResponse {
	title := documentTitle(doc)
	resp := &documentuc.PublicSigningResponse{
		Step:              documentuc.StepProcessing,
		DocumentTitle:     title,
		RecipientName:     recipient.Name,
		ProcessingReason:  "preparing_document",
		RetryAfterSeconds: 5,
	}
	s.applyAccessFlags(resp, doc, recipient, token)
	return resp
}

func applyLongRunningProcessingMetadata(resp *documentuc.PublicSigningResponse, attempt *entity.SigningAttempt) {
	if !isLongRunningProcessingAttempt(attempt) {
		return
	}
	resp.ProcessingReason = "recovering_provider_submission"
	resp.SupportCode = attempt.ID
}

func attemptProcessingAge(attempt *entity.SigningAttempt) time.Duration {
	if attempt == nil {
		return 0
	}
	if attempt.UpdatedAt != nil {
		return time.Since(*attempt.UpdatedAt)
	}
	return time.Since(attempt.CreatedAt)
}

func isLongRunningProcessingAttempt(attempt *entity.SigningAttempt) bool {
	return attemptProcessingAge(attempt) > publicProcessingSoftLimit
}

// buildPreviewPDFResponse builds a preview response with the PDF URL for on-demand rendering.
func (s *PreSigningService) buildPreviewPDFResponse(
	doc *entity.Document,
	recipient *entity.DocumentRecipient,
	title string,
	token string,
) (*documentuc.PublicSigningResponse, error) {
	resp := &documentuc.PublicSigningResponse{
		Step:          documentuc.StepPreview,
		DocumentTitle: title,
		RecipientName: recipient.Name,
		PdfURL:        fmt.Sprintf("/public/sign/%s/pdf", token),
	}
	s.applyAccessFlags(resp, doc, recipient, token)
	return resp, nil
}

//nolint:funlen,gocyclo
func (s *PreSigningService) buildAttemptSigningResponse(
	ctx context.Context,
	doc *entity.Document,
	recipient *entity.DocumentRecipient,
	accessToken *entity.DocumentAccessToken,
	attemptID string,
	title string,
) (*documentuc.PublicSigningResponse, error) {
	attempt, err := s.attemptRepo.FindByID(ctx, attemptID)
	if err != nil {
		return nil, err
	}
	if attempt.DocumentID != doc.ID {
		return nil, entity.ErrInvalidDocumentState
	}
	if doc.ActiveAttemptID == nil || *doc.ActiveAttemptID != attempt.ID {
		return s.buildDocumentUpdatedResponse(doc, recipient, accessToken.Token), nil
	}

	switch attempt.Status {
	case entity.SigningAttemptStatusCreated, entity.SigningAttemptStatusRendering, entity.SigningAttemptStatusPDFReady,
		entity.SigningAttemptStatusReadyToSubmit, entity.SigningAttemptStatusSubmittingProvider,
		entity.SigningAttemptStatusProviderRetryWaiting, entity.SigningAttemptStatusSubmissionUnknown, entity.SigningAttemptStatusReconcilingProvider:
		resp := s.buildProcessingResponse(doc, recipient, accessToken.Token)
		applyLongRunningProcessingMetadata(resp, attempt)
		return resp, nil
	case entity.SigningAttemptStatusSuperseded, entity.SigningAttemptStatusInvalidated, entity.SigningAttemptStatusCancelled:
		return s.buildDocumentUpdatedResponse(doc, recipient, accessToken.Token), nil
	case entity.SigningAttemptStatusFailedPermanent, entity.SigningAttemptStatusRequiresReview:
		return s.buildUnavailableResponse(doc, recipient, accessToken.Token), nil
	case entity.SigningAttemptStatusCompleted:
		resp := &documentuc.PublicSigningResponse{Step: documentuc.StepCompleted, DocumentTitle: title, RecipientName: recipient.Name}
		s.applyAccessFlags(resp, doc, recipient, accessToken.Token)
		return resp, nil
	case entity.SigningAttemptStatusDeclined:
		resp := &documentuc.PublicSigningResponse{Step: documentuc.StepDeclined, DocumentTitle: title, RecipientName: recipient.Name}
		s.applyAccessFlags(resp, doc, recipient, accessToken.Token)
		return resp, nil
	}

	attemptRecipient, err := s.attemptRepo.FindRecipientByAttemptAndDocumentRecipient(ctx, attempt.ID, recipient.ID)
	if err != nil {
		return nil, err
	}
	if waitResp := s.checkAttemptSigningOrder(ctx, doc, recipient, attemptRecipient, accessToken.Token); waitResp != nil {
		return waitResp, nil
	}
	if attempt.ProviderDocumentID == nil || attemptRecipient.ProviderRecipientID == nil {
		resp := s.buildProcessingResponse(doc, recipient, accessToken.Token)
		applyLongRunningProcessingMetadata(resp, attempt)
		return resp, nil
	}
	embeddedResult, err := s.signingProvider.GetAttemptRecipientEmbeddedURL(ctx, &port.GetAttemptRecipientEmbeddedURLRequest{
		ProviderDocumentID:  *attempt.ProviderDocumentID,
		ProviderRecipientID: *attemptRecipient.ProviderRecipientID,
		CallbackURL:         s.buildCallbackURL(accessToken.Token),
	})
	if err != nil {
		if attemptRecipient.SigningURL != nil {
			resp := &documentuc.PublicSigningResponse{Step: documentuc.StepSigning, DocumentTitle: title, RecipientName: recipient.Name, FallbackURL: *attemptRecipient.SigningURL}
			s.applyAccessFlags(resp, doc, recipient, accessToken.Token)
			return resp, nil
		}
		return nil, fmt.Errorf("getting embedded signing URL: %w", err)
	}
	resp := &documentuc.PublicSigningResponse{Step: documentuc.StepSigning, DocumentTitle: title, RecipientName: recipient.Name, EmbeddedSigningURL: embeddedResult.EmbeddedURL}
	s.applyAccessFlags(resp, doc, recipient, accessToken.Token)
	return resp, nil
}

func (s *PreSigningService) buildDocumentUpdatedResponse(doc *entity.Document, recipient *entity.DocumentRecipient, token string) *documentuc.PublicSigningResponse {
	resp := &documentuc.PublicSigningResponse{Step: documentuc.StepDocumentUpdated, DocumentTitle: documentTitle(doc), RecipientName: recipient.Name}
	s.applyAccessFlags(resp, doc, recipient, token)
	return resp
}

func (s *PreSigningService) buildUnavailableResponse(doc *entity.Document, recipient *entity.DocumentRecipient, token string) *documentuc.PublicSigningResponse {
	resp := &documentuc.PublicSigningResponse{Step: documentuc.StepUnavailable, DocumentTitle: documentTitle(doc), RecipientName: recipient.Name}
	s.applyAccessFlags(resp, doc, recipient, token)
	return resp
}

func (s *PreSigningService) checkAttemptSigningOrder(ctx context.Context, doc *entity.Document, recipient *entity.DocumentRecipient, attemptRecipient *entity.SigningAttemptRecipient, token string) *documentuc.PublicSigningResponse {
	if attemptRecipient.SignerOrder <= 1 {
		return nil
	}
	recipients, err := s.attemptRepo.FindRecipientsByAttemptID(ctx, attemptRecipient.AttemptID)
	if err != nil {
		return nil
	}
	for _, r := range recipients {
		if r.SignerOrder < attemptRecipient.SignerOrder && r.Status != entity.RecipientStatusSigned {
			resp := &documentuc.PublicSigningResponse{Step: documentuc.StepWaiting, DocumentTitle: documentTitle(doc), RecipientName: recipient.Name, WaitingForPrevious: true, SigningPosition: attemptRecipient.SignerOrder, TotalSigners: len(recipients)}
			s.applyAccessFlags(resp, doc, recipient, token)
			return resp
		}
	}
	return nil
}

func (s *PreSigningService) signerOrderMap(ctx context.Context, templateVersionID string) (map[string]int, error) {
	roles, err := s.signerRoleRepo.FindByVersionID(ctx, templateVersionID)
	if err != nil {
		return nil, fmt.Errorf("loading signer roles: %w", err)
	}
	orders := make(map[string]int, len(roles))
	for _, role := range roles {
		orders[role.ID] = role.SignerOrder
	}
	return orders, nil
}

func (s *PreSigningService) applyAccessFlags(
	resp *documentuc.PublicSigningResponse,
	doc *entity.Document,
	recipient *entity.DocumentRecipient,
	token string,
) {
	resp.DocumentStatus = string(doc.Status)
	resp.HasCurrentUserSigned = recipient.IsSigned()
	resp.CanSign = doc.IsAwaitingInput() || doc.IsPending() || doc.IsInProgress()
	resp.CanDownload = doc.IsCompleted() && recipient.IsSigned()
	if resp.CanDownload {
		resp.DownloadURL = fmt.Sprintf("/public/sign/%s/download", token)
	}
}

// buildCallbackURL constructs the signing callback bridge URL.
func (s *PreSigningService) buildCallbackURL(token string) string {
	return fmt.Sprintf("%s/public/sign/%s/signing-callback", s.publicURL, token)
}

// --- Existing internal helpers (unchanged logic) ---

// loadSubmissionContext loads the document, recipient, version, and portable doc for a form submission.
func (s *PreSigningService) loadSubmissionContext(
	ctx context.Context,
	accessToken *entity.DocumentAccessToken,
) (*entity.Document, *entity.DocumentRecipient, *entity.TemplateVersion, *portabledoc.Document, error) {
	doc, err := s.documentRepo.FindByID(ctx, accessToken.DocumentID)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("finding document: %w", err)
	}
	if !doc.IsAwaitingInput() {
		return nil, nil, nil, nil, fmt.Errorf("document is not awaiting input")
	}

	recipient, err := s.recipientRepo.FindByID(ctx, accessToken.RecipientID)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("finding recipient: %w", err)
	}

	version, err := s.versionRepo.FindByID(ctx, doc.TemplateVersionID)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("finding template version: %w", err)
	}

	portableDoc, err := parsePortableDocument(version.ContentStructure)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("parsing document content: %w", err)
	}
	if portableDoc == nil {
		return nil, nil, nil, nil, fmt.Errorf("document has no content")
	}

	return doc, recipient, version, portableDoc, nil
}

// validateToken checks that a token exists, is not expired, and is not used.
func (s *PreSigningService) validateToken(ctx context.Context, token string) (*entity.DocumentAccessToken, error) {
	if token == "" {
		return nil, entity.ErrMissingToken
	}

	accessToken, err := s.accessTokenRepo.FindByToken(ctx, token)
	if err != nil {
		return nil, entity.ErrInvalidToken
	}

	if accessToken.UsedAt != nil {
		return nil, fmt.Errorf("access token has already been used")
	}

	if time.Now().UTC().After(accessToken.ExpiresAt) {
		return nil, entity.ErrTokenExpired
	}

	return accessToken, nil
}

// validateTokenAllowUsed is like validateToken but permits already-used tokens.
// Returns the token and whether it was already used. Expiry is only enforced
// for unused tokens — a used token can always proceed so GetPublicSigningPage
// can display terminal states (completed/declined).
func (s *PreSigningService) validateTokenAllowUsed(ctx context.Context, token string) (*entity.DocumentAccessToken, bool, error) {
	if token == "" {
		return nil, false, entity.ErrMissingToken
	}

	accessToken, err := s.accessTokenRepo.FindByToken(ctx, token)
	if err != nil {
		return nil, false, entity.ErrInvalidToken
	}

	wasUsed := accessToken.UsedAt != nil

	if !wasUsed && time.Now().UTC().After(accessToken.ExpiresAt) {
		return nil, false, entity.ErrTokenExpired
	}

	return accessToken, wasUsed, nil
}

// resolveContent serializes the portable doc content with injected values applied.
func (s *PreSigningService) resolveContent(
	ctx context.Context,
	portableDoc *portabledoc.Document,
	doc *entity.Document,
) (json.RawMessage, error) {
	if portableDoc.Content == nil {
		return nil, fmt.Errorf("document has no ProseMirror content")
	}

	signerRoleValues, err := s.loadSignerRoleValues(ctx, doc, portableDoc)
	if err != nil {
		slog.WarnContext(ctx, "failed to load signer role values for preview content",
			slog.String("document_id", doc.ID),
			slog.String("error", err.Error()),
		)
		signerRoleValues = nil
	}

	var injectables map[string]any
	if doc.InjectedValuesSnapshot != nil {
		_ = json.Unmarshal(doc.InjectedValuesSnapshot, &injectables)
	}

	resolvedContent := &portabledoc.ProseMirrorDoc{
		Type:    portableDoc.Content.Type,
		Content: resolvePreviewNodes(portableDoc.Content.Content, injectables, signerRoleValues),
	}

	return json.Marshal(resolvedContent)
}

func (s *PreSigningService) loadSignerRoleValues(
	ctx context.Context,
	doc *entity.Document,
	portableDoc *portabledoc.Document,
) (map[string]port.SignerRoleValue, error) {
	recipients, err := s.recipientRepo.FindByDocumentID(ctx, doc.ID)
	if err != nil {
		return nil, fmt.Errorf("loading recipients: %w", err)
	}

	signerRoles, err := s.signerRoleRepo.FindByVersionID(ctx, doc.TemplateVersionID)
	if err != nil {
		return nil, fmt.Errorf("loading signer roles: %w", err)
	}

	return buildSignerRoleValues(recipients, signerRoles, portableDoc.SignerRoles), nil
}

func resolvePreviewNodes(
	nodes []portabledoc.Node,
	injectables map[string]any,
	signerRoleValues map[string]port.SignerRoleValue,
) []portabledoc.Node {
	if len(nodes) == 0 {
		return nil
	}

	out := make([]portabledoc.Node, 0, len(nodes))
	for _, node := range nodes {
		if node.Type == portabledoc.NodeTypeInjector {
			if text, ok := resolvePreviewInjector(node.Attrs, injectables, signerRoleValues); ok {
				resolvedText := text
				out = append(out, portabledoc.Node{
					Type:  portabledoc.NodeTypeText,
					Marks: node.Marks,
					Text:  &resolvedText,
				})
			}
			continue
		}

		clone := node
		if len(node.Content) > 0 {
			clone.Content = resolvePreviewNodes(node.Content, injectables, signerRoleValues)
		}
		out = append(out, clone)
	}

	return out
}

//nolint:gocognit
func resolvePreviewInjector(
	attrs map[string]any,
	injectables map[string]any,
	signerRoleValues map[string]port.SignerRoleValue,
) (string, bool) {
	variableID, _ := attrs["variableId"].(string)
	isRoleVariable, _ := attrs["isRoleVariable"].(bool)
	roleID, _ := attrs["roleId"].(string)
	propertyKey, _ := attrs["propertyKey"].(string)
	injectorType, _ := attrs["type"].(string)
	format, _ := attrs["format"].(string)
	prefix, _ := attrs["prefix"].(string)
	suffix, _ := attrs["suffix"].(string)
	defaultValue, _ := attrs["defaultValue"].(string)
	showLabelIfEmpty, _ := attrs["showLabelIfEmpty"].(bool)

	value := ""
	if isRoleVariable {
		if roleValue, ok := signerRoleValues[roleID]; ok {
			switch propertyKey {
			case portabledoc.RolePropertyName:
				value = roleValue.Name
			case portabledoc.RolePropertyEmail:
				value = roleValue.Email
			}
		}
	}

	if value == "" && variableID != "" {
		if raw, ok := injectables[variableID]; ok {
			value = formatPreviewInjectableValue(raw, injectorType, format)
		}
	}

	if value == "" {
		value = defaultValue
	}

	if value == "" {
		if !showLabelIfEmpty {
			return "", false
		}
		placeholder := prefix + suffix
		if placeholder == "" {
			return "", false
		}
		return placeholder, true
	}

	return prefix + value + suffix, true
}

func formatPreviewInjectableValue(value any, injectorType, format string) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		if injectorType == portabledoc.InjectorTypeCurrency {
			if format != "" {
				return fmt.Sprintf("%s %.2f", format, v)
			}
			return fmt.Sprintf("%.2f", v)
		}
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case int64:
		return strconv.FormatInt(v, 10)
	case bool:
		if v {
			return "Sí"
		}
		return "No"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// findPortableRoleID maps a DB signer role ID to the portable doc role ID.
func (s *PreSigningService) findPortableRoleID(
	ctx context.Context,
	version *entity.TemplateVersion,
	portableDoc *portabledoc.Document,
	dbRoleID string,
) string {
	signerRoles, err := s.signerRoleRepo.FindByVersionID(ctx, version.ID)
	if err != nil {
		return ""
	}

	var anchor string
	for _, role := range signerRoles {
		if role.ID == dbRoleID {
			anchor = role.AnchorString
			break
		}
	}
	if anchor == "" {
		return ""
	}

	for _, sr := range portableDoc.SignerRoles {
		if portabledoc.GenerateAnchorString(sr.Label) == anchor {
			return sr.ID
		}
	}

	return ""
}

func (s *PreSigningService) resolvePortableRoleID(
	ctx context.Context,
	version *entity.TemplateVersion,
	portableDoc *portabledoc.Document,
	dbRoleID string,
) (string, error) {
	portableRoleID := s.findPortableRoleID(ctx, version, portableDoc, dbRoleID)
	if portableRoleID == "" {
		return "", fmt.Errorf("%w: signer role mapping not found", entity.ErrInvalidDocumentState)
	}
	return portableRoleID, nil
}

// extractInteractiveFieldsForRole extracts interactive fields for a specific role.
func extractInteractiveFieldsForRole(doc *portabledoc.Document, roleID string) []documentuc.InteractiveFieldDTO {
	fields := make([]documentuc.InteractiveFieldDTO, 0, 8)
	if roleID == "" {
		return fields
	}

	for _, node := range doc.CollectNodesOfType(portabledoc.NodeTypeInteractiveField) {
		attrs, err := portabledoc.ParseInteractiveFieldAttrs(node.Attrs)
		if err != nil {
			continue
		}

		if attrs.RoleID != roleID {
			continue
		}

		fields = append(fields, documentuc.InteractiveFieldDTO{
			ID:          attrs.ID,
			FieldType:   attrs.FieldType,
			Label:       attrs.Label,
			Required:    attrs.Required,
			Options:     attrs.Options,
			Placeholder: attrs.Placeholder,
			MaxLength:   attrs.MaxLength,
		})
	}

	return fields
}

// validateResponses validates all field responses against field definitions.
func (s *PreSigningService) validateResponses(
	responses []documentuc.FieldResponseInput,
	fieldDefs []documentuc.InteractiveFieldDTO,
) error {
	defByID := make(map[string]documentuc.InteractiveFieldDTO, len(fieldDefs))
	for _, def := range fieldDefs {
		defByID[def.ID] = def
	}

	submittedIDs := make(map[string]bool, len(responses))
	for _, resp := range responses {
		submittedIDs[resp.FieldID] = true
	}

	for _, def := range fieldDefs {
		if def.Required && !submittedIDs[def.ID] {
			return fmt.Errorf("required field %q (%s) is missing", def.Label, def.ID)
		}
	}

	for _, resp := range responses {
		def, ok := defByID[resp.FieldID]
		if !ok {
			return fmt.Errorf("unknown field ID: %s", resp.FieldID)
		}

		if err := validateSingleResponse(resp, def); err != nil {
			return fmt.Errorf("field %q (%s): %w", def.Label, def.ID, err)
		}
	}

	return nil
}

// validateSingleResponse validates a single field response against its definition.
func validateSingleResponse(resp documentuc.FieldResponseInput, def documentuc.InteractiveFieldDTO) error {
	switch def.FieldType {
	case portabledoc.InteractiveFieldTypeCheckbox, portabledoc.InteractiveFieldTypeRadio:
		return validateSelectionResponse(resp.Response, def)
	case portabledoc.InteractiveFieldTypeText:
		return validateTextResponse(resp.Response, def)
	default:
		return fmt.Errorf("unsupported field type: %s", def.FieldType)
	}
}

type selectionResponse struct {
	SelectedOptionIDs []string `json:"selectedOptionIds"`
}

type textResponse struct {
	Text string `json:"text"`
}

func validateSelectionResponse(responseJSON json.RawMessage, def documentuc.InteractiveFieldDTO) error {
	var resp selectionResponse
	if err := json.Unmarshal(responseJSON, &resp); err != nil {
		return fmt.Errorf("invalid response format: expected {\"selectedOptionIds\":[...]}")
	}

	if def.FieldType == portabledoc.InteractiveFieldTypeRadio && len(resp.SelectedOptionIDs) > 1 {
		return fmt.Errorf("radio field must have at most one selected option")
	}

	validIDs := make(map[string]bool, len(def.Options))
	for _, opt := range def.Options {
		validIDs[opt.ID] = true
	}

	for _, id := range resp.SelectedOptionIDs {
		if !validIDs[id] {
			return fmt.Errorf("invalid option ID: %s", id)
		}
	}

	return nil
}

func validateTextResponse(responseJSON json.RawMessage, def documentuc.InteractiveFieldDTO) error {
	var resp textResponse
	if err := json.Unmarshal(responseJSON, &resp); err != nil {
		return fmt.Errorf("invalid response format: expected {\"text\":\"...\"}")
	}

	if def.MaxLength > 0 && len(resp.Text) > def.MaxLength {
		return fmt.Errorf("text exceeds maximum length of %d characters", def.MaxLength)
	}

	return nil
}

func (s *PreSigningService) saveFieldResponses(
	ctx context.Context,
	documentID, recipientID string,
	responses []documentuc.FieldResponseInput,
	fieldDefs []documentuc.InteractiveFieldDTO,
) error {
	typeByID := make(map[string]string, len(fieldDefs))
	for _, def := range fieldDefs {
		typeByID[def.ID] = def.FieldType
	}

	for _, resp := range responses {
		fieldResponse := &entity.DocumentFieldResponse{
			ID:          uuid.NewString(),
			DocumentID:  documentID,
			RecipientID: recipientID,
			FieldID:     resp.FieldID,
			FieldType:   typeByID[resp.FieldID],
			Response:    resp.Response,
			CreatedAt:   time.Now().UTC(),
		}

		if err := s.fieldResponseRepo.Create(ctx, fieldResponse); err != nil {
			return fmt.Errorf("creating field response for field %s: %w", resp.FieldID, err)
		}
	}

	return nil
}

// Verify PreSigningService implements PreSigningUseCase.
var _ documentuc.PreSigningUseCase = (*PreSigningService)(nil)
