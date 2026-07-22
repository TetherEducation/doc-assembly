package controller

import (
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
)

// signingCallbackTmpl is a minimal HTML page that posts a message to the parent window.
// This acts as a provider-agnostic bridge: the iframe redirects here after signing,
// and the page sends a standardized postMessage to our parent frame.
var signingCallbackTmpl = template.Must(template.New("signing_callback").Parse(`<!DOCTYPE html>
<html><head><title>Signing</title></head><body>
<script>
window.parent.postMessage(
  {type:"SIGNING_EVENT",status:"{{.Status}}"},
  "{{.ParentOrigin}}"
);
</script>
</body></html>`))

// PublicSigningController handles public signing endpoints.
// These endpoints do not require JWT authentication; they are validated by access token only.
type PublicSigningController struct {
	preSigningUC documentuc.PreSigningUseCase
	accessUC     documentuc.DocumentAccessUseCase
	publicURL    string
}

// NewPublicSigningController creates a new public signing controller.
func NewPublicSigningController(
	preSigningUC documentuc.PreSigningUseCase,
	accessUC documentuc.DocumentAccessUseCase,
	publicURL string,
) *PublicSigningController {
	return &PublicSigningController{
		preSigningUC: preSigningUC,
		accessUC:     accessUC,
		publicURL:    publicURL,
	}
}

// RegisterRoutes registers public signing routes.
// These routes are NOT behind the auth middleware chain.
func (c *PublicSigningController) RegisterRoutes(router gin.IRouter) {
	public := router.Group("/public/sign")
	{
		public.GET("/:token", c.GetPublicSigningPage)
		public.POST("/:token", c.SubmitPreSigningForm)
		public.POST("/:token/request-access", c.RequestAccessFromToken)
		public.POST("/:token/proceed", c.ProceedToSigning)
		public.POST("/:token/complete", c.CompleteEmbeddedSigning)
		public.GET("/:token/pdf", c.RenderPreviewPDF)
		public.GET("/:token/download", c.DownloadCompletedPDF)
		public.GET("/:token/refresh", c.RefreshEmbeddedURL)
		public.GET("/:token/signing-callback", c.SigningCallback)
	}
}

// GetPublicSigningPage returns the current signing page state for a given access token.
// @Summary Get public signing page
// @Description Returns document state and content based on token type and document status.
// @Tags Public Signing
// @Produce json
// @Param token path string true "Access token"
// @Success 200 {object} documentuc.PublicSigningResponse
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /public/sign/{token} [get]
func (c *PublicSigningController) GetPublicSigningPage(ctx *gin.Context) {
	token := ctx.Param("token")

	resp, err := c.preSigningUC.GetPublicSigningPage(ctx.Request.Context(), token)
	if err != nil {
		handlePublicError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, resp)
}

// submitFormRequest is the request body for submitting pre-signing form responses.
type submitFormRequest struct {
	Responses []documentuc.FieldResponseInput `json:"responses" binding:"required"`
}

type requestAccessFromTokenBody struct {
	Email string `json:"email" binding:"required,email"`
}

// SubmitPreSigningForm submits field responses and returns the signing state with embedded URL.
// @Summary Submit pre-signing form
// @Description Validates and saves field responses, renders PDF, sends for signing, returns embedded URL.
// @Tags Public Signing
// @Accept json
// @Produce json
// @Param token path string true "Access token"
// @Param request body submitFormRequest true "Field responses"
// @Success 200 {object} documentuc.PublicSigningResponse
// @Failure 400 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /public/sign/{token} [post]
func (c *PublicSigningController) SubmitPreSigningForm(ctx *gin.Context) {
	token := ctx.Param("token")

	var req submitFormRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	resp, err := c.preSigningUC.SubmitPreSigningForm(ctx.Request.Context(), token, req.Responses)
	if err != nil {
		handlePublicError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, resp)
}

// RequestAccessFromToken requests a new access email using a token entrypoint
// (expired-link recovery). Always returns 200 to prevent enumeration.
// @Summary Request access from token
// @Description Requests a new signing link by email using a token entrypoint. Always returns 200.
// @Tags Public Signing
// @Accept json
// @Produce json
// @Param token path string true "Access token"
// @Param language query string false "Public signing UI language (en or es)"
// @Param request body requestAccessFromTokenBody true "Email address"
// @Success 200 {object} map[string]string
// @Router /public/sign/{token}/request-access [post]
func (c *PublicSigningController) RequestAccessFromToken(ctx *gin.Context) {
	token := ctx.Param("token")

	var req requestAccessFromTokenBody
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "a valid email address is required"})
		return
	}

	email := strings.TrimSpace(req.Email)
	_ = c.accessUC.RequestAccessByToken(ctx.Request.Context(), token, email, explicitPublicLanguage(ctx.Query("language")))

	ctx.JSON(http.StatusOK, gin.H{
		"message": "If your email is associated with this document, you will receive a signing link shortly.",
	})
}

// ProceedToSigning transitions a Path A document from preview to embedded signing.
// @Summary Proceed to signing
// @Description For Path A (SIGNING token), transitions from PDF preview to embedded signing iframe.
// @Tags Public Signing
// @Produce json
// @Param token path string true "Access token"
// @Success 200 {object} documentuc.PublicSigningResponse
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /public/sign/{token}/proceed [post]
func (c *PublicSigningController) ProceedToSigning(ctx *gin.Context) {
	token := ctx.Param("token")

	resp, err := c.preSigningUC.ProceedToSigning(ctx.Request.Context(), token)
	if err != nil {
		handlePublicError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, resp)
}

