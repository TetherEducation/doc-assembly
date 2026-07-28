package server

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/controller"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	"github.com/TetherEducation/doc-assembly/core/internal/infra/config"

	_ "github.com/TetherEducation/doc-assembly/core/docs" // swagger generated docs
)

func init() {
	// Register MIME types to avoid OS-level detection inconsistencies (especially on Windows).
	_ = mime.AddExtensionType(".js", "application/javascript")
	_ = mime.AddExtensionType(".css", "text/css")
	_ = mime.AddExtensionType(".woff2", "font/woff2")
	_ = mime.AddExtensionType(".svg", "image/svg+xml")
}

// @title           Doc Engine API
// @version         1.0
// @description     Document Assembly System API - Template management and document generation

// @contact.name    API Support
// @contact.email   support@example.com

// @license.name    MIT
// @license.url     https://opensource.org/licenses/MIT

// @host            localhost:8080
// @BasePath        /api/v1

// @securityDefinitions.apikey BearerAuth
// @in              header
// @name            Authorization
// @description     Type "Bearer" followed by a space and JWT token

// @securityDefinitions.apikey AutomationKey
// @in              header
// @name            X-Automation-Key
// @description     Automation API key for programmatic access

// HTTPServer represents the HTTP server instance.
type HTTPServer struct {
	engine *gin.Engine
	config *config.ServerConfig
}

// NewHTTPServer creates a new HTTP server with all routes and middleware configured.
//
//nolint:funlen // constructor wiring — many injected controllers
func NewHTTPServer(
	cfg *config.Config,
	middlewareProvider *middleware.Provider,
	workspaceController *controller.WorkspaceController,
	injectableController *controller.ContentInjectableController,
	templateController *controller.ContentTemplateController,
	adminController *controller.AdminController,
	meController *controller.MeController,
	tenantController *controller.TenantController,
	documentTypeController *controller.DocumentTypeController,
	processController *controller.ProcessController,
	documentController *controller.DocumentController,
	webhookController *controller.WebhookController,
	internalDocController *controller.InternalDocumentController,
	publicDocAccessController *controller.PublicDocumentAccessController,
	publicSigningController *controller.PublicSigningController,
	publicReadOnlyViewController *controller.PublicReadOnlyViewController,
	signingSessionController *controller.SigningSessionController,
	automationKeyController *controller.AutomationKeyController,
	automationController *controller.AutomationController,
	galleryController *controller.GalleryController,
	publicDocAuthenticator port.PublicDocumentAccessAuthenticator,
	signingSessionAuthenticator port.SigningSessionAuthenticator,
	readOnlyViewLinkAuthenticator port.ReadOnlyViewLinkAuthenticator,
	legacyDocumentHandler port.LegacyDocumentHandler,
	keyRepo port.AutomationAPIKeyRepository,
	frontendFS fs.FS,
) *HTTPServer {
	if cfg.Environment == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.New()
	engine.Use(gin.Recovery())
	engine.Use(gin.Logger())
	engine.Use(corsMiddleware(cfg.Server.CORS))

	// Base path group (e.g. "/doc-assembly" → all routes under /doc-assembly/*)
	basePath := cfg.Server.NormalizedBasePath()
	var base gin.IRouter = &engine.RouterGroup
	if basePath != "" {
		base = engine.Group(basePath)
	}

	// Public signing/document pages are rendered by SPA on browser navigation,
	// but keep JSON responses for API clients on the same endpoints.
	if frontendFS != nil {
		base.Use(publicPageEntrySPAMiddleware(frontendFS, basePath))
	}

	base.GET("/health", healthHandler)
	base.GET("/ready", readyHandler)
	base.GET("/api/v1/config", clientConfigHandler(cfg))
	if cfg.Server.SwaggerUI {
		base.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	}

	registerInternalRoutes(base, cfg, internalDocController, keyRepo)

	requestTimeout := cfg.Server.WriteTimeoutDuration() - 2*time.Second
	if requestTimeout <= 0 {
		requestTimeout = 28 * time.Second
	}

	v1 := setupPanelRoutes(base, cfg, middlewareProvider, requestTimeout)
	registerPanelControllers(v1, middlewareProvider, adminController, meController,
		tenantController, documentTypeController, processController, workspaceController,
		injectableController, templateController, documentController, galleryController,
		readOnlyViewLinkAuthenticator != nil)
	automationKeyController.RegisterRoutes(v1)
	registerSigningSessionRoutes(base, cfg, requestTimeout, signingSessionController, signingSessionAuthenticator)
	registerReadOnlyViewLinkRoute(base, requestTimeout, documentController, readOnlyViewLinkAuthenticator)
	registerLegacyDocumentRoute(base, cfg, requestTimeout, legacyDocumentHandler)

	webhookController.RegisterRoutes(base)

	// Public document access routes (email-verification gate, no auth, no CSP needed).
	if publicDocAuthenticator != nil {
		publicDocAccessController.RegisterRoutes(
			base,
			middleware.CustomPublicDocumentAccess(publicDocAuthenticator),
		)
	} else {
		publicDocAccessController.RegisterRoutes(base)
	}

	// CSP middleware for public signing routes — allows iframe from signing provider domain.
	if cfg.Signing.SigningBaseURL != "" {
		base.Use(signingCSPMiddleware(cfg.Signing.SigningBaseURL, cfg.Server.PublicSigningFrameAncestors))
	}
	publicSigningController.RegisterRoutes(base)
	publicReadOnlyViewController.RegisterRoutes(base)
	automationController.RegisterRoutes(engine, middlewareProvider)

	// Serve embedded SPA if frontendFS is provided, otherwise return 404 for unmatched routes.
	if frontendFS != nil {
		engine.NoRoute(spaHandler(frontendFS, basePath))
	} else {
		engine.NoRoute(func(c *gin.Context) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		})
	}

	return &HTTPServer{
		engine: engine,
		config: &cfg.Server,
	}
}

