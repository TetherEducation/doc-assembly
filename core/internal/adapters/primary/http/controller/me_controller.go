package controller

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/mapper"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	accessuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/access"
	organizationuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/organization"
)

// MeController handles user-specific HTTP requests.
// These routes don't require X-Tenant-ID or X-Workspace-ID headers.
type MeController struct {
	tenantUC            organizationuc.TenantUseCase
	tenantMemberRepo    port.TenantMemberRepository
	workspaceMemberRepo port.WorkspaceMemberRepository
	workspaceRepo       port.WorkspaceRepository
	accessHistoryUC     accessuc.UserAccessHistoryUseCase
}

// NewMeController creates a new me controller.
func NewMeController(
	tenantUC organizationuc.TenantUseCase,
	tenantMemberRepo port.TenantMemberRepository,
	workspaceMemberRepo port.WorkspaceMemberRepository,
	workspaceRepo port.WorkspaceRepository,
	accessHistoryUC accessuc.UserAccessHistoryUseCase,
) *MeController {
	return &MeController{
		tenantUC:            tenantUC,
		tenantMemberRepo:    tenantMemberRepo,
		workspaceMemberRepo: workspaceMemberRepo,
		workspaceRepo:       workspaceRepo,
		accessHistoryUC:     accessHistoryUC,
	}
}

// RegisterRoutes registers all /me routes.
// These routes only require authentication, no tenant or workspace context.
func (c *MeController) RegisterRoutes(rg *gin.RouterGroup) {
	me := rg.Group("/me")
	{
		me.GET("/tenants", c.ListMyTenants)
		me.GET("/roles", c.GetMyRoles)
		me.POST("/access", c.RecordAccess)
	}
}

// ListMyTenants lists tenants the current user is a member of with pagination and optional search.
// @Summary List my tenants with pagination and optional search
// @Description Lists tenants where the user is an active member. Supports pagination and optional search by name/code.
// @Tags Me
// @Accept json
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param perPage query int false "Items per page" default(10)
// @Param q query string false "Search query for tenant name or code"
// @Success 200 {object} dto.PaginatedTenantsWithRoleResponse
// @Failure 401 {object} dto.ErrorResponse
// @Router /api/v1/me/tenants [get]
// @Security BearerAuth
func (c *MeController) ListMyTenants(ctx *gin.Context) {
	userID, ok := middleware.GetInternalUserID(ctx)
	if !ok {
		HandleError(ctx, entity.ErrUnauthorized)
		return
	}

	var req dto.TenantListRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	filters := mapper.TenantMemberListRequestToFilters(req)
	tenants, total, err := c.tenantUC.ListUserTenantsPaginated(ctx.Request.Context(), userID, filters)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	response := mapper.TenantsWithRoleToPaginatedResponse(tenants, total, req.Page, req.PerPage)
	ctx.JSON(http.StatusOK, response)
}

