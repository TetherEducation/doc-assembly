//nolint:gosec
package config

import (
	"strings"
	"time"
)

// Config represents the complete application configuration.
type Config struct {
	Environment        string                   `mapstructure:"environment"`
	EnvironmentAliases map[string][]string      `mapstructure:"environment_aliases"`
	Server             ServerConfig             `mapstructure:"server"`
	Database           DatabaseConfig           `mapstructure:"database"`
	Auth               AuthConfig               `mapstructure:"auth"`
	SigningSessionAuth SigningSessionAuthConfig `mapstructure:"signing_session_auth"`
	InternalAPI        InternalAPIConfig        `mapstructure:"internal_api"`
	Signing            SigningConfig            `mapstructure:"signing"`
	Storage            StorageConfig            `mapstructure:"storage"`
	Logging            LoggingConfig            `mapstructure:"logging"`
	Typst              TypstConfig              `mapstructure:"typst"`
	Bootstrap          BootstrapConfig          `mapstructure:"bootstrap"`
	Scheduler          SchedulerConfig          `mapstructure:"scheduler"`
	Notification       NotificationConfig       `mapstructure:"notification"`
	PublicAccess       PublicAccessConfig       `mapstructure:"public_access"`
	Worker             WorkerConfig             `mapstructure:"worker"`
	InjectableSources  InjectableSourcesConfig  `mapstructure:"injectable_sources"`

	// DummyAuthUserID is the internal DB user ID for dummy auth mode.
	// Set at runtime after seeding the dummy user (not loaded from YAML).
	DummyAuthUserID string `mapstructure:"-"`
}

// ServerConfig holds HTTP server configuration.
type ServerConfig struct {
	Port                        string     `mapstructure:"port"`
	BasePath                    string     `mapstructure:"base_path"`
	PublicURL                   string     `mapstructure:"public_url"` // Public-facing URL for signing links in emails
	ReadTimeout                 int        `mapstructure:"read_timeout"`
	WriteTimeout                int        `mapstructure:"write_timeout"`
	ShutdownTimeout             int        `mapstructure:"shutdown_timeout"`
	SwaggerUI                   bool       `mapstructure:"swagger_ui"`
	CORS                        CORSConfig `mapstructure:"cors"`
	PublicSigningFrameAncestors []string   `mapstructure:"public_signing_frame_ancestors"`
}

// NormalizedBasePath returns the base path with leading slash and no trailing slash.
// Returns empty string if no base path is configured.
func (s ServerConfig) NormalizedBasePath() string {
	bp := strings.TrimSpace(s.BasePath)
	if bp == "" || bp == "/" {
		return ""
	}
	if !strings.HasPrefix(bp, "/") {
		bp = "/" + bp
	}
	return strings.TrimRight(bp, "/")
}

// CORSConfig holds CORS configuration.
type CORSConfig struct {
	AllowedOrigins []string `mapstructure:"allowed_origins"`
	AllowedHeaders []string `mapstructure:"allowed_headers"`
}

// ReadTimeoutDuration returns the read timeout as time.Duration.
func (s ServerConfig) ReadTimeoutDuration() time.Duration {
	return time.Duration(s.ReadTimeout) * time.Second
}

// WriteTimeoutDuration returns the write timeout as time.Duration.
func (s ServerConfig) WriteTimeoutDuration() time.Duration {
	return time.Duration(s.WriteTimeout) * time.Second
}

// ShutdownTimeoutDuration returns the shutdown timeout as time.Duration.
func (s ServerConfig) ShutdownTimeoutDuration() time.Duration {
	return time.Duration(s.ShutdownTimeout) * time.Second
}

// DatabaseConfig holds PostgreSQL connection configuration.
type DatabaseConfig struct {
	Host               string `mapstructure:"host"`
	Port               int    `mapstructure:"port"`
	User               string `mapstructure:"user"`
	Password           string `mapstructure:"password"` //nolint:gosec
	Name               string `mapstructure:"name"`
	SSLMode            string `mapstructure:"ssl_mode"`
	MaxPoolSize        int    `mapstructure:"max_pool_size"`
	MinPoolSize        int    `mapstructure:"min_pool_size"`
	MaxIdleTimeSeconds int    `mapstructure:"max_idle_time_seconds"`
}

// MaxIdleTimeDuration returns the max idle time as time.Duration.
func (d DatabaseConfig) MaxIdleTimeDuration() time.Duration {
	return time.Duration(d.MaxIdleTimeSeconds) * time.Second
}

// AuthConfig groups authentication configuration.
type AuthConfig struct {
	// Dummy forces dummy auth mode (bypass JWT), regardless of OIDC config.
	// Set via DOC_ENGINE_AUTH_DUMMY=true or make run DUMMY=1.
	Dummy bool `mapstructure:"dummy"`
	// Panel is the OIDC provider for web panel login and panel API routes.
	Panel *OIDCProvider `mapstructure:"panel"`
}

