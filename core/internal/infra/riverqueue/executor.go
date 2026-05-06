package riverqueue

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

const maxReconciliationAttempts = 3

// SigningAttemptExecutor contains the attempt-aware worker implementation.
type SigningAttemptExecutor struct {
	pool              *pgxpool.Pool
	client            *river.Client[pgx.Tx]
	documentRepo      port.DocumentRepository
	recipientRepo     port.DocumentRecipientRepository
	attemptRepo       port.SigningAttemptRepository
	versionRepo       port.TemplateVersionRepository
	signerRoleRepo    port.TemplateVersionSignerRoleRepository
	fieldResponseRepo port.DocumentFieldResponseRepository
	pdfRenderer       port.PDFRenderer
	signingProvider   port.SigningProvider
	storageAdapter    port.StorageAdapter
	storageEnabled    bool
	completionHandler port.DocumentCompletedHandler
	failpoints        AttemptFailpoints
}

type SigningAttemptExecutorConfig struct {
	Pool              *pgxpool.Pool
	Client            *river.Client[pgx.Tx]
	DocumentRepo      port.DocumentRepository
	RecipientRepo     port.DocumentRecipientRepository
	AttemptRepo       port.SigningAttemptRepository
	VersionRepo       port.TemplateVersionRepository
	SignerRoleRepo    port.TemplateVersionSignerRoleRepository
	FieldResponseRepo port.DocumentFieldResponseRepository
	PDFRenderer       port.PDFRenderer
	SigningProvider   port.SigningProvider
	StorageAdapter    port.StorageAdapter
	StorageEnabled    bool
	CompletionHandler port.DocumentCompletedHandler
	Failpoints        AttemptFailpoints
}

func NewSigningAttemptExecutor(cfg SigningAttemptExecutorConfig) *SigningAttemptExecutor {
	return &SigningAttemptExecutor{
		pool:              cfg.Pool,
		client:            cfg.Client,
		documentRepo:      cfg.DocumentRepo,
		recipientRepo:     cfg.RecipientRepo,
		attemptRepo:       cfg.AttemptRepo,
		versionRepo:       cfg.VersionRepo,
		signerRoleRepo:    cfg.SignerRoleRepo,
		fieldResponseRepo: cfg.FieldResponseRepo,
		pdfRenderer:       cfg.PDFRenderer,
		signingProvider:   cfg.SigningProvider,
		storageAdapter:    cfg.StorageAdapter,
		storageEnabled:    cfg.StorageEnabled,
		completionHandler: cfg.CompletionHandler,
		failpoints:        cfg.Failpoints,
	}
}

//nolint:funlen
func (e *SigningAttemptExecutor) RenderAttemptPDF(ctx context.Context, attemptID string) error {
	attempt, doc, stale, err := e.loadActiveAttempt(ctx, attemptID, entity.SigningAttemptStatusCreated, entity.SigningAttemptStatusRendering)
	if err != nil || stale {
		return err
	}
	if attempt.IsTerminal() {
		return nil
	}
	if e.failpoints.Enabled(failpointRenderBefore) {
		return failpointErr(failpointRenderBefore)
	}

	old := attempt.Status
	now := time.Now().UTC()
	attempt.Status = entity.SigningAttemptStatusRendering
	attempt.RenderStartedAt = &now
	if err := e.transition(ctx, attempt, "ATTEMPT_RENDER_STARTED", nil, nil); err != nil {
		return err
	}

	renderResult, signerRoles, portableDoc, err := e.renderPDF(ctx, doc)
	if err != nil {
		return e.failPermanent(ctx, attempt, old, entity.ProviderSubmitPhaseBeforeRequest, err)
	}

	checksumBytes := sha256.Sum256(renderResult.PDF)
	checksum := hex.EncodeToString(checksumBytes[:])
	algo := "sha256"
	storagePath := fmt.Sprintf("documents/%s/%s/attempts/%s/pre-signed.pdf", doc.WorkspaceID, doc.ID, attempt.ID)
	if e.storageEnabled {
		if err := e.storageAdapter.Upload(ctx, &port.StorageUploadRequest{Key: storagePath, Data: renderResult.PDF, ContentType: "application/pdf", Environment: entity.EnvironmentProd}); err != nil {
			return err
		}
	}
	if e.failpoints.Enabled(failpointRenderAfterStoreBeforeCommit) {
		return failpointErr(failpointRenderAfterStoreBeforeCommit)
	}

	sigFields := mapSignatureFieldPositions(renderResult.SignatureFields, signerRoles, portableDoc.SignerRoles)
	if len(sigFields) == 0 {
		recipients, recErr := e.recipientRepo.FindByDocumentID(ctx, doc.ID)
		if recErr != nil {
			return recErr
		}
		sigFields = buildDefaultSignatureFieldPositions(recipients)
	}
	sigJSON, _ := json.Marshal(sigFields)
	payload := map[string]any{
		"title":           documentTitle(doc),
		"correlationKey":  correlationKey(doc.ID, attempt.ID),
		"pdfStoragePath":  storagePath,
		"pdfChecksum":     checksum,
		"signatureFields": sigFields,
	}
	payloadJSON, _ := json.Marshal(payload)

	attempt.Status = entity.SigningAttemptStatusReadyToSubmit
	attempt.PDFStoragePath = &storagePath
	attempt.PDFChecksum = &checksum
	attempt.PDFChecksumAlgorithm = &algo
	attempt.SignatureFieldSnapshot = sigJSON
	attempt.ProviderUploadPayload = payloadJSON
	providerName := e.signingProvider.ProviderName()
	attempt.ProviderName = &providerName
	corr := correlationKey(doc.ID, attempt.ID)
	attempt.ProviderCorrelationKey = &corr

	slog.InfoContext(ctx, "signing attempt PDF ready",
		slog.String("document_id", doc.ID), slog.String("attempt_id", attempt.ID), slog.String("pdf_storage_path", storagePath))
	phase := entity.ProviderSubmitPhaseCreateProviderDocument
	attempt.ProviderSubmitPhase = &phase
	return e.transition(ctx, attempt, "ATTEMPT_PDF_READY", ptrPhase(port.SigningJobPhaseAdvanceProviderSubmission), nil)
}

