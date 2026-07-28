package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/infra/config"
)

const (
	// userIDKey is the context key for the authenticated user ID.
	userIDKey = "user_id"
	// userEmailKey is the context key for the authenticated user email.
	userEmailKey = "user_email"
	// userNameKey is the context key for the authenticated user name.
	userNameKey = "user_name"
	// oidcProviderKey is the context key for the matched OIDC provider name.
	oidcProviderKey = "oidc_provider"
)

// providerKeyfunc holds a keyfunc and its associated config for one provider.
type providerKeyfunc struct {
	provider config.OIDCProvider
	keyfunc  keyfunc.Keyfunc
}

// PanelAuth creates middleware for panel (login/UI) authentication.
// Uses only the panel OIDC provider configured in auth.panel.
func PanelAuth(cfg *config.AuthConfig) gin.HandlerFunc {
	panel := cfg.GetPanelOIDC()
	if panel == nil {
		return func(c *gin.Context) {
			slog.ErrorContext(c.Request.Context(), "no panel OIDC provider configured")
			abortWithError(c, http.StatusInternalServerError, entity.ErrInvalidToken)
		}
	}
	return MultiOIDCAuth([]config.OIDCProvider{*panel})
}

// MultiOIDCAuth creates middleware supporting multiple OIDC providers.
// Matches incoming token's issuer against configured providers.
// Returns 401 if issuer is not in the configured list.
// When no providers have JWKS configured (dev mode), tokens are parsed without validation.
func MultiOIDCAuth(providers []config.OIDCProvider) gin.HandlerFunc {
	providerMap := initializeProviders(providers)

	return func(c *gin.Context) {
		// Skip auth for OPTIONS requests (CORS preflight)
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		tokenString, err := extractBearerToken(c)
		if err != nil {
			abortWithError(c, http.StatusUnauthorized, err)
			return
		}

		// Dev mode: no providers initialized (all JWKS URLs empty)
		if len(providerMap) == 0 {
			claims, parseErr := parseUnverifiedToken(tokenString)
			if parseErr != nil {
				abortWithError(c, http.StatusUnauthorized, parseErr)
				return
			}
			storeClaims(c, claims)
			c.Next()
			return
		}

		// Peek at issuer WITHOUT validating signature
		issuer, err := peekTokenIssuer(tokenString)
		if err != nil {
			abortWithError(c, http.StatusUnauthorized, entity.ErrInvalidToken)
			return
		}

		// Find matching provider
		pf, ok := providerMap[issuer]
		if !ok {
			slog.WarnContext(c.Request.Context(), "unknown token issuer",
				slog.String("issuer", issuer),
				slog.String("operation_id", GetOperationID(c)),
			)
			abortWithError(c, http.StatusUnauthorized, entity.ErrUnknownIssuer)
			return
		}

		// Validate token with matched provider's JWKS
		claims, err := validateTokenWithProvider(tokenString, pf)
		if err != nil {
			slog.WarnContext(c.Request.Context(), "token validation failed",
				slog.String("error", err.Error()),
				slog.String("provider", pf.provider.Name),
				slog.String("operation_id", GetOperationID(c)),
			)
			abortWithError(c, http.StatusUnauthorized, err)
			return
		}

		storeClaims(c, claims)
		c.Set(oidcProviderKey, pf.provider.Name)
		c.Next()
	}
}

// initializeProviders creates keyfuncs for each provider, keyed by issuer.
func initializeProviders(providers []config.OIDCProvider) map[string]*providerKeyfunc {
	result := make(map[string]*providerKeyfunc, len(providers))
	ctx := context.Background()

	for _, p := range providers {
		if p.Issuer == "" || p.JWKSURL == "" {
			slog.WarnContext(ctx, "skipping OIDC provider with missing issuer or jwks_url",
				slog.String("name", p.Name))
			continue
		}

		initCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		kf, err := keyfunc.NewDefaultCtx(initCtx, []string{p.JWKSURL})
		cancel()

		if err != nil {
			slog.ErrorContext(ctx, "failed to initialize JWKS for OIDC provider",
				slog.String("name", p.Name),
				slog.String("jwks_url", p.JWKSURL),
				slog.String("error", err.Error()))
			continue // Skip this provider, don't fail startup
		}

		result[p.Issuer] = &providerKeyfunc{
			provider: p,
			keyfunc:  kf,
		}
		slog.InfoContext(ctx, "initialized OIDC provider",
			slog.String("name", p.Name),
			slog.String("issuer", p.Issuer))
	}
	return result
}

// peekTokenIssuer extracts issuer from JWT without signature validation.
func peekTokenIssuer(tokenString string) (string, error) {
	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return "", err
	}
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("invalid claims type")
	}
	issuer, _ := claims["iss"].(string)
	if issuer == "" {
		return "", fmt.Errorf("missing issuer claim")
	}
	return issuer, nil
}

