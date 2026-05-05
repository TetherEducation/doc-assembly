package document

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

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
	searchAdapter   port.TemplateVersionSearchAdapter
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
		searchAdapter: NewTemplateVersionSearchAdapter(
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

	if resolved, ok, err := s.resolveInternalTemplateContextFast(ctx, resolverReq); ok || err != nil {
		return resolved, err
	}

	baseQuery := port.InternalTemplateBaseQuery{
		TenantCode:    tenantCode,
		WorkspaceCode: workspaceCode,
		DocumentType:  documentTypeCode,
	}
	baseContextStartedAt := time.Now()
	baseContext, err := s.templateRepo.FindInternalTemplateBaseContext(ctx, baseQuery)
	slog.InfoContext(ctx, "internal template base context timing",
		slog.Duration("duration", time.Since(baseContextStartedAt)),
		slog.String("tenant_code", tenantCode),
		slog.String("workspace_code", workspaceCode),
		slog.String("document_type_code", documentTypeCode),
	)
	if err != nil {
		if errors.Is(err, entity.ErrTenantNotFound) {
			return nil, fmt.Errorf("resolving tenant by code: %w", err)
		}
		if errors.Is(err, entity.ErrDocumentTypeNotFound) {
			return nil, fmt.Errorf("resolving document type by code: %w", err)
		}
		return nil, fmt.Errorf("resolving internal template base context: %w", err)
	}
	tenant := baseContext.Tenant
	workspace := baseContext.Workspace
	docType := baseContext.DocumentType
	slog.InfoContext(ctx, "resolved tenant", slog.String("tenant_id", tenant.ID), slog.String("tenant_code", tenantCode))

	if workspace == nil {
		if s.customResolver == nil {
			return nil, fmt.Errorf("resolving workspace by code: %w", entity.ErrWorkspaceNotFound)
		}
		slog.InfoContext(ctx, "workspace not found, deferring to template resolver",
			slog.String("workspace_code", workspaceCode))
	} else {
		slog.InfoContext(ctx, "resolved workspace", slog.String("workspace_id", workspace.ID), slog.String("workspace_code", workspaceCode))
	}

	slog.InfoContext(ctx, "resolved document type", slog.String("document_type_id", docType.ID), slog.String("document_type_code", documentTypeCode))

	if workspace != nil {
		if err := s.applySandboxWorkspaceCode(ctx, resolverReq, workspace.ID, cmd.Environment); err != nil {
			return nil, err
		}
	}

	versionID, err := s.resolveTemplateVersionID(ctx, resolverReq)
	if err != nil {
		return nil, err
	}

	versionContextStartedAt := time.Now()
	versionContext, err := s.versionRepo.FindByIDWithDetailsAndTemplateWorkspace(ctx, *versionID)
	slog.InfoContext(ctx, "internal template context timing",
		slog.String("stage", "load_version_context"),
		slog.String("version_id", *versionID),
		slog.Duration("duration", time.Since(versionContextStartedAt)),
	)
	if err != nil {
		return nil, fmt.Errorf("loading resolved template version context: %w", err)
	}
	version := versionContext.Version
	if !version.IsPublished() {
		return nil, entity.ErrInternalTemplateResolutionNotFound
	}

	template := versionContext.Template
	if template.DocumentTypeID == nil || *template.DocumentTypeID != docType.ID {
		return nil, entity.ErrInternalTemplateResolutionNotFound
	}

	if workspace == nil {
		workspace = versionContext.Workspace
	}
	slog.InfoContext(ctx, "resolved internal create context",
		"tenant_id", tenant.ID,
		"tenant_code", tenant.Code,
		"workspace_id", workspace.ID,
		"workspace_code", workspace.Code,
		"document_type_id", docType.ID,
		"document_type_code", docType.Code,
		"template_id", template.ID,
		"version_id", version.ID,
	)

	return &internalResolvedContext{
		tenant:       tenant,
		workspace:    workspace,
		documentType: docType,
		template:     template,
		version:      version,
	}, nil
}

