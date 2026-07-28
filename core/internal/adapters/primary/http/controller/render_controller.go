package controller

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

// RenderController handles document rendering HTTP requests.
type RenderController struct {
	versionUC   templateuc.TemplateVersionUseCase
	pdfRenderer port.PDFRenderer
}

// NewRenderController creates a new render controller.
func NewRenderController(
	versionUC templateuc.TemplateVersionUseCase,
	pdfRenderer port.PDFRenderer,
) *RenderController {
	return &RenderController{
		versionUC:   versionUC,
		pdfRenderer: pdfRenderer,
	}
}

// RegisterRoutes registers all render routes.
// These routes are nested under /content/templates/:templateId/versions/:versionId
func (c *RenderController) RegisterRoutes(versions *gin.RouterGroup) {
	// Preview route requires EDITOR+ role
	versions.POST("/:versionId/preview", middleware.RequireEditor(), c.PreviewVersion)
}

// PreviewVersion generates a preview PDF for a template version.
// @Summary Generate preview PDF
// @Tags Template Versions
// @Accept json
// @Produce application/pdf
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param templateId path string true "Template ID"
// @Param versionId path string true "Version ID"
// @Param request body dto.RenderPreviewRequest true "Injectable values"
// @Success 200 {file} application/pdf
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/versions/{versionId}/preview [post]
func (c *RenderController) PreviewVersion(ctx *gin.Context) {
	versionID := ctx.Param("versionId")

	// Parse request body
	var req dto.RenderPreviewRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		// Allow empty body (no injectables)
		if err.Error() != "EOF" {
			respondError(ctx, http.StatusBadRequest, err)
			return
		}
		req.Injectables = make(map[string]any)
	}

	// Get version with details
	details, err := c.versionUC.GetVersionWithDetails(ctx.Request.Context(), versionID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	// Parse content structure into portable document
	doc, err := portabledoc.Parse(details.ContentStructure)
	if err != nil {
		slog.ErrorContext(ctx.Request.Context(), "failed to parse content structure",
			slog.String("version_id", versionID),
			slog.Any("error", err),
		)
		respondError(ctx, http.StatusInternalServerError, fmt.Errorf("invalid content structure"))
		return
	}

	if doc == nil {
		respondError(ctx, http.StatusBadRequest, fmt.Errorf("version has no content"))
		return
	}

	// Build injectable defaults map from version injectables
	injectableDefaults := buildInjectableDefaults(details.Injectables)

	// Render PDF
	result, err := c.pdfRenderer.RenderPreview(ctx.Request.Context(), &port.RenderPreviewRequest{
		Document:           doc,
		Injectables:        req.Injectables,
		InjectableDefaults: injectableDefaults,
	})
	if err != nil {
		slog.ErrorContext(ctx.Request.Context(), "failed to render PDF",
			slog.String("version_id", versionID),
			slog.Any("error", err),
		)
		respondError(ctx, http.StatusInternalServerError, fmt.Errorf("failed to generate PDF"))
		return
	}

	// Set response headers
	ctx.Header("Content-Type", "application/pdf")
	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", result.Filename))
	ctx.Header("Content-Length", fmt.Sprintf("%d", len(result.PDF)))

	// Write PDF bytes
	ctx.Data(http.StatusOK, "application/pdf", result.PDF)
}

// buildInjectableDefaults builds a map of default values from version injectables.
// Priority: TemplateVersionInjectable.DefaultValue > InjectableDefinition.DefaultValue
func buildInjectableDefaults(injectables []*entity.VersionInjectableWithDefinition) map[string]string {
	defaults := make(map[string]string)

	for _, injectable := range injectables {
		// Get the variable ID (either from definition key or system key)
		var variableID string
		if injectable.Definition != nil {
			variableID = injectable.Definition.Key
		} else if injectable.SystemInjectableKey != nil {
			variableID = *injectable.SystemInjectableKey
		} else {
			continue
		}

		// First, try template version specific default
		if injectable.DefaultValue != nil && *injectable.DefaultValue != "" {
			defaults[variableID] = *injectable.DefaultValue
			continue
		}

		// Fallback to injectable definition default
		if injectable.Definition != nil && injectable.Definition.DefaultValue != nil && *injectable.Definition.DefaultValue != "" {
			defaults[variableID] = *injectable.Definition.DefaultValue
		}
	}

	return defaults
}
