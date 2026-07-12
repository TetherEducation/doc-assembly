package document

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
)

// NewDocumentService creates a new document service.
func NewDocumentService(
	documentRepo port.DocumentRepository,
	recipientRepo port.DocumentRecipientRepository,
	attemptRepo port.SigningAttemptRepository,
	signingUOW port.SigningExecutionUnitOfWork,
	templateRepo port.TemplateRepository,
	versionRepo port.TemplateVersionRepository,
	signerRoleRepo port.TemplateVersionSignerRoleRepository,
	pdfRenderer port.PDFRenderer,
	signingProvider port.SigningProvider,
	storageAdapter port.StorageAdapter,
	eventEmitter *EventEmitter,
	notificationSvc *NotificationService,
	expirationDays int,
	accessTokenRepo port.DocumentAccessTokenRepository,
	fieldResponseRepo port.DocumentFieldResponseRepository,
	storageEnabled bool,
) *DocumentService {
	return &DocumentService{
		documentRepo:      documentRepo,
		recipientRepo:     recipientRepo,
		attemptRepo:       attemptRepo,
		signingUOW:        signingUOW,
		templateRepo:      templateRepo,
		versionRepo:       versionRepo,
		signerRoleRepo:    signerRoleRepo,
		pdfRenderer:       pdfRenderer,
		signingProvider:   signingProvider,
		storageAdapter:    storageAdapter,
		eventEmitter:      eventEmitter,
		notificationSvc:   notificationSvc,
		expirationDays:    expirationDays,
		accessTokenRepo:   accessTokenRepo,
		fieldResponseRepo: fieldResponseRepo,
		storageEnabled:    storageEnabled,
	}
}

// DocumentService implements document business logic.
type DocumentService struct {
	documentRepo      port.DocumentRepository
	recipientRepo     port.DocumentRecipientRepository
	attemptRepo       port.SigningAttemptRepository
	signingUOW        port.SigningExecutionUnitOfWork
	templateRepo      port.TemplateRepository
	versionRepo       port.TemplateVersionRepository
	signerRoleRepo    port.TemplateVersionSignerRoleRepository
	pdfRenderer       port.PDFRenderer
	signingProvider   port.SigningProvider
	storageAdapter    port.StorageAdapter
	eventEmitter      *EventEmitter
	notificationSvc   *NotificationService
	expirationDays    int
	accessTokenRepo   port.DocumentAccessTokenRepository
	fieldResponseRepo port.DocumentFieldResponseRepository
	storageEnabled    bool
}

// CreateAndSendDocument creates a document, generates the PDF, and sends it for signing.
// If the template has interactive fields and exactly one unsigned signer, the document
// is placed in AWAITING_INPUT status with an access token instead of rendering/uploading.
func (s *DocumentService) CreateAndSendDocument(ctx context.Context, cmd documentuc.CreateDocumentCommand) (*entity.DocumentWithRecipients, error) {
	if err := s.validateOperationType(ctx, cmd); err != nil {
		return nil, err
	}

	version, _, err := s.validateTemplateAndRoles(ctx, cmd)
	if err != nil {
		return nil, err
	}

	templateWorkspace, err := s.templateRepo.FindTemplateWorkspaceByTemplateID(ctx, version.TemplateID)
	if err != nil {
		return nil, fmt.Errorf("finding template from version: %w", err)
	}
	if templateWorkspace == nil || templateWorkspace.Template == nil {
		return nil, entity.ErrTemplateNotFound
	}
	if templateWorkspace.Workspace != nil && templateWorkspace.Workspace.Type == entity.WorkspaceTypeSystem {
		return nil, entity.ErrInvalidWorkspaceType
	}
	template := templateWorkspace.Template
	if template.DocumentTypeID == nil || *template.DocumentTypeID == "" {
		return nil, entity.ErrDocumentTypeNotFound
	}

	document, err := s.createDocument(ctx, cmd, *template.DocumentTypeID)
	if err != nil {
		return nil, err
	}

	recipients, err := s.createRecipients(ctx, document.ID, cmd.Recipients)
	if err != nil {
		return nil, err
	}

	// All documents enter AWAITING_INPUT. Tokens are generated on-demand
	// via the email-verification flow (DocumentAccessService).
	if err := s.transitionToAwaitingInput(ctx, document); err != nil {
		return nil, err
	}

	s.notificationSvc.NotifyDocumentCreated(ctx, document.ID)

	return &entity.DocumentWithRecipients{
		Document:   *document,
		Recipients: recipients,
	}, nil
}

