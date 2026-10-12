package config

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDiscoveryServer(t *testing.T) *httptest.Server {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != wellKnownSuffix {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 "https://idp.example/realms/team",
			"jwks_uri":               "https://idp.example/realms/team/certs",
			"authorization_endpoint": "https://idp.example/realms/team/auth",
			"token_endpoint":         "https://idp.example/realms/team/token",
			"userinfo_endpoint":      "https://idp.example/realms/team/userinfo",
			"end_session_endpoint":   "https://idp.example/realms/team/logout",
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDiscoverOIDC_FillsFrontendEndpoints(t *testing.T) {
	srv := newDiscoveryServer(t)
	provider := &OIDCProvider{DiscoveryURL: srv.URL}

	require.NoError(t, discoverOIDC(context.Background(), provider))

	assert.Equal(t, "https://idp.example/realms/team/auth", provider.AuthorizationEndpoint)
	assert.Equal(t, "https://idp.example/realms/team/token", provider.TokenEndpoint)
	assert.Equal(t, "https://idp.example/realms/team/userinfo", provider.UserinfoEndpoint)
	assert.Equal(t, "https://idp.example/realms/team/logout", provider.EndSessionEndpoint)
}

func TestDiscoverOIDC_KeepsConfiguredAuthorizationEndpoint(t *testing.T) {
	srv := newDiscoveryServer(t)
	provider := &OIDCProvider{
		DiscoveryURL:          srv.URL,
		AuthorizationEndpoint: "https://login.example/custom-auth",
	}

	require.NoError(t, discoverOIDC(context.Background(), provider))

	assert.Equal(t, "https://login.example/custom-auth", provider.AuthorizationEndpoint,
		"an explicitly configured endpoint must win over discovery")
}
