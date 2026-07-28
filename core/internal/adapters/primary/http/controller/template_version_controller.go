package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/mapper"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

// TemplateVersionController handles template version HTTP requests.
type TemplateVersionController struct {
	versionUC        templateuc.TemplateVersionUseCase
	versionMapper    *mapper.TemplateVersionMapper
	templateMapper   *mapper.TemplateMapper
	renderController *RenderController
}

// NewTemplateVersionController creates a new template version controller.
func NewTemplateVersionController(
	versionUC templateuc.TemplateVersionUseCase,
	versionMapper *mapper.TemplateVersionMapper,
	templateMapper *mapper.TemplateMapper,
	renderController *RenderController,
) *TemplateVersionController {
	return &TemplateVersionController{
		versionUC:        versionUC,
		versionMapper:    versionMapper,
		templateMapper:   templateMapper,
		renderController: renderController,
	}
}

// RegisterRoutes registers all template version routes.
// These routes are nested under /content/templates/:templateId/versions
func (c *TemplateVersionController) RegisterRoutes(templates *gin.RouterGroup) {
	versions := templates.Group("/:templateId/versions")
	{
		// Version CRUD
		versions.GET("", c.ListVersions)                                                         // VIEWER+
		versions.POST("", middleware.RequireEditor(), c.CreateVersion)                           // EDITOR+
		versions.POST("/from-existing", middleware.RequireEditor(), c.CreateVersionFromExisting) // EDITOR+
		versions.GET("/:versionId", c.GetVersion)                                                // VIEWER+
		versions.PUT("/:versionId", middleware.RequireEditor(), c.UpdateVersion)                 // EDITOR+
		versions.DELETE("/:versionId", middleware.RequireAdmin(), c.DeleteVersion)               // ADMIN+

		// Lifecycle actions - ADMIN+
		versions.POST("/:versionId/publish", middleware.RequireAdmin(), c.PublishVersion)
		versions.POST("/:versionId/archive", middleware.RequireAdmin(), c.ArchiveVersion)
		versions.POST("/:versionId/schedule-publish", middleware.RequireAdmin(), c.SchedulePublish)
		versions.POST("/:versionId/schedule-archive", middleware.RequireAdmin(), c.ScheduleArchive)
		versions.DELETE("/:versionId/schedule", middleware.RequireAdmin(), c.CancelSchedule)

		// Injectables - EDITOR+
		versions.POST("/:versionId/injectables", middleware.RequireEditor(), c.AddInjectable)
		versions.DELETE("/:versionId/injectables/:injectableId", middleware.RequireEditor(), c.RemoveInjectable)

		// Promotion - EDITOR+
		versions.POST("/:versionId/promote", middleware.RequireEditor(), c.PromoteVersion)

		// Render routes - EDITOR+ (delegates to RenderController)
		if c.renderController != nil {
			c.renderController.RegisterRoutes(versions)
		}
	}
}

// --- Version CRUD Handlers ---

// ListVersions lists all versions for a template.
// @Summary List template versions
// @Tags Template Versions
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Success 200 {object} dto.ListTemplateVersionsResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/versions [get]
func (c *TemplateVersionController) ListVersions(ctx *gin.Context) {
	templateID := ctx.Param("templateId")

	versions, err := c.versionUC.ListVersions(ctx.Request.Context(), templateID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.versionMapper.ToListResponse(versions))
}

// CreateVersion creates a new version for a template.
// @Summary Create template version
// @Tags Template Versions
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Param request body dto.CreateVersionRequest true "Version data"
// @Success 201 {object} dto.TemplateVersionResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/versions [post]
func (c *TemplateVersionController) CreateVersion(ctx *gin.Context) {
	templateID := ctx.Param("templateId")
	userID, _ := middleware.GetInternalUserID(ctx)

	var req dto.CreateVersionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := c.versionMapper.ToCreateCommand(templateID, &req, userID)
	version, err := c.versionUC.CreateVersion(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, c.versionMapper.ToResponse(version))
}