// validateTemplateAndRoles validates the template version and returns the role map.
func (s *DocumentService) validateTemplateAndRoles(ctx context.Context, cmd documentuc.CreateDocumentCommand) (*entity.TemplateVersion, map[string]*entity.TemplateVersionSignerRole, error) {
	version, err := s.versionRepo.FindByID(ctx, cmd.TemplateVersionID)
	if err != nil {
		return nil, nil, fmt.Errorf("finding template version: %w", err)
	}

	if !version.IsPublished() {
		return nil, nil, fmt.Errorf("template version is not published")
	}

	signerRoles, err := s.signerRoleRepo.FindByVersionID(ctx, cmd.TemplateVersionID)
	if err != nil {
		return nil, nil, fmt.Errorf("finding signer roles: %w", err)
	}

	if len(cmd.Recipients) != len(signerRoles) {
		return nil, nil, fmt.Errorf("recipient count (%d) does not match signer role count (%d)", len(cmd.Recipients), len(signerRoles))
	}

	roleMap := make(map[string]*entity.TemplateVersionSignerRole)
	for _, role := range signerRoles {
		roleMap[role.ID] = role
	}

	for _, r := range cmd.Recipients {
		if _, ok := roleMap[r.RoleID]; !ok {
			return nil, nil, fmt.Errorf("invalid role ID: %s", r.RoleID)
		}
	}

	return version, roleMap, nil
}

// validateOperationType validates the operation type and related document for RENEW/AMEND.
func (s *DocumentService) validateOperationType(ctx context.Context, cmd documentuc.CreateDocumentCommand) error {
	opType := cmd.OperationType
	if opType == "" {
		opType = entity.OperationCreate
	}

	if !opType.IsValid() {
		return entity.ErrInvalidOperationType
	}

	if opType == entity.OperationCreate {
		return nil
	}

	if opType != entity.OperationRenew && opType != entity.OperationAmend {
		return nil
	}

	if cmd.RelatedDocumentID == nil {
		return entity.ErrRelatedDocumentRequired
	}

	relatedDoc, err := s.documentRepo.FindByID(ctx, *cmd.RelatedDocumentID)
	if err != nil {
		return fmt.Errorf("finding related document: %w", err)
	}

	if relatedDoc.WorkspaceID != cmd.WorkspaceID {
		return entity.ErrRelatedDocumentSameWorkspace
	}

	switch opType {
	case entity.OperationRenew:
		if !relatedDoc.IsCompleted() {
			return entity.ErrDocumentNotCompleted
		}
	case entity.OperationAmend:
		if !relatedDoc.IsTerminal() {
			return entity.ErrDocumentNotTerminal
		}
	}

	return nil
}

// createDocument creates and persists the document entity.
func (s *DocumentService) createDocument(ctx context.Context, cmd documentuc.CreateDocumentCommand, documentTypeID string) (*entity.Document, error) {
	document := entity.NewDocument(cmd.WorkspaceID, cmd.TemplateVersionID)
	document.DocumentTypeID = documentTypeID
	document.ID = uuid.NewString()
	document.SetTitle(cmd.Title)

	if cmd.OperationType != "" && cmd.OperationType != entity.OperationCreate {
		document.SetOperationType(cmd.OperationType)
	}

	if cmd.RelatedDocumentID != nil {
		document.SetRelatedDocumentID(*cmd.RelatedDocumentID)
	}

	if cmd.ClientExternalReferenceID != nil {
		document.SetExternalReference(*cmd.ClientExternalReferenceID)
	}
	// Ensure transactional_id is always present for traceability and DB constraints.
	document.SetTransactionalID(uuid.NewString())

	if cmd.InjectedValues != nil {
		valuesJSON, err := json.Marshal(cmd.InjectedValues)
		if err != nil {
			return nil, fmt.Errorf("marshaling injected values: %w", err)
		}
		document.SetInjectedValues(valuesJSON)
	}

	if err := document.Validate(); err != nil {
		return nil, fmt.Errorf("validating document: %w", err)
	}

	docID, err := s.documentRepo.Create(ctx, document)
	if err != nil {
		return nil, fmt.Errorf("creating document: %w", err)
	}
	document.ID = docID

	return document, nil
}

