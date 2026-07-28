package controller

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/mapper"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

// ContentTemplateController handles template-related HTTP requests.
type ContentTemplateController struct {
	templateUC        templateuc.TemplateUseCase
	templateMapper    *mapper.TemplateMapper
	versionController *TemplateVersionController
}

// NewContentTemplateController creates a new template controller.
func NewContentTemplateController(
	templateUC templateuc.TemplateUseCase,
	templateMapper *mapper.TemplateMapper,
	versionController *TemplateVersionController,
) *ContentTemplateController {
	return &ContentTemplateController{
		templateUC:        templateUC,
		templateMapper:    templateMapper,
		versionController: versionController,
	}
}

// RegisterRoutes registers all template routes.
// All template routes require X-Workspace-ID header.
// Template and version routes support sandbox mode via X-Sandbox-Mode header.
func (c *ContentTemplateController) RegisterRoutes(rg *gin.RouterGroup, middlewareProvider *middleware.Provider) {
	// Content group requires X-Workspace-ID header
	content := rg.Group("/content")
	content.Use(middlewareProvider.WorkspaceContext())
	content.Use(middlewareProvider.SandboxContext()) // Sandbox support for templates
	{
		// Template routes
		templates := content.Group("/templates")
		{
			templates.GET("", c.ListTemplates)                                                // VIEWER+
			templates.POST("", middleware.RequireEditor(), c.CreateTemplate)                  // EDITOR+
			templates.GET("/:templateId", c.GetTemplate)                                      // VIEWER+
			templates.GET("/:templateId/all-versions", c.GetTemplateWithAllVersions)          // VIEWER+
			templates.PUT("/:templateId", middleware.RequireEditor(), c.UpdateTemplate)       // EDITOR+
			templates.DELETE("/:templateId", middleware.RequireAdmin(), c.DeleteTemplate)     // ADMIN+
			templates.POST("/:templateId/clone", middleware.RequireEditor(), c.CloneTemplate) // EDITOR+

			// Template tag routes (tags belong to templates, not versions)
			templates.POST("/:templateId/tags", middleware.RequireEditor(), c.AddTemplateTags)            // EDITOR+
			templates.DELETE("/:templateId/tags/:tagId", middleware.RequireEditor(), c.RemoveTemplateTag) // EDITOR+

			// Process fields
			templates.PUT("/:templateId/process", middleware.RequireEditor(), c.SetProcessFields) // EDITOR+

			// Document type assignment
			templates.PUT("/:templateId/document-type", middleware.RequireEditor(), c.AssignDocumentType) // EDITOR+

			// Version routes (nested under templates)
			c.versionController.RegisterRoutes(templates)
		}
	}
}

// ListTemplates lists all templates in a workspace.
// @Summary List templates
// @Tags Templates
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param folderId query string false "Filter by folder ID. Use 'root' to get only root-level templates (no folder)"
// @Param hasPublishedVersion query bool false "Filter by published status"
// @Param tagIds query []string false "Filter by tag IDs"
// @Param search query string false "Search by title"
// @Param limit query int false "Limit results"
// @Param offset query int false "Offset results"
// @Success 200 {object} dto.ListTemplatesResponse
// @Failure 401 {object} dto.ErrorResponse
// @Router /api/v1/content/templates [get]
func (c *ContentTemplateController) ListTemplates(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	var filtersReq dto.TemplateFiltersRequest
	if err := ctx.ShouldBindQuery(&filtersReq); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	filters := c.templateMapper.ToFilters(&filtersReq)
	templates, err := c.templateUC.ListTemplates(ctx.Request.Context(), workspaceID, filters)
	if err != nil {
		slog.ErrorContext(ctx.Request.Context(), "failed to list templates",
			slog.String("workspace_id", workspaceID),
			slog.Any("error", err),
		)
		respondError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, c.templateMapper.ToListResponse(templates, filtersReq.Limit, filtersReq.Offset))
}

// CreateTemplate creates a new template with an initial draft version.
// @Summary Create template
// @Tags Templates
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param request body dto.CreateTemplateRequest true "Template data"
// @Success 201 {object} dto.TemplateCreateResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Router /api/v1/content/templates [post]
func (c *ContentTemplateController) CreateTemplate(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)
	userID, _ := middleware.GetInternalUserID(ctx)

	var req dto.CreateTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := c.templateMapper.ToCreateCommand(&req, workspaceID, userID)
	template, version, err := c.templateUC.CreateTemplate(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, c.templateMapper.ToCreateResponse(template, version))
}

// GetTemplate retrieves a template by ID with published version details.
// @Summary Get template
// @Tags Templates
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Success 200 {object} dto.TemplateWithDetailsResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId} [get]
func (c *ContentTemplateController) GetTemplate(ctx *gin.Context) {
	templateID := ctx.Param("templateId")

	details, err := c.templateUC.GetTemplateWithDetails(ctx.Request.Context(), templateID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.templateMapper.ToDetailsResponse(details))
}

