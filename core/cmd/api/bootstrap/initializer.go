package bootstrap

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/controller"
	httpmapper "github.com/rendis/doc-assembly/core/internal/adapters/primary/http/mapper"
	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres"
	automationapikeyrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/automation_api_key_repo"
	automationauditlogrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/automation_audit_log_repo"
	documentaccesstokenrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/document_access_token_repo"
	documenteventrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/document_event_repo"
	documentfieldresponserepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/document_field_response_repo"
	documentrecipientrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/document_recipient_repo"
	documentrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/document_repo"
	documenttyperepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/document_type_repo"
	folderrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/folder_repo"
	galleryassetrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/gallery_asset_repo"
	injectablerepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/injectable_repo"
	processrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/process_repo"
	signingattemptrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/signing_attempt_repo"
	systeminjectablerepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/system_injectable_repo"
	systemrolerepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/system_role_repo"
	tagrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/tag_repo"
	templaterepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/template_repo"
	templatetagrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/template_tag_repo"
	templateversioninjectablerepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/template_version_injectable_repo"
	templateversionrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/template_version_repo"
	templateversionsignerrolerepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/template_version_signer_role_repo"
	tenantmemberrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/tenant_member_repo"
	tenantrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/tenant_repo"
	useraccesshistoryrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/user_access_history_repo"
	userrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/user_repo"
	workspaceinjectablerepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/workspace_injectable_repo"
	workspacememberrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/workspace_member_repo"
	workspacerepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/workspace_repo"
	gmailnotification "github.com/rendis/doc-assembly/core/internal/adapters/secondary/notification/gmail"
	noopnotification "github.com/rendis/doc-assembly/core/internal/adapters/secondary/notification/noop"
	smtpnotification "github.com/rendis/doc-assembly/core/internal/adapters/secondary/notification/smtp"
	"github.com/rendis/doc-assembly/core/internal/adapters/secondary/signing/documenso"
	mocksigning "github.com/rendis/doc-assembly/core/internal/adapters/secondary/signing/mock"
	localstorage "github.com/rendis/doc-assembly/core/internal/adapters/secondary/storage/local"
	s3storage "github.com/rendis/doc-assembly/core/internal/adapters/secondary/storage/s3"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	accesssvc "github.com/rendis/doc-assembly/core/internal/core/service/access"
	catalogsvc "github.com/rendis/doc-assembly/core/internal/core/service/catalog"
	documentsvc "github.com/rendis/doc-assembly/core/internal/core/service/document"
	gallerysvc "github.com/rendis/doc-assembly/core/internal/core/service/gallery"
	injectablesvc "github.com/rendis/doc-assembly/core/internal/core/service/injectable"
	organizationsvc "github.com/rendis/doc-assembly/core/internal/core/service/organization"
	"github.com/rendis/doc-assembly/core/internal/core/service/rendering/pdfrenderer"
	templatesvc "github.com/rendis/doc-assembly/core/internal/core/service/template"
	"github.com/rendis/doc-assembly/core/internal/core/service/template/contentvalidator"
	automationuc "github.com/rendis/doc-assembly/core/internal/core/usecase/automation"
	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
	"github.com/rendis/doc-assembly/core/internal/extensions/injectors/datetime"
	"github.com/rendis/doc-assembly/core/internal/frontend"
	"github.com/rendis/doc-assembly/core/internal/infra/config"
	"github.com/rendis/doc-assembly/core/internal/infra/registry"
	"github.com/rendis/doc-assembly/core/internal/infra/riverqueue"
	"github.com/rendis/doc-assembly/core/internal/infra/scheduler"
	"github.com/rendis/doc-assembly/core/internal/infra/server"
)

// appComponents holds all initialized components.
type appComponents struct {
	httpServer  *server.HTTPServer
	dbPool      *pgxpool.Pool
	scheduler   *scheduler.Scheduler
	riverSvc    *riverqueue.RiverService
	hasFrontend bool
}

func (a *appComponents) cleanup() {
	slog.Info("cleaning up resources")
	if a.riverSvc != nil {
		if err := a.riverSvc.Stop(context.Background()); err != nil {
			slog.Error("failed to stop river", slog.Any("error", err))
		}
	}
	a.scheduler.Stop()
	postgres.Close(a.dbPool)
	slog.Info("cleanup complete")
}

