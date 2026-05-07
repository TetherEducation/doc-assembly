package document

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
)

// InternalDocumentService implements usecase.InternalDocumentUseCase.
type InternalDocumentService struct {
	generator       *DocumentGenerator
	documentRepo    port.DocumentRepository
	tenantRepo      port.TenantRepository
	workspaceRepo   port.WorkspaceRepository
	docTypeRepo     port.DocumentTypeRepository
	templateRepo    port.TemplateRepository
	versionRepo     port.TemplateVersionRepository
	customResolver  port.TemplateResolver
	defaultResolver port.TemplateResolver
	searchAdapter   port.InternalTemplateContextSearchAdapter
}

// NewInternalDocumentService creates a new InternalDocumentService.
func NewInternalDocumentService(
	generator *DocumentGenerator,
	documentRepo port.DocumentRepository,
	tenantRepo port.TenantRepository,
	workspaceRepo port.WorkspaceRepository,
	docTypeRepo port.DocumentTypeRepository,
	templateRepo port.TemplateRepository,
	versionRepo port.TemplateVersionRepository,
	customResolver port.TemplateResolver,
) documentuc.InternalDocumentUseCase {
	return &InternalDocumentService{
		generator:       generator,
		documentRepo:    documentRepo,
		tenantRepo:      tenantRepo,
		workspaceRepo:   workspaceRepo,
		docTypeRepo:     docTypeRepo,
		templateRepo:    templateRepo,
		versionRepo:     versionRepo,
		customResolver:  customResolver,
		defaultResolver: NewDefaultTemplateResolver(),
		searchAdapter: NewTemplateContextSearchAdapter(
			tenantRepo,
			workspaceRepo,
			docTypeRepo,
			templateRepo,
			versionRepo,
		),
	}
}

// CreateDocument creates or replays a document using the extension system.
//
//nolint:funlen // Service orchestration is intentionally linear for traceability.
func (s *InternalDocumentService) CreateDocument(
	ctx context.Context,
	cmd documentuc.InternalCreateCommand,
) (*documentuc.InternalCreateResult, error) {
	slog.InfoContext(ctx, "creating document via internal API",
		"tenant_code", cmd.TenantCode,
		"workspace_code", cmd.WorkspaceCode,
		"document_type", cmd.DocumentType,
		"external_id", cmd.ExternalID,
		"transactional_id", cmd.TransactionalID,
		"environment", cmd.Environment,
	)

	resolved, err := s.resolveTemplateContext(ctx, cmd)
	if err != nil {
		return nil, err
	}

	if !cmd.ForceCreate {
		replay, ok, err := s.findInternalCreateReplay(ctx, cmd, resolved)
		if err != nil {
			return nil, err
		}
		if ok {
			return replay, nil
		}
	}

	mapCtx := &port.MapperContext{
		ExternalID:        cmd.ExternalID,
		TemplateID:        resolved.template.ID,
		TemplateVersionID: resolved.version.ID,
		DocumentTypeID:    resolved.documentType.ID,
		TenantCode:        resolved.tenant.Code,
		WorkspaceCode:     resolved.workspace.Code,
		DocumentTypeCode:  cmd.DocumentType,
		TransactionalID:   cmd.TransactionalID,
		Operation:         entity.OperationCreate,
		Environment:       cmd.Environment,
		Headers:           cmd.Headers,
		RawBody:           cmd.PayloadRaw,
	}
	slog.InfoContext(ctx, "internal create mapper context prepared",
		"template_id", mapCtx.TemplateID,
		"version_id", mapCtx.TemplateVersionID,
		"document_type_id", mapCtx.DocumentTypeID,
		"tenant_code", mapCtx.TenantCode,
		"workspace_code", mapCtx.WorkspaceCode,
		"document_type_code", mapCtx.DocumentTypeCode,
		"external_id", mapCtx.ExternalID,
		"transactional_id", mapCtx.TransactionalID,
		"payload_bytes", len(mapCtx.RawBody),
	)

	prepared, err := s.generator.prepareDocumentWithResolvedTemplate(
		ctx,
		mapCtx,
		resolved.workspace.ID,
		resolved.version,
		resolved.dbInjectables,
		resolved.activeSystemKeys,
	)
	if err != nil {
		return nil, fmt.Errorf("preparing internal document: %w", err)
	}

	doc, err := s.buildInternalDocument(cmd, resolved, prepared)
	if err != nil {
		return nil, err
	}

	txResult, err := s.documentRepo.InternalCreateOrReplay(ctx, &port.InternalCreateRequest{
		WorkspaceID:     resolved.workspace.ID,
		DocumentTypeID:  resolved.documentType.ID,
		ExternalID:      cmd.ExternalID,
		TransactionalID: cmd.TransactionalID,
		ForceCreate:     cmd.ForceCreate,
		SupersedeReason: cmd.SupersedeReason,
		Document:        doc,
		Recipients:      prepared.Recipients,
	})
	if err != nil {
		return nil, fmt.Errorf("persisting internal document: %w", err)
	}

	stored := txResult.Document
	if stored == nil {
		var err error
		stored, err = s.documentRepo.FindByIDWithRecipients(ctx, txResult.DocumentID)
		if err != nil {
			return nil, fmt.Errorf("loading internal document after persistence: %w", err)
		}
	}

	result := &documentuc.InternalCreateResult{
		Document:                     stored,
		IdempotentReplay:             txResult.IdempotentReplay,
		SupersededPreviousDocumentID: txResult.SupersededPreviousDocumentID,
	}

	return result, nil
}