// registerInternalRoutes registers internal API routes with configured API key authentication.
func registerInternalRoutes(router gin.IRouter, cfg *config.Config, internalDocController *controller.InternalDocumentController, keyRepo port.AutomationAPIKeyRepository) {
	if cfg.InternalAPI.Enabled {
		internalV1 := router.Group("/api/v1")
		internalV1.Use(middleware.Operation())
		authMode := cfg.InternalAPI.NormalizedAPIKeyAuthMode()
		authMiddleware := middleware.InternalKeyAuth(keyRepo)
		if authMode == config.InternalAPIKeyAuthModeMemory {
			authMiddleware = middleware.InternalKeyAuthPreloaded(keyRepo)
		}
		internalDocController.RegisterRoutes(internalV1, authMiddleware)
		slog.InfoContext(context.Background(), "internal API routes registered",
			slog.String("api_key_auth_mode", authMode))
	} else {
		slog.WarnContext(context.Background(), "internal API routes disabled")
	}
}

func registerSigningSessionRoutes(
	router gin.IRouter,
	cfg *config.Config,
	requestTimeout time.Duration,
	signingSessionController *controller.SigningSessionController,
	signingSessionAuthenticator port.SigningSessionAuthenticator,
) {
	if signingSessionController == nil {
		return
	}

	v1 := router.Group("/api/v1")
	v1.Use(noCacheAPI())
	v1.Use(middleware.Operation())
	v1.Use(middleware.RequestTimeout(requestTimeout))

	mode := cfg.SigningSessionAuth.NormalizedMode()
	switch mode {
	case config.SigningSessionAuthModeOIDC:
		provider, err := resolveSigningSessionOIDCProvider(cfg)
		if err != nil {
			slog.Error("failed to configure signing session OIDC auth", slog.Any("error", err))
			return
		}

		emailClaim := strings.TrimSpace(cfg.SigningSessionAuth.OIDC.EmailClaim)
		v1.Use(middleware.MultiOIDCAuth([]config.OIDCProvider{*provider}))
		v1.Use(middleware.SigningSessionOIDCClaims(emailClaim, provider.Name))
	case config.SigningSessionAuthModeCustom:
		v1.Use(middleware.SigningSessionCustomAuth(signingSessionAuthenticator))
	default:
		slog.Error("invalid signing session auth mode", slog.String("mode", mode))
		return
	}

	signingSessionController.RegisterRoutes(v1)
}

