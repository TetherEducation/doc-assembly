package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

const (
	readOnlyViewLinkClaimsKey = "read_only_view_link_auth_claims"

	// ReadOnlyViewLinkWorkspaceCodeHeader is the workspace business-code header
	// required by external read-only link creation flows.
	ReadOnlyViewLinkWorkspaceCodeHeader = "X-Workspace-Code"
)

// ReadOnlyViewLinkCustomAuth authenticates requests to
// /api/v1/documents/:documentId/view-link using a custom authenticator.
func ReadOnlyViewLinkCustomAuth(auth port.ReadOnlyViewLinkAuthenticator) gin.HandlerFunc {
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
		workspaceCode := strings.TrimSpace(c.GetHeader(ReadOnlyViewLinkWorkspaceCodeHeader))
		claims, err := auth.Authenticate(c, &port.ReadOnlyViewLinkAuthenticateRequest{
			DocumentID:    documentID,
			WorkspaceCode: workspaceCode,
			Environment:   readOnlyViewLinkEnvironment(c),
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

		c.Set(readOnlyViewLinkClaimsKey, claims)
		c.Next()
	}
}

// GetReadOnlyViewLinkAuthClaims returns claims previously stored by
// ReadOnlyViewLinkCustomAuth.
func GetReadOnlyViewLinkAuthClaims(c *gin.Context) (*port.ReadOnlyViewLinkAuthClaims, bool) {
	val, ok := c.Get(readOnlyViewLinkClaimsKey)
	if !ok {
		return nil, false
	}
	claims, castOK := val.(*port.ReadOnlyViewLinkAuthClaims)
	return claims, castOK && claims != nil
}

func readOnlyViewLinkEnvironment(c *gin.Context) entity.Environment {
	if raw := strings.TrimSpace(c.GetHeader("X-Environment")); raw != "" {
		if env, err := entity.ParseEnvironment(raw); err == nil {
			return env
		}
	}
	return GetEnvironment(c)
}
