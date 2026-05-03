package config

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/viper"
)

const serverPublicSigningFrameAncestorsEnv = "DOC_ENGINE_SERVER_PUBLIC_SIGNING_FRAME_ANCESTORS"

// Load reads configuration from YAML files and environment variables.
// Environment variables take precedence over YAML values.
// Env prefix: DOC_ENGINE_ (e.g., DOC_ENGINE_SERVER_PORT)
func Load() (*Config, error) {
	v := viper.New()

	// Set config file settings
	v.SetConfigName("app")
	v.SetConfigType("yaml")

	// Add config paths (searched in order)
	v.AddConfigPath("./settings")
	v.AddConfigPath("../settings")
	v.AddConfigPath("../../settings")
	v.AddConfigPath(".")

	// Environment variable settings
	v.SetEnvPrefix("DOC_ENGINE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config file: %w", err)
		}
		// Config file not found is acceptable, we'll use env vars and defaults
	}

	// Set defaults
	setDefaults(v)

	// Unmarshal into struct
	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	// Special handling for PORT env var (common in container environments)
	if cfg.Server.Port == "" {
		if port := os.Getenv("PORT"); port != "" {
			cfg.Server.Port = port
		}
	}

	// Explicit env override for dummy auth flag.
	// Viper's AutomaticEnv does not reliably propagate nested env vars during Unmarshal.
	if os.Getenv("DOC_ENGINE_AUTH_DUMMY") == "true" {
		cfg.Auth.Dummy = true
	}

	// Set default DummyAuthUserID when in dummy auth mode.
	// Uses the well-known dummy UUID; override with DOC_ENGINE_DUMMY_AUTH_USER_ID env var.
	if cfg.Auth.IsDummyAuth() && cfg.DummyAuthUserID == "" {
		if id := os.Getenv("DOC_ENGINE_DUMMY_AUTH_USER_ID"); id != "" {
			cfg.DummyAuthUserID = id
		} else {
			cfg.DummyAuthUserID = "00000000-0000-0000-0000-000000000001"
		}
	}

	// Explicit env overrides for nested config (same Viper nested env issue).
	applyDatabaseEnvOverrides(&cfg.Database)
	applyServerEnvOverrides(&cfg.Server)
	applySigningEnvOverrides(&cfg.Signing)
	applyStorageEnvOverrides(&cfg.Storage)
	applyAuthPanelEnvOverrides(cfg.Auth.Panel)
	applySigningSessionAuthEnvOverrides(&cfg.SigningSessionAuth)
	applyBootstrapEnvOverrides(&cfg.Bootstrap)
	applyWorkerEnvOverrides(&cfg.Worker)

	// Run OIDC discovery to populate issuer/jwks_url from discovery endpoints.
	// Non-fatal: dev mode (no OIDC) and manual config still work.
	if err := cfg.Auth.DiscoverAll(context.Background()); err != nil {
		slog.WarnContext(context.Background(), "OIDC discovery failed (non-fatal)",
			slog.String("error", err.Error()))
	}

	return &cfg, nil
}

// setDefaults sets default configuration values.
func setDefaults(v *viper.Viper) {
	// Server defaults
	v.SetDefault("server.port", "8080")
	v.SetDefault("server.base_path", "")
	v.SetDefault("server.read_timeout", 30)
	v.SetDefault("server.write_timeout", 30)
	v.SetDefault("server.shutdown_timeout", 10)
	v.SetDefault("server.public_signing_frame_ancestors", []string{})

	// Database defaults
	v.SetDefault("database.host", "localhost")
	v.SetDefault("database.port", 5432)
	v.SetDefault("database.user", "postgres")
	v.SetDefault("database.name", "doc_engine")
	v.SetDefault("database.ssl_mode", "disable")
	v.SetDefault("database.max_pool_size", 10)
	v.SetDefault("database.min_pool_size", 2)
	v.SetDefault("database.max_idle_time", 300)

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")

	// Bootstrap defaults
	v.SetDefault("bootstrap.enabled", true)

	// Storage defaults
	v.SetDefault("storage.enabled", true)
	v.SetDefault("storage.provider", "local")
	v.SetDefault("storage.local_dir", "./data/storage")

	// Internal API defaults
	v.SetDefault("internal_api.enabled", true)

	// Worker defaults
	v.SetDefault("worker.enabled", false)
	v.SetDefault("worker.max_workers", 10)
	v.SetDefault("worker.failpoints_enabled", false)
	v.SetDefault("worker.failpoints", []string{})

	// Signing session auth defaults
	// NOTE: mode intentionally has no default and must be provided explicitly.
	v.SetDefault("signing_session_auth.oidc.provider", "panel")
	v.SetDefault("signing_session_auth.oidc.email_claim", "email")

	// Environment default
	v.SetDefault("environment", "development")

	// Environment aliases — map environment names to canonical dev/prod
	v.SetDefault("environment_aliases", map[string][]string{
		"dev":  {"dev", "develop", "development", "staging", "uat", "qa", "local", "sandbox"},
		"prod": {"prod", "production"},
	})

	// Injectable source selection by canonical environment.
	v.SetDefault("injectable_sources.env_resolution", map[string]map[string][]string{
		"dev": {
			"order": {"dev", "prod"},
		},
		"prod": {
			"order": {"prod"},
		},
	})
}