// createRecipients creates and persists the document recipients.
func (s *DocumentService) createRecipients(ctx context.Context, docID string, cmdRecipients []documentuc.DocumentRecipientCommand) ([]*entity.DocumentRecipient, error) {
	recipients := make([]*entity.DocumentRecipient, len(cmdRecipients))

	for i, r := range cmdRecipients {
		recipient := entity.NewDocumentRecipient(docID, r.RoleID, r.Name, r.Email)
		recipient.ID = uuid.NewString()

		if err := recipient.Validate(); err != nil {
			return nil, fmt.Errorf("validating recipient: %w", err)
		}

		recipients[i] = recipient
	}

	if err := s.recipientRepo.CreateBatch(ctx, recipients); err != nil {
		return nil, fmt.Errorf("creating recipients: %w", err)
	}

	return recipients, nil
}

// GetDocument retrieves a document by ID.
func (s *DocumentService) GetDocument(ctx context.Context, id string) (*entity.Document, error) {
	return s.documentRepo.FindByID(ctx, id)
}

// GetDocumentWithRecipients retrieves a document with all its recipients.
func (s *DocumentService) GetDocumentWithRecipients(ctx context.Context, id string) (*entity.DocumentWithRecipients, error) {
	return s.documentRepo.FindByIDWithRecipients(ctx, id)
}

// ListDocuments lists documents in a workspace with optional filters.
func (s *DocumentService) ListDocuments(ctx context.Context, workspaceID string, filters port.DocumentFilters) ([]*entity.DocumentListItem, error) {
	return s.documentRepo.FindByWorkspace(ctx, workspaceID, filters)
}

// ListDocumentTypeFilterOptions lists distinct document types on workspace documents.
func (s *DocumentService) ListDocumentTypeFilterOptions(ctx context.Context, workspaceID string) ([]*entity.DocumentTypeFilterOption, error) {
	return s.documentRepo.ListDistinctDocumentTypesForWorkspace(ctx, workspaceID)
}

// GetSigningURL retrieves the signing URL for a specific recipient.
func (s *DocumentService) GetSigningURL(ctx context.Context, documentID, recipientID string) (string, error) {
	doc, err := s.documentRepo.FindByIDWithRecipients(ctx, documentID)
	if err != nil {
		return "", fmt.Errorf("finding document: %w", err)
	}
	if doc.ActiveAttemptID == nil || *doc.ActiveAttemptID == "" {
		return "", entity.ErrSigningAttemptNotFound
	}
	// Keep embed prefill consistent with the public signing path.
	recipientName := ""
	for _, rec := range doc.Recipients {
		if rec != nil && rec.ID == recipientID {
			recipientName = rec.Name
			break
		}
	}
	attempt, err := s.attemptRepo.FindByID(ctx, *doc.ActiveAttemptID)
	if err != nil {
		return "", fmt.Errorf("finding active signing attempt: %w", err)
	}
	if attempt.ProviderDocumentID == nil || *attempt.ProviderDocumentID == "" {
		return "", fmt.Errorf("signing provider document is not ready")
	}
	attemptRecipients, err := s.attemptRepo.FindRecipientsByAttemptID(ctx, attempt.ID)
	if err != nil {
		return "", fmt.Errorf("finding attempt recipients: %w", err)
	}
	for _, r := range attemptRecipients {
		if r.DocumentRecipientID == nil || *r.DocumentRecipientID != recipientID || r.ProviderRecipientID == nil {
			continue
		}
		result, err := s.signingProvider.GetAttemptRecipientEmbeddedURL(ctx, &port.GetAttemptRecipientEmbeddedURLRequest{
			ProviderDocumentID:  *attempt.ProviderDocumentID,
			ProviderRecipientID: *r.ProviderRecipientID,
			Environment:         entity.EnvironmentProd,
			RecipientName:       recipientName,
		})
		if err != nil {
			return "", fmt.Errorf("getting attempt signing URL: %w", err)
		}
		return result.EmbeddedURL, nil
	}
	return "", entity.ErrRecordNotFound
}

// RefreshDocumentStatus requests an attempt-scoped River refresh and returns the current projection.
func (s *DocumentService) RefreshDocumentStatus(ctx context.Context, documentID string) (*entity.DocumentWithRecipients, error) {
	doc, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("finding document: %w", err)
	}
	if doc.ActiveAttemptID == nil || *doc.ActiveAttemptID == "" {
		return s.documentRepo.FindByIDWithRecipients(ctx, documentID)
	}
	attempt, err := s.attemptRepo.FindByID(ctx, *doc.ActiveAttemptID)
	if err != nil {
		return nil, fmt.Errorf("finding active signing attempt: %w", err)
	}
	if attempt.ProviderDocumentID == nil || attempt.IsTerminal() {
		return s.documentRepo.FindByIDWithRecipients(ctx, documentID)
	}
	if err := s.signingUOW.TransitionAndEnqueue(ctx, attempt, port.SigningJobPhaseRefreshProviderStatus, "ATTEMPT_REFRESH_REQUESTED"); err != nil {
		return nil, fmt.Errorf("enqueueing attempt refresh: %w", err)
	}
	return s.documentRepo.FindByIDWithRecipients(ctx, documentID)
}

