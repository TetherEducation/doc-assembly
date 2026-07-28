package bootstrap

import (
	"context"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	"github.com/TetherEducation/doc-assembly/core/internal/core/service/rendering/pdfrenderer"
	"github.com/TetherEducation/doc-assembly/core/internal/infra/config"
	"github.com/TetherEducation/doc-assembly/core/internal/infra/logging"
	"github.com/TetherEducation/doc-assembly/core/internal/migrations"
)

// Engine is the main entry point for doc-assembly.
// Create with New(), register extensions, then call Run().
type Engine struct {
	configFilePath string
	config         *config.Config
	i18nFilePath   string

	injectors             []port.Injector
	mapper                port.RequestMapper
	templateResolver      port.TemplateResolver
	initFunc              port.InitFunc
	workspaceProvider     port.WorkspaceInjectableProvider
	publicDocAuth         port.PublicDocumentAccessAuthenticator
	signingSessionAuth    port.SigningSessionAuthenticator
	readOnlyViewAuth      port.ReadOnlyViewLinkAuthenticator
	legacyDocumentHandler port.LegacyDocumentHandler
	signingSessionMode    string
	designTokens          *pdfrenderer.TypstDesignTokens
	frontendFS            fs.FS // Embedded SPA filesystem; nil = no frontend served
	frontendOverridden    bool  // True if SetFrontendFS was called (even with nil)

	// Process resolver (optional)
	processResolver port.ProcessResolver

	// doc-assembly specific extension points
	signingProvider      port.SigningProvider
	storageAdapter       port.StorageAdapter
	notificationProvider port.NotificationProvider
	webhookHandlers      map[string]port.WebhookHandler

	// Middleware
	globalMiddleware []gin.HandlerFunc // Applied to all routes (after CORS, before auth)
	apiMiddleware    []gin.HandlerFunc // Applied to /api/v1/* routes (after auth)

	// Worker
	documentCompletedHandler port.DocumentCompletedHandler

	// Lifecycle hooks
	onStartHooks    []func(ctx context.Context) error // Run after config/preflight, before HTTP server
	onShutdownHooks []func(ctx context.Context) error // Run after HTTP server stops, before exit
}

// OnDocumentCompleted registers a handler that is called when a document
// reaches COMPLETED status. The handler runs inside a River job worker,
// so returning an error causes automatic retry with backoff.
func (e *Engine) OnDocumentCompleted(fn port.DocumentCompletedHandler) *Engine {
	e.documentCompletedHandler = fn
	return e
}

// New creates a new Engine with default configuration.
func New() *Engine {
	return &Engine{}
}

// NewWithConfig creates a new Engine that loads config from the given file path.
func NewWithConfig(configPath string) *Engine {
	return &Engine{
		configFilePath: configPath,
	}
}

// SetI18nFilePath sets the path to user-provided i18n translations file.
func (e *Engine) SetI18nFilePath(path string) *Engine {
	e.i18nFilePath = path
	return e
}

// RegisterInjector adds a custom injector to the engine.
// Multiple injectors can be registered.
func (e *Engine) RegisterInjector(inj port.Injector) *Engine {
	e.injectors = append(e.injectors, inj)
	return e
}

// SetMapper sets the request mapper for render requests.
// Only ONE mapper is supported.
func (e *Engine) SetMapper(m port.RequestMapper) *Engine {
	e.mapper = m
	return e
}

// SetTemplateResolver sets an optional custom template resolver for internal create flow.
func (e *Engine) SetTemplateResolver(r port.TemplateResolver) *Engine {
	e.templateResolver = r
	return e
}

// GetTemplateResolver returns the registered custom template resolver.
func (e *Engine) GetTemplateResolver() port.TemplateResolver {
	return e.templateResolver
}

// SetProcessResolver sets an optional process resolver for process validation and discovery.
func (e *Engine) SetProcessResolver(r port.ProcessResolver) *Engine {
	e.processResolver = r
	return e
}

// GetProcessResolver returns the registered process resolver.
func (e *Engine) GetProcessResolver() port.ProcessResolver {
	return e.processResolver
}

// SetInitFunc sets the global initialization function.
// Runs once before all injectors on each render request.
func (e *Engine) SetInitFunc(fn port.InitFunc) *Engine {
	e.initFunc = fn
	return e
}

// SetWorkspaceInjectableProvider sets the provider for workspace-specific injectables.
func (e *Engine) SetWorkspaceInjectableProvider(p port.WorkspaceInjectableProvider) *Engine {
	e.workspaceProvider = p
	return e
}

