//go:build integration

package testhelper

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/controller"
	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/mapper"
	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/middleware"
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
	noopnotification "github.com/rendis/doc-assembly/core/internal/adapters/secondary/notification/noop"
	mocksigning "github.com/rendis/doc-assembly/core/internal/adapters/secondary/signing/mock"
	localstorage "github.com/rendis/doc-assembly/core/internal/adapters/secondary/storage/local"
	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	accesssvc "github.com/rendis/doc-assembly/core/internal/core/service/access"
	catalogsvc "github.com/rendis/doc-assembly/core/internal/core/service/catalog"
	documentsvc "github.com/rendis/doc-assembly/core/internal/core/service/document"
	gallerysvc "github.com/rendis/doc-assembly/core/internal/core/service/gallery"
	injectablesvc "github.com/rendis/doc-assembly/core/internal/core/service/injectable"
	organizationsvc "github.com/rendis/doc-assembly/core/internal/core/service/organization"
	templatesvc "github.com/rendis/doc-assembly/core/internal/core/service/template"
	contentvalidator "github.com/rendis/doc-assembly/core/internal/core/service/template/contentvalidator"
	automationuc "github.com/rendis/doc-assembly/core/internal/core/usecase/automation"
	"github.com/rendis/doc-assembly/core/internal/infra/config"
	"github.com/rendis/doc-assembly/core/internal/infra/registry"
	"github.com/rendis/doc-assembly/core/internal/infra/riverqueue"
)

// TestInternalAPIKey is the API key used by integration tests for internal endpoints.
const TestInternalAPIKey = "test-internal-api-key"

// MockPDFRenderer implements port.PDFRenderer for testing.
type MockPDFRenderer struct{}

// RenderPreview returns minimal PDF bytes.
func (m *MockPDFRenderer) RenderPreview(_ context.Context, _ *port.RenderPreviewRequest) (*port.RenderPreviewResult, error) {
	return &port.RenderPreviewResult{
		PDF:      []byte("%PDF-1.4 mock test pdf"),
		Filename: "test.pdf",
	}, nil
}

// TestRequestMapper is a minimal mapper for internal API integration tests.
type TestRequestMapper struct{}

// Map parses payload JSON and returns it as generic data.
func (m *TestRequestMapper) Map(_ context.Context, mapCtx *port.MapperContext) (any, error) {
	if len(mapCtx.RawBody) == 0 {
		return map[string]any{}, nil
	}

	var payload any
	if err := json.Unmarshal(mapCtx.RawBody, &payload); err != nil {
		return nil, err
	}

	return payload, nil
}

// Close is a no-op.
func (m *MockPDFRenderer) Close() error { return nil }

// TestServer wraps an httptest.Server with helper methods for E2E testing.
type TestServer struct {
	Server             *httptest.Server
	Engine             *gin.Engine
	Pool               *pgxpool.Pool
	MockSigningAdapter *mocksigning.Adapter
	t                  *testing.T
}

// NewTestServer creates a test HTTP server with all real dependencies.
// It uses the test database pool and configures the server for E2E testing.
func NewTestServer(t *testing.T, pool *pgxpool.Pool) *TestServer {
	return NewTestServerWithResolver(t, pool, nil)
}