// Signing session auth mode values.
const (
	SigningSessionAuthModeOIDC   = "oidc"
	SigningSessionAuthModeCustom = "custom"
)

// SigningSessionAuthConfig configures authentication for
// /api/v1/signing-sessions/:documentId.
type SigningSessionAuthConfig struct {
	Mode string                       `mapstructure:"mode"`
	OIDC SigningSessionOIDCAuthConfig `mapstructure:"oidc"`
}

// NormalizedMode returns the signing session auth mode in lowercase.
func (c SigningSessionAuthConfig) NormalizedMode() string {
	return strings.ToLower(strings.TrimSpace(c.Mode))
}

// SigningSessionOIDCAuthConfig configures SDK-managed OIDC auth mode.
type SigningSessionOIDCAuthConfig struct {
	Provider   string `mapstructure:"provider"`    // "panel" or configured provider name
	EmailClaim string `mapstructure:"email_claim"` // claim key used as recipient email
}

// GetPanelOIDC returns the OIDC provider for panel (login/UI) authentication.
// Returns nil if the provider is not meaningfully configured (no issuer and no discovery URL).
func (a *AuthConfig) GetPanelOIDC() *OIDCProvider {
	if a != nil && a.Panel != nil && (a.Panel.Issuer != "" || a.Panel.DiscoveryURL != "") {
		return a.Panel
	}
	return nil
}

// IsDummyAuth returns true if dummy mode is forced or no panel OIDC provider is configured.
func (a *AuthConfig) IsDummyAuth() bool {
	if a.Dummy {
		return true
	}
	return a.GetPanelOIDC() == nil
}

// OIDCProvider represents a single OIDC identity provider configuration.
type OIDCProvider struct {
	Name         string `mapstructure:"name"`          // Human-readable name for logging
	DiscoveryURL string `mapstructure:"discovery_url"` // OpenID Connect discovery URL (optional)
	Issuer       string `mapstructure:"issuer"`        // Expected token issuer (iss claim)
	JWKSURL      string `mapstructure:"jwks_url"`      // JWKS endpoint URL
	Audience     string `mapstructure:"audience"`      // Optional audience (aud claim)

	// Frontend OIDC endpoints (populated from discovery or manual config)
	ClientID           string `mapstructure:"client_id"`            // OIDC client ID for frontend
	TokenEndpoint      string `mapstructure:"token_endpoint"`       // Token endpoint URL
	UserinfoEndpoint   string `mapstructure:"userinfo_endpoint"`    // Userinfo endpoint URL
	EndSessionEndpoint string `mapstructure:"end_session_endpoint"` // Logout/end session endpoint URL
}

// InternalAPIConfig holds configuration for internal service-to-service API.
type InternalAPIConfig struct {
	Enabled        bool   `mapstructure:"enabled"`
	APIKeyAuthMode string `mapstructure:"api_key_auth_mode"`
}

const (
	InternalAPIKeyAuthModeMemory = "memory"
	InternalAPIKeyAuthModeDB     = "db"
)

// NormalizedAPIKeyAuthMode returns the configured internal API key auth mode.
func (c InternalAPIConfig) NormalizedAPIKeyAuthMode() string {
	mode := strings.ToLower(strings.TrimSpace(c.APIKeyAuthMode))
	if mode == "" {
		return InternalAPIKeyAuthModeMemory
	}
	return mode
}

// SigningConfig holds signing provider configuration.
type SigningConfig struct {
	Provider       string `mapstructure:"provider"` // documenso
	APIKey         string `mapstructure:"api_key"`  //nolint:gosec
	BaseURL        string `mapstructure:"base_url"`
	SigningBaseURL string `mapstructure:"signing_base_url"` // Base URL for signing links (without /api/v2)
	WebhookSecret  string `mapstructure:"webhook_secret"`
	WebhookURL     string `mapstructure:"webhook_url"` // Public URL for webhook endpoint
}

