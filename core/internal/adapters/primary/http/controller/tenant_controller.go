package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/mapper"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	organizationuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/organization"
)

// TenantController handles tenant-scoped HTTP requests.
// All routes require X-Tenant-ID header and appropriate tenant role.
type TenantController struct {
	tenantUC       organizationuc.TenantUseCase
	workspaceUC    organizationuc.WorkspaceUseCase
	tenantMemberUC organizationuc.TenantMemberUseCase
}

// NewTenantController creates a new tenant controller.
func NewTenantController(
	tenantUC organizationuc.TenantUseCase,
	workspaceUC organizationuc.WorkspaceUseCase,
	tenantMemberUC organizationuc.TenantMemberUseCase,
) *TenantController {
	return &TenantController{
		tenantUC:       tenantUC,
		workspaceUC:    workspaceUC,
		tenantMemberUC: tenantMemberUC,
	}
}

// RegisterRoutes registers all /tenant routes.
// These routes require X-Tenant-ID header and tenant context.
func (c *TenantController) RegisterRoutes(rg *gin.RouterGroup, middlewareProvider *middleware.Provider) {
	tenant := rg.Group("/tenant")
	tenant.Use(middlewareProvider.TenantContext())
	{
		// Tenant info
		tenant.GET("", middleware.AuthorizeTenantRole(entity.TenantRoleAdmin), c.GetTenant)
		tenant.PUT("", middleware.AuthorizeTenantRole(entity.TenantRoleOwner), c.UpdateCurrentTenant)

		// Workspace routes within tenant
		tenant.GET("/workspaces", middleware.AuthorizeTenantRole(entity.TenantRoleAdmin), c.ListWorkspaces)
		tenant.POST("/workspaces", middleware.AuthorizeTenantRole(entity.TenantRoleOwner), c.CreateWorkspace)
		tenant.PATCH("/workspaces/:workspaceId/status", middleware.AuthorizeTenantRole(entity.TenantRoleAdmin), c.UpdateWorkspaceStatus)
		tenant.DELETE("/workspaces/:workspaceId", middleware.AuthorizeTenantRole(entity.TenantRoleOwner), c.DeleteWorkspace)

		// Tenant member routes
		tenant.GET("/members", middleware.AuthorizeTenantRole(entity.TenantRoleAdmin), c.ListTenantMembers)
		tenant.POST("/members", middleware.AuthorizeTenantRole(entity.TenantRoleOwner), c.AddTenantMember)
		tenant.GET("/members/:memberId", middleware.AuthorizeTenantRole(entity.TenantRoleAdmin), c.GetTenantMember)
		tenant.PUT("/members/:memberId", middleware.AuthorizeTenantRole(entity.TenantRoleOwner), c.UpdateTenantMemberRole)
		tenant.DELETE("/members/:memberId", middleware.AuthorizeTenantRole(entity.TenantRoleOwner), c.RemoveTenantMember)
	}
}

// GetTenant retrieves the current tenant info.
// @Summary Get current tenant
// @Tags Tenant
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Success 200 {object} dto.TenantResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/tenant [get]
// @Security BearerAuth
func (c *TenantController) GetTenant(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	tenant, err := c.tenantUC.GetTenant(ctx.Request.Context(), tenantID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.TenantToResponse(tenant))
}

// UpdateCurrentTenant updates the current tenant's info.
// @Summary Update current tenant
// @Tags Tenant
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param request body dto.UpdateTenantRequest true "Tenant data"
// @Success 200 {object} dto.TenantResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/tenant [put]
// @Security BearerAuth
func (c *TenantController) UpdateCurrentTenant(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	var req dto.UpdateTenantRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	cmd := mapper.UpdateTenantRequestToCommand(tenantID, req)
	tenant, err := c.tenantUC.UpdateTenant(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.TenantToResponse(tenant))
}

// ListWorkspaces lists workspaces with pagination and optional search in the current tenant.
// @Summary List workspaces with pagination and optional search
// @Tags Tenant - Workspaces
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param page query int false "Page number" default(1)
// @Param perPage query int false "Items per page" default(10)
// @Param q query string false "Search query for workspace name"
// @Param status query string false "Filter by status (ACTIVE, SUSPENDED, ARCHIVED)"
// @Success 200 {object} dto.PaginatedWorkspacesResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /api/v1/tenant/workspaces [get]
// @Security BearerAuth
func (c *TenantController) ListWorkspaces(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	userID, ok := middleware.GetInternalUserID(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, dto.NewErrorResponse(entity.ErrUnauthorized))
		return
	}

	var req dto.WorkspaceListRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	filters := mapper.WorkspaceListRequestToFilters(req)
	workspaces, total, err := c.workspaceUC.ListWorkspacesPaginated(ctx.Request.Context(), tenantID, userID, filters)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.WorkspacesToPaginatedResponse(workspaces, total, req.Page, req.PerPage))
}

// CreateWorkspace creates a new workspace in the current tenant.
// @Summary Create workspace in tenant
// @Tags Tenant - Workspaces
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param request body dto.CreateWorkspaceRequest true "Workspace data"
// @Success 201 {object} dto.WorkspaceResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /api/v1/tenant/workspaces [post]
// @Security BearerAuth
func (c *TenantController) CreateWorkspace(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	userID, ok := middleware.GetInternalUserID(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, dto.NewErrorResponse(entity.ErrUnauthorized))
		return
	}

	var req dto.CreateWorkspaceRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	cmd := mapper.CreateWorkspaceRequestToCommand(req, userID)
	cmd.TenantID = &tenantID // Override with tenant from context

	workspace, err := c.workspaceUC.CreateWorkspace(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, mapper.WorkspaceToResponse(workspace))
}