// initialize creates all components using manual DI.
func (e *Engine) initialize(ctx context.Context) (*appComponents, error) { //nolint:funlen // DI composition is inherently sequential
	cfg := e.config
	if strings.TrimSpace(e.signingSessionMode) != "" {
		cfg.SigningSessionAuth.Mode = strings.TrimSpace(e.signingSessionMode)
	}

	// --- Database ---
	pool, err := postgres.NewPool(ctx, &cfg.Database)
	if err != nil {
		return nil, err
	}

	// --- Dummy Auth: seed default user ---
	if cfg.Auth.IsDummyAuth() {
		userID, seedErr := seedDummyUser(ctx, pool)
		if seedErr != nil {
			return nil, seedErr
		}
		cfg.DummyAuthUserID = userID
		slog.InfoContext(ctx, "dummy auth user seeded", slog.String("user_id", userID))
	}

	// --- Repositories: Organizational ---
	userRepo := userrepo.New(pool)
	systemRoleRepo := systemrolerepo.New(pool)
	workspaceRepo := workspacerepo.New(pool)
	workspaceMemberRepo := workspacememberrepo.New(pool)
	tenantMemberRepo := tenantmemberrepo.New(pool)
	tenantRepo := tenantrepo.New(pool)
	userAccessHistoryRepo := useraccesshistoryrepo.New(pool)

	// --- Repositories: Catalog ---
	folderRepo := folderrepo.New(pool)
	tagRepo := tagrepo.New(pool)

	// --- Repositories: Content ---
	injectableRepo := injectablerepo.New(pool)
	systemInjectableRepo := systeminjectablerepo.New(pool)
	workspaceInjectableRepo := workspaceinjectablerepo.New(pool)
	templateRepo := templaterepo.New(pool)
	templateVersionRepo := templateversionrepo.New(pool)
	templateTagRepo := templatetagrepo.New(pool)
	templateVersionInjectableRepo := templateversioninjectablerepo.New(pool)
	templateVersionSignerRoleRepo := templateversionsignerrolerepo.New(pool)
	documentTypeRepo := documenttyperepo.New(pool)
	processRepo := processrepo.New(pool)

	// --- Repositories: Execution ---
	documentRepoConcrete := documentrepo.NewConcrete(pool)
	var documentRepo port.DocumentRepository = documentRepoConcrete
	documentRecipientRepo := documentrecipientrepo.New(pool)
	documentEventRepo := documenteventrepo.New(pool)
	documentFieldResponseRepo := documentfieldresponserepo.New(pool)
	documentAccessTokenRepo := documentaccesstokenrepo.New(pool)
	signingAttemptRepo := signingattemptrepo.New(pool)

	// --- Repositories: Automation ---
	automationAPIKeyRepo := automationapikeyrepo.New(pool)
	automationAuditLogRepo := automationauditlogrepo.New(pool)

	// --- Middleware ---
	middlewareProvider := middleware.NewProvider(
		pool, cfg.Bootstrap.Enabled,
		userRepo, systemRoleRepo, workspaceRepo, workspaceMemberRepo, tenantMemberRepo,
	)

	// --- Extensibility: Registries ---
	injReg, mapReg, err := e.buildRegistries()
	if err != nil {
		return nil, err
	}

	// --- Services: Organization ---
	workspaceSvc := organizationsvc.NewWorkspaceService(
		workspaceRepo,
		tenantRepo,
		workspaceMemberRepo,
		tenantMemberRepo,
		systemRoleRepo,
		userAccessHistoryRepo,
	)
	tenantSvc := organizationsvc.NewTenantService(tenantRepo, workspaceRepo, tenantMemberRepo, systemRoleRepo, userAccessHistoryRepo)
	workspaceMemberSvc := organizationsvc.NewWorkspaceMemberService(
		workspaceMemberRepo,
		userRepo,
		systemRoleRepo,
		workspaceRepo,
		tenantRepo,
		tenantMemberRepo,
	)
	tenantMemberSvc := organizationsvc.NewTenantMemberService(tenantMemberRepo, userRepo, tenantRepo)

	// --- Services: Catalog ---
	folderSvc := catalogsvc.NewFolderService(folderRepo)
	tagSvc := catalogsvc.NewTagService(tagRepo)
	documentTypeSvc := catalogsvc.NewDocumentTypeService(documentTypeRepo, templateRepo)
	processSvc := catalogsvc.NewProcessService(processRepo, templateRepo)

	// --- Services: Access ---
	systemRoleSvc := accesssvc.NewSystemRoleService(systemRoleRepo, userRepo)
	userAccessHistorySvc := accesssvc.NewUserAccessHistoryService(userAccessHistoryRepo)

	// --- Services: Injectable ---
	injectableSvc := injectablesvc.NewInjectableService(
		injectableRepo, systemInjectableRepo, injReg,
		workspaceRepo, tenantRepo, e.workspaceProvider,
	)
	workspaceInjectableSvc := injectablesvc.NewWorkspaceInjectableService(workspaceInjectableRepo)
	systemInjectableSvc := injectablesvc.NewSystemInjectableService(systemInjectableRepo, injReg)

	// --- Services: Template ---
	templateSvc := templatesvc.NewTemplateService(templateRepo, templateVersionRepo, templateTagRepo, workspaceRepo, e.processResolver)
	contentValidator := contentvalidator.New(injectableSvc)
	templateVersionSvc := templatesvc.NewTemplateVersionService(
		templateVersionRepo, templateVersionInjectableRepo, templateVersionSignerRoleRepo,
		systemInjectableRepo, templateRepo, templateTagRepo, contentValidator, workspaceRepo,
	)

	// --- Storage Adapter ---
	storageAdapter, err := e.resolveStorageAdapter(cfg)
	if err != nil {
		return nil, err
	}

	// --- PDF Renderer ---
	pdfRenderer, err := buildPDFRenderer(cfg, e.designTokens, storageAdapter)
	if err != nil {
		return nil, err
	}

	// --- Signing Provider ---
	signingProvider, err := e.resolveSigningProvider(cfg)
	if err != nil {
		return nil, err
	}

	// --- Gallery ---
	var galleryCtrl *controller.GalleryController
	if cfg.Storage.Enabled {
		galleryAssetRepo := galleryassetrepo.New(pool)
		gallerySvc := gallerysvc.New(storageAdapter, galleryAssetRepo, cfg.Server.PublicURL)
		galleryCtrl = controller.NewGalleryController(gallerySvc)
	}

	// --- Notification Provider ---
	notificationProvider := e.resolveNotificationProvider(cfg)

	// --- Webhook Handlers ---
	webhookHandlers, err := e.resolveWebhookHandlers(cfg)
	if err != nil {
		return nil, err
	}

	// --- Services: Document ---
	eventEmitter := documentsvc.NewEventEmitter(documentEventRepo)
	publicURL := cfg.Server.PublicURL
	notificationSvc := documentsvc.NewNotificationService(notificationProvider, documentRecipientRepo, documentRepo, documentAccessTokenRepo, publicURL)

	// --- River Job Queue (mandatory for signing attempts; workers optional by config) ---
	workerCfg := cfg.Worker
	workerCfg.RuntimeEnvironment = cfg.Environment
	riverSvc, err := riverqueue.New(ctx, pool, workerCfg, riverqueue.Dependencies{
		DocumentRepo:      documentRepo,
		RecipientRepo:     documentRecipientRepo,
		AttemptRepo:       signingAttemptRepo,
		VersionRepo:       templateVersionRepo,
		SignerRoleRepo:    templateVersionSignerRoleRepo,
		FieldResponseRepo: documentFieldResponseRepo,
		PDFRenderer:       pdfRenderer,
		SigningProvider:   signingProvider,
		StorageAdapter:    storageAdapter,
		StorageEnabled:    cfg.Storage.Enabled,
		CompletionHandler: e.documentCompletedHandler,
	})
	if err != nil {
		return nil, fmt.Errorf("river: %w", err)
	}
	signingUOW := riverSvc.SigningExecutionUOW()

	documentSvcConcrete := documentsvc.NewDocumentService(
		documentRepo, documentRecipientRepo, signingAttemptRepo, signingUOW, templateRepo, templateVersionRepo, templateVersionSignerRoleRepo,
		pdfRenderer, signingProvider, storageAdapter,
		eventEmitter, notificationSvc,
		cfg.Scheduler.ExpirationDays,
		documentAccessTokenRepo, documentFieldResponseRepo,
		cfg.Storage.Enabled,
	)

	var documentSvc documentuc.DocumentUseCase = documentSvcConcrete
	injectableResolver := injectablesvc.NewInjectableResolverService(injReg)
	documentGenerator := documentsvc.NewDocumentGenerator(
		templateRepo, templateVersionRepo, documentRepo, documentRecipientRepo,
		injectableSvc, mapReg, injectableResolver,
	)
	internalDocSvc := documentsvc.NewInternalDocumentService(
		documentGenerator,
		documentRepo,
		tenantRepo,
		workspaceRepo,
		documentTypeRepo,
		templateRepo,
		templateVersionRepo,
		e.templateResolver,
	)
	preSigningSvc := documentsvc.NewPreSigningService(
		documentAccessTokenRepo, documentFieldResponseRepo,
		documentRepo, documentRecipientRepo, signingAttemptRepo, signingUOW, templateVersionRepo, templateVersionSignerRoleRepo,
		pdfRenderer, signingProvider, storageAdapter, cfg.Storage.Enabled, eventEmitter, publicURL,
	)
	signingSessionSvc := documentsvc.NewSigningSessionService(
		documentRepo,
		documentRecipientRepo,
		templateVersionRepo,
		documentAccessTokenRepo,
		preSigningSvc,
		storageAdapter,
		cfg.PublicAccess.TokenTTLHours,
		cfg.Storage.Enabled,
	)

	// --- Use Cases: Automation ---
	automationAPIKeyUC := automationuc.NewAPIKeyUseCase(automationAPIKeyRepo, automationAuditLogRepo)

	// --- HTTP Mappers ---
	injectableMapper := httpmapper.NewInjectableMapper()
	templateVersionMapper := httpmapper.NewTemplateVersionMapper(injectableMapper)
	tagMapper := httpmapper.NewTagMapper()
	folderMapper := httpmapper.NewFolderMapper()
	templateMapper := httpmapper.NewTemplateMapper(templateVersionMapper, tagMapper, folderMapper)
	docTypeMapper := httpmapper.NewDocumentTypeMapper()
	processMapper := httpmapper.NewProcessMapper()

	// --- Controllers ---
	workspaceCtrl := controller.NewWorkspaceController(
		workspaceSvc, folderSvc, tagSvc, workspaceMemberSvc, workspaceInjectableSvc, injectableMapper,
	)
	injectableCtrl := controller.NewContentInjectableController(injectableSvc, injectableMapper)
	renderCtrl := controller.NewRenderController(templateVersionSvc, pdfRenderer)
	templateVersionCtrl := controller.NewTemplateVersionController(
		templateVersionSvc, templateVersionMapper, templateMapper, renderCtrl,
	)
	templateCtrl := controller.NewContentTemplateController(templateSvc, templateMapper, templateVersionCtrl)
	adminCtrl := controller.NewAdminController(tenantSvc, systemRoleSvc, systemInjectableSvc)
	meCtrl := controller.NewMeController(tenantSvc, tenantMemberRepo, workspaceMemberRepo, workspaceRepo, userAccessHistorySvc)
	tenantCtrl := controller.NewTenantController(tenantSvc, workspaceSvc, tenantMemberSvc)
	documentTypeCtrl := controller.NewDocumentTypeController(documentTypeSvc, templateSvc, templateMapper)
	processCtrl := controller.NewProcessController(processSvc, processMapper)
	webhookCtrl := controller.NewWebhookController(documentSvc, webhookHandlers)
	internalDocCtrl := controller.NewInternalDocumentController(internalDocSvc, documentSvc)
	// --- Document Access Service (email-verification gate) ---
	paCfg := &cfg.PublicAccess
	rateLimitMax := paCfg.RateLimitMax
	if rateLimitMax <= 0 {
		rateLimitMax = 3
	}
	rateLimitWindowMin := paCfg.RateLimitWindowMin
	if rateLimitWindowMin <= 0 {
		rateLimitWindowMin = 60
	}
	tokenTTLHours := paCfg.TokenTTLHours
	if tokenTTLHours <= 0 {
		tokenTTLHours = 48
	}
	documentAccessSvc := documentsvc.NewDocumentAccessService(
		documentRepo, documentRecipientRepo, templateVersionRepo, documentAccessTokenRepo,
		notificationSvc, publicURL, rateLimitMax, rateLimitWindowMin, tokenTTLHours,
	)
	readOnlyViewSvc := documentsvc.NewReadOnlyViewService(
		documentRepo,
		documentAccessTokenRepo,
		documentRecipientRepo,
		templateVersionRepo,
		templateVersionSignerRoleRepo,
		documentFieldResponseRepo,
		pdfRenderer,
		storageAdapter,
		cfg.Storage.Enabled,
		tokenTTLHours,
		readOnlyViewPublicURL(cfg.Server),
	).SetWorkspaceRepository(workspaceRepo).
		SetCompletedPDFProvider(signingProvider, signingAttemptRepo)
	documentCtrl := controller.NewDocumentController(documentSvc, preSigningSvc, readOnlyViewSvc, eventEmitter)
	publicDocAccessCtrl := controller.NewPublicDocumentAccessController(documentAccessSvc)
	publicSigningCtrl := controller.NewPublicSigningController(preSigningSvc, documentAccessSvc, publicURL)
	publicReadOnlyViewCtrl := controller.NewPublicReadOnlyViewController(readOnlyViewSvc)
	signingSessionCtrl := controller.NewSigningSessionController(signingSessionSvc)
	automationKeyCtrl := controller.NewAutomationKeyController(automationAPIKeyUC)
	automationCtrl := controller.NewAutomationController(
		tenantSvc, workspaceSvc, injectableSvc,
		templateSvc, templateVersionSvc, documentTypeSvc,
		automationAPIKeyRepo, automationAuditLogRepo,
		templateMapper, templateVersionMapper, injectableMapper, docTypeMapper,
	)

	publicDocAuth := e.publicDocAuth

	// --- Frontend FS ---
	frontendFS := e.resolveFrontendFS()

	// --- HTTP Server ---
	httpServer := server.NewHTTPServer(
		cfg,
		middlewareProvider,
		workspaceCtrl,
		injectableCtrl,
		templateCtrl,
		adminCtrl,
		meCtrl,
		tenantCtrl,
		documentTypeCtrl,
		processCtrl,
		documentCtrl,
		webhookCtrl,
		internalDocCtrl,
		publicDocAccessCtrl,
		publicSigningCtrl,
		publicReadOnlyViewCtrl,
		signingSessionCtrl,
		automationKeyCtrl,
		automationCtrl,
		galleryCtrl,
		publicDocAuth,
		e.signingSessionAuth,
		e.readOnlyViewAuth,
		e.legacyDocumentHandler,
		automationAPIKeyRepo,
		frontendFS,
	)

	// --- Background Scheduler ---
	sched := scheduler.New(cfg.Scheduler.Enabled)
	registerSchedulerJobs(sched, &cfg.Scheduler, documentSvc)

	return &appComponents{
		httpServer:  httpServer,
		dbPool:      pool,
		scheduler:   sched,
		riverSvc:    riverSvc,
		hasFrontend: frontendFS != nil,
	}, nil
}