// NewTestServerWithResolver creates a test HTTP server with an optional custom internal template resolver.
func NewTestServerWithResolver(t *testing.T, pool *pgxpool.Pool, templateResolver port.TemplateResolver) *TestServer {
	t.Helper()
	gin.SetMode(gin.TestMode)

	// Create repositories - Identity & Tenancy
	userRepo := userrepo.New(pool)
	systemRoleRepo := systemrolerepo.New(pool)
	tenantRepo := tenantrepo.New(pool)
	workspaceRepo := workspacerepo.New(pool)
	workspaceMemberRepo := workspacememberrepo.New(pool)
	tenantMemberRepo := tenantmemberrepo.New(pool)
	folderRepo := folderrepo.New(pool)
	tagRepo := tagrepo.New(pool)
	userAccessHistoryRepo := useraccesshistoryrepo.New(pool)

	// Create repositories - Content
	injectableRepo := injectablerepo.New(pool)
	systemInjectableRepo := systeminjectablerepo.New(pool)
	galleryRepo := galleryassetrepo.New(pool)
	templateRepo := templaterepo.New(pool)
	templateTagRepo := templatetagrepo.New(pool)
	templateVersionRepo := templateversionrepo.New(pool)
	templateVersionInjectableRepo := templateversioninjectablerepo.New(pool)
	templateVersionSignerRoleRepo := templateversionsignerrolerepo.New(pool)
	workspaceInjectableRepo := workspaceinjectablerepo.New(pool)
	documentTypeRepo := documenttyperepo.New(pool)

	// Create services - Identity & Tenancy
	tenantService := organizationsvc.NewTenantService(tenantRepo, workspaceRepo, tenantMemberRepo, systemRoleRepo, userAccessHistoryRepo)
	workspaceService := organizationsvc.NewWorkspaceService(
		workspaceRepo,
		tenantRepo,
		workspaceMemberRepo,
		tenantMemberRepo,
		systemRoleRepo,
		userAccessHistoryRepo,
	)
	systemRoleService := accesssvc.NewSystemRoleService(systemRoleRepo, userRepo)
	tenantMemberService := organizationsvc.NewTenantMemberService(tenantMemberRepo, userRepo, tenantRepo)
	folderService := catalogsvc.NewFolderService(folderRepo)
	tagService := catalogsvc.NewTagService(tagRepo)
	workspaceMemberService := organizationsvc.NewWorkspaceMemberService(
		workspaceMemberRepo,
		userRepo,
		systemRoleRepo,
		workspaceRepo,
		tenantRepo,
		tenantMemberRepo,
	)
	userAccessHistoryService := accesssvc.NewUserAccessHistoryService(userAccessHistoryRepo)
	workspaceInjectableService := injectablesvc.NewWorkspaceInjectableService(workspaceInjectableRepo)
	systemInjectableService := injectablesvc.NewSystemInjectableService(systemInjectableRepo, nil)

	// Create services - Content
	injectableService := injectablesvc.NewInjectableService(injectableRepo, systemInjectableRepo, nil, workspaceRepo, tenantRepo, nil)

	// Create content validator
	contentValidator := contentvalidator.New(injectableService)

	// Create services - Content
	templateService := templatesvc.NewTemplateService(templateRepo, templateVersionRepo, templateTagRepo, nil)
	templateVersionService := templatesvc.NewTemplateVersionService(
		templateVersionRepo,
		templateVersionInjectableRepo,
		templateVersionSignerRoleRepo,
		templateRepo,
		templateTagRepo,
		contentValidator,
		workspaceRepo,
	)

	// Create repositories - Document/Execution
	docRepo := documentrepo.New(pool)
	docRecipientRepo := documentrecipientrepo.New(pool)
	docEventRepo := documenteventrepo.New(pool)

	// Mock signing provider
	mockSigningAdapter := mocksigning.New()

	// Mock PDF renderer
	mockPDFRenderer := &MockPDFRenderer{}

	// Local storage in temp dir
	storageDir := t.TempDir()
	storageAdapter, err := localstorage.New(storageDir)
	require.NoError(t, err, "failed to create local storage adapter")

	// Access token repo + field response repo
	docAccessTokenRepo := documentaccesstokenrepo.New(pool)
	docFieldResponseRepo := documentfieldresponserepo.New(pool)
	signingAttemptRepo := signingattemptrepo.New(pool)

	// Event emitter + notification
	eventEmitter := documentsvc.NewEventEmitter(docEventRepo)
	noopNotifier := noopnotification.New()
	testPublicURL := "http://localhost:8080"
	galleryService := gallerysvc.New(storageAdapter, galleryRepo, testPublicURL)
	notificationSvc := documentsvc.NewNotificationService(noopNotifier, docRecipientRepo, docRepo, docAccessTokenRepo, testPublicURL)

	// River attempt UoW in insert-only mode for integration helpers.
	riverSvc, err := riverqueue.New(context.Background(), pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{
		DocumentRepo:      docRepo,
		RecipientRepo:     docRecipientRepo,
		AttemptRepo:       signingAttemptRepo,
		VersionRepo:       templateVersionRepo,
		SignerRoleRepo:    templateVersionSignerRoleRepo,
		FieldResponseRepo: docFieldResponseRepo,
		PDFRenderer:       mockPDFRenderer,
		SigningProvider:   mockSigningAdapter,
		StorageAdapter:    storageAdapter,
		StorageEnabled:    true,
	})
	require.NoError(t, err)

	// Document service
	documentService := documentsvc.NewDocumentService(
		docRepo,
		docRecipientRepo,
		signingAttemptRepo,
		riverSvc.SigningExecutionUOW(),
		templateRepo,
		templateVersionRepo,
		templateVersionSignerRoleRepo,
		mockPDFRenderer,
		mockSigningAdapter,
		storageAdapter,
		eventEmitter,
		notificationSvc,
		30, // expirationDays
		docAccessTokenRepo,
		docFieldResponseRepo,
		true,
	)

	// Pre-signing service
	preSigningService := documentsvc.NewPreSigningService(
		docAccessTokenRepo, docFieldResponseRepo,
		docRepo, docRecipientRepo, signingAttemptRepo, riverSvc.SigningExecutionUOW(), templateVersionRepo, templateVersionSignerRoleRepo,
		mockPDFRenderer, mockSigningAdapter, storageAdapter, true, eventEmitter,
		testPublicURL,
	)

	// Internal create service infrastructure
	injReg := registry.NewInjectorRegistry(nil)
	mapReg := registry.NewMapperRegistry()
	require.NoError(t, mapReg.Set(&TestRequestMapper{}), "failed to set test mapper")
	injectableResolver := injectablesvc.NewInjectableResolverService(injReg)
	documentGenerator := documentsvc.NewDocumentGenerator(
		templateRepo,
		templateVersionRepo,
		docRepo,
		docRecipientRepo,
		injectableService,
		mapReg,
		injectableResolver,
	)
	internalDocService := documentsvc.NewInternalDocumentService(
		documentGenerator,
		docRepo,
		tenantRepo,
		workspaceRepo,
		documentTypeRepo,
		templateRepo,
		templateVersionRepo,
		templateResolver,
	)
	internalDocController := controller.NewInternalDocumentController(internalDocService)

	// Create mappers
	injectableMapper := mapper.NewInjectableMapper()
	templateVersionMapper := mapper.NewTemplateVersionMapper(injectableMapper)
	tagMapper := mapper.NewTagMapper()
	folderMapper := mapper.NewFolderMapper()
	templateMapper := mapper.NewTemplateMapper(templateVersionMapper, tagMapper, folderMapper)
	workspaceInjectableMapper := mapper.NewInjectableMapper()

	// Create middleware provider (nil pool + bootstrap disabled for tests)
	middlewareProvider := middleware.NewProvider(
		nil, false,
		userRepo,
		systemRoleRepo,
		workspaceRepo,
		workspaceMemberRepo,
		tenantMemberRepo,
	)

	// Create controllers - Admin, Me, Tenant, Workspace
	adminController := controller.NewAdminController(tenantService, systemRoleService, systemInjectableService)
	meController := controller.NewMeController(tenantService, tenantMemberRepo, workspaceMemberRepo, workspaceRepo, userAccessHistoryService)
	tenantController := controller.NewTenantController(tenantService, workspaceService, tenantMemberService)
	workspaceController := controller.NewWorkspaceController(
		workspaceService,
		folderService,
		tagService,
		workspaceMemberService,
		workspaceInjectableService,
		workspaceInjectableMapper,
	)

	// Create controllers - Content
	// Note: RenderController is nil for tests (no PDF renderer configured)
	templateVersionController := controller.NewTemplateVersionController(templateVersionService, templateVersionMapper, templateMapper, nil)
	injectableController := controller.NewContentInjectableController(
		injectableService,
		injectableMapper,
	)
	templateController := controller.NewContentTemplateController(
		templateService,
		templateMapper,
		templateVersionController,
	)
	galleryController := controller.NewGalleryController(galleryService)

	// Create controllers - Document & Webhook
	documentController := controller.NewDocumentController(documentService, preSigningService, eventEmitter)
	// Document access service (email-verification gate)
	documentAccessService := documentsvc.NewDocumentAccessService(
		docRepo, docRecipientRepo, templateVersionRepo, docAccessTokenRepo,
		notificationSvc, testPublicURL, 3, 60, 48,
	)
	publicDocAccessController := controller.NewPublicDocumentAccessController(documentAccessService)
	publicSigningController := controller.NewPublicSigningController(preSigningService, documentAccessService, testPublicURL)
	webhookHandlers := map[string]port.WebhookHandler{
		"mock": mockSigningAdapter,
	}
	webhookController := controller.NewWebhookController(documentService, webhookHandlers)

	// Build engine with middleware chain
	engine := gin.New()
	engine.Use(gin.Recovery())

	// Empty auth config = dev mode (no panel OIDC provider, uses ParseUnverified)
	// This allows unsigned test tokens to be accepted
	authCfg := &config.AuthConfig{}

	// API v1 group with middleware chain
	v1 := engine.Group("/api/v1")
	v1.Use(middleware.Operation())
	var providers []config.OIDCProvider
	if panel := authCfg.GetPanelOIDC(); panel != nil {
		providers = append(providers, *panel)
	}
	v1.Use(middleware.MultiOIDCAuth(providers))
	v1.Use(middlewareProvider.IdentityContext())
	v1.Use(middlewareProvider.SystemRoleContext())

	// Register routes
	adminController.RegisterRoutes(v1)
	meController.RegisterRoutes(v1)
	tenantController.RegisterRoutes(v1, middlewareProvider)
	workspaceController.RegisterRoutes(v1, middlewareProvider)
	injectableController.RegisterRoutes(v1, middlewareProvider)
	templateController.RegisterRoutes(v1, middlewareProvider)

	// Document routes (within workspace context)
	wsGroup := v1.Group("", middlewareProvider.WorkspaceContext())
	documentController.RegisterRoutes(wsGroup)
	galleryController.RegisterRoutes(v1, middlewareProvider)

	// --- Automation infrastructure (created early, needed by internal routes) ---
	automationKeyRepo := automationapikeyrepo.New(pool)
	automationAuditRepo := automationauditlogrepo.New(pool)

	// Seed a test internal API key in the DB so InternalKeyAuth middleware can validate it.
	seedTestInternalAPIKey(t, automationKeyRepo)

	// Internal API routes (DB-backed API key auth, no JWT middleware)
	internalV1 := engine.Group("/api/v1")
	internalV1.Use(middleware.Operation())
	internalDocController.RegisterRoutes(internalV1, middleware.InternalKeyAuth(automationKeyRepo))

	// Webhook routes (no auth, registered on engine root)
	webhookController.RegisterRoutes(engine)

	// Public document access routes (email-verification gate, no auth)
	publicDocAccessController.RegisterRoutes(engine)

	// Public signing routes (no auth, registered on engine root)
	publicSigningController.RegisterRoutes(engine)
	apiKeyUseCase := automationuc.NewAPIKeyUseCase(automationKeyRepo, automationAuditRepo)
	documentTypeService := catalogsvc.NewDocumentTypeService(documentTypeRepo, templateRepo)
	docTypeMapper := mapper.NewDocumentTypeMapper()

	automationKeyCtrl := controller.NewAutomationKeyController(apiKeyUseCase)
	automationCtrl := controller.NewAutomationController(
		tenantService, workspaceService, injectableService,
		templateService, templateVersionService, documentTypeService,
		automationKeyRepo, automationAuditRepo,
		templateMapper, templateVersionMapper, injectableMapper, docTypeMapper,
	)

	// Register automation routes
	automationKeyCtrl.RegisterRoutes(v1)
	automationCtrl.RegisterRoutes(engine, middlewareProvider)

	// Create test server
	server := httptest.NewServer(engine)
	t.Cleanup(func() { server.Close() })

	return &TestServer{
		Server:             server,
		Engine:             engine,
		Pool:               pool,
		MockSigningAdapter: mockSigningAdapter,
		t:                  t,
	}
}