// ResetUnsignedDocument recreates a logical document with force-create semantics after ensuring the
// current active document, when present, has not completed signing.
func (s *InternalDocumentService) ResetUnsignedDocument(
	ctx context.Context,
	cmd documentuc.InternalCreateCommand,
) (*documentuc.InternalCreateResult, error) {
	resolved, err := s.resolveTemplateContext(ctx, cmd)
	if err != nil {
		return nil, err
	}
	if err := s.ensureActiveDocumentCanReset(ctx, cmd, resolved); err != nil {
		return nil, err
	}

	cmd.ForceCreate = true
	if cmd.SupersedeReason == nil {
		reason := "internal reset"
		cmd.SupersedeReason = &reason
	}
	return s.CreateDocument(ctx, cmd)
}

func (s *InternalDocumentService) ensureActiveDocumentCanReset(
	ctx context.Context,
	cmd documentuc.InternalCreateCommand,
	resolved *internalResolvedContext,
) error {
	activeDocID, ok, err := s.documentRepo.FindActiveByLogicalKey(
		ctx,
		resolved.workspace.ID,
		resolved.documentType.ID,
		cmd.ExternalID,
	)
	if err != nil {
		return fmt.Errorf("checking active internal document before reset: %w", err)
	}
	if !ok {
		return nil
	}

	doc, err := s.documentRepo.FindByID(ctx, activeDocID)
	if err != nil {
		return fmt.Errorf("loading active internal document before reset: %w", err)
	}
	if doc.Status == entity.DocumentStatusCompleted {
		return entity.ErrInvalidDocumentState
	}
	return nil
}

func (s *InternalDocumentService) findInternalCreateReplay(
	ctx context.Context,
	cmd documentuc.InternalCreateCommand,
	resolved *internalResolvedContext,
) (*documentuc.InternalCreateResult, bool, error) {
	docID, ok, err := s.documentRepo.FindInternalCreateReplay(
		ctx,
		resolved.workspace.ID,
		resolved.documentType.ID,
		cmd.ExternalID,
		cmd.TransactionalID,
	)
	if err != nil {
		return nil, false, fmt.Errorf("checking internal document replay: %w", err)
	}
	if !ok {
		return nil, false, nil
	}

	stored, err := s.documentRepo.FindByIDWithRecipients(ctx, docID)
	if err != nil {
		return nil, false, fmt.Errorf("loading internal document replay: %w", err)
	}

	slog.InfoContext(ctx, "internal create replay found before generation",
		"document_id", docID,
		"external_id", cmd.ExternalID,
		"transactional_id", cmd.TransactionalID,
	)
	return &documentuc.InternalCreateResult{
		Document:         stored,
		IdempotentReplay: true,
	}, true, nil
}

