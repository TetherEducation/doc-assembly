package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/mapper"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	cataloguc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/catalog"
)

// ProcessController handles process HTTP requests.
// All routes require X-Tenant-ID header and appropriate tenant role.
type ProcessController struct {
	processUC     cataloguc.ProcessUseCase
	processMapper *mapper.ProcessMapper
}

// NewProcessController creates a new process controller.
func NewProcessController(
	processUC cataloguc.ProcessUseCase,
	processMapper *mapper.ProcessMapper,
) *ProcessController {
	return &ProcessController{
		processUC:     processUC,
		processMapper: processMapper,
	}
}

// RegisterRoutes registers all /tenant/processes routes.
// These routes require X-Tenant-ID header and tenant context.
func (c *ProcessController) RegisterRoutes(rg *gin.RouterGroup, middlewareProvider *middleware.Provider) {
	processes := rg.Group("/tenant/processes")
	processes.Use(middlewareProvider.TenantContext())
	{
		processes.GET("", middleware.AuthorizeTenantRole(entity.TenantRoleAdmin), c.ListProcesses)
		processes.GET("/:id", middleware.AuthorizeTenantRole(entity.TenantRoleAdmin), c.GetProcess)
		processes.GET("/code/:code", middleware.AuthorizeTenantRole(entity.TenantRoleAdmin), c.GetProcessByCode)
		processes.GET("/code/:code/templates", middleware.AuthorizeTenantRole(entity.TenantRoleAdmin), c.ListTemplatesByProcessCode)
		processes.POST("", middleware.AuthorizeTenantRole(entity.TenantRoleOwner), c.CreateProcess)
		processes.PUT("/:id", middleware.AuthorizeTenantRole(entity.TenantRoleOwner), c.UpdateProcess)
		processes.DELETE("/:id", middleware.AuthorizeTenantRole(entity.TenantRoleOwner), c.DeleteProcess)
	}
}

// ListProcesses lists all processes for the current tenant with pagination.
// @Summary List processes
// @Tags Tenant - Processes
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param page query int false "Page number" default(1)
// @Param perPage query int false "Items per page" default(10)
// @Param q query string false "Search query for process name or code"
// @Success 200 {object} dto.PaginatedProcessesResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /api/v1/tenant/processes [get]
// @Security BearerAuth
func (c *ProcessController) ListProcesses(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	var req dto.ProcessListRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	filters := mapper.ProcessListRequestToFilters(req)
	processes, total, err := c.processUC.ListProcessesWithCount(ctx.Request.Context(), tenantID, filters)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.processMapper.ToPaginatedResponse(processes, total, req.Page, req.PerPage))
}

// GetProcess retrieves a process by ID.
// @Summary Get process by ID
// @Tags Tenant - Processes
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param id path string true "Process ID"
// @Success 200 {object} dto.ProcessResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/tenant/processes/{id} [get]
// @Security BearerAuth
func (c *ProcessController) GetProcess(ctx *gin.Context) {
	id := ctx.Param("id")

	process, err := c.processUC.GetProcess(ctx.Request.Context(), id)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.processMapper.ToResponse(process))
}

// GetProcessByCode retrieves a process by code within the current tenant.
// @Summary Get process by code
// @Tags Tenant - Processes
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param code path string true "Process Code"
// @Success 200 {object} dto.ProcessResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/tenant/processes/code/{code} [get]
// @Security BearerAuth
func (c *ProcessController) GetProcessByCode(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	code := ctx.Param("code")

	process, err := c.processUC.GetProcessByCode(ctx.Request.Context(), tenantID, code)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.processMapper.ToResponse(process))
}

// CreateProcess creates a new process.
// @Summary Create process
// @Tags Tenant - Processes
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param request body dto.CreateProcessRequest true "Process data"
// @Success 201 {object} dto.ProcessResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Router /api/v1/tenant/processes [post]
// @Security BearerAuth
func (c *ProcessController) CreateProcess(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	var req dto.CreateProcessRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	cmd := mapper.CreateProcessRequestToCommand(tenantID, req)
	process, err := c.processUC.CreateProcess(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, c.processMapper.ToResponse(process))
}

// UpdateProcess updates a process's name and description.
// @Summary Update process
// @Tags Tenant - Processes
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param id path string true "Process ID"
// @Param request body dto.UpdateProcessRequest true "Process data"
// @Success 200 {object} dto.ProcessResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/tenant/processes/{id} [put]
// @Security BearerAuth
func (c *ProcessController) UpdateProcess(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	id := ctx.Param("id")

	var req dto.UpdateProcessRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	cmd := mapper.UpdateProcessRequestToCommand(id, tenantID, req)
	process, err := c.processUC.UpdateProcess(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.processMapper.ToResponse(process))
}

// DeleteProcess attempts to delete a process.
// If templates are assigned, returns information about them without deleting.
// Use force=true to delete anyway (templates will have their process reset to DEFAULT).
// Use replaceWithCode to replace the process in all templates before deleting.
// @Summary Delete process
// @Tags Tenant - Processes
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param id path string true "Process ID"
// @Param request body dto.DeleteProcessRequest false "Delete options"
// @Success 200 {object} dto.DeleteProcessResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/tenant/processes/{id} [delete]
// @Security BearerAuth
func (c *ProcessController) DeleteProcess(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	id := ctx.Param("id")

	var req dto.DeleteProcessRequest
	// Bind JSON body if present, but don't fail if body is empty
	_ = ctx.ShouldBindJSON(&req)

	cmd := mapper.DeleteProcessRequestToCommand(id, tenantID, req)
	result, err := c.processUC.DeleteProcess(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.processMapper.ToDeleteResponse(result))
}

// ListTemplatesByProcessCode lists all templates using a specific process code across the tenant.
// @Summary List templates by process code
// @Tags Tenant - Processes
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param code path string true "Process Code"
// @Success 200 {array} dto.ProcessTemplateInfoResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/tenant/processes/code/{code}/templates [get]
// @Security BearerAuth
func (c *ProcessController) ListTemplatesByProcessCode(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	code := ctx.Param("code")

	// Validate the process exists (with global fallback for non-SYS tenants)
	process, err := c.processUC.GetProcessByCode(ctx.Request.Context(), tenantID, code)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	// Dry-run delete (no force, no replace) to retrieve the templates list.
	// This returns Deleted=false with the template list when templates exist,
	// or Deleted=true with empty templates when none exist.
	result, err := c.processUC.DeleteProcess(ctx.Request.Context(), cataloguc.DeleteProcessCommand{
		ID:       process.ID,
		TenantID: tenantID,
	})
	if err != nil {
		// Global or DEFAULT processes cannot be deleted, but we still want to
		// list their templates. Since we cannot query templates through the use case
		// in this case, return an empty list.
		ctx.JSON(http.StatusOK, []*dto.ProcessTemplateInfoResponse{})
		return
	}

	ctx.JSON(http.StatusOK, c.processMapper.ToTemplateInfoResponses(result.Templates))
}
