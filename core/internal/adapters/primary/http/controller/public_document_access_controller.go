package controller

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/middleware"
	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
)

// PublicDocumentAccessController handles public document access endpoints.
// These endpoints implement the email-verification gate for public signing.
type PublicDocumentAccessController struct {
	accessUC documentuc.DocumentAccessUseCase
}

// NewPublicDocumentAccessController creates a new public document access controller.
func NewPublicDocumentAccessController(accessUC documentuc.DocumentAccessUseCase) *PublicDocumentAccessController {
	return &PublicDocumentAccessController{accessUC: accessUC}
}

// RegisterRoutes registers public document access routes.
// These routes are NOT behind the auth middleware chain.
// authMiddlewares (e.g. CustomPublicDocumentAccess) are applied to both GET and POST.
func (c *PublicDocumentAccessController) RegisterRoutes(router gin.IRouter, authMiddlewares ...gin.HandlerFunc) {
	public := router.Group("/public/doc")
	{
		getHandlers := make([]gin.HandlerFunc, 0, len(authMiddlewares)+1)
		getHandlers = append(getHandlers, authMiddlewares...)
		getHandlers = append(getHandlers, c.GetPublicDocumentInfo)
		public.GET("/:documentId", getHandlers...)

		postHandlers := make([]gin.HandlerFunc, 0, len(authMiddlewares)+1)
		postHandlers = append(postHandlers, authMiddlewares...)
		postHandlers = append(postHandlers, c.RequestAccess)
		public.POST("/:documentId/request-access", postHandlers...)
	}
}

// requestAccessBody is the request body for requesting document access.
type requestAccessBody struct {
	Email string `json:"email" binding:"required,email"`
}

// GetPublicDocumentInfo returns minimal public info about a document.
// @Summary Get public document info
// @Description Returns document title and status for the public access page.
// @Tags Public Document Access
// @Produce json
// @Param documentId path string true "Document ID"
// @Param language query string false "Public signing UI language (en or es)"
// @Success 200 {object} documentuc.PublicDocumentInfoResponse
// @Failure 404 {object} map[string]string
// @Router /public/doc/{documentId} [get]
func (c *PublicDocumentAccessController) GetPublicDocumentInfo(ctx *gin.Context) {
	documentID := ctx.Param("documentId")

	// Optional custom middleware path: if claims are present and valid, bypass
	// the email gate and redirect directly to /public/sign/:token.
	if claims, ok := middleware.GetPublicDocumentAccessClaims(ctx); ok {
		redirectURL, err := c.accessUC.RequestDirectAccess(ctx.Request.Context(), documentID, claims.Email)
		if err == nil && redirectURL != "" {
			ctx.Redirect(http.StatusSeeOther, appendPublicLanguage(redirectURL, ctx.Query("language")))
			return
		}
	}

	resp, err := c.accessUC.GetPublicDocumentInfo(ctx.Request.Context(), documentID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "document not found"})
		return
	}

	ctx.JSON(http.StatusOK, resp)
}

// RequestAccess validates email and sends an access link.
// If custom auth claims are present (via CustomPublicDocumentAccess middleware),
// returns a signing URL directly without sending email.
// Always returns 200 to prevent email enumeration.
// @Summary Request document access
// @Description Validates email against document recipients and sends an access link via email. With valid auth, returns signing URL directly. Always returns 200.
// @Tags Public Document Access
// @Accept json
// @Produce json
// @Param documentId path string true "Document ID"
// @Param language query string false "Public signing UI language (en or es)"
// @Param request body requestAccessBody false "Email address (not required when auth is provided)"
// @Success 200 {object} map[string]string
// @Router /public/doc/{documentId}/request-access [post]
func (c *PublicDocumentAccessController) RequestAccess(ctx *gin.Context) {
	documentID := ctx.Param("documentId")

	// If custom auth claims are present, return signing URL directly.
	if claims, ok := middleware.GetPublicDocumentAccessClaims(ctx); ok {
		signingURL, err := c.accessUC.RequestDirectAccess(ctx.Request.Context(), documentID, claims.Email)
		if err == nil && signingURL != "" {
			ctx.JSON(http.StatusOK, gin.H{"signingUrl": appendPublicLanguage(signingURL, ctx.Query("language"))})
			return
		}
	}

	// Fallback: standard email gate flow.
	var req requestAccessBody
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "a valid email address is required"})
		return
	}

	email := strings.TrimSpace(req.Email)
	_ = c.accessUC.RequestAccess(ctx.Request.Context(), documentID, email, explicitPublicLanguage(ctx.Query("language")))

	ctx.JSON(http.StatusOK, gin.H{
		"message": "If your email is associated with this document, you will receive a signing link shortly.",
	})
}