type internalResolvedContext struct {
	tenant           *entity.Tenant
	workspace        *entity.Workspace
	documentType     *entity.DocumentType
	template         *entity.Template
	version          *entity.TemplateVersionWithDetails
	dbInjectables    []*entity.InjectableDefinition
	activeSystemKeys []string
}

//nolint:funlen,gocognit,gocyclo // Resolution has explicit fallback and validation stages.
func (s *InternalDocumentService) resolveTemplateContext(
	ctx context.Context,
	cmd documentuc.InternalCreateCommand,
) (*internalResolvedContext, error) {
	tenantCode := strings.ToUpper(strings.TrimSpace(cmd.TenantCode))
	workspaceCode := strings.ToUpper(strings.TrimSpace(cmd.WorkspaceCode))
	documentTypeCode := strings.ToUpper(strings.TrimSpace(cmd.DocumentType))
	slog.InfoContext(ctx, "resolving internal template context",
		"tenant_code_raw", cmd.TenantCode,
		"workspace_code_raw", cmd.WorkspaceCode,
		"document_type_raw", cmd.DocumentType,
		"tenant_code", tenantCode,
		"workspace_code", workspaceCode,
		"document_type_code", documentTypeCode,
	)

	process := strings.ToUpper(strings.TrimSpace(cmd.Process))
	if process == "" {
		process = entity.DefaultProcess
	}
	processType := cmd.ProcessType
	if processType == "" {
		processType = string(entity.DefaultProcessType)
	}
	resolverReq := &port.TemplateResolverRequest{
		TenantCode:      tenantCode,
		WorkspaceCode:   workspaceCode,
		DocumentType:    documentTypeCode,
		Process:         process,
		ProcessType:     processType,
		ExternalID:      cmd.ExternalID,
		TransactionalID: cmd.TransactionalID,
		ForceCreate:     cmd.ForceCreate,
		SupersedeReason: cmd.SupersedeReason,
		Headers:         cmd.Headers,
		RawBody:         cmd.PayloadRaw,
		Environment:     cmd.Environment,
	}
	if err := s.applySandboxWorkspaceCode(ctx, resolverReq); err != nil {
		return nil, err
	}

	resolutionCtx := withTemplateResolutionEnvironment(ctx, resolverReq.Environment)
	resolved, err := s.resolveTemplateContextWithResolvers(resolutionCtx, resolverReq)
	if err != nil {
		return nil, err
	}
	return internalResolvedContextFromTemplateContext(resolved)
}

func (s *InternalDocumentService) resolveTemplateContextWithResolvers(
	ctx context.Context,
	req *port.TemplateResolverRequest,
) (*port.InternalTemplateContext, error) {
	if s.customResolver != nil {
		resolved, err := s.customResolver.Resolve(ctx, req, s.searchAdapter)
		if err != nil {
			slog.ErrorContext(ctx, "custom template resolver error", slog.Any("error", err))
			return nil, fmt.Errorf("custom template resolver failed: %w", err)
		}
		if resolved != nil {
			slog.InfoContext(ctx, "custom resolver hit", slog.String("versionID", resolved.Version.ID))
			return resolved, nil
		}
		slog.InfoContext(ctx, "custom resolver miss, falling back to default")
	}

	resolved, err := s.defaultResolver.Resolve(ctx, req, s.searchAdapter)
	if err != nil {
		slog.ErrorContext(ctx, "default template resolver error", slog.Any("error", err))
		return nil, err
	}
	if resolved == nil {
		slog.WarnContext(ctx, "default resolver miss, no template context found")
		return nil, entity.ErrInternalTemplateResolutionNotFound
	}
	slog.InfoContext(ctx, "default resolver hit", slog.String("versionID", resolved.Version.ID))
	return resolved, nil
}