// applyDatabaseEnvOverrides reads DOC_ENGINE_DATABASE_* env vars into DatabaseConfig.
func applyDatabaseEnvOverrides(cfg *DatabaseConfig) {
	if cfg == nil {
		return
	}
	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_DATABASE_HOST")); v != "" {
		cfg.Host = v
	}
	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_DATABASE_PORT")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.Port = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_DATABASE_USER")); v != "" {
		cfg.User = v
	}
	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_DATABASE_PASSWORD")); v != "" {
		cfg.Password = v
	}
	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_DATABASE_NAME")); v != "" {
		cfg.Name = v
	}
	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_DATABASE_SSL_MODE")); v != "" {
		cfg.SSLMode = v
	}
}

// applySigningEnvOverrides reads DOC_ENGINE_SIGNING_* env vars into SigningConfig.
func applySigningEnvOverrides(cfg *SigningConfig) {
	if v := os.Getenv("DOC_ENGINE_SIGNING_PROVIDER"); v != "" {
		cfg.Provider = v
	}
	if v := os.Getenv("DOC_ENGINE_SIGNING_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("DOC_ENGINE_SIGNING_BASE_URL"); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv("DOC_ENGINE_SIGNING_SIGNING_BASE_URL"); v != "" {
		cfg.SigningBaseURL = v
	}
	if v := os.Getenv("DOC_ENGINE_SIGNING_WEBHOOK_SECRET"); v != "" {
		cfg.WebhookSecret = v
	}
	if v := os.Getenv("DOC_ENGINE_SIGNING_WEBHOOK_URL"); v != "" {
		cfg.WebhookURL = v
	}
}

// applyStorageEnvOverrides reads DOC_ENGINE_STORAGE_* env vars into StorageConfig.
func applyStorageEnvOverrides(cfg *StorageConfig) {
	if cfg == nil {
		return
	}

	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_STORAGE_PROVIDER")); v != "" {
		cfg.Provider = v
	}
	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_STORAGE_LOCAL_DIR")); v != "" {
		cfg.LocalDir = v
	}
	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_STORAGE_BUCKET")); v != "" {
		cfg.Bucket = v
	}
	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_STORAGE_REGION")); v != "" {
		cfg.Region = v
	}
	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_STORAGE_ENDPOINT")); v != "" {
		cfg.Endpoint = v
	}
	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_STORAGE_ENABLED")); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.Enabled = parsed
		}
	}
}

// applyServerEnvOverrides reads DOC_ENGINE_SERVER_* env vars into ServerConfig.
func applyServerEnvOverrides(cfg *ServerConfig) {
	if cfg == nil {
		return
	}

	raw := strings.TrimSpace(os.Getenv(serverPublicSigningFrameAncestorsEnv))
	if raw == "" {
		return
	}

	cfg.PublicSigningFrameAncestors = parseCSVList(raw)
}