// parseUnverifiedToken parses a JWT without signature validation (dev mode).
func parseUnverifiedToken(tokenString string) (*OIDCClaims, error) {
	var claims OIDCClaims
	_, _, err := jwt.NewParser().ParseUnverified(tokenString, &claims)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", entity.ErrInvalidToken, err)
	}
	return &claims, nil
}

// validateTokenWithProvider validates token against a specific provider.
func validateTokenWithProvider(tokenString string, pf *providerKeyfunc) (*OIDCClaims, error) {
	var claims OIDCClaims

	if err := parseTokenWithJWKS(tokenString, &claims, pf.keyfunc); err != nil {
		return nil, err
	}

	// Validate issuer (must match exactly)
	if err := validateIssuer(&claims, pf.provider.Issuer); err != nil {
		return nil, err
	}

	// Validate audience (if configured)
	if err := validateAudience(&claims, pf.provider.Audience); err != nil {
		return nil, err
	}

	return &claims, nil
}

// extractBearerToken extracts the Bearer token from the Authorization header.
func extractBearerToken(c *gin.Context) (string, error) {
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		return "", entity.ErrMissingToken
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", entity.ErrInvalidToken
	}
	return parts[1], nil
}

// storeClaims stores user claims in the gin context.
func storeClaims(c *gin.Context, claims *OIDCClaims) {
	c.Set(userIDKey, claims.Subject)
	if claims.Email != "" {
		c.Set(userEmailKey, claims.Email)
	}
	if claims.Name != "" {
		c.Set(userNameKey, claims.Name)
	}
}

// OIDCClaims represents standard OIDC JWT claims.
type OIDCClaims struct {
	jwt.RegisteredClaims
	Email         string `json:"email,omitempty"`
	EmailVerified bool   `json:"email_verified,omitempty"`
	Name          string `json:"name,omitempty"`
	PreferredUser string `json:"preferred_username,omitempty"`
}

// parseTokenWithJWKS parses and validates the token with JWKS.
func parseTokenWithJWKS(tokenString string, claims *OIDCClaims, jwks keyfunc.Keyfunc) error {
	token, err := jwt.ParseWithClaims(tokenString, claims, jwks.Keyfunc,
		jwt.WithValidMethods([]string{"RS256", "RS384", "RS512"}),
		jwt.WithExpirationRequired(),
	)
	if err != nil {
		if strings.Contains(err.Error(), "expired") {
			return entity.ErrTokenExpired
		}
		return fmt.Errorf("%w: %v", entity.ErrInvalidToken, err)
	}

	if !token.Valid {
		return entity.ErrInvalidToken
	}
	return nil
}

// validateIssuer checks if the token issuer matches the expected value.
func validateIssuer(claims *OIDCClaims, expectedIssuer string) error {
	if expectedIssuer == "" {
		return nil
	}

	issuer, err := claims.GetIssuer()
	if err != nil || issuer != expectedIssuer {
		return entity.ErrInvalidToken
	}
	return nil
}

// validateAudience checks if the token audience contains the expected value.
func validateAudience(claims *OIDCClaims, expectedAudience string) error {
	if expectedAudience == "" {
		return nil
	}

	audience, err := claims.GetAudience()
	if err != nil {
		return entity.ErrInvalidToken
	}

	for _, aud := range audience {
		if aud == expectedAudience {
			return nil
		}
	}
	return entity.ErrInvalidToken
}

// GetOIDCProvider retrieves the matched OIDC provider name from the Gin context.
func GetOIDCProvider(c *gin.Context) (string, bool) {
	return getStringFromContext(c, oidcProviderKey)
}

// GetUserID retrieves the authenticated user ID from the Gin context.
func GetUserID(c *gin.Context) (string, bool) {
	return getStringFromContext(c, userIDKey)
}

// GetUserEmail retrieves the authenticated user email from the Gin context.
func GetUserEmail(c *gin.Context) (string, bool) {
	return getStringFromContext(c, userEmailKey)
}

// GetUserName retrieves the authenticated user name from the Gin context.
func GetUserName(c *gin.Context) (string, bool) {
	return getStringFromContext(c, userNameKey)
}

// getStringFromContext retrieves a non-empty string value from the Gin context.
func getStringFromContext(c *gin.Context, key string) (string, bool) {
	if val, exists := c.Get(key); exists {
		if str, ok := val.(string); ok && str != "" {
			return str, true
		}
	}
	return "", false
}

// abortWithError aborts the request with a JSON error response.
func abortWithError(c *gin.Context, status int, err error) {
	c.AbortWithStatusJSON(status, gin.H{
		"error": err.Error(),
	})
}
