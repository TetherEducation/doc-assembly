package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/infra/config"
)

func getClientConfig(t *testing.T, cfg *config.Config) map[string]any {
	t.Helper()
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.GET("/api/v1/config", clientConfigHandler(cfg))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/config", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

func TestClientConfig_ExposesPanelAuthorizationEndpoint(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			Panel: &config.OIDCProvider{
				Name:                  "keycloak",
				Issuer:                "https://idp.example/realms/team",
				ClientID:              "team-signing",
				AuthorizationEndpoint: "https://idp.example/realms/team/auth",
				TokenEndpoint:         "https://idp.example/realms/team/token",
			},
		},
	}

	body := getClientConfig(t, cfg)

	panel, ok := body["panelProvider"].(map[string]any)
	require.True(t, ok, "panelProvider missing from %v", body)
	assert.Equal(t, "https://idp.example/realms/team/auth", panel["authorizationEndpoint"])
	assert.Equal(t, "https://idp.example/realms/team/token", panel["tokenEndpoint"])
	assert.Equal(t, "team-signing", panel["clientId"])
}

func TestClientConfig_OmitsUnknownAuthorizationEndpoint(t *testing.T) {
	cfg := &config.Config{
		Auth: config.AuthConfig{
			Panel: &config.OIDCProvider{
				Issuer:        "https://idp.example/realms/team",
				TokenEndpoint: "https://idp.example/realms/team/token",
			},
		},
	}

	body := getClientConfig(t, cfg)

	panel, ok := body["panelProvider"].(map[string]any)
	require.True(t, ok, "panelProvider missing from %v", body)
	_, present := panel["authorizationEndpoint"]
	assert.False(t, present, "an empty endpoint must be omitted, not sent as \"\"")
}