func registerReadOnlyViewLinkRoute(
	router gin.IRouter,
	requestTimeout time.Duration,
	documentController *controller.DocumentController,
	auth port.ReadOnlyViewLinkAuthenticator,
) {
	if documentController == nil || auth == nil {
		return
	}

	v1 := router.Group("/api/v1")
	v1.Use(noCacheAPI())
	v1.Use(middleware.Operation())
	v1.Use(middleware.RequestTimeout(requestTimeout))
	v1.POST(
		"/documents/:documentId/view-link",
		middleware.ReadOnlyViewLinkCustomAuth(auth),
		documentController.CreateReadOnlyViewLinkByWorkspaceCode,
	)
	v1.GET(
		"/documents/:documentId/print-pdf",
		middleware.ReadOnlyViewLinkCustomAuth(auth),
		documentController.GetDocumentPrintPDFByWorkspaceCode,
	)
}

func registerLegacyDocumentRoute(
	router gin.IRouter,
	cfg *config.Config,
	requestTimeout time.Duration,
	handler port.LegacyDocumentHandler,
) {
	if handler == nil {
		return
	}

	v1 := router.Group("/api/v1")
	v1.Use(noCacheAPI())
	v1.Use(middleware.Operation())
	v1.Use(middleware.RequestTimeout(requestTimeout))

	controller.NewLegacyDocumentController(
		handler,
		cfg.LegacyDocuments.MaxBodyBytesOrDefault(),
	).RegisterRoutes(v1)
}

func resolveSigningSessionOIDCProvider(cfg *config.Config) (*config.OIDCProvider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("missing config")
	}

	panel := cfg.Auth.GetPanelOIDC()
	if panel == nil {
		return nil, fmt.Errorf("signing_session_auth.mode=oidc requires auth.panel configuration")
	}

	requested := strings.TrimSpace(cfg.SigningSessionAuth.OIDC.Provider)
	if requested == "" || strings.EqualFold(requested, "panel") || strings.EqualFold(requested, panel.Name) {
		return panel, nil
	}

	return nil, fmt.Errorf("unsupported signing_session_auth.oidc.provider=%q (supported: panel or %q)", requested, panel.Name)
}

// setupPanelRoutes creates the panel route group with authentication middleware.
// Uses DummyAuth in dev mode or PanelAuth + IdentityContext + SystemRoleContext in production.
func setupPanelRoutes(
	router gin.IRouter,
	cfg *config.Config,
	middlewareProvider *middleware.Provider,
	requestTimeout time.Duration,
) *gin.RouterGroup {
	v1 := router.Group("/api/v1")
	v1.Use(noCacheAPI())
	v1.Use(middleware.Operation())
	v1.Use(middleware.RequestTimeout(requestTimeout))

	if cfg.Auth.IsDummyAuth() {
		v1.Use(middleware.DummyAuth())
		v1.Use(middleware.DummyIdentityAndRoles(cfg.DummyAuthUserID))
	} else {
		v1.Use(middleware.PanelAuth(&cfg.Auth))
		v1.Use(middlewareProvider.IdentityContext())
		v1.Use(middlewareProvider.SystemRoleContext())
	}

	return v1
}