// RenderPreviewPDF renders the document PDF on-demand for preview.
// @Summary Render preview PDF
// @Description Renders the document PDF on-demand using the access token. Used for previewing before signing.
// @Tags Public Signing
// @Produce application/pdf
// @Param token path string true "Access token"
// @Success 200 {file} binary
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /public/sign/{token}/pdf [get]
func (c *PublicSigningController) RenderPreviewPDF(ctx *gin.Context) {
	token := ctx.Param("token")

	pdfBytes, err := c.preSigningUC.RenderPreviewPDF(ctx.Request.Context(), token)
	if err != nil {
		handlePublicError(ctx, err)
		return
	}

	ctx.Data(http.StatusOK, "application/pdf", pdfBytes)
}

// DownloadCompletedPDF downloads the signed PDF for completed documents.
// @Summary Download completed PDF
// @Description Downloads the completed/signed PDF when the token recipient is authorized.
// @Tags Public Signing
// @Produce application/pdf
// @Param token path string true "Access token"
// @Success 200 {file} binary
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /public/sign/{token}/download [get]
func (c *PublicSigningController) DownloadCompletedPDF(ctx *gin.Context) {
	token := ctx.Param("token")

	pdfBytes, filename, err := c.preSigningUC.DownloadCompletedPDF(ctx.Request.Context(), token)
	if err != nil {
		handlePublicError(ctx, err)
		return
	}

	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	ctx.Data(http.StatusOK, "application/pdf", pdfBytes)
}

// CompleteEmbeddedSigning marks the token as used after signing completion.
// @Summary Complete embedded signing
// @Description Marks the access token as used after the signer completes signing in the iframe.
// @Tags Public Signing
// @Produce json
// @Param token path string true "Access token"
// @Success 200 {object} map[string]string
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /public/sign/{token}/complete [post]
func (c *PublicSigningController) CompleteEmbeddedSigning(ctx *gin.Context) {
	token := ctx.Param("token")

	if err := c.preSigningUC.CompleteEmbeddedSigning(ctx.Request.Context(), token); err != nil {
		handlePublicError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"status": "completed"})
}

// RefreshEmbeddedURL refreshes an expired embedded signing URL.
// @Summary Refresh embedded URL
// @Description Gets a fresh embedded signing URL when the previous one has expired.
// @Tags Public Signing
// @Produce json
// @Param token path string true "Access token"
// @Success 200 {object} documentuc.PublicSigningResponse
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /public/sign/{token}/refresh [get]
func (c *PublicSigningController) RefreshEmbeddedURL(ctx *gin.Context) {
	token := ctx.Param("token")

	resp, err := c.preSigningUC.RefreshEmbeddedURL(ctx.Request.Context(), token)
	if err != nil {
		handlePublicError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, resp)
}

// SigningCallback serves the callback bridge page for provider-agnostic signing completion.
// When a signing provider redirects the iframe here, this page sends a postMessage
// to the parent window with a standardized event — the parent never listens to the
// provider's domain, only to its own origin.
// @Summary Signing callback bridge
// @Description Serves an HTML page that posts SIGNING_EVENT to parent window.
// @Tags Public Signing
// @Produce html
// @Param token path string true "Access token"
// @Param status query string false "Signing status" default(signed)
// @Router /public/sign/{token}/signing-callback [get]
func (c *PublicSigningController) SigningCallback(ctx *gin.Context) {
	status := ctx.DefaultQuery("status", "signed")
	event := ctx.DefaultQuery("event", "")

	normalizedStatus := normalizeCallbackStatus(status, event)

	ctx.Header("Content-Type", "text/html; charset=utf-8")
	ctx.Status(http.StatusOK)

	_ = signingCallbackTmpl.Execute(ctx.Writer, struct {
		Status       string
		ParentOrigin string
	}{
		Status:       normalizedStatus,
		ParentOrigin: c.publicURL,
	})
}

// normalizeCallbackStatus normalizes various provider status values to our standard.
func normalizeCallbackStatus(status, event string) string {
	s := strings.ToLower(status)
	switch s {
	case "signed", "completed", "signing_complete":
		return "signed"
	case "declined", "voided", "cancel":
		return "declined"
	default:
		// Check event parameter as fallback.
		e := strings.ToLower(event)
		if strings.Contains(e, "sign") || strings.Contains(e, "complete") {
			return "signed"
		}
		if strings.Contains(e, "decline") || strings.Contains(e, "cancel") {
			return "declined"
		}
		return "signed"
	}
}

// handlePublicError maps errors to appropriate HTTP responses for public endpoints.
func handlePublicError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, entity.ErrInvalidToken):
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or unknown token"})
	case errors.Is(err, entity.ErrTokenExpired):
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "token has expired"})
	case errors.Is(err, entity.ErrMissingToken):
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "missing token"})
	case errors.Is(err, entity.ErrForbidden):
		ctx.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
	case errors.Is(err, entity.ErrInvalidDocumentState):
		ctx.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case isPublicUserError(err):
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		slog.ErrorContext(ctx.Request.Context(), "public signing error", slog.String("error", err.Error()))
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "an internal error occurred"})
	}
}

// isPublicUserError checks if the error is safe to return to the public user.
func isPublicUserError(err error) bool {
	msg := err.Error()
	return strings.HasPrefix(msg, "required field") ||
		strings.HasPrefix(msg, "unknown field") ||
		strings.HasPrefix(msg, "field ") ||
		strings.HasPrefix(msg, "document is not awaiting input") ||
		strings.HasPrefix(msg, "access token has already been used") ||
		strings.HasPrefix(msg, "token is not a signing token") ||
		strings.HasPrefix(msg, "document is not pending signing") ||
		strings.HasPrefix(msg, "document is not in a valid state") ||
		strings.HasPrefix(msg, "document is not completed") ||
		strings.HasPrefix(msg, "completed PDF is not available") ||
		strings.HasPrefix(msg, "signed PDF not available for this document")
}
