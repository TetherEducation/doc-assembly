//nolint:gosec
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	// wellKnownSuffix is the standard OpenID Connect discovery endpoint suffix.
	wellKnownSuffix = "/.well-known/openid-configuration"

	// discoveryTimeout is the HTTP timeout for discovery requests.
	discoveryTimeout = 10 * time.Second
)

// discoveryResponse represents the OpenID Connect discovery document.
type discoveryResponse struct {
	Issuer             string `json:"issuer"`
	JWKSURI            string `json:"jwks_uri"`
	TokenEndpoint      string `json:"token_endpoint"`
	UserinfoEndpoint   string `json:"userinfo_endpoint"`
	EndSessionEndpoint string `json:"end_session_endpoint"`
}

// DiscoverAll runs OIDC discovery for configured auth providers.
// Providers with discovery_url will have their issuer and jwks_url populated.
func (a *AuthConfig) DiscoverAll(ctx context.Context) error {
	if a == nil {
		return nil
	}

	// Discover panel provider
	if a.Panel != nil {
		if err := discoverOIDC(ctx, a.Panel); err != nil {
			return fmt.Errorf("panel OIDC discovery: %w", err)
		}
	}

	return nil
}

// discoverOIDC fetches OpenID configuration from the discovery URL
// and populates issuer and jwks_url if not already set.
func discoverOIDC(ctx context.Context, provider *OIDCProvider) error {
	if provider.DiscoveryURL == "" {
		return nil // No discovery URL configured, nothing to do
	}

	// Build discovery URL (append well-known suffix if needed)
	discoveryURL := buildDiscoveryURL(provider.DiscoveryURL)

	name := provider.Name
	if name == "" {
		name = "unnamed"
	}

	slog.InfoContext(ctx, "OIDC discovery started",
		slog.String("name", name),
		slog.String("url", discoveryURL))

	start := time.Now()

	// Fetch discovery document
	doc, err := fetchDiscoveryDocument(ctx, discoveryURL)
	if err != nil {
		slog.WarnContext(ctx, "OIDC discovery failed",
			slog.String("name", name),
			slog.String("url", discoveryURL),
			slog.String("error", err.Error()),
			slog.Duration("duration", time.Since(start)))
		return err
	}

	// Set issuer if not configured
	if provider.Issuer == "" {
		provider.Issuer = doc.Issuer
	}

	// Set JWKS URL if not configured
	if provider.JWKSURL == "" {
		provider.JWKSURL = doc.JWKSURI
	}

	// Set frontend endpoints if not configured
	if provider.TokenEndpoint == "" {
		provider.TokenEndpoint = doc.TokenEndpoint
	}
	if provider.UserinfoEndpoint == "" {
		provider.UserinfoEndpoint = doc.UserinfoEndpoint
	}
	if provider.EndSessionEndpoint == "" {
		provider.EndSessionEndpoint = doc.EndSessionEndpoint
	}

	slog.InfoContext(ctx, "OIDC discovery completed",
		slog.String("name", name),
		slog.String("issuer", provider.Issuer),
		slog.String("jwks_uri", provider.JWKSURL),
		slog.Duration("duration", time.Since(start)))

	return nil
}

// buildDiscoveryURL ensures the URL ends with the well-known suffix.
func buildDiscoveryURL(url string) string {
	url = strings.TrimSuffix(url, "/")
	if strings.HasSuffix(url, wellKnownSuffix) {
		return url
	}
	return url + wellKnownSuffix
}

// fetchDiscoveryDocument fetches and parses the OpenID Connect discovery document.
func fetchDiscoveryDocument(ctx context.Context, url string) (*discoveryResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, discoveryTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	var doc discoveryResponse
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if doc.Issuer == "" {
		return nil, fmt.Errorf("discovery response missing issuer")
	}
	if doc.JWKSURI == "" {
		return nil, fmt.Errorf("discovery response missing jwks_uri")
	}

	return &doc, nil
}