// GetTemplateWithAllVersions retrieves a template with all its versions.
// @Summary Get template with all versions
// @Tags Templates
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Success 200 {object} dto.TemplateWithAllVersionsResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/all-versions [get]
func (c *ContentTemplateController) GetTemplateWithAllVersions(ctx *gin.Context) {
	templateID := ctx.Param("templateId")

	details, err := c.templateUC.GetTemplateWithAllVersions(ctx.Request.Context(), templateID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.templateMapper.ToAllVersionsResponse(details))
}

// UpdateTemplate updates a template's metadata.
// @Summary Update template
// @Description Updates a template's metadata. Use folderId="root" to move the template to the root folder.
// @Tags Templates
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Param request body dto.UpdateTemplateRequest true "Template data (folderId can be a folder UUID or 'root' to move to root)"
// @Success 200 {object} dto.TemplateResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId} [put]
func (c *ContentTemplateController) UpdateTemplate(ctx *gin.Context) {
	templateID := ctx.Param("templateId")

	var req dto.UpdateTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := c.templateMapper.ToUpdateCommand(templateID, &req)
	template, err := c.templateUC.UpdateTemplate(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.templateMapper.ToResponse(template))
}

// DeleteTemplate deletes a template and all its versions.
// @Summary Delete template
// @Tags Templates
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Success 204 "No Content"
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId} [delete]
func (c *ContentTemplateController) DeleteTemplate(ctx *gin.Context) {
	templateID := ctx.Param("templateId")

	if err := c.templateUC.DeleteTemplate(ctx.Request.Context(), templateID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// CloneTemplate creates a copy of an existing template from a specific version.
// @Summary Clone template from specific version
// @Description Clones a template using the content from a specific version (identified by versionId in request body). The versionId must belong to the specified templateId.
// @Tags Templates
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Param request body dto.CloneTemplateRequest true "Clone data (versionId is required and must belong to the template)"
// @Success 201 {object} dto.TemplateCreateResponse
// @Failure 400 {object} dto.ErrorResponse "Bad request (invalid versionId, version doesn't belong to template, validation error)"
// @Failure 404 {object} dto.ErrorResponse "Template or version not found"
// @Failure 409 {object} dto.ErrorResponse "Template title already exists"
// @Router /api/v1/content/templates/{templateId}/clone [post]
func (c *ContentTemplateController) CloneTemplate(ctx *gin.Context) {
	templateID := ctx.Param("templateId")
	userID, _ := middleware.GetInternalUserID(ctx)

	var req dto.CloneTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := c.templateMapper.ToCloneCommand(templateID, &req, userID)
	template, version, err := c.templateUC.CloneTemplate(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, c.templateMapper.ToCreateResponse(template, version))
}

// AddTemplateTags adds tags to a template.
// @Summary Add tags to template
// @Tags Templates
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Param request body dto.AddTagsRequest true "Tag IDs"
// @Success 204 "No Content"
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/tags [post]
func (c *ContentTemplateController) AddTemplateTags(ctx *gin.Context) {
	templateID := ctx.Param("templateId")

	var req dto.AddTagsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	for _, tagID := range req.TagIDs {
		if err := c.templateUC.AddTag(ctx.Request.Context(), templateID, tagID); err != nil {
			HandleError(ctx, err)
			return
		}
	}

	ctx.Status(http.StatusNoContent)
}

// RemoveTemplateTag removes a tag from a template.
// @Summary Remove tag from template
// @Tags Templates
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Param tagId path string true "Tag ID"
// @Success 204 "No Content"
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/tags/{tagId} [delete]
func (c *ContentTemplateController) RemoveTemplateTag(ctx *gin.Context) {
	templateID := ctx.Param("templateId")
	tagID := ctx.Param("tagId")

	if err := c.templateUC.RemoveTag(ctx.Request.Context(), templateID, tagID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// SetProcessFields sets the process and processType on a template.
// @Summary Set process fields on template
// @Tags Templates
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Param request body dto.SetProcessFieldsRequest true "Process fields"
// @Success 200 {object} dto.TemplateResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/process [put]
func (c *ContentTemplateController) SetProcessFields(ctx *gin.Context) {
	templateID := ctx.Param("templateId")
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	var req dto.SetProcessFieldsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := templateuc.SetProcessFieldsCommand{
		TemplateID:  templateID,
		WorkspaceID: workspaceID,
		Process:     req.Process,
		ProcessType: req.ProcessType,
	}

	template, err := c.templateUC.SetProcessFields(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.templateMapper.ToResponse(template))
}

// AssignDocumentType assigns or unassigns a document type to a template.
// @Summary Assign document type to template
// @Description Assigns a document type to a template. If the type is already assigned to another template in the workspace and force=false, returns conflict info. Use force=true to reassign the type (previous template will have its type unassigned).
// @Tags Templates
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Param request body dto.AssignDocumentTypeRequest true "Document type assignment data"
// @Success 200 {object} dto.AssignDocumentTypeResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/document-type [put]
func (c *ContentTemplateController) AssignDocumentType(ctx *gin.Context) {
	templateID := ctx.Param("templateId")
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	var req dto.AssignDocumentTypeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := mapper.AssignDocumentTypeRequestToCommand(templateID, workspaceID, req)
	result, err := c.templateUC.AssignDocumentType(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.AssignResultToResponse(result, c.templateMapper))
}
