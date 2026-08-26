package controller

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
)

// InternalTemplateController serves read-only template questions to trusted services.
//
// These exist because the equivalent automation endpoints require an X-Automation-Key,
// which the middleware explicitly refuses to accept an internal key for. Provisioning an
// automation key for a service that only needs to READ two versions would hand it the
// power to create, publish and archive templates — so the read paths live here instead,
// behind the internal key those services already hold.
type InternalTemplateController struct {
	internalDocUC     documentuc.InternalDocumentUseCase
	templateVersionUC TemplateVersionReader
	versionMapper     TemplateVersionMapper
}

// TemplateVersionReader is the read-only slice of the template version use case this
// controller needs. Declared narrowly rather than taking the full use case: these
// endpoints must never be able to mutate a template, and a two-method dependency makes
// that structural instead of a promise.
type TemplateVersionReader interface {
	ListVersions(ctx context.Context, templateID string) ([]*entity.TemplateVersion, error)
	GetVersionWithDetails(ctx context.Context, id string) (*entity.TemplateVersionWithDetails, error)
}

// TemplateVersionMapper projects version entities into API responses. Declared here as
// the narrow slice this controller needs, so it can be faked in tests without standing up
// the whole mapper.
type TemplateVersionMapper interface {
	ToDetailResponse(details *entity.TemplateVersionWithDetails) *dto.TemplateVersionDetailResponse
	ToListResponse(versions []*entity.TemplateVersion) *dto.ListTemplateVersionsResponse
}

// NewInternalTemplateController creates the internal template controller.
func NewInternalTemplateController(
	internalDocUC documentuc.InternalDocumentUseCase,
	templateVersionUC TemplateVersionReader,
	versionMapper TemplateVersionMapper,
) *InternalTemplateController {
	return &InternalTemplateController{
		internalDocUC:     internalDocUC,
		templateVersionUC: templateVersionUC,
		versionMapper:     versionMapper,
	}
}

// RegisterRoutes registers the internal template routes behind the API key middleware.
func (c *InternalTemplateController) RegisterRoutes(api *gin.RouterGroup, authMiddleware gin.HandlerFunc) {
	templates := api.Group("/internal/templates")
	templates.Use(authMiddleware)
	{
		templates.GET("/resolve", c.ResolveTemplate)
		templates.GET("/:templateId/versions", c.ListVersions)
		templates.GET("/:templateId/versions/:versionId/content", c.GetVersionContent)
	}
}

// ResolveTemplate reports which template version a create would use, without creating one.
// @Summary Resolve the template a create would use
// @Description Runs the configured template resolver and reports the version a document created now would use. Creates nothing.
// @Tags Internal
// @Produce json
// @Param X-API-Key header string true "API Key for authentication"
// @Param X-Tenant-Code header string true "Tenant business code"
// @Param X-Workspace-Code header string true "Workspace business code"
// @Param X-Document-Type header string true "Document type code"
// @Param X-Environment header string true "Environment (prod|dev)"
// @Param X-Process header string false "Process key (default: default)"
// @Success 200 {object} dto.InternalResolveTemplateResponse
// @Failure 400 {object} dto.InternalErrorResponse
// @Failure 401 {object} dto.InternalErrorResponse
// @Failure 404 {object} dto.InternalErrorResponse
// @Failure 500 {object} dto.InternalErrorResponse
// @Router /api/v1/internal/templates/resolve [get]
func (c *InternalTemplateController) ResolveTemplate(ctx *gin.Context) {
	if c.internalDocUC == nil {
		ctx.JSON(http.StatusInternalServerError, dto.InternalErrorResponse{
			Error: "internal document use case is not configured",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	// Deliberately a lighter header contract than create: external and transactional ids
	// identify a document being made, and this makes none.
	cmd := documentuc.InternalResolveTemplateCommand{
		TenantCode:    ctx.GetHeader(HeaderTenantCode),
		WorkspaceCode: ctx.GetHeader(HeaderWorkspaceCode),
		DocumentType:  ctx.GetHeader(HeaderDocumentType),
		Process:       ctx.GetHeader(HeaderProcess),
		ProcessType:   ctx.GetHeader(HeaderProcessType),
		Environment:   entity.Environment(ctx.GetHeader(HeaderEnvironment)),
	}

	var missing []string
	if cmd.TenantCode == "" {
		missing = append(missing, HeaderTenantCode)
	}
	if cmd.WorkspaceCode == "" {
		missing = append(missing, HeaderWorkspaceCode)
	}
	if cmd.DocumentType == "" {
		missing = append(missing, HeaderDocumentType)
	}
	if cmd.Environment == "" {
		missing = append(missing, HeaderEnvironment)
	}
	if len(missing) > 0 {
		ctx.JSON(http.StatusBadRequest, dto.InternalErrorResponse{
			Error:   "missing required headers",
			Code:    "MISSING_HEADERS",
			Details: missing,
		})
		return
	}

	result, err := c.internalDocUC.ResolveTemplate(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, buildResolveTemplateResponse(result))
}

// ListVersions lists every version of a template, newest first.
// @Summary List template versions via internal API
// @Tags Internal
// @Produce json
// @Param X-API-Key header string true "API Key for authentication"
// @Param templateId path string true "Template ID"
// @Success 200 {array} dto.TemplateVersionResponse
// @Failure 401 {object} dto.InternalErrorResponse
// @Failure 404 {object} dto.InternalErrorResponse
// @Router /api/v1/internal/templates/{templateId}/versions [get]
func (c *InternalTemplateController) ListVersions(ctx *gin.Context) {
	if c.templateVersionUC == nil {
		ctx.JSON(http.StatusInternalServerError, dto.InternalErrorResponse{
			Error: "template version use case is not configured",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	versions, err := c.templateVersionUC.ListVersions(ctx.Request.Context(), ctx.Param("templateId"))
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.versionMapper.ToListResponse(versions))
}

// GetVersionContent returns one version with its full content structure.
// @Summary Get template version content via internal API
// @Tags Internal
// @Produce json
// @Param X-API-Key header string true "API Key for authentication"
// @Param templateId path string true "Template ID"
// @Param versionId path string true "Version ID"
// @Success 200 {object} dto.TemplateVersionDetailResponse
// @Failure 401 {object} dto.InternalErrorResponse
// @Failure 404 {object} dto.InternalErrorResponse
// @Router /api/v1/internal/templates/{templateId}/versions/{versionId}/content [get]
func (c *InternalTemplateController) GetVersionContent(ctx *gin.Context) {
	if c.templateVersionUC == nil {
		ctx.JSON(http.StatusInternalServerError, dto.InternalErrorResponse{
			Error: "template version use case is not configured",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	details, err := c.templateVersionUC.GetVersionWithDetails(ctx.Request.Context(), ctx.Param("versionId"))
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.versionMapper.ToDetailResponse(details))
}