// UpdateWorkspaceStatus updates a workspace's status in the current tenant.
// @Summary Update workspace status
// @Tags Tenant - Workspaces
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param workspaceId path string true "Workspace ID"
// @Param request body dto.UpdateWorkspaceStatusRequest true "Status data"
// @Success 200 {object} dto.WorkspaceResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/tenant/workspaces/{workspaceId}/status [patch]
// @Security BearerAuth
func (c *TenantController) UpdateWorkspaceStatus(ctx *gin.Context) {
	workspaceID := ctx.Param("workspaceId")

	var req dto.UpdateWorkspaceStatusRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	cmd := organizationuc.UpdateWorkspaceStatusCommand{
		ID:     workspaceID,
		Status: entity.WorkspaceStatus(req.Status),
	}
	workspace, err := c.workspaceUC.UpdateWorkspaceStatus(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.WorkspaceToResponse(workspace))
}

// DeleteWorkspace deletes a workspace from the current tenant.
// @Summary Delete workspace from tenant
// @Tags Tenant - Workspaces
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param workspaceId path string true "Workspace ID"
// @Success 204 "No Content"
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/tenant/workspaces/{workspaceId} [delete]
// @Security BearerAuth
func (c *TenantController) DeleteWorkspace(ctx *gin.Context) {
	workspaceID := ctx.Param("workspaceId")

	// TODO: Verify workspace belongs to current tenant before deleting
	if err := c.workspaceUC.ArchiveWorkspace(ctx.Request.Context(), workspaceID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// ListTenantMembers lists all members of the current tenant.
// @Summary List tenant members
// @Tags Tenant - Members
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Success 200 {object} dto.ListResponse[dto.TenantMemberResponse]
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /api/v1/tenant/members [get]
// @Security BearerAuth
func (c *TenantController) ListTenantMembers(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	members, err := c.tenantMemberUC.ListMembers(ctx.Request.Context(), tenantID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	responses := mapper.TenantMembersToResponses(members)
	ctx.JSON(http.StatusOK, dto.NewListResponse(responses))
}

// AddTenantMember adds a user to the current tenant.
// @Summary Add tenant member
// @Tags Tenant - Members
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param request body dto.AddTenantMemberRequest true "Member data"
// @Success 201 {object} dto.TenantMemberResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Router /api/v1/tenant/members [post]
// @Security BearerAuth
func (c *TenantController) AddTenantMember(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	userID, ok := middleware.GetInternalUserID(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, dto.NewErrorResponse(entity.ErrUnauthorized))
		return
	}

	var req dto.AddTenantMemberRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	cmd := mapper.AddTenantMemberRequestToCommand(tenantID, req, userID)
	member, err := c.tenantMemberUC.AddMember(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, mapper.TenantMemberToResponse(member))
}

// GetTenantMember retrieves a specific tenant member.
// @Summary Get tenant member
// @Tags Tenant - Members
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param memberId path string true "Member ID"
// @Success 200 {object} dto.TenantMemberResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/tenant/members/{memberId} [get]
// @Security BearerAuth
func (c *TenantController) GetTenantMember(ctx *gin.Context) {
	memberID := ctx.Param("memberId")

	member, err := c.tenantMemberUC.GetMember(ctx.Request.Context(), memberID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.TenantMemberToResponse(member))
}

// UpdateTenantMemberRole updates a tenant member's role.
// @Summary Update tenant member role
// @Tags Tenant - Members
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param memberId path string true "Member ID"
// @Param request body dto.UpdateTenantMemberRoleRequest true "Role data"
// @Success 200 {object} dto.TenantMemberResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/tenant/members/{memberId} [put]
// @Security BearerAuth
func (c *TenantController) UpdateTenantMemberRole(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	userID, ok := middleware.GetInternalUserID(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, dto.NewErrorResponse(entity.ErrUnauthorized))
		return
	}

	memberID := ctx.Param("memberId")

	var req dto.UpdateTenantMemberRoleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	cmd := mapper.UpdateTenantMemberRoleRequestToCommand(memberID, tenantID, req, userID)
	member, err := c.tenantMemberUC.UpdateMemberRole(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.TenantMemberToResponse(member))
}

// RemoveTenantMember removes a member from the current tenant.
// @Summary Remove tenant member
// @Tags Tenant - Members
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param memberId path string true "Member ID"
// @Success 204 "No Content"
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/tenant/members/{memberId} [delete]
// @Security BearerAuth
func (c *TenantController) RemoveTenantMember(ctx *gin.Context) {
	tenantID, ok := middleware.GetTenantID(ctx)
	if !ok {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(entity.ErrMissingTenantID))
		return
	}

	userID, ok := middleware.GetInternalUserID(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, dto.NewErrorResponse(entity.ErrUnauthorized))
		return
	}

	memberID := ctx.Param("memberId")

	cmd := mapper.RemoveTenantMemberToCommand(memberID, tenantID, userID)
	if err := c.tenantMemberUC.RemoveMember(ctx.Request.Context(), cmd); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}