// CancelDocument cancels/voids a document that is pending signatures.
//
//nolint:nestif
func (s *DocumentService) CancelDocument(ctx context.Context, documentID string) error {
	doc, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		return fmt.Errorf("finding document: %w", err)
	}

	oldStatus := string(doc.Status)

	if doc.IsTerminal() {
		return fmt.Errorf("cannot cancel document in terminal state: %s", doc.Status)
	}

	if doc.ActiveAttemptID != nil && *doc.ActiveAttemptID != "" {
		attempt, err := s.attemptRepo.FindByID(ctx, *doc.ActiveAttemptID)
		if err != nil {
			return fmt.Errorf("finding active attempt: %w", err)
		}
		if err := s.signingUOW.TerminateActiveAttempt(ctx, attempt, entity.SigningAttemptStatusCancelled, "cancelled by user", "ATTEMPT_CANCELLED"); err != nil {
			return fmt.Errorf("cancelling active attempt: %w", err)
		}
	} else {
		if err := doc.MarkAsVoided(); err != nil {
			return fmt.Errorf("marking document as cancelled: %w", err)
		}
		if err := s.documentRepo.Update(ctx, doc); err != nil {
			return fmt.Errorf("updating document: %w", err)
		}
	}

	s.eventEmitter.EmitDocumentEvent(ctx, documentID, entity.EventDocumentCancelled, entity.ActorUser, "", oldStatus, string(entity.DocumentStatusCancelled), nil)

	slog.InfoContext(ctx, "document cancelled", slog.String("document_id", documentID))

	return nil
}

// DeprecateDocument invalidates a completed document and performs best-effort provider cleanup.
func (s *DocumentService) DeprecateDocument(ctx context.Context, documentID string, reason *string) (*documentuc.DeprecateDocumentResult, error) {
	doc, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("finding document: %w", err)
	}
	if doc.Status != entity.DocumentStatusCompleted {
		return nil, entity.ErrInvalidDocumentState
	}

	oldStatus := string(doc.Status)
	cleanup := s.cleanupProviderForDeprecatedDocument(ctx, doc)

	if err := doc.UpdateStatus(entity.DocumentStatusInvalidated); err != nil {
		return nil, fmt.Errorf("marking document deprecated: %w", err)
	}
	doc.IsActive = false
	now := time.Now().UTC()
	doc.SupersededAt = &now
	doc.SupersedeReason = reason
	if err := s.documentRepo.Update(ctx, doc); err != nil {
		return nil, fmt.Errorf("updating deprecated document: %w", err)
	}

	metadata, _ := json.Marshal(map[string]any{
		"reason":           reasonString(reason),
		"providerCleanup":  cleanup,
		"deprecationState": string(entity.DocumentStatusInvalidated),
	})
	s.eventEmitter.EmitDocumentEvent(ctx, documentID, entity.EventDocumentDeprecated, entity.ActorUser, "", oldStatus, string(entity.DocumentStatusInvalidated), metadata)

	return &documentuc.DeprecateDocumentResult{Document: doc, ProviderCleanup: cleanup}, nil
}