// SetPublicDocumentAccessAuthenticator sets custom authentication for
// /public/doc/:documentId.
// When auth succeeds, the request can bypass the email gate and be redirected
// directly to a tokenized /public/sign/:token URL.
func (e *Engine) SetPublicDocumentAccessAuthenticator(auth port.PublicDocumentAccessAuthenticator) *Engine {
	e.publicDocAuth = auth
	return e
}

// GetPublicDocumentAccessAuthenticator returns the registered public access
// authenticator, or nil if not set.
func (e *Engine) GetPublicDocumentAccessAuthenticator() port.PublicDocumentAccessAuthenticator {
	return e.publicDocAuth
}

// SetSigningSessionAuthenticator sets custom authentication for
// /api/v1/signing-sessions/:documentId when mode is "custom".
func (e *Engine) SetSigningSessionAuthenticator(auth port.SigningSessionAuthenticator) *Engine {
	e.signingSessionAuth = auth
	return e
}

// GetSigningSessionAuthenticator returns the registered signing session
// authenticator, or nil if not set.
func (e *Engine) GetSigningSessionAuthenticator() port.SigningSessionAuthenticator {
	return e.signingSessionAuth
}

// SetReadOnlyViewLinkAuthenticator sets custom authentication for
// /api/v1/documents/:documentId/view-link.
func (e *Engine) SetReadOnlyViewLinkAuthenticator(auth port.ReadOnlyViewLinkAuthenticator) *Engine {
	e.readOnlyViewAuth = auth
	return e
}

// GetReadOnlyViewLinkAuthenticator returns the registered read-only view link
// authenticator, or nil if not set.
func (e *Engine) GetReadOnlyViewLinkAuthenticator() port.ReadOnlyViewLinkAuthenticator {
	return e.readOnlyViewAuth
}

// SetLegacyDocumentHandler sets the host-defined handler for
// /api/v1/legacy-documents/proxy.
func (e *Engine) SetLegacyDocumentHandler(handler port.LegacyDocumentHandler) *Engine {
	e.legacyDocumentHandler = handler
	return e
}

// GetLegacyDocumentHandler returns the registered legacy document handler, or
// nil if not set.
func (e *Engine) GetLegacyDocumentHandler() port.LegacyDocumentHandler {
	return e.legacyDocumentHandler
}

// SetSigningSessionAuthMode overrides signing session auth mode configured via
// settings/app.yaml (signing_session_auth.mode).
func (e *Engine) SetSigningSessionAuthMode(mode string) *Engine {
	e.signingSessionMode = strings.TrimSpace(mode)
	return e
}

// SetFrontendFS overrides the embedded frontend filesystem.
// By default, the engine loads the embedded SPA from internal/frontend/dist.
// Pass a custom fs.FS to serve a different frontend, or nil to disable frontend serving.
func (e *Engine) SetFrontendFS(fsys fs.FS) *Engine {
	e.frontendFS = fsys
	e.frontendOverridden = true
	return e
}

// SetDesignTokens sets custom design tokens for PDF rendering.
// Controls fonts, colors, spacing, and heading styles in Typst output.
// If not set, DefaultDesignTokens() is used.
func (e *Engine) SetDesignTokens(tokens pdfrenderer.TypstDesignTokens) *Engine {
	e.designTokens = &tokens
	return e
}

// SetSigningProvider overrides the signing provider.
// Default: auto-selected from config (mock/documenso).
func (e *Engine) SetSigningProvider(sp port.SigningProvider) *Engine {
	e.signingProvider = sp
	return e
}

// SetStorageAdapter overrides the storage adapter.
// Default: auto-selected from config (local/s3).
func (e *Engine) SetStorageAdapter(sa port.StorageAdapter) *Engine {
	e.storageAdapter = sa
	return e
}

// SetNotificationProvider overrides the notification provider.
// Default: auto-selected from config (noop/smtp/gmail).
func (e *Engine) SetNotificationProvider(np port.NotificationProvider) *Engine {
	e.notificationProvider = np
	return e
}

// SetWebhookHandlers overrides the webhook handlers by provider name.
// Default: auto-selected from signing config.
func (e *Engine) SetWebhookHandlers(handlers map[string]port.WebhookHandler) *Engine {
	e.webhookHandlers = handlers
	return e
}

// UseMiddleware adds middleware to be applied globally to all routes.
// Execution order: Recovery -> Logger -> CORS -> [User Global Middleware] -> Routes
func (e *Engine) UseMiddleware(mw gin.HandlerFunc) *Engine {
	e.globalMiddleware = append(e.globalMiddleware, mw)
	return e
}

// UseAPIMiddleware adds middleware to /api/v1/* routes only.
// Execution order: Operation -> Auth -> Identity -> Roles -> [User API Middleware] -> Controller
func (e *Engine) UseAPIMiddleware(mw gin.HandlerFunc) *Engine {
	e.apiMiddleware = append(e.apiMiddleware, mw)
	return e
}

