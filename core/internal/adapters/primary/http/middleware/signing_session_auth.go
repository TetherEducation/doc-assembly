package middleware

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

const signingSessionClaimsKey = "signing_session_auth_claims"

// SigningSessionCustomAuth authenticates requests to
// /api/v1/signing-sessions/:documentId using a custom authenticator.
func SigningSessionCustomAuth(auth port.SigningSessionAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		if auth == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": entity.ErrUnauthorized.Error()})
			return
		}

		documentID := strings.TrimSpace(c.Param("documentId"))
		claims, err := auth.Authenticate(c, &port.SigningSessionAuthenticateRequest{
			DocumentID:  documentID,
			Environment: signingSessionEnvironment(c),
		})
		if c.IsAborted() {
			return
		}
		if err != nil {
			status := http.StatusUnauthorized
			if errors.Is(err, entity.ErrForbidden) {
				status = http.StatusForbidden
			}
			c.AbortWithStatusJSON(status, gin.H{"error": err.Error()})
			return
		}
		if claims == nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": entity.ErrUnauthorized.Error()})
			return
		}

		c.Set(signingSessionClaimsKey, claims)
		c.Next()
	}
}

// SigningSessionOIDCClaims extracts signing session claims after OIDC auth
// middleware validated the JWT.
func SigningSessionOIDCClaims(emailClaim, fallbackProvider string) gin.HandlerFunc {
	normalizedEmailClaim := strings.TrimSpace(emailClaim)
	if normalizedEmailClaim == "" {
		normalizedEmailClaim = "email"
	}

	return func(c *gin.Context) {
		subject, _ := GetUserID(c)
		email := oidcEmailFromContext(c, normalizedEmailClaim)

		if email == "" {
			tokenString, err := extractBearerToken(c)
			if err == nil {
				email = extractStringClaim(tokenString, normalizedEmailClaim)
			}
		}

		if email == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing required email claim"})
			return
		}

		provider, ok := GetOIDCProvider(c)
		if !ok || strings.TrimSpace(provider) == "" {
			provider = strings.TrimSpace(fallbackProvider)
		}

		c.Set(signingSessionClaimsKey, &port.SigningSessionAuthClaims{
			Email:    email,
			Subject:  subject,
			Provider: provider,
		})
		c.Next()
	}
}

// GetSigningSessionAuthClaims returns claims previously stored by
// SigningSessionCustomAuth or SigningSessionOIDCClaims.
func GetSigningSessionAuthClaims(c *gin.Context) (*port.SigningSessionAuthClaims, bool) {
	val, ok := c.Get(signingSessionClaimsKey)
	if !ok {
		return nil, false
	}
	claims, castOK := val.(*port.SigningSessionAuthClaims)
	return claims, castOK && claims != nil
}

func oidcEmailFromContext(c *gin.Context, claim string) string {
	claim = strings.TrimSpace(claim)
	if strings.EqualFold(claim, "email") {
		if email, ok := GetUserEmail(c); ok {
			return strings.TrimSpace(email)
		}
	}
	return ""
}

func extractStringClaim(tokenString, claim string) string {
	parser := jwt.NewParser()
	token, _, err := parser.ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		slog.Debug("signing-session: failed to parse JWT claims", slog.String("error", err.Error()))
		return ""
	}

	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return ""
	}

	raw, ok := mapClaims[claim]
	if !ok || raw == nil {
		return ""
	}

	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(value)
}

func signingSessionEnvironment(c *gin.Context) entity.Environment {
	if raw := strings.TrimSpace(c.GetHeader("X-Environment")); raw != "" {
		if env, err := entity.ParseEnvironment(raw); err == nil {
			return env
		}
	}
	return GetEnvironment(c)
}