// GetMyRoles returns the roles of the current user.
// Optionally includes tenant role and effective workspace role if X-Tenant-ID / X-Workspace-ID headers are provided.
// @Summary Get my roles
// @Description Returns the current user's roles. Always includes system role if assigned.
// @Description Optionally includes tenant role if X-Tenant-ID header is provided.
// @Description Optionally includes workspace role if X-Workspace-ID header is provided.
// @Tags Me
// @Accept json
// @Produce json
// @Param X-Tenant-ID header string false "Tenant ID to check role for"
// @Param X-Workspace-ID header string false "Workspace ID to check role for"
// @Success 200 {object} dto.MyRolesResponse
// @Failure 401 {object} dto.ErrorResponse
// @Router /api/v1/me/roles [get]
// @Security BearerAuth
//
//nolint:funlen,gocognit,gocyclo,nestif
func (c *MeController) GetMyRoles(ctx *gin.Context) {
	userID, ok := middleware.GetInternalUserID(ctx)
	if !ok {
		HandleError(ctx, entity.ErrUnauthorized)
		return
	}

	roles := []dto.RoleEntry{}

	// Check for system role (already loaded by SystemRoleContext middleware)
	if systemRole, ok := middleware.GetSystemRole(ctx); ok {
		roles = append(roles, dto.RoleEntry{
			Type:       "SYSTEM",
			Role:       string(systemRole),
			ResourceID: nil,
		})
	}

	// Check for tenant role if X-Tenant-ID header is provided
	if tenantID, ok := middleware.GetTenantIDFromHeader(ctx); ok {
		member, err := c.tenantMemberRepo.FindActiveByUserAndTenant(ctx.Request.Context(), userID, tenantID)
		if err == nil && member != nil {
			roles = append(roles, dto.RoleEntry{
				Type:       "TENANT",
				Role:       string(member.Role),
				ResourceID: &tenantID,
			})
		}
	}

	// Check for workspace role if X-Workspace-ID header is provided
	if workspaceID, ok := middleware.GetWorkspaceIDFromHeader(ctx); ok {
		if systemRole, ok := middleware.GetSystemRole(ctx); ok && systemRole.HasPermission(entity.SystemRoleSuperAdmin) {
			roles = append(roles, dto.RoleEntry{
				Type:       "WORKSPACE",
				Role:       string(entity.WorkspaceRoleOwner),
				ResourceID: &workspaceID,
			})
		} else if workspace, err := c.workspaceRepo.FindByID(ctx.Request.Context(), workspaceID); err == nil {
			if workspace.TenantID != nil {
				tenantMember, err := c.tenantMemberRepo.FindActiveByUserAndTenant(ctx.Request.Context(), userID, *workspace.TenantID)
				if err == nil && tenantMember != nil && tenantMember.Role.HasPermission(entity.TenantRoleOwner) {
					roles = append(roles, dto.RoleEntry{
						Type:       "WORKSPACE",
						Role:       string(entity.WorkspaceRoleOwner),
						ResourceID: &workspaceID,
					})
				} else if err == nil || errors.Is(err, entity.ErrTenantMemberNotFound) {
					member, err := c.workspaceMemberRepo.FindActiveByUserAndWorkspace(ctx.Request.Context(), userID, workspaceID)
					if err == nil && member != nil {
						roles = append(roles, dto.RoleEntry{
							Type:       "WORKSPACE",
							Role:       string(member.Role),
							ResourceID: &workspaceID,
						})
					}
				}
			} else {
				member, err := c.workspaceMemberRepo.FindActiveByUserAndWorkspace(ctx.Request.Context(), userID, workspaceID)
				if err == nil && member != nil {
					roles = append(roles, dto.RoleEntry{
						Type:       "WORKSPACE",
						Role:       string(member.Role),
						ResourceID: &workspaceID,
					})
				}
			}
		}
	}

	ctx.JSON(http.StatusOK, dto.NewMyRolesResponse(roles))
}

// RecordAccess records that the user accessed a tenant or workspace.
// @Summary Record resource access
// @Description Records that the user accessed a tenant or workspace for quick access history
// @Tags Me
// @Accept json
// @Produce json
// @Param request body dto.RecordAccessRequest true "Access details"
// @Success 204 "Access recorded"
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /api/v1/me/access [post]
// @Security BearerAuth
func (c *MeController) RecordAccess(ctx *gin.Context) {
	userID, ok := middleware.GetInternalUserID(ctx)
	if !ok {
		HandleError(ctx, entity.ErrUnauthorized)
		return
	}

	var req dto.RecordAccessRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	if err := req.Validate(); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
		return
	}

	var err error
	switch entity.AccessEntityType(req.EntityType) {
	case entity.AccessEntityTypeTenant:
		err = c.accessHistoryUC.RecordTenantAccess(ctx.Request.Context(), userID, req.EntityID)
	case entity.AccessEntityTypeWorkspace:
		err = c.accessHistoryUC.RecordWorkspaceAccess(ctx.Request.Context(), userID, req.EntityID)
	}

	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}