func (e *SigningAttemptExecutor) SubmitAttemptToProvider(ctx context.Context, attemptID string) error {
	return e.AdvanceProviderSubmission(ctx, attemptID)
}

//nolint:gocognit,gocyclo
func (e *SigningAttemptExecutor) AdvanceProviderSubmission(ctx context.Context, attemptID string) error {
	attempt, doc, stale, err := e.loadActiveAttempt(
		ctx,
		attemptID,
		entity.SigningAttemptStatusReadyToSubmit,
		entity.SigningAttemptStatusProviderRetryWaiting,
		entity.SigningAttemptStatusSubmittingProvider,
		entity.SigningAttemptStatusSubmissionUnknown,
		entity.SigningAttemptStatusReconcilingProvider,
	)
	if err != nil || stale {
		return err
	}
	if attempt.IsTerminal() {
		return nil
	}
	if e.signingProvider == nil {
		return fmt.Errorf("signing provider is not configured")
	}

	phase := providerSubmitPhase(attempt)
	if phase == "" {
		phase = entity.ProviderSubmitPhaseCreateProviderDocument
		attempt.ProviderSubmitPhase = &phase
	}

	switch phase {
	case entity.ProviderSubmitPhaseCreateProviderDocument:
		return e.advanceCreateProviderDocument(ctx, attempt, doc)
	case entity.ProviderSubmitPhaseAddRecipients:
		return e.advanceProviderRecipients(ctx, attempt)
	case entity.ProviderSubmitPhaseCreateFields:
		return e.advanceProviderFields(ctx, attempt)
	case entity.ProviderSubmitPhaseDistributeDocument:
		return e.advanceProviderDistribution(ctx, attempt)
	case entity.ProviderSubmitPhaseFetchSigningReferences:
		return e.advanceProviderSigningReferences(ctx, attempt)
	default:
		return e.failPermanent(ctx, attempt, attempt.Status, phase, fmt.Errorf("unknown provider submit phase %q", phase))
	}
}

func (e *SigningAttemptExecutor) advanceCreateProviderDocument(ctx context.Context, attempt *entity.SigningAttempt, doc *entity.Document) error {
	pdf, err := e.loadAttemptPDF(ctx, attempt)
	if err != nil {
		return err
	}
	if e.failpoints.Enabled(failpointSubmitBeforeProvider) {
		return failpointErr(failpointSubmitBeforeProvider)
	}
	result, err := e.signingProvider.EnsureProviderDocument(ctx, &port.EnsureProviderDocumentRequest{
		AttemptID:      attempt.ID,
		DocumentID:     doc.ID,
		CorrelationKey: deref(attempt.ProviderCorrelationKey),
		PDF:            pdf,
		PDFChecksum:    deref(attempt.PDFChecksum),
		Title:          documentTitle(doc),
		Environment:    entity.EnvironmentProd,
	})
	if err != nil {
		return e.handleProviderStepError(ctx, attempt, entity.ProviderSubmitPhaseCreateProviderDocument, err)
	}
	if e.failpoints.Enabled(failpointSubmitAfterEnvelopeBeforeRecipients) {
		return failpointErr(failpointSubmitAfterEnvelopeBeforeRecipients)
	}

	attempt.ProviderDocumentID = &result.ProviderDocumentID
	attempt.ProviderName = &result.ProviderName
	corr := result.CorrelationKey
	if corr == "" {
		corr = correlationKey(doc.ID, attempt.ID)
	}
	attempt.ProviderCorrelationKey = &corr
	attempt.Status = entity.SigningAttemptStatusSubmittingProvider
	next := entity.ProviderSubmitPhaseAddRecipients
	attempt.ProviderSubmitPhase = &next
	return e.transition(ctx, attempt, "ATTEMPT_PROVIDER_DOCUMENT_ENSURED", ptrPhase(port.SigningJobPhaseAdvanceProviderSubmission), nil)
}

func (e *SigningAttemptExecutor) advanceProviderRecipients(ctx context.Context, attempt *entity.SigningAttempt) error {
	if attempt.ProviderDocumentID == nil {
		return e.reconcileProviderPhase(ctx, attempt, "ATTEMPT_PROVIDER_DOCUMENT_MISSING_FOR_RECIPIENTS")
	}
	attemptRecipients, err := e.attemptRepo.FindRecipientsByAttemptID(ctx, attempt.ID)
	if err != nil {
		return err
	}
	result, err := e.signingProvider.EnsureProviderRecipients(ctx, &port.EnsureProviderRecipientsRequest{
		ProviderDocumentID: *attempt.ProviderDocumentID,
		CorrelationKey:     deref(attempt.ProviderCorrelationKey),
		Recipients:         signingRecipientsFromAttempts(attemptRecipients),
		Environment:        entity.EnvironmentProd,
	})
	if err != nil {
		return e.handleProviderStepError(ctx, attempt, entity.ProviderSubmitPhaseAddRecipients, err)
	}
	if e.failpoints.Enabled(failpointSubmitAfterRecipientsBeforeFields) {
		return failpointErr(failpointSubmitAfterRecipientsBeforeFields)
	}

	attempt.Status = entity.SigningAttemptStatusSubmittingProvider
	next := entity.ProviderSubmitPhaseCreateFields
	attempt.ProviderSubmitPhase = &next
	return e.transitionWithRecipientResults(
		ctx,
		attempt,
		result.Recipients,
		"ATTEMPT_PROVIDER_RECIPIENTS_ENSURED",
		ptrPhase(port.SigningJobPhaseAdvanceProviderSubmission),
		nil,
	)
}