// applyAuthPanelEnvOverrides reads DOC_ENGINE_AUTH_PANEL_* env vars into OIDCProvider.
func applyAuthPanelEnvOverrides(panel *OIDCProvider) {
	if panel == nil {
		return
	}
	if v := os.Getenv("DOC_ENGINE_AUTH_PANEL_NAME"); v != "" {
		panel.Name = v
	}
	if v := os.Getenv("DOC_ENGINE_AUTH_PANEL_DISCOVERY_URL"); v != "" {
		panel.DiscoveryURL = v
	}
	if v := os.Getenv("DOC_ENGINE_AUTH_PANEL_ISSUER"); v != "" {
		panel.Issuer = v
	}
	if v := os.Getenv("DOC_ENGINE_AUTH_PANEL_AUDIENCE"); v != "" {
		panel.Audience = v
	}
	if v := os.Getenv("DOC_ENGINE_AUTH_PANEL_CLIENT_ID"); v != "" {
		panel.ClientID = v
	}
}

// applyBootstrapEnvOverrides reads DOC_ENGINE_BOOTSTRAP_* env vars into BootstrapConfig.
func applyBootstrapEnvOverrides(cfg *BootstrapConfig) {
	if v := os.Getenv("DOC_ENGINE_BOOTSTRAP_ENABLED"); v == "false" {
		cfg.Enabled = false
	}
}

// applyWorkerEnvOverrides reads DOC_ENGINE_WORKER_* env vars into WorkerConfig.
func applyWorkerEnvOverrides(cfg *WorkerConfig) {
	if cfg == nil {
		return
	}
	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_WORKER_ENABLED")); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.Enabled = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_WORKER_MAX_WORKERS")); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			cfg.MaxWorkers = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_WORKER_FAILPOINTS_ENABLED")); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			cfg.FailpointsEnabled = parsed
		}
	}
	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_WORKER_FAILPOINTS")); v != "" {
		cfg.Failpoints = parseCSVList(v)
	}
}

// applySigningSessionAuthEnvOverrides reads DOC_ENGINE_SIGNING_SESSION_AUTH_* env vars.
func applySigningSessionAuthEnvOverrides(cfg *SigningSessionAuthConfig) {
	if cfg == nil {
		return
	}
	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_SIGNING_SESSION_AUTH_MODE")); v != "" {
		cfg.Mode = v
	}
	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_SIGNING_SESSION_AUTH_OIDC_PROVIDER")); v != "" {
		cfg.OIDC.Provider = v
	}
	if v := strings.TrimSpace(os.Getenv("DOC_ENGINE_SIGNING_SESSION_AUTH_OIDC_EMAIL_CLAIM")); v != "" {
		cfg.OIDC.EmailClaim = v
	}
}

// LoadFromFile reads configuration from a specific YAML file path.
// Environment variables still apply as overrides.
func LoadFromFile(filePath string) (*Config, error) {
	v := viper.New()

	v.SetConfigFile(filePath)

	// Environment variable settings
	v.SetEnvPrefix("DOC_ENGINE")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("reading config file %s: %w", filePath, err)
	}

	setDefaults(v)

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if cfg.Server.Port == "" {
		if port := os.Getenv("PORT"); port != "" {
			cfg.Server.Port = port
		}
	}

	if os.Getenv("DOC_ENGINE_AUTH_DUMMY") == "true" {
		cfg.Auth.Dummy = true
	}

	if cfg.Auth.IsDummyAuth() && cfg.DummyAuthUserID == "" {
		if id := os.Getenv("DOC_ENGINE_DUMMY_AUTH_USER_ID"); id != "" {
			cfg.DummyAuthUserID = id
		} else {
			cfg.DummyAuthUserID = "00000000-0000-0000-0000-000000000001"
		}
	}

	applySigningEnvOverrides(&cfg.Signing)
	applyStorageEnvOverrides(&cfg.Storage)
	applyServerEnvOverrides(&cfg.Server)
	applyAuthPanelEnvOverrides(cfg.Auth.Panel)
	applySigningSessionAuthEnvOverrides(&cfg.SigningSessionAuth)
	applyBootstrapEnvOverrides(&cfg.Bootstrap)

	if err := cfg.Auth.DiscoverAll(context.Background()); err != nil {
		slog.WarnContext(context.Background(), "OIDC discovery failed (non-fatal)",
			slog.String("error", err.Error()))
	}

	return &cfg, nil
}

// MustLoad loads configuration and panics on error.
// Use this only in main() or initialization code.
func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}
	return cfg
}

func parseCSVList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	for _, part := range parts {
		value := strings.TrimSpace(part)
		if value == "" {
			continue
		}

		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}

	return out
}