// URL returns the base URL of the test server.
func (ts *TestServer) URL() string {
	return ts.Server.URL
}

// Close closes the test server.
func (ts *TestServer) Close() {
	ts.Server.Close()
}

// seedTestInternalAPIKey inserts a pre-defined internal API key into the DB
// so the InternalKeyAuth middleware can authenticate requests using TestInternalAPIKey.
// Uses create-and-handle-error to avoid TOCTOU race when parallel test packages
// share the same DB singleton.
func seedTestInternalAPIKey(t *testing.T, repo port.AutomationAPIKeyRepository) {
	t.Helper()
	sum := sha256.Sum256([]byte(TestInternalAPIKey))
	keyHash := hex.EncodeToString(sum[:])

	keyPrefix := TestInternalAPIKey
	if len(keyPrefix) > 12 {
		keyPrefix = keyPrefix[:12]
	}
	_, err := repo.Create(context.Background(), &entity.AutomationAPIKey{
		Name:      "Test Internal Key",
		KeyHash:   keyHash,
		KeyPrefix: keyPrefix,
		KeyType:   entity.KeyTypeInternal,
		IsActive:  true,
		CreatedBy: "00000000-0000-0000-0000-000000000000",
	})
	// Idempotent: unique constraint violation means key already seeded.
	if err != nil && !strings.Contains(err.Error(), "uq_api_keys_key_hash") {
		require.NoError(t, err, "seed test internal API key")
	}
}