// registerPanelControllers registers all panel route controllers.
func registerPanelControllers(
	v1 *gin.RouterGroup,
	middlewareProvider *middleware.Provider,
	adminController *controller.AdminController,
	meController *controller.MeController,
	tenantController *controller.TenantController,
	documentTypeController *controller.DocumentTypeController,
	processController *controller.ProcessController,
	workspaceController *controller.WorkspaceController,
	injectableController *controller.ContentInjectableController,
	templateController *controller.ContentTemplateController,
	documentController *controller.DocumentController,
	galleryController *controller.GalleryController,
	skipReadOnlyViewLinkRoute bool,
) {
	v1.GET("/ping", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
	})

	adminController.RegisterRoutes(v1)
	meController.RegisterRoutes(v1)
	tenantController.RegisterRoutes(v1, middlewareProvider)
	documentTypeController.RegisterRoutes(v1, middlewareProvider)
	processController.RegisterRoutes(v1, middlewareProvider)
	workspaceController.RegisterRoutes(v1, middlewareProvider)
	injectableController.RegisterRoutes(v1, middlewareProvider)
	templateController.RegisterRoutes(v1, middlewareProvider)

	// Document routes (within workspace context; sandbox via X-Sandbox-Mode)
	wsGroup := v1.Group("", middlewareProvider.WorkspaceContext(), middlewareProvider.SandboxContext())
	if skipReadOnlyViewLinkRoute {
		documentController.RegisterRoutes(wsGroup, controller.WithoutReadOnlyViewLinkRoute())
	} else {
		documentController.RegisterRoutes(wsGroup)
	}

	if galleryController != nil {
		galleryController.RegisterRoutes(v1, middlewareProvider)
	}
}

// Start starts the HTTP server.
func (s *HTTPServer) Start(ctx context.Context) error {
	addr := fmt.Sprintf(":%s", s.config.Port)

	srv := &http.Server{
		Addr:         addr,
		Handler:      s.engine,
		ReadTimeout:  s.config.ReadTimeoutDuration(),
		WriteTimeout: s.config.WriteTimeoutDuration(),
	}

	// Channel to catch server errors
	errChan := make(chan error, 1)

	// Start server in goroutine
	go func() {
		slog.InfoContext(ctx, "starting HTTP server", slog.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		slog.InfoContext(ctx, "shutting down HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.config.ShutdownTimeoutDuration())
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server shutdown: %w", err)
		}
		slog.InfoContext(shutdownCtx, "HTTP server stopped gracefully")
		return nil

	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	}
}

// Engine returns the underlying Gin engine.
// Useful for testing.
func (s *HTTPServer) Engine() *gin.Engine {
	return s.engine
}

// healthHandler returns OK if the service is running.
func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "doc-engine",
	})
}

// readyHandler returns OK if the service is ready to accept traffic.
func readyHandler(c *gin.Context) {
	// TODO: Add database connectivity check
	c.JSON(http.StatusOK, gin.H{
		"status": "ready",
	})
}

// clientConfigHandler returns a handler that exposes non-sensitive config to the frontend.
func clientConfigHandler(cfg *config.Config) gin.HandlerFunc {
	type providerInfo struct {
		Name               string `json:"name"`
		Issuer             string `json:"issuer"`
		TokenEndpoint      string `json:"tokenEndpoint,omitempty"`
		UserinfoEndpoint   string `json:"userinfoEndpoint,omitempty"`
		EndSessionEndpoint string `json:"endSessionEndpoint,omitempty"`
		ClientID           string `json:"clientId,omitempty"`
	}

	type features struct {
		Gallery bool `json:"gallery"`
	}

	type clientConfig struct {
		DummyAuth     bool          `json:"dummyAuth"`
		BasePath      string        `json:"basePath"`
		PanelProvider *providerInfo `json:"panelProvider,omitempty"`
		Features      features      `json:"features"`
	}

	var panelProvider *providerInfo
	if panel := cfg.Auth.GetPanelOIDC(); panel != nil {
		panelProvider = &providerInfo{
			Name:               panel.Name,
			Issuer:             panel.Issuer,
			TokenEndpoint:      panel.TokenEndpoint,
			UserinfoEndpoint:   panel.UserinfoEndpoint,
			EndSessionEndpoint: panel.EndSessionEndpoint,
			ClientID:           panel.ClientID,
		}
	}

	resp := clientConfig{
		DummyAuth:     cfg.Auth.IsDummyAuth(),
		BasePath:      cfg.Server.NormalizedBasePath(),
		PanelProvider: panelProvider,
		Features: features{
			Gallery: cfg.Storage.Enabled,
		},
	}

	return func(c *gin.Context) {
		c.JSON(http.StatusOK, resp)
	}
}

