package controller

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/dto"
	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
)

// PublicReadOnlyViewController handles public read-only document view endpoints.
// These endpoints do not require JWT authentication; they are validated by VIEW_ONLY token only.
type PublicReadOnlyViewController struct {
	readOnlyViewUC documentuc.ReadOnlyViewUseCase
}

// NewPublicReadOnlyViewController creates a new public read-only view controller.
func NewPublicReadOnlyViewController(readOnlyViewUC documentuc.ReadOnlyViewUseCase) *PublicReadOnlyViewController {
	return &PublicReadOnlyViewController{readOnlyViewUC: readOnlyViewUC}
}

// RegisterRoutes registers public read-only view routes.
// These routes are NOT behind the auth middleware chain.
func (c *PublicReadOnlyViewController) RegisterRoutes(router gin.IRouter) {
	public := router.Group("/public/view")
	{
		public.GET("/:token", c.GetReadOnlyView)
		public.GET("/:token/pdf", c.GetReadOnlyViewPDF)
		public.HEAD("/:token/pdf", c.HeadReadOnlyViewPDF)
	}
}

// GetReadOnlyView returns the current read-only view state for a given access token.
// @Summary Get public read-only view
// @Description Returns read-only document metadata and content/PDF mode for a VIEW_ONLY token.
// @Tags Public Read-Only View
// @Produce json
// @Param token path string true "Read-only view token"
// @Success 200 {object} dto.ReadOnlyViewResponse
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /public/view/{token} [get]
func (c *PublicReadOnlyViewController) GetReadOnlyView(ctx *gin.Context) {
	token := ctx.Param("token")

	resp, err := c.readOnlyViewUC.GetReadOnlyView(ctx.Request.Context(), token)
	if err != nil {
		handlePublicError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dto.NewReadOnlyViewResponse(resp))
}

// GetReadOnlyViewPDF serves the read-only PDF inline for a given access token.
// @Summary Get public read-only view PDF
// @Description Returns the document PDF for a VIEW_ONLY token when the document is in PDF mode.
// @Tags Public Read-Only View
// @Produce application/pdf
// @Param token path string true "Read-only view token"
// @Success 200 {file} binary
// @Failure 401 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /public/view/{token}/pdf [get]
func (c *PublicReadOnlyViewController) GetReadOnlyViewPDF(ctx *gin.Context) {
	token := ctx.Param("token")

	pdfBytes, filename, err := c.readOnlyViewUC.GetReadOnlyViewPDF(ctx.Request.Context(), token)
	if err != nil {
		handlePublicError(ctx, err)
		return
	}

	writeReadOnlyPDFHeaders(ctx, filename, len(pdfBytes))
	ctx.Data(http.StatusOK, "application/pdf", pdfBytes)
}

// HeadReadOnlyViewPDF returns PDF metadata without a body for browser/PDF.js probes.
func (c *PublicReadOnlyViewController) HeadReadOnlyViewPDF(ctx *gin.Context) {
	token := ctx.Param("token")

	pdfBytes, filename, err := c.readOnlyViewUC.GetReadOnlyViewPDF(ctx.Request.Context(), token)
	if err != nil {
		handlePublicError(ctx, err)
		return
	}

	writeReadOnlyPDFHeaders(ctx, filename, len(pdfBytes))
	ctx.Status(http.StatusOK)
}

func writeReadOnlyPDFHeaders(ctx *gin.Context, filename string, size int) {
	ctx.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", filename))
	ctx.Header("Content-Length", fmt.Sprintf("%d", size))
	ctx.Header("Content-Type", "application/pdf")
}
