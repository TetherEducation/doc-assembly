package port

import (
	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// AuthenticateRequest contains the data needed for public document authentication.
type AuthenticateRequest struct {
	DocumentID  string
	Environment entity.Environment
}

// PublicDocumentAccessAuthenticator defines custom authentication for public
// document access endpoints (/public/doc/:documentId).
//
// Implementations can validate session/JWT/domain-specific context and, when
// authorized, provide the recipient email so the backend can generate a
// standard tokenized signing URL without requiring the email gate step.
type PublicDocumentAccessAuthenticator interface {
	// Authenticate validates access for a specific document request.
	// Return (nil, nil) to indicate "fallback to email gate".
	// Return (claims, nil) to indicate direct access is allowed.
	// Return (nil, err) to indicate auth failed (also falls back to email gate).
	Authenticate(c *gin.Context, req *AuthenticateRequest) (*PublicDocumentAccessClaims, error)
}

// PublicDocumentAccessClaims contains the resolved recipient identity.
type PublicDocumentAccessClaims struct {
	Email    string         // Recipient email to match against document recipients.
	Subject  string         // Optional subject/user identifier from upstream auth.
	Provider string         // Optional provider/method identifier (e.g. "custom-jwt").
	Extra    map[string]any // Optional custom claims.
}