func (e *SigningAttemptExecutor) advanceProviderFields(ctx context.Context, attempt *entity.SigningAttempt) error {
	if attempt.ProviderDocumentID == nil {
		return e.reconcileProviderPhase(ctx, attempt, "ATTEMPT_PROVIDER_DOCUMENT_MISSING_FOR_FIELDS")
	}
	attemptRecipients, err := e.attemptRepo.FindRecipientsByAttemptID(ctx, attempt.ID)
	if err != nil {
		return err
	}
	providerRecipients := recipientResultsFromAttempts(attemptRecipients)
	if len(providerRecipients) == 0 {
		result, ensureErr := e.signingProvider.EnsureProviderRecipients(ctx, &port.EnsureProviderRecipientsRequest{
			ProviderDocumentID: *attempt.ProviderDocumentID,
			CorrelationKey:     deref(attempt.ProviderCorrelationKey),
			Recipients:         signingRecipientsFromAttempts(attemptRecipients),
			Environment:        entity.EnvironmentProd,
		})
		if ensureErr != nil {
			return e.handleProviderStepError(ctx, attempt, entity.ProviderSubmitPhaseAddRecipients, ensureErr)
		}
		providerRecipients = result.Recipients
		if persistErr := e.persistProviderRecipients(ctx, attempt, providerRecipients); persistErr != nil {
			return persistErr
		}
	}
	sigFields, err := decodeSignatureFields(attempt.SignatureFieldSnapshot)
	if err != nil {
		return e.failPermanent(ctx, attempt, attempt.Status, entity.ProviderSubmitPhaseCreateFields, err)
	}
	_, err = e.signingProvider.EnsureProviderFields(ctx, &port.EnsureProviderFieldsRequest{
		ProviderDocumentID: *attempt.ProviderDocumentID,
		CorrelationKey:     deref(attempt.ProviderCorrelationKey),
		Recipients:         providerRecipients,
		SignatureFields:    sigFields,
		Environment:        entity.EnvironmentProd,
	})
	if err != nil {
		return e.handleProviderStepError(ctx, attempt, entity.ProviderSubmitPhaseCreateFields, err)
	}
	if e.failpoints.Enabled(failpointSubmitAfterFieldsBeforeDistribute) {
		return failpointErr(failpointSubmitAfterFieldsBeforeDistribute)
	}

	attempt.Status = entity.SigningAttemptStatusSubmittingProvider
	next := entity.ProviderSubmitPhaseDistributeDocument
	attempt.ProviderSubmitPhase = &next
	return e.transition(ctx, attempt, "ATTEMPT_PROVIDER_FIELDS_ENSURED", ptrPhase(port.SigningJobPhaseAdvanceProviderSubmission), nil)
}

func (e *SigningAttemptExecutor) advanceProviderDistribution(ctx context.Context, attempt *entity.SigningAttempt) error {
	if attempt.ProviderDocumentID == nil {
		return e.reconcileProviderPhase(ctx, attempt, "ATTEMPT_PROVIDER_DOCUMENT_MISSING_FOR_DISTRIBUTION")
	}
	_, err := e.signingProvider.EnsureProviderDistributed(ctx, &port.EnsureProviderDistributedRequest{
		ProviderDocumentID: *attempt.ProviderDocumentID,
		CorrelationKey:     deref(attempt.ProviderCorrelationKey),
		Environment:        entity.EnvironmentProd,
	})
	if err != nil {
		return e.handleProviderStepError(ctx, attempt, entity.ProviderSubmitPhaseDistributeDocument, err)
	}
	if e.failpoints.Enabled(failpointSubmitAfterDistributeBeforeRefs) {
		return failpointErr(failpointSubmitAfterDistributeBeforeRefs)
	}

	attempt.Status = entity.SigningAttemptStatusSubmittingProvider
	next := entity.ProviderSubmitPhaseFetchSigningReferences
	attempt.ProviderSubmitPhase = &next
	return e.transition(ctx, attempt, "ATTEMPT_PROVIDER_DISTRIBUTED", ptrPhase(port.SigningJobPhaseAdvanceProviderSubmission), nil)
}

func (e *SigningAttemptExecutor) advanceProviderSigningReferences(ctx context.Context, attempt *entity.SigningAttempt) error {
	if attempt.ProviderDocumentID == nil {
		return e.reconcileProviderPhase(ctx, attempt, "ATTEMPT_PROVIDER_DOCUMENT_MISSING_FOR_REFERENCES")
	}
	result, err := e.signingProvider.FetchProviderSigningReferences(ctx, &port.FetchProviderSigningReferencesRequest{
		ProviderDocumentID: *attempt.ProviderDocumentID,
		CorrelationKey:     deref(attempt.ProviderCorrelationKey),
		Environment:        entity.EnvironmentProd,
	})
	if err != nil {
		return e.handleProviderStepError(ctx, attempt, entity.ProviderSubmitPhaseFetchSigningReferences, err)
	}

	attempt.Status = result.Status
	if attempt.Status == "" {
		attempt.Status = entity.SigningAttemptStatusSigningReady
	}
	attempt.ProviderSubmitPhase = nil
	attempt.RetryCount = 0
	attempt.LastErrorClass = nil
	attempt.LastErrorMessage = nil
	return e.persistProviderSuccess(ctx, attempt, result.Recipients)
}

