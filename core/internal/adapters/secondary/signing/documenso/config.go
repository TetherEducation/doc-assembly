package documenso

import (
	"errors"
	"net/url"
	"path"
	"strings"
)

// Config contains the configuration for the Documenso signing provider.
type Config struct {
	// APIKey is the Documenso API key for authentication.
	APIKey string //nolint:gosec

	// BaseURL is the base URL for the Documenso API.
	// Defaults to "https://app.documenso.com/api/v2" if not set.
	BaseURL string

	// SigningBaseURL is the base URL for signing links (without /api/v2).
	// Defaults to deriving from BaseURL (e.g., "https://app.documenso.com").
	SigningBaseURL string

	// WebhookSecret is the secret key used to validate incoming webhooks.
	WebhookSecret string

	// WebhookURL is the URL where Documenso should send webhook events.
	// This is the public URL of your application's webhook endpoint.
	WebhookURL string

	// Embed customizes the embedded signing widget (theme, language,
	// branding, signer-name prefill). Zero value keeps embedded URLs
	// byte-identical to previous releases.
	Embed EmbedOptions
}

// Validate checks if the configuration is valid.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.APIKey) == "" {
		return errors.New("documenso: API key is required")
	}

	// Set default base URL if not provided
	if strings.TrimSpace(c.BaseURL) == "" {
		c.BaseURL = "https://app.documenso.com/api/v2"
	}

	// Ensure base URL doesn't have trailing slash
	c.BaseURL = strings.TrimSuffix(c.BaseURL, "/")
	c.BaseURL = normalizeDocumensoAPIBaseURL(c.BaseURL)

	// Derive signing base URL from API base URL if not set
	if strings.TrimSpace(c.SigningBaseURL) == "" {
		// Remove /api/v2 or /api/v1 suffix to get the base signing URL
		signingBase := c.BaseURL
		signingBase = strings.TrimSuffix(signingBase, "/api/v2-beta")
		signingBase = strings.TrimSuffix(signingBase, "/api/v2")
		signingBase = strings.TrimSuffix(signingBase, "/api/v1")
		c.SigningBaseURL = signingBase
	}
	c.SigningBaseURL = strings.TrimSuffix(c.SigningBaseURL, "/")

	return nil
}

// DefaultConfig returns a configuration with default values.
func DefaultConfig() *Config {
	return &Config{
		BaseURL: "https://app.documenso.com/api/v2",
	}
}

func normalizeDocumensoAPIBaseURL(baseURL string) string {
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return strings.TrimSuffix(baseURL, "/")
	}

	normalizedPath := strings.TrimSuffix(parsed.Path, "/")
	if normalizedPath == "" {
		normalizedPath = "/"
	}
	if strings.Contains(normalizedPath, "/api/") {
		return strings.TrimSuffix(baseURL, "/")
	}

	parsed.Path = path.Join(normalizedPath, "/api/v2")
	return strings.TrimSuffix(parsed.String(), "/")
}