// StorageConfig holds object storage configuration.
type StorageConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Provider string `mapstructure:"provider"`  // "s3" or "local"
	LocalDir string `mapstructure:"local_dir"` // Base directory for local storage
	Bucket   string `mapstructure:"bucket"`
	Region   string `mapstructure:"region"`
	Endpoint string `mapstructure:"endpoint"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// InjectableSourcesConfig holds source selection policy for environment-based injectable resolution.
type InjectableSourcesConfig struct {
	EnvResolution map[string]InjectableEnvConfig `mapstructure:"env_resolution"`
}

// InjectableEnvConfig configures datasource resolution order for one canonical environment key.
type InjectableEnvConfig struct {
	Order []string `mapstructure:"order"`
}

// TypstConfig holds Typst-based PDF renderer configuration.
type TypstConfig struct {
	BinPath                      string   `mapstructure:"bin_path"`
	TimeoutSeconds               int      `mapstructure:"timeout_seconds"`
	FontDirs                     []string `mapstructure:"font_dirs"`
	MaxConcurrent                int      `mapstructure:"max_concurrent"`
	AcquireTimeoutSeconds        int      `mapstructure:"acquire_timeout_seconds"`
	TemplateCacheTTL             int      `mapstructure:"template_cache_ttl_seconds"`
	TemplateCacheMax             int      `mapstructure:"template_cache_max_entries"`
	ImageCacheDir                string   `mapstructure:"image_cache_dir"`
	ImageCacheMaxAgeSeconds      int      `mapstructure:"image_cache_max_age_seconds"`
	ImageCacheCleanupIntervalSec int      `mapstructure:"image_cache_cleanup_interval_seconds"`
}

// TimeoutDuration returns the compilation timeout as time.Duration.
func (t TypstConfig) TimeoutDuration() time.Duration {
	return time.Duration(t.TimeoutSeconds) * time.Second
}

// AcquireTimeoutDuration returns the semaphore acquire timeout as time.Duration.
func (t TypstConfig) AcquireTimeoutDuration() time.Duration {
	return time.Duration(t.AcquireTimeoutSeconds) * time.Second
}

// ImageCacheMaxAgeDuration returns the image cache TTL as time.Duration.
func (t TypstConfig) ImageCacheMaxAgeDuration() time.Duration {
	return time.Duration(t.ImageCacheMaxAgeSeconds) * time.Second
}

// ImageCacheCleanupIntervalDuration returns the cache cleanup interval as time.Duration.
func (t TypstConfig) ImageCacheCleanupIntervalDuration() time.Duration {
	return time.Duration(t.ImageCacheCleanupIntervalSec) * time.Second
}

// SchedulerConfig holds background job scheduler configuration.
type SchedulerConfig struct {
	Enabled            bool `mapstructure:"enabled"`
	PollingIntervalSec int  `mapstructure:"polling_interval_sec"`
	PollingBatchSize   int  `mapstructure:"polling_batch_size"`
	ExpirationDays     int  `mapstructure:"expiration_days"`
	RetryMaxRetries    int  `mapstructure:"retry_max_retries"`
	RetryBatchSize     int  `mapstructure:"retry_batch_size"`
	RetryIntervalSec   int  `mapstructure:"retry_interval_sec"`
}

// PollingIntervalDuration returns the polling interval as time.Duration.
func (s SchedulerConfig) PollingIntervalDuration() time.Duration {
	return time.Duration(s.PollingIntervalSec) * time.Second
}

// RetryIntervalDuration returns the retry job interval as time.Duration.
func (s SchedulerConfig) RetryIntervalDuration() time.Duration {
	return time.Duration(s.RetryIntervalSec) * time.Second
}

// BootstrapConfig holds first-user bootstrap configuration.
type BootstrapConfig struct {
	// Enabled controls whether the first user to login is auto-created as SUPERADMIN.
	// Only takes effect when the database has zero users.
	Enabled bool `mapstructure:"enabled"`
}

// NotificationConfig holds email notification configuration.
type NotificationConfig struct {
	Provider string `mapstructure:"provider"` // "smtp" or "noop"
	From     string `mapstructure:"from"`     // Sender email address
	Host     string `mapstructure:"host"`     // SMTP host
	Port     int    `mapstructure:"port"`     // SMTP port
	Username string `mapstructure:"username"` // SMTP username
	Password string `mapstructure:"password"` // SMTP password //nolint:gosec
}

// PublicAccessConfig holds configuration for public document access (email-verification gate).
type PublicAccessConfig struct {
	RateLimitMax       int `mapstructure:"rate_limit_max"`        // Max access requests per recipient per window
	RateLimitWindowMin int `mapstructure:"rate_limit_window_min"` // Rate limit window in minutes
	TokenTTLHours      int `mapstructure:"token_ttl_hours"`       // Access token TTL in hours
}

// WorkerConfig holds River job queue worker configuration.
type WorkerConfig struct {
	Enabled           bool     `mapstructure:"enabled"`
	MaxWorkers        int      `mapstructure:"max_workers"`
	FailpointsEnabled bool     `mapstructure:"failpoints_enabled"`
	Failpoints        []string `mapstructure:"failpoints"`

	// RuntimeEnvironment is set by bootstrap from Config.Environment and is not loaded directly.
	RuntimeEnvironment string `mapstructure:"-"`
}

// MaxWorkersOrDefault returns MaxWorkers if set, otherwise defaults to 10.
func (w WorkerConfig) MaxWorkersOrDefault() int {
	if w.MaxWorkers > 0 {
		return w.MaxWorkers
	}
	return 10
}
