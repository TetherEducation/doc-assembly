package port

import (
	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// ReadOnlyViewLinkAuthenticateRequest contains data needed to authenticate
// authenticated read-only view link creation requests.
type ReadOnlyViewLinkAuthenticateRequest struct {
	DocumentID    string
	WorkspaceCode string
	Environment   entity.Environment
}

// ReadOnlyViewLinkAuthenticator defines custom authentication for
// /api/v1/documents/:documentId/view-link.
type ReadOnlyViewLinkAuthenticator interface {
	Authenticate(c *gin.Context, req *ReadOnlyViewLinkAuthenticateRequest) (*ReadOnlyViewLinkAuthClaims, error)
}

// ReadOnlyViewLinkAuthClaims contains resolved identity for read-only view link auth.
type ReadOnlyViewLinkAuthClaims struct {
	Email    string
	Subject  string
	Provider string

	// AuthorizedWorkspaceCodes contains additional workspace business codes that
	// the authenticated request may use for read-only document ownership checks.
	AuthorizedWorkspaceCodes []string
}