// CreateVersionFromExisting creates a new version copying content from an existing one.
// @Summary Create version from existing
// @Tags Template Versions
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Param request body dto.CreateVersionFromExistingRequest true "Version data"
// @Success 201 {object} dto.TemplateVersionResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/versions/from-existing [post]
func (c *TemplateVersionController) CreateVersionFromExisting(ctx *gin.Context) {
	userID, _ := middleware.GetInternalUserID(ctx)

	var req dto.CreateVersionFromExistingRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	version, err := c.versionUC.CreateVersionFromExisting(
		ctx.Request.Context(),
		req.SourceVersionID,
		req.Name,
		req.Description,
		&userID,
	)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, c.versionMapper.ToResponse(version))
}

// GetVersion retrieves a version by ID with details.
// @Summary Get template version
// @Tags Template Versions
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Param versionId path string true "Version ID"
// @Success 200 {object} dto.TemplateVersionDetailResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/versions/{versionId} [get]
func (c *TemplateVersionController) GetVersion(ctx *gin.Context) {
	versionID := ctx.Param("versionId")

	details, err := c.versionUC.GetVersionWithDetails(ctx.Request.Context(), versionID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.versionMapper.ToDetailResponse(details))
}

// UpdateVersion updates a version.
// @Summary Update template version
// @Tags Template Versions
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Param versionId path string true "Version ID"
// @Param request body dto.UpdateVersionRequest true "Version data"
// @Success 200 {object} dto.TemplateVersionResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/versions/{versionId} [put]
func (c *TemplateVersionController) UpdateVersion(ctx *gin.Context) {
	versionID := ctx.Param("versionId")

	var req dto.UpdateVersionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := c.versionMapper.ToUpdateCommand(versionID, &req)
	version, err := c.versionUC.UpdateVersion(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.versionMapper.ToResponse(version))
}