// noCacheAPI ensures browsers never cache API responses.
// Without explicit Cache-Control headers, Chrome applies heuristic caching to GET
// requests, which can cause stale or corrupted cache entries that result in requests
// stuck as "pending" indefinitely — even across page reloads.
func noCacheAPI() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Next()
	}
}

// corsMiddleware configures CORS for the API using allowed origins from config.
// Access-Control-Allow-Origin only accepts a single origin or "*".
// When multiple origins are configured, we check the request Origin header
// and respond with that origin if it's in the allowed list.
func corsMiddleware(corsCfg config.CORSConfig) gin.HandlerFunc {
	allowed := make(map[string]bool, len(corsCfg.AllowedOrigins))
	wildcard := false
	for _, o := range corsCfg.AllowedOrigins {
		if o == "*" {
			wildcard = true
		}
		allowed[o] = true
	}
	if len(corsCfg.AllowedOrigins) == 0 {
		wildcard = true
	}

	baseHeaders := make([]string, 0, 20+len(corsCfg.AllowedHeaders))
	baseHeaders = append(baseHeaders,
		"Origin", "Content-Type", "Accept", "Authorization",
		"Cache-Control", "Pragma",
		"X-Workspace-ID", "X-Tenant-ID", "X-Tenant-Code", "X-Sandbox-Mode", "X-API-Key",
		"X-Environment", "X-Automation-Key", "X-Operation-ID",
		"X-Workspace-Code", "X-Document-Type",
		"X-External-ID", "X-Transactional-ID",
		"X-Process", "X-Process-Type",
	)
	allowedHeaders := strings.Join(append(baseHeaders, corsCfg.AllowedHeaders...), ", ")

	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")

		if wildcard {
			c.Header("Access-Control-Allow-Origin", "*")
		} else if allowed[origin] {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
		}

		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", allowedHeaders)
		c.Header("Access-Control-Expose-Headers", "Content-Length")
		c.Header("Access-Control-Allow-Credentials", "true")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}

// publicPageEntrySPAMiddleware serves SPA index.html for browser navigations to
// public entry pages while preserving JSON behavior for API clients.
func publicPageEntrySPAMiddleware(frontendFS fs.FS, basePath string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if frontendFS == nil || c.Request.Method != http.MethodGet {
			c.Next()
			return
		}
		if !requestWantsHTML(c.GetHeader("Accept")) {
			c.Next()
			return
		}

		strippedPath, ok := stripBasePath(c.Request.URL.Path, basePath)
		if !ok || !isPublicPageEntryPath(strippedPath) {
			c.Next()
			return
		}

		serveIndexHTML(c, frontendFS)
		c.Abort()
	}
}

// signingCSPMiddleware adds Content-Security-Policy headers for pages that embed the signing
// provider in an iframe. Only applied to /public/sign/* routes.
func signingCSPMiddleware(signingBaseURL string, frameAncestors []string) gin.HandlerFunc {
	csp := buildSigningCSP(signingBaseURL, frameAncestors)
	if csp == "" {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/public/sign/") ||
			strings.Contains(c.Request.URL.Path, "/public/sign/") {
			c.Header("Content-Security-Policy", csp)
		}
		c.Next()
	}
}

func buildSigningCSP(signingBaseURL string, frameAncestors []string) string {
	frameSrc := normalizeCSPOrigin(signingBaseURL)
	if frameSrc == "" {
		return ""
	}

	ancestors := normalizeCSPOrigins(frameAncestors)
	ancestorsClause := "'self'"
	if len(ancestors) > 0 {
		ancestorsClause = strings.Join(ancestors, " ")
	}

	return fmt.Sprintf("frame-src %s; frame-ancestors %s", frameSrc, ancestorsClause)
}