// buildPDFRenderer creates the Typst-based PDF renderer service.
func buildPDFRenderer(cfg *config.Config, customTokens *pdfrenderer.TypstDesignTokens, storageAdapter port.StorageAdapter) (port.PDFRenderer, error) {
	typstCfg := &cfg.Typst
	opts := pdfrenderer.TypstOptions{
		BinPath:        typstCfg.BinPath,
		Timeout:        typstCfg.TimeoutDuration(),
		FontDirs:       resolveTypstFontDirs(typstCfg.FontDirs),
		MaxConcurrent:  typstCfg.MaxConcurrent,
		AcquireTimeout: typstCfg.AcquireTimeoutDuration(),
	}

	var imageCache *pdfrenderer.ImageCache
	if typstCfg.ImageCacheDir != "" || typstCfg.ImageCacheMaxAgeSeconds > 0 {
		var err error
		imageCache, err = pdfrenderer.NewImageCache(pdfrenderer.ImageCacheOptions{
			Dir:             typstCfg.ImageCacheDir,
			MaxAge:          typstCfg.ImageCacheMaxAgeDuration(),
			CleanupInterval: typstCfg.ImageCacheCleanupIntervalDuration(),
		})
		if err != nil {
			return nil, fmt.Errorf("creating image cache: %w", err)
		}
	}

	tokens := pdfrenderer.DefaultDesignTokens()
	if customTokens != nil {
		tokens = *customTokens
	}

	factory := pdfrenderer.NewTypstConverterFactory(tokens)
	return pdfrenderer.NewService(opts, imageCache, factory, tokens, storageAdapter)
}