func (e *SigningAttemptExecutor) loadAttemptPDF(ctx context.Context, attempt *entity.SigningAttempt) ([]byte, error) {
	old := attempt.Status
	if attempt.PDFStoragePath == nil || attempt.PDFChecksum == nil {
		return nil, e.failPermanent(ctx, attempt, old, entity.ProviderSubmitPhaseBeforeRequest, fmt.Errorf("attempt PDF artifact is missing"))
	}
	pdf, err := e.storageAdapter.Download(ctx, &port.StorageRequest{Key: *attempt.PDFStoragePath, Environment: entity.EnvironmentProd})
	if err != nil {
		return nil, e.failPermanent(ctx, attempt, old, entity.ProviderSubmitPhaseBeforeRequest, fmt.Errorf("download attempt PDF: %w", err))
	}
	if e.failpoints.Enabled(failpointSubmitCorruptPDFChecksum) {
		pdf = append(append([]byte(nil), pdf...), byte('x'))
	}
	if !checksumMatches(pdf, *attempt.PDFChecksum) {
		return nil, e.failPermanent(ctx, attempt, old, entity.ProviderSubmitPhaseBeforeRequest, fmt.Errorf("attempt PDF checksum mismatch"))
	}
	return pdf, nil
}

func (e *SigningAttemptExecutor) handleProviderStepError(
	ctx context.Context,
	attempt *entity.SigningAttempt,
	phase entity.ProviderSubmitPhase,
	err error,
) error {
	var providerErr *port.ProviderError
	if errors.As(err, &providerErr) {
		if providerErr.Phase == "" {
			providerErr.Phase = phase
		}
		return e.handleProviderError(ctx, attempt, providerErr)
	}
	return err
}

func (e *SigningAttemptExecutor) ReconcileProviderSubmission(ctx context.Context, attemptID string) error {
	attempt, _, stale, err := e.loadActiveAttempt(ctx, attemptID, entity.SigningAttemptStatusSubmissionUnknown, entity.SigningAttemptStatusReconcilingProvider)
	if err != nil || stale {
		return err
	}
	if attempt.ProviderName == nil || attempt.ProviderCorrelationKey == nil {
		return e.failPermanent(ctx, attempt, attempt.Status, entity.ProviderSubmitPhaseBeforeRequest, fmt.Errorf("missing provider correlation data"))
	}
	caps := e.signingProvider.ProviderCapabilities()
	if !caps.CanFindByCorrelationKey {
		attempt.ReconciliationCount++
		if attempt.ReconciliationCount >= maxReconciliationAttempts {
			attempt.Status = entity.SigningAttemptStatusRequiresReview
			msg := "provider cannot reconcile by correlation key"
			attempt.LastErrorMessage = &msg
			return e.transition(ctx, attempt, "ATTEMPT_REQUIRES_REVIEW", nil, nil)
		}
		attempt.Status = entity.SigningAttemptStatusSubmissionUnknown
		return e.transition(ctx, attempt, "ATTEMPT_PROVIDER_RECONCILIATION_UNSUPPORTED", ptrPhase(port.SigningJobPhaseReconcileProvider), nil)
	}

	old := attempt.Status
	attempt.Status = entity.SigningAttemptStatusReconcilingProvider
	if err := e.transition(ctx, attempt, "ATTEMPT_PROVIDER_RECONCILIATION_STARTED", nil, nil); err != nil {
		return err
	}
	snapshot, err := e.signingProvider.InspectProviderSubmission(ctx, &port.FindProviderDocumentRequest{
		ProviderName:   *attempt.ProviderName,
		CorrelationKey: *attempt.ProviderCorrelationKey,
		Environment:    entity.EnvironmentProd,
	})
	if err != nil {
		return err
	}
	if snapshot == nil {
		snapshot = &port.ProviderSubmissionSnapshot{
			ProviderName:   *attempt.ProviderName,
			CorrelationKey: *attempt.ProviderCorrelationKey,
		}
	}

	nextPhase := nextPhaseFromProviderSnapshot(snapshot)
	if snapshot.HasDocument && snapshot.ProviderDocumentID != "" {
		attempt.ProviderDocumentID = &snapshot.ProviderDocumentID
	}
	if snapshot.ProviderName != "" {
		attempt.ProviderName = &snapshot.ProviderName
	}
	if snapshot.CorrelationKey != "" {
		attempt.ProviderCorrelationKey = &snapshot.CorrelationKey
	}
	attempt.Status = entity.SigningAttemptStatusSubmissionUnknown
	attempt.ProviderSubmitPhase = &nextPhase
	if !snapshot.HasDocument {
		attempt.Status = entity.SigningAttemptStatusReadyToSubmit
	}
	return e.transition(ctx, attempt, "ATTEMPT_PROVIDER_RECONCILED", ptrPhase(port.SigningJobPhaseAdvanceProviderSubmission), &old)
}

func (e *SigningAttemptExecutor) RefreshAttemptProviderStatus(ctx context.Context, attemptID string) error {
	attempt, _, stale, err := e.loadActiveAttempt(ctx, attemptID, entity.SigningAttemptStatusSigningReady, entity.SigningAttemptStatusSigning)
	if err != nil || stale || attempt.ProviderDocumentID == nil {
		return err
	}
	status, err := e.signingProvider.GetProviderDocumentStatus(ctx, &port.GetProviderDocumentStatusRequest{ProviderDocumentID: *attempt.ProviderDocumentID, Environment: entity.EnvironmentProd})
	if err != nil {
		return err
	}
	attempt.Status = status.Status
	if status.CompletedPDFURL != nil {
		attempt.ProviderUploadPayload = mergeJSON(attempt.ProviderUploadPayload, "completedPdfUrl", *status.CompletedPDFURL)
	}
	phase := (*port.SigningJobPhase)(nil)
	if attempt.Status == entity.SigningAttemptStatusCompleted {
		phase = ptrPhase(port.SigningJobPhaseDispatchCompletion)
	}
	return e.transition(ctx, attempt, "ATTEMPT_PROVIDER_STATUS_REFRESHED", phase, nil)
}