func normalizeCSPOrigins(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))

	for _, value := range values {
		normalized := normalizeCSPOrigin(value)
		if normalized == "" {
			continue
		}

		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}

	return out
}

func normalizeCSPOrigin(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}

	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return ""
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return ""
	}

	return strings.ToLower(parsed.Scheme) + "://" + strings.ToLower(parsed.Host)
}

func requestWantsHTML(acceptHeader string) bool {
	for _, part := range strings.Split(strings.ToLower(acceptHeader), ",") {
		mediaType := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if mediaType == "text/html" || mediaType == "application/xhtml+xml" {
			return true
		}
	}
	return false
}

func isPublicPageEntryPath(p string) bool {
	trimmed := strings.Trim(strings.TrimSpace(strings.ToLower(p)), "/")
	if trimmed == "" {
		return false
	}

	parts := strings.Split(trimmed, "/")
	if len(parts) != 3 || parts[0] != "public" || parts[2] == "" {
		return false
	}

	return parts[1] == "sign" || parts[1] == "doc" || parts[1] == "view"
}

// stripBasePath removes the basePath prefix from reqPath.
// Returns the stripped path and true, or empty and false if the prefix doesn't match.
func stripBasePath(reqPath, basePath string) (string, bool) {
	if basePath == "" {
		return reqPath, true
	}
	if !strings.HasPrefix(reqPath, basePath) {
		return "", false
	}
	stripped := strings.TrimPrefix(reqPath, basePath)
	if stripped == "" {
		return "/", true
	}
	return stripped, true
}

// isBackendPath returns true if the path belongs to backend-owned prefixes.
func isBackendPath(p string) bool {
	return strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/swagger/") || strings.HasPrefix(p, "/webhooks/") || strings.HasPrefix(p, "/public/")
}

// spaHandler returns a Gin handler that serves the embedded SPA frontend.
// Explicit routes (/health, /ready, /api/v1/*) are matched by Gin before NoRoute.
// This handler only runs for unmatched paths: static files get served with cache
// headers, unknown paths get index.html (SPA client-side routing).
// basePath is stripped from the request URL before filesystem lookup.
func spaHandler(fsys fs.FS, basePath string) gin.HandlerFunc {
	var fileServer http.Handler
	if fsys != nil {
		fileServer = http.StripPrefix(basePath, http.FileServer(http.FS(fsys)))
	}

	return func(c *gin.Context) {
		stripped, ok := stripBasePath(c.Request.URL.Path, basePath)
		if !ok || isBackendPath(stripped) || fsys == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		// Normalize path for fs lookup
		cleanPath := path.Clean(strings.TrimPrefix(stripped, "/"))
		if cleanPath == "." || cleanPath == "" {
			cleanPath = "index.html"
		}

		// Try serving the exact file
		f, err := fsys.Open(cleanPath)
		if err == nil {
			_ = f.Close()
			if strings.HasPrefix(cleanPath, "assets/") {
				c.Header("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		// SPA fallback → serve index.html
		serveIndexHTML(c, fsys)
	}
}

// serveIndexHTML serves index.html with no-cache headers for SPA fallback routing.
func serveIndexHTML(c *gin.Context, fsys fs.FS) {
	indexFile, err := fsys.Open("index.html")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	defer func() { _ = indexFile.Close() }()

	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Content-Type", "text/html; charset=utf-8")
	stat, _ := indexFile.Stat()

	if rs, ok := indexFile.(io.ReadSeeker); ok {
		http.ServeContent(c.Writer, c.Request, "index.html", stat.ModTime(), rs)
		return
	}

	c.Writer.WriteHeader(http.StatusOK)
	_, _ = io.Copy(c.Writer, indexFile)
}