func resolveTypstFontDirs(fontDirs []string) []string {
	resolved := make([]string, 0, len(fontDirs))
	for _, dir := range fontDirs {
		if strings.TrimSpace(dir) == "" {
			continue
		}

		candidates := []string{dir}
		if !filepath.IsAbs(dir) {
			candidates = append(candidates, filepath.Join("..", dir), filepath.Join("core", dir))
		}

		chosen := dir
		for _, candidate := range candidates {
			info, err := os.Stat(candidate)
			if err == nil && info.IsDir() {
				chosen = candidate
				break
			}
		}

		if abs, err := filepath.Abs(chosen); err == nil {
			chosen = abs
		}

		resolved = append(resolved, chosen)
	}

	return resolved
}

// resolveSigningProvider returns the engine override or auto-selects from config.
func (e *Engine) resolveSigningProvider(cfg *config.Config) (port.SigningProvider, error) {
	if e.signingProvider != nil {
		return e.signingProvider, nil
	}
	switch cfg.Signing.Provider {
	case "mock":
		return mocksigning.New(), nil
	default:
		return documenso.New(&documenso.Config{
			APIKey:         cfg.Signing.APIKey,
			BaseURL:        cfg.Signing.BaseURL,
			SigningBaseURL: cfg.Signing.SigningBaseURL,
			WebhookSecret:  cfg.Signing.WebhookSecret,
			WebhookURL:     cfg.Signing.WebhookURL,
		})
	}
}