func (s *DocumentService) cleanupProviderForDeprecatedDocument(ctx context.Context, doc *entity.Document) *documentuc.ProviderCleanupResult {
	if doc.ActiveAttemptID == nil || *doc.ActiveAttemptID == "" {
		return nil
	}
	attempt, err := s.attemptRepo.FindByID(ctx, *doc.ActiveAttemptID)
	if err != nil || attempt.ProviderDocumentID == nil || *attempt.ProviderDocumentID == "" {
		return nil
	}

	status := "UNSUPPORTED"
	result := &documentuc.ProviderCleanupResult{Status: status, Reason: "provider cleanup is not supported"}
	if s.signingProvider != nil && s.signingProvider.ProviderCapabilities().CanCancel {
		providerResult, cleanupErr := s.signingProvider.CleanupProviderDocument(ctx, &port.CleanupProviderDocumentRequest{
			ProviderDocumentID: *attempt.ProviderDocumentID,
			Environment:        entity.EnvironmentProd,
		})
		if cleanupErr != nil {
			status = "FAILED_RETRYABLE"
			result = &documentuc.ProviderCleanupResult{Status: status, Reason: cleanupErr.Error()}
		} else {
			status = providerResult.Status
			result = &documentuc.ProviderCleanupResult{
				Action: providerResult.Action,
				Status: providerResult.Status,
				Reason: providerResult.Reason,
			}
		}
	}

	attempt.CleanupStatus = &status
	if result.Action != "" {
		attempt.CleanupAction = &result.Action
	}
	if result.Reason != "" && status != "SUCCEEDED" {
		attempt.CleanupError = &result.Reason
	}
	if err := s.attemptRepo.Update(ctx, attempt); err != nil {
		slog.WarnContext(ctx, "failed to persist deprecated document provider cleanup",
			"document_id", doc.ID,
			"attempt_id", attempt.ID,
			"error", err,
		)
	}
	return result
}

func reasonString(reason *string) string {
	if reason == nil {
		return ""
	}
	return *reason
}

// HandleWebhookEvent processes an incoming webhook event from the signing provider.
//
//nolint:gocognit,gocyclo,nestif,funlen
func (s *DocumentService) HandleWebhookEvent(ctx context.Context, event *port.WebhookEvent) error {
	attempt, err := s.findAttemptForWebhook(ctx, event)
	if err != nil {
		return fmt.Errorf("finding signing attempt for webhook: %w", err)
	}
	doc, err := s.documentRepo.FindByID(ctx, attempt.DocumentID)
	if err != nil {
		return fmt.Errorf("finding webhook document: %w", err)
	}

	slog.InfoContext(ctx, "processing attempt webhook event",
		slog.String("event_type", event.EventType),
		slog.String("document_id", doc.ID),
		slog.String("attempt_id", attempt.ID),
		slog.String("provider_document_id", event.ProviderDocumentID),
	)

	newStatus := attempt.Status
	oldStatus := attempt.Status
	if event.DocumentStatus != nil {
		newStatus = *event.DocumentStatus
		attempt.Status = newStatus
	}
	if event.ProviderCorrelationKey != "" {
		attempt.ProviderCorrelationKey = &event.ProviderCorrelationKey
	}
	if event.ProviderDocumentID != "" {
		attempt.ProviderDocumentID = &event.ProviderDocumentID
	}
	if event.DocumentStatus != nil && attempt.Status.IsTerminal() {
		now := time.Now().UTC()
		attempt.TerminalAt = &now
	}
	if event.ProviderRecipientID != "" && event.RecipientStatus != nil {
		if err := s.updateAttemptRecipientFromWebhook(ctx, attempt.ID, event.ProviderRecipientID, *event.RecipientStatus); err != nil {
			slog.WarnContext(ctx, "failed to update attempt recipient from webhook", slog.String("error", err.Error()))
		}
	}
	if event.DocumentStatus != nil && *event.DocumentStatus == entity.SigningAttemptStatusCompleted {
		if err := s.markAttemptRecipientsSigned(ctx, attempt.ID); err != nil {
			return fmt.Errorf("marking completed attempt recipients signed: %w", err)
		}
	}

	// Historical attempts are auditable only; they must never mutate the active document projection.
	if doc.ActiveAttemptID == nil || *doc.ActiveAttemptID != attempt.ID {
		if err := s.attemptRepo.Update(ctx, attempt); err != nil {
			return fmt.Errorf("updating historical webhook attempt: %w", err)
		}
		_ = s.attemptRepo.InsertEvent(ctx, &entity.SigningAttemptEvent{AttemptID: attempt.ID, DocumentID: attempt.DocumentID, EventType: "ATTEMPT_WEBHOOK_RECEIVED", OldStatus: &oldStatus, NewStatus: &newStatus, ProviderName: &event.ProviderName, ProviderDocumentID: &event.ProviderDocumentID, CorrelationKey: &event.ProviderCorrelationKey, RawPayload: event.RawPayload})
		return nil
	}
	if event.DocumentStatus != nil {
		if *event.DocumentStatus == entity.SigningAttemptStatusCompleted {
			if err := s.signingUOW.TransitionAndEnqueue(ctx, attempt, port.SigningJobPhaseDispatchCompletion, "ATTEMPT_COMPLETED"); err != nil {
				return fmt.Errorf("persisting completed attempt from webhook: %w", err)
			}
		} else if err := s.signingUOW.Transition(ctx, attempt, "ATTEMPT_WEBHOOK_STATUS_UPDATED"); err != nil {
			return fmt.Errorf("persisting active attempt from webhook: %w", err)
		}
	} else if err := s.attemptRepo.Update(ctx, attempt); err != nil {
		return fmt.Errorf("updating active webhook attempt: %w", err)
	}
	_ = s.attemptRepo.InsertEvent(ctx, &entity.SigningAttemptEvent{AttemptID: attempt.ID, DocumentID: attempt.DocumentID, EventType: "ATTEMPT_WEBHOOK_RECEIVED", OldStatus: &oldStatus, NewStatus: &newStatus, ProviderName: &event.ProviderName, ProviderDocumentID: &event.ProviderDocumentID, CorrelationKey: &event.ProviderCorrelationKey, RawPayload: event.RawPayload})
	return nil
}