// OnStart registers a hook that runs AFTER config/preflight, BEFORE HTTP server starts.
// Hooks run synchronously in registration order.
func (e *Engine) OnStart(fn func(ctx context.Context) error) *Engine {
	e.onStartHooks = append(e.onStartHooks, fn)
	return e
}

// OnShutdown registers a hook that runs AFTER HTTP server stops, BEFORE exit.
// Hooks run synchronously in REVERSE registration order (LIFO).
func (e *Engine) OnShutdown(fn func(ctx context.Context) error) *Engine {
	e.onShutdownHooks = append(e.onShutdownHooks, fn)
	return e
}

// Run starts the engine: loads config, runs preflight checks,
// initializes all components, and starts the HTTP server.
// Blocks until shutdown signal (SIGINT/SIGTERM).
func (e *Engine) Run() error {
	ctx := context.Background()

	// Setup structured logging
	configureDefaultLogger(slog.LevelInfo)

	slog.InfoContext(ctx, "starting doc-assembly engine")

	// Load configuration
	if err := e.loadConfig(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	configuredLevel := parseSlogLevel(e.config.Logging.Level)
	configureDefaultLogger(configuredLevel)
	slog.InfoContext(ctx, "logger configured",
		"level", strings.ToLower(configuredLevel.String()),
	)

	// Initialize environment aliases from config
	entity.InitEnvironmentAliases(e.config.EnvironmentAliases)

	// Preflight checks
	if err := e.preflightChecks(ctx); err != nil {
		return err
	}

	// Initialize all components (manual DI)
	app, err := e.initialize(ctx)
	if err != nil {
		return fmt.Errorf("init: %w", err)
	}

	// Run with signal handling
	return e.runWithSignals(ctx, app)
}

func configureDefaultLogger(level slog.Level) {
	handler := logging.NewContextHandler(
		slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		}),
	)
	slog.SetDefault(slog.New(handler))
}

func parseSlogLevel(level string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// RunMigrations loads config and applies all pending database migrations.
func (e *Engine) RunMigrations() error {
	if err := e.loadConfig(); err != nil {
		return fmt.Errorf("config: %w", err)
	}
	return migrations.Run(&e.config.Database)
}

// loadConfig loads configuration from file or uses the provided config.
func (e *Engine) loadConfig() error {
	if e.config != nil {
		return nil
	}

	if e.configFilePath != "" {
		cfg, err := config.LoadFromFile(e.configFilePath)
		if err != nil {
			return err
		}
		e.config = cfg
		return nil
	}

	// Default: try standard locations
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	e.config = cfg
	return nil
}

// runWithSignals starts the app and waits for shutdown signal.
func (e *Engine) runWithSignals(ctx context.Context, app *appComponents) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Run OnStart hooks (sync, in registration order)
	for i, hook := range e.onStartHooks {
		if err := hook(ctx); err != nil {
			return fmt.Errorf("onStart hook %d: %w", i, err)
		}
	}

	// Start background scheduler
	app.scheduler.Start(ctx)

	// Start River job queue workers (if configured)
	if app.riverSvc != nil {
		if err := app.riverSvc.Start(ctx); err != nil {
			return fmt.Errorf("river: %w", err)
		}
	}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	errChan := make(chan error, 1)
	go func() {
		if err := app.httpServer.Start(ctx); err != nil {
			errChan <- err
		}
	}()

	// Startup banner
	port := e.config.Server.Port
	fmt.Println()
	fmt.Println("  doc-assembly is running")
	fmt.Println()
	fmt.Printf("  API:       http://localhost:%s/api/v1\n", port)
	if e.config.Server.SwaggerUI {
		fmt.Printf("  Swagger:   http://localhost:%s/swagger/index.html\n", port)
	}
	if app.hasFrontend {
		fmt.Printf("  Frontend:  http://localhost:%s\n", port)
	}
	fmt.Printf("  Health:    http://localhost:%s/health\n", port)
	fmt.Println()

	select {
	case sig := <-sigChan:
		slog.InfoContext(ctx, "received shutdown signal", slog.String("signal", sig.String()))
		cancel()
	case err := <-errChan:
		slog.ErrorContext(ctx, "server error", slog.String("error", err.Error()))
		return err
	}

	// Run OnShutdown hooks (sync, reverse order - LIFO)
	for i := len(e.onShutdownHooks) - 1; i >= 0; i-- {
		if err := e.onShutdownHooks[i](ctx); err != nil {
			slog.ErrorContext(ctx, "onShutdown hook error", slog.Int("hook", i), slog.Any("error", err))
		}
	}

	app.cleanup()
	slog.InfoContext(ctx, "doc-assembly engine stopped")
	return nil
}