// resolveStorageAdapter returns the engine override or auto-selects from config.
func (e *Engine) resolveStorageAdapter(cfg *config.Config) (port.StorageAdapter, error) {
	if e.storageAdapter != nil {
		return e.storageAdapter, nil
	}
	if !cfg.Storage.Enabled {
		return localstorage.New(cfg.Storage.LocalDir)
	}
	switch cfg.Storage.Provider {
	case "s3":
		return s3storage.New(&s3storage.Config{
			Bucket:   cfg.Storage.Bucket,
			Region:   cfg.Storage.Region,
			Endpoint: cfg.Storage.Endpoint,
		})
	default:
		return localstorage.New(cfg.Storage.LocalDir)
	}
}

// resolveNotificationProvider returns the engine override or auto-selects from config.
func (e *Engine) resolveNotificationProvider(cfg *config.Config) port.NotificationProvider {
	if e.notificationProvider != nil {
		return e.notificationProvider
	}
	switch cfg.Notification.Provider {
	case "smtp":
		return smtpnotification.New(&smtpnotification.Config{
			Host:     cfg.Notification.Host,
			Port:     cfg.Notification.Port,
			Username: cfg.Notification.Username,
			Password: cfg.Notification.Password,
			From:     cfg.Notification.From,
		})
	case "gmail":
		return gmailnotification.New(
			cfg.Notification.Username,
			cfg.Notification.Password,
			cfg.Notification.From,
		)
	default:
		return noopnotification.New()
	}
}