func (s *DocumentService) findAttemptForWebhook(ctx context.Context, event *port.WebhookEvent) (*entity.SigningAttempt, error) {
	if event.ProviderDocumentID != "" {
		attempt, err := s.attemptRepo.FindByProviderDocumentID(ctx, event.ProviderName, event.ProviderDocumentID)
		if err == nil {
			return attempt, nil
		}
		if !errors.Is(err, entity.ErrRecordNotFound) || event.ProviderCorrelationKey == "" {
			return nil, err
		}
	}
	if event.ProviderCorrelationKey != "" {
		return s.attemptRepo.FindByProviderCorrelationKey(ctx, event.ProviderName, event.ProviderCorrelationKey)
	}
	return nil, entity.ErrRecordNotFound
}

func (s *DocumentService) updateAttemptRecipientFromWebhook(ctx context.Context, attemptID, providerRecipientID string, status entity.RecipientStatus) error {
	recipients, err := s.attemptRepo.FindRecipientsByAttemptID(ctx, attemptID)
	if err != nil {
		return err
	}
	for _, recipient := range recipients {
		if recipient.ProviderRecipientID == nil || *recipient.ProviderRecipientID != providerRecipientID {
			continue
		}
		recipient.Status = status.Normalize()
		if recipient.Status == entity.RecipientStatusSigned {
			now := time.Now().UTC()
			recipient.SignedAt = &now
		}
		return s.attemptRepo.UpdateRecipient(ctx, recipient)
	}
	return entity.ErrRecordNotFound
}

func (s *DocumentService) markAttemptRecipientsSigned(ctx context.Context, attemptID string) error {
	recipients, err := s.attemptRepo.FindRecipientsByAttemptID(ctx, attemptID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, recipient := range recipients {
		if err := s.markAttemptRecipientSigned(ctx, recipient, now); err != nil {
			return err
		}
	}
	return nil
}

func (s *DocumentService) markAttemptRecipientSigned(ctx context.Context, recipient *entity.SigningAttemptRecipient, signedAt time.Time) error {
	if recipient.Status != entity.RecipientStatusSigned {
		recipient.Status = entity.RecipientStatusSigned
		recipient.SignedAt = &signedAt
		if err := s.attemptRepo.UpdateRecipient(ctx, recipient); err != nil {
			return err
		}
	}
	if recipient.DocumentRecipientID == nil {
		return nil
	}
	return s.markDocumentRecipientSigned(ctx, *recipient.DocumentRecipientID)
}

func (s *DocumentService) markDocumentRecipientSigned(ctx context.Context, recipientID string) error {
	recipient, err := s.recipientRepo.FindByID(ctx, recipientID)
	if err != nil {
		return err
	}
	if recipient.Status == entity.RecipientStatusSigned {
		return nil
	}
	if err := recipient.MarkAsSigned(); err != nil {
		return err
	}
	return s.recipientRepo.Update(ctx, recipient)
}

// GetDocumentsByExternalRef finds documents by the client's external reference ID.
func (s *DocumentService) GetDocumentsByExternalRef(ctx context.Context, workspaceID, externalRef string) ([]*entity.Document, error) {
	return s.documentRepo.FindByClientExternalRef(ctx, workspaceID, externalRef)
}

// GetDocumentRecipients retrieves all recipients for a document with their role information.
func (s *DocumentService) GetDocumentRecipients(ctx context.Context, documentID string) ([]*entity.DocumentRecipientWithRole, error) {
	if _, err := s.documentRepo.FindByID(ctx, documentID); err != nil {
		return nil, err
	}
	return s.recipientRepo.FindByDocumentIDWithRoles(ctx, documentID)
}

// GetDocumentStatistics returns document statistics for a workspace.
func (s *DocumentService) GetDocumentStatistics(ctx context.Context, workspaceID string) (*documentuc.DocumentStatistics, error) {
	total, err := s.documentRepo.CountByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("counting documents: %w", err)
	}

	pending, err := s.documentRepo.CountByStatus(ctx, workspaceID, entity.DocumentStatusReadyToSign)
	if err != nil {
		return nil, fmt.Errorf("counting pending documents: %w", err)
	}

	inProgress, err := s.documentRepo.CountByStatus(ctx, workspaceID, entity.DocumentStatusSigning)
	if err != nil {
		return nil, fmt.Errorf("counting in-progress documents: %w", err)
	}

	completed, err := s.documentRepo.CountByStatus(ctx, workspaceID, entity.DocumentStatusCompleted)
	if err != nil {
		return nil, fmt.Errorf("counting completed documents: %w", err)
	}

	declined, err := s.documentRepo.CountByStatus(ctx, workspaceID, entity.DocumentStatusDeclined)
	if err != nil {
		return nil, fmt.Errorf("counting declined documents: %w", err)
	}

	return &documentuc.DocumentStatistics{
		Total:      total,
		Pending:    pending,
		InProgress: inProgress,
		Completed:  completed,
		Declined:   declined,
		ByStatus: map[string]int{
			entity.DocumentStatusReadyToSign.String(): pending,
			entity.DocumentStatusSigning.String():     inProgress,
			entity.DocumentStatusCompleted.String():   completed,
			entity.DocumentStatusDeclined.String():    declined,
		},
	}, nil
}