func internalResolvedContextFromTemplateContext(
	resolved *port.InternalTemplateContext,
) (*internalResolvedContext, error) {
	if resolved == nil || resolved.Tenant == nil || resolved.DocumentType == nil || resolved.Template == nil ||
		resolved.Workspace == nil || resolved.Version == nil {
		return nil, entity.ErrInternalTemplateResolutionNotFound
	}
	if !resolved.Version.IsPublished() {
		return nil, entity.ErrInternalTemplateResolutionNotFound
	}
	if resolved.Template.DocumentTypeID == nil || *resolved.Template.DocumentTypeID != resolved.DocumentType.ID {
		return nil, entity.ErrInternalTemplateResolutionNotFound
	}

	return &internalResolvedContext{
		tenant:           resolved.Tenant,
		workspace:        resolved.Workspace,
		documentType:     resolved.DocumentType,
		template:         resolved.Template,
		version:          resolved.Version,
		dbInjectables:    resolved.DBInjectables,
		activeSystemKeys: resolved.ActiveSystemKeys,
	}, nil
}

// applySandboxWorkspaceCode populates SandboxWorkspaceCode when environment is dev.
func (s *InternalDocumentService) applySandboxWorkspaceCode(
	ctx context.Context,
	req *port.TemplateResolverRequest,
) error {
	if req.Environment != entity.EnvironmentDev {
		return nil
	}
	tenant, err := s.tenantRepo.FindByCode(ctx, req.TenantCode)
	if err != nil {
		return fmt.Errorf("resolving tenant for sandbox workspace: %w", err)
	}
	workspace, err := s.workspaceRepo.FindByCode(ctx, tenant.ID, req.WorkspaceCode)
	if err != nil {
		if errors.Is(err, entity.ErrWorkspaceNotFound) {
			return nil
		}
		return fmt.Errorf("resolving workspace for sandbox lookup: %w", err)
	}
	sandbox, err := s.workspaceRepo.FindSandboxByParentID(ctx, workspace.ID)
	if err == nil {
		req.SandboxWorkspaceCode = sandbox.Code
		return nil
	}
	if errors.Is(err, entity.ErrSandboxNotFound) {
		return nil
	}
	return fmt.Errorf("resolving sandbox workspace: %w", err)
}

func (s *InternalDocumentService) buildInternalDocument(
	cmd documentuc.InternalCreateCommand,
	resolved *internalResolvedContext,
	prepared *PreparedDocumentData,
) (*entity.Document, error) {
	doc := entity.NewDocument(resolved.workspace.ID, resolved.version.ID)
	doc.DocumentTypeID = resolved.documentType.ID
	doc.SetOperationType(entity.OperationCreate)
	doc.SetExternalReference(cmd.ExternalID)
	doc.SetTransactionalID(cmd.TransactionalID)

	if len(cmd.Metadata) > 0 {
		if err := doc.SetMetadata(cmd.Metadata); err != nil {
			return nil, fmt.Errorf("setting metadata: %w", err)
		}
	}

	if err := doc.SetInjectedValuesSnapshot(prepared.ResolvedValues); err != nil {
		return nil, fmt.Errorf("setting injected values snapshot: %w", err)
	}

	if len(prepared.Recipients) > 0 {
		if err := doc.MarkAsAwaitingInput(); err != nil {
			return nil, fmt.Errorf("marking internal document as awaiting input: %w", err)
		}
	}

	if err := doc.Validate(); err != nil {
		return nil, fmt.Errorf("validating internal document: %w", err)
	}

	return doc, nil
}