// resolveWebhookHandlers returns the engine override or auto-selects from config.
func (e *Engine) resolveWebhookHandlers(cfg *config.Config) (map[string]port.WebhookHandler, error) {
	if e.webhookHandlers != nil {
		return e.webhookHandlers, nil
	}
	switch cfg.Signing.Provider {
	case "mock":
		adapter := mocksigning.New()
		return map[string]port.WebhookHandler{"mock": adapter}, nil
	default:
		documensoAdapter, err := documenso.New(&documenso.Config{
			APIKey:         cfg.Signing.APIKey,
			BaseURL:        cfg.Signing.BaseURL,
			SigningBaseURL: cfg.Signing.SigningBaseURL,
			WebhookSecret:  cfg.Signing.WebhookSecret,
			WebhookURL:     cfg.Signing.WebhookURL,
		})
		if err != nil {
			return nil, err
		}
		return map[string]port.WebhookHandler{"documenso": documensoAdapter}, nil
	}
}

// buildRegistries creates and populates injector/mapper registries with built-in and user extensions.
func (e *Engine) buildRegistries() (port.InjectorRegistry, port.MapperRegistry, error) {
	i18nCfg, err := config.LoadInjectorI18n()
	if err != nil {
		return nil, nil, err
	}
	if e.i18nFilePath != "" {
		userI18n, mergeErr := config.LoadInjectorI18nFromFile(e.i18nFilePath)
		if mergeErr != nil {
			return nil, nil, mergeErr
		}
		i18nCfg.Merge(userI18n)
	}

	mapReg := registry.NewMapperRegistry()
	injReg := registry.NewInjectorRegistry(i18nCfg)

	builtinInjectors := []port.Injector{
		&datetime.DateNowInjector{}, &datetime.DateTimeNowInjector{},
		&datetime.DayNowInjector{}, &datetime.MonthNowInjector{},
		&datetime.TimeNowInjector{}, &datetime.YearNowInjector{},
	}
	for _, inj := range builtinInjectors {
		_ = injReg.Register(inj)
	}
	for _, inj := range e.injectors {
		if regErr := injReg.Register(inj); regErr != nil {
			return nil, nil, regErr
		}
	}
	if e.mapper != nil {
		if setErr := mapReg.Set(e.mapper); setErr != nil {
			return nil, nil, setErr
		}
	}
	if e.initFunc != nil {
		injReg.SetInitFunc(e.initFunc)
	}
	if e.workspaceProvider != nil {
		injReg.SetWorkspaceInjectableProvider(e.workspaceProvider)
	}

	return injReg, mapReg, nil
}

