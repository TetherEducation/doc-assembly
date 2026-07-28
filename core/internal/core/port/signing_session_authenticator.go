package port

import (
	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// SigningSessionAuthenticateRequest contains data needed to authenticate
// authenticated signing session creation requests.
type SigningSessionAuthenticateRequest struct {
	DocumentID  string
	Environment entity.Environment
}

// SigningSessionAuthenticator defines custom authentication for authenticated
// signing sessions (/api/v1/signing-sessions/:documentId).
//
// Implementations should validate upstream credentials and return recipient
// identity claims used to resolve the document recipient.
type SigningSessionAuthenticator interface {
	Authenticate(c *gin.Context, req *SigningSessionAuthenticateRequest) (*SigningSessionAuthClaims, error)
}

// SigningSessionAuthClaims contains resolved identity for signing session auth.
type SigningSessionAuthClaims struct {
	Email    string         // Recipient email to match against document recipients.
	Subject  string         // Optional subject/user identifier from upstream auth.
	Provider string         // Optional provider identifier (e.g. "oidc", "custom-jwt").
	Extra    map[string]any // Optional custom claims.
}