func (s *InternalDocumentService) resolveInternalTemplateContextFast(
	ctx context.Context,
	req *port.TemplateResolverRequest,
) (*internalResolvedContext, bool, error) {
	resolver, ok := s.customResolver.(port.InternalTemplateContextResolver)
	if !ok {
		return nil, false, nil
	}

	startedAt := time.Now()
	resolved, err := resolver.ResolveInternalTemplateContext(ctx, req, s.searchAdapter.(port.InternalTemplateContextSearchAdapter))
	slog.InfoContext(ctx, "internal template fast context timing",
		slog.Duration("duration", time.Since(startedAt)),
	)
	if err != nil {
		slog.ErrorContext(ctx, "custom template context resolver error", slog.Any("error", err))
		return nil, true, fmt.Errorf("custom template context resolver failed: %w", err)
	}
	if resolved == nil {
		return nil, false, nil
	}
	if resolved.Tenant == nil || resolved.DocumentType == nil || resolved.Template == nil ||
		resolved.Workspace == nil || resolved.Version == nil {
		return nil, true, entity.ErrInternalTemplateResolutionNotFound
	}
	if !resolved.Version.IsPublished() {
		return nil, true, entity.ErrInternalTemplateResolutionNotFound
	}
	if resolved.Template.DocumentTypeID == nil || *resolved.Template.DocumentTypeID != resolved.DocumentType.ID {
		return nil, true, entity.ErrInternalTemplateResolutionNotFound
	}

	slog.InfoContext(ctx, "resolved internal create context",
		"tenant_id", resolved.Tenant.ID,
		"tenant_code", resolved.Tenant.Code,
		"workspace_id", resolved.Workspace.ID,
		"workspace_code", resolved.Workspace.Code,
		"document_type_id", resolved.DocumentType.ID,
		"document_type_code", resolved.DocumentType.Code,
		"template_id", resolved.Template.ID,
		"version_id", resolved.Version.ID,
		"fast_context", true,
	)
	return &internalResolvedContext{
		tenant:           resolved.Tenant,
		workspace:        resolved.Workspace,
		documentType:     resolved.DocumentType,
		template:         resolved.Template,
		version:          resolved.Version,
		dbInjectables:    resolved.DBInjectables,
		activeSystemKeys: resolved.ActiveSystemKeys,
	}, true, nil
}

// applySandboxWorkspaceCode populates SandboxWorkspaceCode when environment is dev.
func (s *InternalDocumentService) applySandboxWorkspaceCode(
	ctx context.Context,
	req *port.TemplateResolverRequest,
	workspaceID string,
	env entity.Environment,
) error {
	if env != entity.EnvironmentDev {
		return nil
	}
	sandbox, err := s.workspaceRepo.FindSandboxByParentID(ctx, workspaceID)
	if err == nil {
		req.SandboxWorkspaceCode = sandbox.Code
		return nil
	}
	if errors.Is(err, entity.ErrSandboxNotFound) {
		return nil
	}
	return fmt.Errorf("resolving sandbox workspace: %w", err)
}

// resolveTemplateVersionID runs custom then default resolver to find a template version.
func (s *InternalDocumentService) resolveTemplateVersionID(
	ctx context.Context,
	req *port.TemplateResolverRequest,
) (*string, error) {
	var versionID *string
	if s.customResolver != nil {
		vID, err := s.customResolver.Resolve(ctx, req, s.searchAdapter)
		if err != nil {
			slog.ErrorContext(ctx, "custom template resolver error", slog.Any("error", err))
			return nil, fmt.Errorf("custom template resolver failed: %w", err)
		}
		if vID != nil {
			slog.InfoContext(ctx, "custom resolver hit", slog.String("versionID", *vID))
		} else {
			slog.InfoContext(ctx, "custom resolver miss, falling back to default")
		}
		versionID = vID
	}

	if versionID == nil {
		vID, err := s.defaultResolver.Resolve(ctx, req, s.searchAdapter)
		if err != nil {
			slog.ErrorContext(ctx, "default template resolver error", slog.Any("error", err))
			return nil, err
		}
		if vID != nil {
			slog.InfoContext(ctx, "default resolver hit", slog.String("versionID", *vID))
		} else {
			slog.WarnContext(ctx, "default resolver miss, no template version found")
		}
		versionID = vID
	}
	if versionID == nil || *versionID == "" {
		return nil, entity.ErrInternalTemplateResolutionNotFound
	}
	return versionID, nil
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