// resolveFrontendFS returns the frontend filesystem to serve.
// Priority: user override via SetFrontendFS() → embedded dist FS → nil.
func (e *Engine) resolveFrontendFS() fs.FS {
	if e.frontendOverridden {
		return e.frontendFS // may be nil (user explicitly disabled frontend)
	}

	// Check if embedded dist has actual content
	entries, err := fs.ReadDir(frontend.DistFS, "dist")
	if err != nil || len(entries) == 0 {
		slog.Warn("no embedded frontend found (run 'make embed-app' to embed)")
		return nil
	}

	sub, err := fs.Sub(frontend.DistFS, "dist")
	if err != nil {
		slog.Error("failed to create sub-FS for frontend", slog.Any("error", err))
		return nil
	}
	slog.Info("serving embedded frontend")
	return sub
}

func readOnlyViewPublicURL(serverCfg config.ServerConfig) string {
	if publicURL := strings.TrimSpace(serverCfg.PublicURL); publicURL != "" {
		return publicURL
	}
	return serverCfg.NormalizedBasePath()
}

// registerSchedulerJobs registers background polling jobs.
func registerSchedulerJobs(s *scheduler.Scheduler, cfg *config.SchedulerConfig, docUC documentuc.DocumentUseCase) {
	// Signing provider upload, retry, polling and reconciliation are intentionally
	// not registered here. The clean-slate signing model uses attempt-scoped River
	// jobs as the only durable engine for signing side effects.
	s.RegisterJob("expire-documents", cfg.PollingIntervalDuration(), func(ctx context.Context) error {
		return docUC.ExpireDocuments(ctx, cfg.PollingBatchSize)
	})
}

// seedDummyUser ensures a default admin user exists in the DB for dummy auth mode.
func seedDummyUser(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	const (
		email      = "admin@docengine.local"
		fullName   = "Doc Engine Admin"
		externalID = "00000000-0000-0000-0000-000000000001"
	)

	var userID string
	err := pool.QueryRow(ctx, `
		INSERT INTO identity.users (email, external_identity_id, full_name, status)
		VALUES ($1, $2, $3, 'ACTIVE')
		ON CONFLICT (email) DO UPDATE SET full_name = EXCLUDED.full_name
		RETURNING id
	`, email, externalID, fullName).Scan(&userID)
	if err != nil {
		return "", fmt.Errorf("seeding dummy user: %w", err)
	}

	_, err = pool.Exec(ctx, `
		INSERT INTO identity.system_roles (user_id, role)
		VALUES ($1, 'SUPERADMIN')
		ON CONFLICT (user_id) DO NOTHING
	`, userID)
	if err != nil {
		return "", fmt.Errorf("seeding dummy system role: %w", err)
	}

	return userID, nil
}