func (e *SigningAttemptExecutor) CleanupProviderAttempt(ctx context.Context, attemptID string) error {
	attempt, err := e.attemptRepo.FindByID(ctx, attemptID)
	if err != nil || attempt.ProviderDocumentID == nil {
		return err
	}
	if e.failpoints.Enabled(failpointCleanupFail) {
		status := "FAILED_RETRYABLE"
		msg := failpointErr(failpointCleanupFail).Error()
		attempt.CleanupStatus = &status
		attempt.CleanupError = &msg
		return e.persistProviderCleanup(ctx, attempt, "ATTEMPT_PROVIDER_CLEANUP_FINISHED")
	}
	caps := e.signingProvider.ProviderCapabilities()
	if !caps.CanCancel {
		status := "UNSUPPORTED"
		attempt.CleanupStatus = &status
		return e.persistProviderCleanup(ctx, attempt, "ATTEMPT_PROVIDER_CLEANUP_FINISHED")
	}
	result, err := e.signingProvider.CleanupProviderDocument(ctx, &port.CleanupProviderDocumentRequest{ProviderDocumentID: *attempt.ProviderDocumentID, Environment: entity.EnvironmentProd})
	status := "SUCCEEDED"
	if err != nil {
		status = "FAILED_RETRYABLE"
		msg := err.Error()
		attempt.CleanupError = &msg
	} else {
		attempt.CleanupAction = &result.Action
	}
	attempt.CleanupStatus = &status
	return e.persistProviderCleanup(ctx, attempt, "ATTEMPT_PROVIDER_CLEANUP_FINISHED")
}

