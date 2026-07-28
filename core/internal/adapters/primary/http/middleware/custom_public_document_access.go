package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

const publicDocAccessClaimsKey = "public_doc_access_claims"

// CustomPublicDocumentAccess runs a custom authenticator for
// public /public/doc/:documentId requests (GET and POST).
//
// By default this middleware does not block requests: on auth success, claims
// are stored in context so the controller can redirect directly to
// /public/sign/:token or return a signing URL. On auth failure/miss, flow
// falls back to the standard email gate.
// If the authenticator explicitly aborts the context, the chain stops.
func CustomPublicDocumentAccess(auth port.PublicDocumentAccessAuthenticator) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}

		documentID := c.Param("documentId")
		if documentID == "" {
			c.Next()
			return
		}

		claims, err := auth.Authenticate(c, &port.AuthenticateRequest{
			DocumentID:  documentID,
			Environment: GetEnvironment(c),
		})
		if c.IsAborted() {
			return
		}

		if err != nil {
			slog.InfoContext(c.Request.Context(), "custom public doc auth fallback",
				slog.String("document_id", documentID),
				slog.String("error", err.Error()),
				slog.String("operation_id", GetOperationID(c)),
			)
			c.Next()
			return
		}

		if claims == nil || strings.TrimSpace(claims.Email) == "" {
			c.Next()
			return
		}

		c.Set(publicDocAccessClaimsKey, claims)
		c.Next()
	}
}

// GetPublicDocumentAccessClaims returns claims injected by CustomPublicDocumentAccess.
func GetPublicDocumentAccessClaims(c *gin.Context) (*port.PublicDocumentAccessClaims, bool) {
	val, ok := c.Get(publicDocAccessClaimsKey)
	if !ok {
		return nil, false
	}
	claims, castOK := val.(*port.PublicDocumentAccessClaims)
	return claims, castOK && claims != nil
}