// DeleteVersion deletes a draft version.
// @Summary Delete template version
// @Tags Template Versions
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Param versionId path string true "Version ID"
// @Success 204 "No Content"
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/versions/{versionId} [delete]
func (c *TemplateVersionController) DeleteVersion(ctx *gin.Context) {
	versionID := ctx.Param("versionId")

	if err := c.versionUC.DeleteVersion(ctx.Request.Context(), versionID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// --- Lifecycle Handlers ---

// PublishVersion publishes a version.
// @Summary Publish template version
// @Tags Template Versions
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Param versionId path string true "Version ID"
// @Success 204 "No Content"
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/versions/{versionId}/publish [post]
func (c *TemplateVersionController) PublishVersion(ctx *gin.Context) {
	versionID := ctx.Param("versionId")
	userID, _ := middleware.GetInternalUserID(ctx)

	if err := c.versionUC.PublishVersion(ctx.Request.Context(), versionID, userID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// ArchiveVersion archives a published version.
// @Summary Archive template version
// @Tags Template Versions
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Param versionId path string true "Version ID"
// @Success 204 "No Content"
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/versions/{versionId}/archive [post]
func (c *TemplateVersionController) ArchiveVersion(ctx *gin.Context) {
	versionID := ctx.Param("versionId")
	userID, _ := middleware.GetInternalUserID(ctx)

	if err := c.versionUC.ArchiveVersion(ctx.Request.Context(), versionID, userID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// SchedulePublish schedules a version for future publication.
// @Summary Schedule version publication
// @Tags Template Versions
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Param versionId path string true "Version ID"
// @Param request body dto.SchedulePublishRequest true "Schedule data"
// @Success 204 "No Content"
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/versions/{versionId}/schedule-publish [post]
func (c *TemplateVersionController) SchedulePublish(ctx *gin.Context) {
	versionID := ctx.Param("versionId")

	var req dto.SchedulePublishRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := c.versionMapper.ToSchedulePublishCommand(versionID, &req)
	if err := c.versionUC.SchedulePublish(ctx.Request.Context(), cmd); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// ScheduleArchive schedules the published version for future archival.
// @Summary Schedule version archival
// @Tags Template Versions
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Param versionId path string true "Version ID"
// @Param request body dto.ScheduleArchiveRequest true "Schedule data"
// @Success 204 "No Content"
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/versions/{versionId}/schedule-archive [post]
func (c *TemplateVersionController) ScheduleArchive(ctx *gin.Context) {
	versionID := ctx.Param("versionId")

	var req dto.ScheduleArchiveRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := c.versionMapper.ToScheduleArchiveCommand(versionID, &req)
	if err := c.versionUC.ScheduleArchive(ctx.Request.Context(), cmd); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// CancelSchedule cancels any scheduled publication or archival.
// @Summary Cancel scheduled action
// @Tags Template Versions
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Param versionId path string true "Version ID"
// @Success 204 "No Content"
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/versions/{versionId}/schedule [delete]
func (c *TemplateVersionController) CancelSchedule(ctx *gin.Context) {
	versionID := ctx.Param("versionId")

	if err := c.versionUC.CancelSchedule(ctx.Request.Context(), versionID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// --- Injectable Handlers ---

// AddInjectable adds an injectable to a version.
// @Summary Add injectable to version
// @Tags Template Versions
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Param versionId path string true "Version ID"
// @Param request body dto.AddVersionInjectableRequest true "Injectable data"
// @Success 201 {object} dto.TemplateVersionInjectableResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/versions/{versionId}/injectables [post]
func (c *TemplateVersionController) AddInjectable(ctx *gin.Context) {
	versionID := ctx.Param("versionId")

	var req dto.AddVersionInjectableRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := c.versionMapper.ToAddInjectableCommand(versionID, &req)
	injectable, err := c.versionUC.AddInjectable(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	// Return basic response
	ctx.JSON(http.StatusCreated, gin.H{
		"id":                     injectable.ID,
		"templateVersionId":      injectable.TemplateVersionID,
		"injectableDefinitionId": injectable.InjectableDefinitionID,
		"isRequired":             injectable.IsRequired,
		"defaultValue":           injectable.DefaultValue,
		"createdAt":              injectable.CreatedAt,
	})
}

// RemoveInjectable removes an injectable from a version.
// @Summary Remove injectable from version
// @Tags Template Versions
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param templateId path string true "Template ID"
// @Param versionId path string true "Version ID"
// @Param injectableId path string true "Injectable ID"
// @Success 204 "No Content"
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/versions/{versionId}/injectables/{injectableId} [delete]
func (c *TemplateVersionController) RemoveInjectable(ctx *gin.Context) {
	injectableID := ctx.Param("injectableId")

	if err := c.versionUC.RemoveInjectable(ctx.Request.Context(), injectableID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// --- Promotion Handlers ---

// PromoteVersion promotes a published version from sandbox to production.
// @Summary Promote version to production
// @Description Promotes a PUBLISHED version from sandbox to production workspace.
// Can create a new template (NEW_TEMPLATE mode) or add as a new version to an existing template (NEW_VERSION mode).
// **Important**: The target workspace (X-Workspace-ID) must be a production workspace, not a sandbox.
// Promoting to sandbox workspaces is not allowed.
// @Tags Template Versions
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Target Production Workspace ID (cannot be a sandbox workspace)"
// @Param templateId path string true "Source Template ID (in sandbox)"
// @Param versionId path string true "Source Version ID (must be PUBLISHED)"
// @Param request body dto.PromoteVersionRequest true "Promotion configuration"
// @Success 201 {object} dto.PromoteAsNewTemplateResponse "When mode=NEW_TEMPLATE"
// @Success 201 {object} dto.PromoteAsNewVersionResponse "When mode=NEW_VERSION"
// @Failure 400 {object} dto.ErrorResponse "Bad request or target workspace is a sandbox"
// @Failure 404 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Router /api/v1/content/templates/{templateId}/versions/{versionId}/promote [post]
func (c *TemplateVersionController) PromoteVersion(ctx *gin.Context) {
	templateID := ctx.Param("templateId")
	versionID := ctx.Param("versionId")
	targetWorkspaceID, _ := middleware.GetWorkspaceID(ctx)
	userID, _ := middleware.GetInternalUserID(ctx)

	var req dto.PromoteVersionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := c.versionMapper.ToPromoteCommand(templateID, versionID, &req, targetWorkspaceID, userID)
	result, err := c.versionUC.PromoteVersion(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	// Respond based on mode
	if result.Template != nil {
		ctx.JSON(http.StatusCreated, dto.PromoteAsNewTemplateResponse{
			Template: *c.templateMapper.ToResponse(result.Template),
			Version:  *c.versionMapper.ToResponse(result.Version),
		})
	} else {
		ctx.JSON(http.StatusCreated, dto.PromoteAsNewVersionResponse{
			Version: *c.versionMapper.ToResponse(result.Version),
		})
	}
}