// GetDocumentPDF returns the signed PDF for a completed document.
func (s *DocumentService) GetDocumentPDF(ctx context.Context, documentID string) ([]byte, string, error) {
	doc, err := s.documentRepo.FindByID(ctx, documentID)
	if err != nil {
		return nil, "", fmt.Errorf("finding document: %w", err)
	}
	if !s.storageEnabled {
		return nil, "", fmt.Errorf("signed PDF storage is disabled")
	}

	storageKey := completedPDFStorageKey(doc.CompletedPDFURL)
	var completedAttempt *entity.SigningAttempt
	if storageKey == "" && doc.ActiveAttemptID != nil && *doc.ActiveAttemptID != "" {
		attempt, err := s.attemptRepo.FindByID(ctx, *doc.ActiveAttemptID)
		if err != nil {
			return nil, "", fmt.Errorf("finding active completed attempt: %w", err)
		}
		completedAttempt = attempt
		storageKey = completedPDFStorageKey(stringValueFromJSON(attempt.ProviderUploadPayload, "completedPdfUrl"))
	}
	if storageKey == "" {
		if completedAttempt == nil {
			return nil, "", fmt.Errorf("signed PDF not available for this document")
		}
		result, err := s.downloadProviderCompletedPDF(ctx, completedAttempt)
		if err != nil {
			return nil, "", err
		}
		return result.PDF, providerCompletedPDFFilename(result.Filename, doc), nil
	}

	data, err := s.storageAdapter.Download(ctx, &port.StorageRequest{Key: storageKey})
	if err != nil {
		return nil, "", fmt.Errorf("downloading PDF from storage: %w", err)
	}

	return data, signedDocumentFilename(doc), nil
}

func (s *DocumentService) downloadProviderCompletedPDF(ctx context.Context, attempt *entity.SigningAttempt) (*port.DownloadCompletedPDFResult, error) {
	if s.signingProvider == nil || !s.signingProvider.ProviderCapabilities().CanDownloadCompletedPDF {
		return nil, fmt.Errorf("signed PDF not available for this document")
	}
	if attempt.ProviderDocumentID == nil || *attempt.ProviderDocumentID == "" {
		return nil, fmt.Errorf("signed PDF not available for this document")
	}
	result, err := s.signingProvider.DownloadCompletedPDF(ctx, &port.DownloadCompletedPDFRequest{
		ProviderDocumentID: *attempt.ProviderDocumentID,
		Environment:        entity.EnvironmentProd,
	})
	if err != nil {
		return nil, fmt.Errorf("downloading completed PDF from provider: %w", err)
	}
	if len(result.PDF) == 0 {
		return nil, fmt.Errorf("signed PDF not available for this document")
	}
	return result, nil
}