func (e *SigningAttemptExecutor) persistProviderCleanup(ctx context.Context, attempt *entity.SigningAttempt, eventType string) error {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	current, err := e.lockTransitionAttemptTx(ctx, tx, attempt.ID)
	if err != nil {
		return err
	}
	old := current.Status
	current.CleanupStatus = attempt.CleanupStatus
	current.CleanupAction = attempt.CleanupAction
	current.CleanupError = attempt.CleanupError
	if err := e.attemptRepo.UpdateTx(ctx, tx, current); err != nil {
		return err
	}
	if err := e.insertTransitionEventTx(ctx, tx, current, eventType, old); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (e *SigningAttemptExecutor) DispatchAttemptCompletion(ctx context.Context, attemptID string) error {
	attempt, _, stale, err := e.loadActiveAttempt(ctx, attemptID, entity.SigningAttemptStatusCompleted)
	if err != nil || stale {
		return err
	}
	if e.completionHandler == nil {
		return nil
	}
	event, err := buildCompletedEvent(ctx, e.pool, attempt.DocumentID, attempt.ID)
	if err != nil {
		return err
	}
	return e.completionHandler(ctx, event)
}

func (e *SigningAttemptExecutor) loadActiveAttempt(ctx context.Context, attemptID string, allowed ...entity.SigningAttemptStatus) (*entity.SigningAttempt, *entity.Document, bool, error) {
	attempt, err := e.attemptRepo.FindByID(ctx, attemptID)
	if err != nil {
		return nil, nil, false, err
	}
	doc, err := e.documentRepo.FindByID(ctx, attempt.DocumentID)
	if err != nil {
		return nil, nil, false, err
	}
	if doc.ActiveAttemptID == nil || *doc.ActiveAttemptID != attempt.ID {
		return attempt, doc, true, nil
	}
	if len(allowed) > 0 {
		ok := false
		for _, s := range allowed {
			if attempt.Status == s {
				ok = true
				break
			}
		}
		if !ok {
			return attempt, doc, true, nil
		}
	}
	return attempt, doc, false, nil
}

func (e *SigningAttemptExecutor) renderPDF(ctx context.Context, doc *entity.Document) (*port.RenderPreviewResult, []*entity.TemplateVersionSignerRole, *portabledoc.Document, error) {
	version, err := e.versionRepo.FindByID(ctx, doc.TemplateVersionID)
	if err != nil {
		return nil, nil, nil, err
	}
	portableDoc, err := portabledoc.Parse(version.ContentStructure)
	if err != nil {
		return nil, nil, nil, err
	}
	recipients, err := e.recipientRepo.FindByDocumentID(ctx, doc.ID)
	if err != nil {
		return nil, nil, nil, err
	}
	signerRoles, err := e.signerRoleRepo.FindByVersionID(ctx, doc.TemplateVersionID)
	if err != nil {
		return nil, nil, nil, err
	}
	var injectables map[string]any
	if doc.InjectedValuesSnapshot != nil {
		_ = json.Unmarshal(doc.InjectedValuesSnapshot, &injectables)
	}
	renderResult, err := e.pdfRenderer.RenderPreview(ctx, &port.RenderPreviewRequest{
		Document:         portableDoc,
		Injectables:      injectables,
		SignerRoleValues: buildSignerRoleValues(recipients, signerRoles, portableDoc.SignerRoles),
		FieldResponses:   loadFieldResponseMap(ctx, e.fieldResponseRepo, doc.ID),
	})
	return renderResult, signerRoles, portableDoc, err
}

func (e *SigningAttemptExecutor) handleProviderError(ctx context.Context, attempt *entity.SigningAttempt, providerErr *port.ProviderError) error {
	attempt.ProviderSubmitPhase = &providerErr.Phase
	attempt.LastErrorClass = &providerErr.Class
	msg := providerErr.Error()
	attempt.LastErrorMessage = &msg
	if providerErr.ProviderDocumentID != nil {
		attempt.ProviderDocumentID = providerErr.ProviderDocumentID
	}
	attempt.Status = entity.AttemptStatusForProviderError(providerErr.Class)
	attempt.RetryCount++
	switch providerErr.Class {
	case entity.ProviderErrorClassTransient:
		if err := e.transition(ctx, attempt, "ATTEMPT_PROVIDER_SUBMIT_RETRY_WAITING", nil, nil); err != nil {
			return err
		}
		return providerErr
	case entity.ProviderErrorClassAmbiguous:
		return e.transition(ctx, attempt, "ATTEMPT_PROVIDER_SUBMISSION_UNKNOWN", ptrPhase(port.SigningJobPhaseReconcileProvider), nil)
	case entity.ProviderErrorClassConflictStale:
		return e.transition(ctx, attempt, "ATTEMPT_REQUIRES_REVIEW", nil, nil)
	default:
		return e.transition(ctx, attempt, "ATTEMPT_FAILED_PERMANENT", nil, nil)
	}
}

func (e *SigningAttemptExecutor) failPermanent(ctx context.Context, attempt *entity.SigningAttempt, old entity.SigningAttemptStatus, phase entity.ProviderSubmitPhase, err error) error {
	class := entity.ProviderErrorClassPermanent
	attempt.Status = entity.SigningAttemptStatusFailedPermanent
	attempt.ProviderSubmitPhase = &phase
	attempt.LastErrorClass = &class
	msg := err.Error()
	attempt.LastErrorMessage = &msg
	now := time.Now().UTC()
	attempt.TerminalAt = &now
	return e.transition(ctx, attempt, "ATTEMPT_FAILED_PERMANENT", nil, &old)
}

func (e *SigningAttemptExecutor) transition(ctx context.Context, attempt *entity.SigningAttempt, eventType string, phase *port.SigningJobPhase, forcedOld *entity.SigningAttemptStatus) error {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	current, err := scanAttemptRow(tx.QueryRow(ctx, `
		SELECT id, document_id, sequence, status, render_started_at, pdf_storage_path, pdf_checksum,
		       pdf_checksum_algorithm, render_metadata, signature_field_snapshot, provider_upload_payload,
		       provider_name, provider_correlation_key, provider_document_id, provider_submit_phase,
		       retry_count, next_retry_at, last_error_class, last_error_message,
		       reconciliation_count, next_reconciliation_at, cleanup_status, cleanup_action, cleanup_error,
		       processing_lease_owner, processing_lease_expires_at, invalidation_reason,
		       created_at, updated_at, terminal_at
		FROM execution.signing_attempts WHERE id = $1 FOR UPDATE`, attempt.ID))
	if err != nil {
		return err
	}
	active, err := e.transitionStillOwnsAttemptTx(ctx, tx, current)
	if err != nil {
		return err
	}
	if !active {
		return nil
	}

	old := current.Status
	if forcedOld != nil {
		old = *forcedOld
	}
	if err := e.attemptRepo.UpdateTx(ctx, tx, attempt); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE execution.documents SET status=$3, updated_at=now() WHERE id=$1 AND active_attempt_id=$2`, attempt.DocumentID, attempt.ID, entity.ProjectDocumentStatusFromAttempt(attempt.Status)); err != nil {
		return err
	}
	newStatus := attempt.Status
	if err := e.attemptRepo.InsertEventTx(ctx, tx, &entity.SigningAttemptEvent{AttemptID: attempt.ID, DocumentID: attempt.DocumentID, EventType: eventType, OldStatus: &old, NewStatus: &newStatus, ProviderName: attempt.ProviderName, ProviderDocumentID: attempt.ProviderDocumentID, CorrelationKey: attempt.ProviderCorrelationKey, ErrorClass: attempt.LastErrorClass}); err != nil {
		return err
	}
	if phase != nil {
		if err := insertPhaseTx(ctx, e.client, tx, *phase, attempt); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

//nolint:gocognit
func (e *SigningAttemptExecutor) persistProviderSuccess(ctx context.Context, attempt *entity.SigningAttempt, results []port.RecipientResult) error {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	current, err := e.lockTransitionAttemptTx(ctx, tx, attempt.ID)
	if err != nil {
		return err
	}
	active, err := e.transitionStillOwnsAttemptTx(ctx, tx, current)
	if err != nil {
		return err
	}
	if !active {
		return nil
	}

	if err := e.updateAttemptRecipientsTx(ctx, tx, attempt.ID, results, true); err != nil {
		return err
	}
	old := current.Status
	if err := e.attemptRepo.UpdateTx(ctx, tx, attempt); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE execution.documents SET status=$3, updated_at=now() WHERE id=$1 AND active_attempt_id=$2`, attempt.DocumentID, attempt.ID, entity.ProjectDocumentStatusFromAttempt(attempt.Status)); err != nil {
		return err
	}
	newStatus := attempt.Status
	if err := e.attemptRepo.InsertEventTx(ctx, tx, &entity.SigningAttemptEvent{AttemptID: attempt.ID, DocumentID: attempt.DocumentID, EventType: "ATTEMPT_SIGNING_READY", OldStatus: &old, NewStatus: &newStatus, ProviderName: attempt.ProviderName, ProviderDocumentID: attempt.ProviderDocumentID, CorrelationKey: attempt.ProviderCorrelationKey}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (e *SigningAttemptExecutor) reconcileProviderPhase(ctx context.Context, attempt *entity.SigningAttempt, eventType string) error {
	if attempt.ProviderName == nil || attempt.ProviderCorrelationKey == nil {
		return e.failPermanent(ctx, attempt, attempt.Status, entity.ProviderSubmitPhaseBeforeRequest, fmt.Errorf("missing provider correlation data"))
	}
	snapshot, err := e.signingProvider.InspectProviderSubmission(ctx, &port.FindProviderDocumentRequest{
		ProviderName:   deref(attempt.ProviderName),
		CorrelationKey: deref(attempt.ProviderCorrelationKey),
		Environment:    entity.EnvironmentProd,
	})
	if err != nil {
		return err
	}
	if snapshot == nil {
		snapshot = &port.ProviderSubmissionSnapshot{
			ProviderName:   deref(attempt.ProviderName),
			CorrelationKey: deref(attempt.ProviderCorrelationKey),
		}
	}
	phase := nextPhaseFromProviderSnapshot(snapshot)
	attempt.ProviderSubmitPhase = &phase
	if snapshot.ProviderDocumentID != "" {
		attempt.ProviderDocumentID = &snapshot.ProviderDocumentID
	}
	if snapshot.ProviderName != "" {
		attempt.ProviderName = &snapshot.ProviderName
	}
	attempt.Status = entity.SigningAttemptStatusSubmissionUnknown
	return e.transition(ctx, attempt, eventType, ptrPhase(port.SigningJobPhaseAdvanceProviderSubmission), nil)
}

func (e *SigningAttemptExecutor) persistProviderRecipients(ctx context.Context, attempt *entity.SigningAttempt, results []port.RecipientResult) error {
	return e.transitionWithRecipientResults(ctx, attempt, results, "ATTEMPT_PROVIDER_RECIPIENTS_REFERENCES_SAVED", nil, nil)
}

//nolint:gocognit
func (e *SigningAttemptExecutor) transitionWithRecipientResults(
	ctx context.Context,
	attempt *entity.SigningAttempt,
	results []port.RecipientResult,
	eventType string,
	phase *port.SigningJobPhase,
	forcedOld *entity.SigningAttemptStatus,
) error {
	tx, err := e.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	current, err := e.lockTransitionAttemptTx(ctx, tx, attempt.ID)
	if err != nil {
		return err
	}
	active, err := e.transitionStillOwnsAttemptTx(ctx, tx, current)
	if err != nil {
		return err
	}
	if !active {
		return nil
	}

	old := current.Status
	if forcedOld != nil {
		old = *forcedOld
	}
	if err := e.updateAttemptRecipientsTx(ctx, tx, attempt.ID, results, false); err != nil {
		return err
	}
	if err := e.attemptRepo.UpdateTx(ctx, tx, attempt); err != nil {
		return err
	}
	if _, err := tx.Exec(
		ctx,
		`UPDATE execution.documents SET status=$3, updated_at=now() WHERE id=$1 AND active_attempt_id=$2`,
		attempt.DocumentID,
		attempt.ID,
		entity.ProjectDocumentStatusFromAttempt(attempt.Status),
	); err != nil {
		return err
	}
	if err := e.insertTransitionEventTx(ctx, tx, attempt, eventType, old); err != nil {
		return err
	}
	if phase != nil {
		if err := insertPhaseTx(ctx, e.client, tx, *phase, attempt); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (e *SigningAttemptExecutor) lockTransitionAttemptTx(ctx context.Context, tx pgx.Tx, attemptID string) (*entity.SigningAttempt, error) {
	return scanAttemptRow(tx.QueryRow(ctx, `
		SELECT id, document_id, sequence, status, render_started_at, pdf_storage_path, pdf_checksum,
		       pdf_checksum_algorithm, render_metadata, signature_field_snapshot, provider_upload_payload,
		       provider_name, provider_correlation_key, provider_document_id, provider_submit_phase,
		       retry_count, next_retry_at, last_error_class, last_error_message,
		       reconciliation_count, next_reconciliation_at, cleanup_status, cleanup_action, cleanup_error,
		       processing_lease_owner, processing_lease_expires_at, invalidation_reason,
		       created_at, updated_at, terminal_at
		FROM execution.signing_attempts WHERE id = $1 FOR UPDATE`, attemptID))
}

func (e *SigningAttemptExecutor) transitionStillOwnsAttemptTx(ctx context.Context, tx pgx.Tx, current *entity.SigningAttempt) (bool, error) {
	if current.Status.IsTerminal() {
		return false, nil
	}
	var activeAttemptID sql.NullString
	if err := tx.QueryRow(ctx, `SELECT active_attempt_id FROM execution.documents WHERE id=$1`, current.DocumentID).Scan(&activeAttemptID); err != nil {
		return false, err
	}
	return activeAttemptID.Valid && activeAttemptID.String == current.ID, nil
}

func (e *SigningAttemptExecutor) updateAttemptRecipientsTx(
	ctx context.Context,
	tx pgx.Tx,
	attemptID string,
	results []port.RecipientResult,
	defaultSent bool,
) error {
	attemptRecipients, err := e.attemptRepo.FindRecipientsByAttemptID(ctx, attemptID)
	if err != nil {
		return err
	}
	byRole := recipientResultsByRole(results)
	for _, rec := range attemptRecipients {
		result, ok := byRole[rec.TemplateVersionRoleID]
		if !ok {
			continue
		}
		applyProviderRecipientResult(rec, result, defaultSent)
		if err := e.attemptRepo.UpdateRecipientTx(ctx, tx, rec); err != nil {
			return err
		}
	}
	return nil
}

func (e *SigningAttemptExecutor) insertTransitionEventTx(
	ctx context.Context,
	tx pgx.Tx,
	attempt *entity.SigningAttempt,
	eventType string,
	old entity.SigningAttemptStatus,
) error {
	if eventType == "" {
		return nil
	}
	newStatus := attempt.Status
	return e.attemptRepo.InsertEventTx(ctx, tx, &entity.SigningAttemptEvent{
		AttemptID:          attempt.ID,
		DocumentID:         attempt.DocumentID,
		EventType:          eventType,
		OldStatus:          &old,
		NewStatus:          &newStatus,
		ProviderName:       attempt.ProviderName,
		ProviderDocumentID: attempt.ProviderDocumentID,
		CorrelationKey:     attempt.ProviderCorrelationKey,
		ErrorClass:         attempt.LastErrorClass,
	})
}

func recipientResultsByRole(results []port.RecipientResult) map[string]port.RecipientResult {
	byRole := make(map[string]port.RecipientResult, len(results))
	for _, result := range results {
		byRole[result.RoleID] = result
	}
	return byRole
}

func applyProviderRecipientResult(rec *entity.SigningAttemptRecipient, result port.RecipientResult, defaultSent bool) {
	rec.ProviderRecipientID = stringPtr(result.ProviderRecipientID)
	rec.ProviderSigningToken = stringPtr(result.ProviderSigningToken)
	rec.SigningURL = stringPtr(result.SigningURL)
	if result.Status != "" {
		rec.Status = result.Status.Normalize()
		return
	}
	if defaultSent {
		rec.Status = entity.RecipientStatusSent
	}
}

func providerSubmitPhase(attempt *entity.SigningAttempt) entity.ProviderSubmitPhase {
	if attempt == nil || attempt.ProviderSubmitPhase == nil {
		return ""
	}
	return *attempt.ProviderSubmitPhase
}

func providerSubmitPhaseArg(phase *entity.ProviderSubmitPhase) string {
	if phase == nil {
		return ""
	}
	return string(*phase)
}

func nextPhaseFromProviderSnapshot(snapshot *port.ProviderSubmissionSnapshot) entity.ProviderSubmitPhase {
	if snapshot == nil || !snapshot.HasDocument {
		return entity.ProviderSubmitPhaseCreateProviderDocument
	}
	if !snapshot.HasRecipients {
		return entity.ProviderSubmitPhaseAddRecipients
	}
	if !snapshot.HasFields {
		return entity.ProviderSubmitPhaseCreateFields
	}
	if !snapshot.IsDistributed {
		return entity.ProviderSubmitPhaseDistributeDocument
	}
	return entity.ProviderSubmitPhaseFetchSigningReferences
}

func recipientResultsFromAttempts(recipients []*entity.SigningAttemptRecipient) []port.RecipientResult {
	out := make([]port.RecipientResult, 0, len(recipients))
	for _, r := range recipients {
		if r.ProviderRecipientID == nil || *r.ProviderRecipientID == "" {
			return nil
		}
		result := port.RecipientResult{
			RoleID:              r.TemplateVersionRoleID,
			ProviderRecipientID: *r.ProviderRecipientID,
			Status:              r.Status,
		}
		if r.ProviderSigningToken != nil {
			result.ProviderSigningToken = *r.ProviderSigningToken
		}
		if r.SigningURL != nil {
			result.SigningURL = *r.SigningURL
		}
		out = append(out, result)
	}
	return out
}

func insertPhaseTx(ctx context.Context, client *river.Client[pgx.Tx], tx pgx.Tx, phase port.SigningJobPhase, attempt *entity.SigningAttempt) error {
	switch phase {
	case port.SigningJobPhaseRenderAttemptPDF:
		_, err := client.InsertTx(ctx, tx, RenderAttemptPDFArgs{AttemptID: attempt.ID}, nil)
		return err
	case port.SigningJobPhaseSubmitAttemptToProvider, port.SigningJobPhaseAdvanceProviderSubmission:
		_, err := client.InsertTx(ctx, tx, AdvanceProviderSubmissionArgs{
			AttemptID:           attempt.ID,
			ProviderSubmitPhase: providerSubmitPhaseArg(attempt.ProviderSubmitPhase),
		}, nil)
		return err
	case port.SigningJobPhaseReconcileProvider:
		_, err := client.InsertTx(ctx, tx, ReconcileProviderSubmissionArgs{AttemptID: attempt.ID}, nil)
		return err
	case port.SigningJobPhaseRefreshProviderStatus:
		_, err := client.InsertTx(ctx, tx, RefreshAttemptProviderStatusArgs{AttemptID: attempt.ID}, nil)
		return err
	case port.SigningJobPhaseCleanupProviderAttempt:
		_, err := client.InsertTx(ctx, tx, CleanupProviderAttemptArgs{AttemptID: attempt.ID}, nil)
		return err
	case port.SigningJobPhaseDispatchCompletion:
		_, err := client.InsertTx(ctx, tx, DispatchAttemptCompletionArgs{AttemptID: attempt.ID}, nil)
		return err
	default:
		return fmt.Errorf("unknown signing job phase %q", phase)
	}
}

func checksumMatches(pdf []byte, expected string) bool {
	sum := sha256.Sum256(pdf)
	return bytes.Equal([]byte(hex.EncodeToString(sum[:])), []byte(expected))
}
func correlationKey(documentID, attemptID string) string { return documentID + ":" + attemptID }
func deref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func stringPtr(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
func ptrPhase(p port.SigningJobPhase) *port.SigningJobPhase { return &p }

func decodeSignatureFields(raw json.RawMessage) ([]port.SignatureFieldPosition, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("signature field snapshot missing")
	}
	var fields []port.SignatureFieldPosition
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	return fields, nil
}

func signingRecipientsFromAttempts(recipients []*entity.SigningAttemptRecipient) []port.SigningRecipient {
	out := make([]port.SigningRecipient, len(recipients))
	for i, r := range recipients {
		out[i] = port.SigningRecipient{Email: r.Email, Name: r.Name, RoleID: r.TemplateVersionRoleID, SignerOrder: r.SignerOrder}
	}
	return out
}

func mergeJSON(raw json.RawMessage, key string, value any) json.RawMessage {
	m := map[string]any{}
	_ = json.Unmarshal(raw, &m)
	m[key] = value
	out, _ := json.Marshal(m)
	return out
}
