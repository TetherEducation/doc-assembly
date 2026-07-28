package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/mapper"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	cataloguc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/catalog"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

// DocumentTypeController handles document type HTTP requests.
// All routes require X-Tenant-ID header and appropriate tenant role.
type DocumentTypeController struct {
	docTypeUC      cataloguc.DocumentTypeUseCase
	templateUC     templateuc.TemplateUseCase
	docTypeMapper  *mapper.DocumentTypeMapper
	templateMapper *mapper.TemplateMapper
}

// NewDocumentTypeController creates a new document type controller.
func NewDocumentTypeController(
	docTypeUC cataloguc.DocumentTypeUseCase,
	templateUC templateuc.TemplateUseCase,
	templateMapper *mapper.TemplateMapper,
) *DocumentTypeController {
	return &DocumentTypeController{
		docTypeUC:      docTypeUC,
		templateUC:     templateUC,
		docTypeMapper:  mapper.NewDocumentTypeMapper(),
		templateMapper: templateMapper,
	}
}

// RegisterRoutes registers all /tenant/document-types routes.
// These routes require X-Tenant-ID header and tenant context.
func (c *DocumentTypeController) RegisterRoutes(rg *gin.RouterGroup, middlewareProvider *middleware.Provider) {
	docTypes := rg.Group("/tenant/document-types")
	docTypes.Use(middlewareProvider.TenantContext())
	{
		docTypes.GET("", middleware.AuthorizeTenantRole(entity.TenantRoleAdmin), c.ListDocumentTypes)
		docTypes.GET("/:id", middleware.AuthorizeTenantRole(entity.TenantRoleAdmin), c.GetDocumentType)
		docTypes.GET("/code/:code", middleware.AuthorizeTenantRole(entity.TenantRoleAdmin), c.GetDocumentTypeByCode)
		docTypes.GET("/code/:code/templates", middleware.AuthorizeTenantRole(entity.TenantRoleAdmin), c.ListTemplatesByTypeCode)
		docTypes.POST("", middleware.AuthorizeTenantRole(entity.TenantRoleOwner), c.CreateDocumentType)
		docTypes.PUT("/:id", middleware.AuthorizeTenantRole(entity.TenantRoleOwner), c.UpdateDocumentType)
		docTypes.DELETE("/:id", middleware.AuthorizeTenantRole(entity.TenantRoleOwner), c.DeleteDocumentType)
	}
}

// ListDocumentTypes lists all document types for the current tenant with pagination.
// @Summary List document types
// @Tags Tenant - Document Types
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param page query int false "Page number" default(1)
// @Param perPage query int false "Items per page" default(10)
// @Param q query string false "Search query for document type name or code"
// @Success 200 {object} dto.PaginatedDocumentTypesResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /api/v1/tenant/document-types [get]
// @Security BearerAuth
func (c *DocumentTypeController) ListDocumentTypes(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	var req dto.DocumentTypeListRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	filters := mapper.DocumentTypeListRequestToFilters(req)
	docTypes, total, err := c.docTypeUC.ListDocumentTypesWithCount(ctx.Request.Context(), tenantID, filters)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.docTypeMapper.ToPaginatedResponse(docTypes, total, req.Page, req.PerPage))
}

// GetDocumentType retrieves a document type by ID.
// @Summary Get document type by ID
// @Tags Tenant - Document Types
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param id path string true "Document Type ID"
// @Success 200 {object} dto.DocumentTypeResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/tenant/document-types/{id} [get]
// @Security BearerAuth
func (c *DocumentTypeController) GetDocumentType(ctx *gin.Context) {
	id := ctx.Param("id")

	docType, err := c.docTypeUC.GetDocumentType(ctx.Request.Context(), id)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.docTypeMapper.ToResponse(docType))
}

// GetDocumentTypeByCode retrieves a document type by code within the current tenant.
// @Summary Get document type by code
// @Tags Tenant - Document Types
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param code path string true "Document Type Code"
// @Success 200 {object} dto.DocumentTypeResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/tenant/document-types/code/{code} [get]
// @Security BearerAuth
func (c *DocumentTypeController) GetDocumentTypeByCode(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	code := ctx.Param("code")

	docType, err := c.docTypeUC.GetDocumentTypeByCode(ctx.Request.Context(), tenantID, code)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.docTypeMapper.ToResponse(docType))
}

// CreateDocumentType creates a new document type.
// @Summary Create document type
// @Tags Tenant - Document Types
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param request body dto.CreateDocumentTypeRequest true "Document type data"
// @Success 201 {object} dto.DocumentTypeResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Router /api/v1/tenant/document-types [post]
// @Security BearerAuth
func (c *DocumentTypeController) CreateDocumentType(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	var req dto.CreateDocumentTypeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	cmd := mapper.CreateDocumentTypeRequestToCommand(tenantID, req)
	docType, err := c.docTypeUC.CreateDocumentType(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, c.docTypeMapper.ToResponse(docType))
}

// UpdateDocumentType updates a document type's name and description.
// @Summary Update document type
// @Tags Tenant - Document Types
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param id path string true "Document Type ID"
// @Param request body dto.UpdateDocumentTypeRequest true "Document type data"
// @Success 200 {object} dto.DocumentTypeResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/tenant/document-types/{id} [put]
// @Security BearerAuth
func (c *DocumentTypeController) UpdateDocumentType(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	id := ctx.Param("id")

	var req dto.UpdateDocumentTypeRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	cmd := mapper.UpdateDocumentTypeRequestToCommand(id, tenantID, req)
	docType, err := c.docTypeUC.UpdateDocumentType(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.docTypeMapper.ToResponse(docType))
}

// DeleteDocumentType attempts to delete a document type.
// If templates are assigned, returns information about them without deleting.
// Use force=true to delete anyway (templates will have their type set to null).
// Use replaceWithId to replace the type in all templates before deleting.
// @Summary Delete document type
// @Tags Tenant - Document Types
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param id path string true "Document Type ID"
// @Param request body dto.DeleteDocumentTypeRequest false "Delete options"
// @Success 200 {object} dto.DeleteDocumentTypeResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/tenant/document-types/{id} [delete]
// @Security BearerAuth
func (c *DocumentTypeController) DeleteDocumentType(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	id := ctx.Param("id")

	var req dto.DeleteDocumentTypeRequest
	// Bind JSON body if present, but don't fail if body is empty
	_ = ctx.ShouldBindJSON(&req)

	cmd := mapper.DeleteDocumentTypeRequestToCommand(id, tenantID, req)
	result, err := c.docTypeUC.DeleteDocumentType(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.docTypeMapper.ToDeleteResponse(result))
}

// ListTemplatesByTypeCode lists all templates using a specific document type code across the tenant.
// @Summary List templates by document type code
// @Tags Tenant - Document Types
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param code path string true "Document Type Code"
// @Success 200 {object} dto.ListTemplatesResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/tenant/document-types/code/{code}/templates [get]
// @Security BearerAuth
func (c *DocumentTypeController) ListTemplatesByTypeCode(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	code := ctx.Param("code")

	templates, err := c.templateUC.FindByDocumentTypeCode(ctx.Request.Context(), tenantID, code)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.templateMapper.ToListResponse(templates, len(templates), 0))
}