// ExpireDocuments finds and expires documents that have passed their expiration time.
func (s *DocumentService) ExpireDocuments(ctx context.Context, limit int) error {
	docs, err := s.documentRepo.FindExpired(ctx, limit)
	if err != nil {
		return fmt.Errorf("finding expired documents: %w", err)
	}

	for _, doc := range docs {
		s.expireSingleDocument(ctx, doc)
	}

	if len(docs) > 0 {
		slog.InfoContext(ctx, "expired documents processed", slog.Int("count", len(docs)))
	}

	return nil
}

// expireSingleDocument expires a single document and cancels it with the provider.
func (s *DocumentService) expireSingleDocument(ctx context.Context, doc *entity.Document) {
	oldStatus := string(doc.Status)

	if doc.ActiveAttemptID != nil && *doc.ActiveAttemptID != "" {
		attempt, err := s.attemptRepo.FindByID(ctx, *doc.ActiveAttemptID)
		if err != nil {
			slog.WarnContext(ctx, "failed to load expired document attempt",
				slog.String("document_id", doc.ID),
				slog.String("error", err.Error()),
			)
			return
		}
		if err := s.signingUOW.TerminateActiveAttempt(ctx, attempt, entity.SigningAttemptStatusInvalidated, "document expired", "ATTEMPT_EXPIRED"); err != nil {
			slog.WarnContext(ctx, "failed to invalidate expired attempt",
				slog.String("document_id", doc.ID),
				slog.String("attempt_id", attempt.ID),
				slog.String("error", err.Error()),
			)
			return
		}
	} else if err := doc.MarkAsExpired(); err != nil {
		slog.WarnContext(ctx, "failed to mark document as expired",
			slog.String("document_id", doc.ID),
			slog.String("error", err.Error()),
		)
		return
	}

	if err := s.documentRepo.Update(ctx, doc); err != nil {
		slog.WarnContext(ctx, "failed to update expired document",
			slog.String("document_id", doc.ID),
			slog.String("error", err.Error()),
		)
		return
	}

	s.eventEmitter.EmitDocumentEvent(ctx, doc.ID, entity.EventDocumentExpired, entity.ActorSystem, "", oldStatus, string(entity.DocumentStatusInvalidated), nil)

	slog.InfoContext(ctx, "document expired",
		slog.String("document_id", doc.ID),
		slog.String("previous_status", oldStatus),
	)
}

// CreateDocumentsBatch creates multiple documents in a single batch.
func (s *DocumentService) CreateDocumentsBatch(ctx context.Context, cmds []documentuc.CreateDocumentCommand) ([]documentuc.BatchDocumentResult, error) {
	results := make([]documentuc.BatchDocumentResult, len(cmds))

	for i, cmd := range cmds {
		doc, err := s.CreateAndSendDocument(ctx, cmd)
		results[i] = documentuc.BatchDocumentResult{
			Index:    i,
			Document: doc,
			Error:    err,
		}
	}

	return results, nil
}

// SendReminder sends reminder notifications to pending recipients of a document.
func (s *DocumentService) SendReminder(ctx context.Context, documentID string) error {
	return s.notificationSvc.SendReminder(ctx, documentID)
}

// parsePortableDocument is a helper to parse document content.
func parsePortableDocument(content json.RawMessage) (*portabledoc.Document, error) {
	return portabledoc.Parse(content)
}

// transitionToAwaitingInput marks the document as AWAITING_INPUT.
// Tokens are now generated on-demand via the email-verification flow (DocumentAccessService).
func (s *DocumentService) transitionToAwaitingInput(
	ctx context.Context,
	document *entity.Document,
) error {
	if err := document.MarkAsAwaitingInput(); err != nil {
		return fmt.Errorf("marking document as awaiting input: %w", err)
	}

	if err := s.documentRepo.Update(ctx, document); err != nil {
		return fmt.Errorf("updating document to awaiting input: %w", err)
	}

	s.eventEmitter.EmitDocumentEvent(ctx, document.ID, entity.EventDocumentCreated, entity.ActorUser, "", "", string(entity.DocumentStatusDraft), nil)

	slog.InfoContext(ctx, "document awaiting input",
		slog.String("document_id", document.ID),
	)

	return nil
}

// generateAccessToken creates a cryptographically random hex-encoded token (128 chars).
func generateAccessToken() (string, error) {
	b := make([]byte, 64)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating random bytes: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Verify DocumentService implements DocumentUseCase
var _ documentuc.DocumentUseCase = (*DocumentService)(nil)
