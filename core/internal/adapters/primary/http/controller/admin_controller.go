package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/mapper"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	accessuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/access"
	injectableuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/injectable"
	organizationuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/organization"
)

// NewAdminController creates a new admin controller.
func NewAdminController(
	tenantUC organizationuc.TenantUseCase,
	systemRoleUC accessuc.SystemRoleUseCase,
	systemInjectableUC injectableuc.SystemInjectableUseCase,
) *AdminController {
	return &AdminController{
		tenantUC:           tenantUC,
		systemRoleUC:       systemRoleUC,
		systemInjectableUC: systemInjectableUC,
	}
}

// AdminController handles admin-related HTTP requests.
// All routes require system-level roles (SUPERADMIN or PLATFORM_ADMIN).
type AdminController struct {
	tenantUC           organizationuc.TenantUseCase
	systemRoleUC       accessuc.SystemRoleUseCase
	systemInjectableUC injectableuc.SystemInjectableUseCase
}

// RegisterRoutes registers all admin routes.
// System routes do NOT require X-Workspace-ID or X-Tenant-ID headers.
func (c *AdminController) RegisterRoutes(rg *gin.RouterGroup) {
	system := rg.Group("/system")
	system.Use(middleware.RequirePlatformAdmin()) // Base requirement: PLATFORM_ADMIN
	{
		// Tenant routes
		// List and Get: PLATFORM_ADMIN
		// Create and Delete: SUPERADMIN
		system.GET("/tenants", c.ListTenantsPaginated)
		system.POST("/tenants", middleware.RequireSuperAdmin(), c.CreateTenant)
		system.GET("/tenants/:tenantId", c.GetTenant)
		system.PUT("/tenants/:tenantId", c.UpdateTenant)
		system.PATCH("/tenants/:tenantId/status", middleware.RequireSuperAdmin(), c.UpdateTenantStatus)
		system.DELETE("/tenants/:tenantId", middleware.RequireSuperAdmin(), c.DeleteTenant)
		system.GET("/tenants/:tenantId/workspaces", c.ListTenantWorkspaces)

		// System roles management (SUPERADMIN only)
		system.GET("/users", middleware.RequireSuperAdmin(), c.ListSystemUsers)
		system.POST("/users", middleware.RequireSuperAdmin(), c.AddSystemRole)
		system.POST("/users/:userId/role", middleware.RequireSuperAdmin(), c.AssignSystemRole)
		system.DELETE("/users/:userId/role", middleware.RequireSuperAdmin(), c.RevokeSystemRole)

		// System injectables management
		// List: PLATFORM_ADMIN+
		// Activate/Deactivate and assignments: SUPERADMIN only
		injectables := system.Group("/injectables")
		{
			injectables.GET("", c.ListSystemInjectables)
			injectables.PATCH("/:key/activate", middleware.RequireSuperAdmin(), c.ActivateInjectable)
			injectables.PATCH("/:key/deactivate", middleware.RequireSuperAdmin(), c.DeactivateInjectable)
			injectables.GET("/:key/assignments", c.ListAssignments)
			injectables.POST("/:key/assignments", middleware.RequireSuperAdmin(), c.CreateAssignment)
			injectables.DELETE("/:key/assignments/:assignmentId", middleware.RequireSuperAdmin(), c.DeleteAssignment)
			injectables.PATCH("/:key/assignments/:assignmentId/exclude", middleware.RequireSuperAdmin(), c.ExcludeAssignment)
			injectables.PATCH("/:key/assignments/:assignmentId/include", middleware.RequireSuperAdmin(), c.IncludeAssignment)

			// Bulk operations for PUBLIC assignments
			injectables.POST("/bulk/public", middleware.RequireSuperAdmin(), c.BulkCreatePublicAssignments)
			injectables.DELETE("/bulk/public", middleware.RequireSuperAdmin(), c.BulkDeletePublicAssignments)
		}
	}
}

// --- Tenant Handlers ---

// ListTenantsPaginated lists tenants with pagination and optional search.
// @Summary List tenants with pagination
// @Tags System - Tenants
// @Accept json
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param perPage query int false "Items per page" default(10)
// @Param q query string false "Search query (name or code)"
// @Success 200 {object} dto.PaginatedTenantsResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /api/v1/system/tenants [get]
// @Security BearerAuth
func (c *AdminController) ListTenantsPaginated(ctx *gin.Context) {
	var req dto.TenantListRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	filters := mapper.TenantListRequestToFilters(req)
	tenants, total, err := c.tenantUC.ListTenantsPaginated(ctx.Request.Context(), filters)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.TenantsToPaginatedResponse(tenants, total, req.Page, req.PerPage))
}

// CreateTenant creates a new tenant.
// Requires SUPERADMIN role.
// @Summary Create tenant
// @Tags System - Tenants
// @Accept json
// @Produce json
// @Param request body dto.CreateTenantRequest true "Tenant data"
// @Success 201 {object} dto.TenantResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Router /api/v1/system/tenants [post]
// @Security BearerAuth
func (c *AdminController) CreateTenant(ctx *gin.Context) {
	var req dto.CreateTenantRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	if err := req.Validate(); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := mapper.CreateTenantRequestToCommand(req)
	tenant, err := c.tenantUC.CreateTenant(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, mapper.TenantToResponse(tenant))
}

// GetTenant retrieves a tenant by ID.
// @Summary Get tenant
// @Tags System - Tenants
// @Accept json
// @Produce json
// @Param tenantId path string true "Tenant ID"
// @Success 200 {object} dto.TenantResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/system/tenants/{tenantId} [get]
// @Security BearerAuth
func (c *AdminController) GetTenant(ctx *gin.Context) {
	tenantID := ctx.Param("tenantId")

	tenant, err := c.tenantUC.GetTenant(ctx.Request.Context(), tenantID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.TenantToResponse(tenant))
}

// UpdateTenant updates a tenant.
// @Summary Update tenant
// @Tags System - Tenants
// @Accept json
// @Produce json
// @Param tenantId path string true "Tenant ID"
// @Param request body dto.UpdateTenantRequest true "Tenant data"
// @Success 200 {object} dto.TenantResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/system/tenants/{tenantId} [put]
// @Security BearerAuth
func (c *AdminController) UpdateTenant(ctx *gin.Context) {
	tenantID := ctx.Param("tenantId")

	var req dto.UpdateTenantRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	if err := req.Validate(); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
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

// UpdateTenantStatus updates a tenant's status.
// Requires SUPERADMIN role.
// @Summary Update tenant status
// @Tags System - Tenants
// @Accept json
// @Produce json
// @Param tenantId path string true "Tenant ID"
// @Param request body dto.UpdateTenantStatusRequest true "Status data"
// @Success 200 {object} dto.TenantResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/system/tenants/{tenantId}/status [patch]
// @Security BearerAuth
func (c *AdminController) UpdateTenantStatus(ctx *gin.Context) {
	tenantID := ctx.Param("tenantId")

	var req dto.UpdateTenantStatusRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	if err := req.Validate(); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := organizationuc.UpdateTenantStatusCommand{
		ID:     tenantID,
		Status: entity.TenantStatus(req.Status),
	}
	tenant, err := c.tenantUC.UpdateTenantStatus(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.TenantToResponse(tenant))
}

// DeleteTenant deletes a tenant.
// @Summary Delete tenant
// @Tags System - Tenants
// @Accept json
// @Produce json
// @Param tenantId path string true "Tenant ID"
// @Success 204 "No Content"
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/system/tenants/{tenantId} [delete]
// @Security BearerAuth
func (c *AdminController) DeleteTenant(ctx *gin.Context) {
	tenantID := ctx.Param("tenantId")

	if err := c.tenantUC.DeleteTenant(ctx.Request.Context(), tenantID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// ListTenantWorkspaces lists workspaces for a specific tenant with optional search.
// @Summary List tenant workspaces
// @Tags System - Tenants
// @Accept json
// @Produce json
// @Param tenantId path string true "Tenant ID"
// @Param page query int false "Page number" default(1)
// @Param perPage query int false "Items per page" default(10)
// @Param q query string false "Search query (name)"
// @Success 200 {object} dto.PaginatedWorkspacesResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/system/tenants/{tenantId}/workspaces [get]
// @Security BearerAuth
func (c *AdminController) ListTenantWorkspaces(ctx *gin.Context) {
	tenantID := ctx.Param("tenantId")

	var req dto.WorkspaceListRequest
	if err := ctx.ShouldBindQuery(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	filters := mapper.WorkspaceListRequestToFilters(req)
	workspaces, total, err := c.tenantUC.ListTenantWorkspaces(ctx.Request.Context(), tenantID, filters)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.WorkspacesToPaginatedResponse(workspaces, total, req.Page, req.PerPage))
}

// --- System Role Handlers ---

// ListSystemUsers lists all users with system roles.
// Requires SUPERADMIN role.
// @Summary List users with system roles
// @Tags System - Users
// @Accept json
// @Produce json
// @Success 200 {object} dto.ListResponse[dto.SystemRoleWithUserResponse]
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /api/v1/system/users [get]
// @Security BearerAuth
func (c *AdminController) ListSystemUsers(ctx *gin.Context) {
	users, err := c.systemRoleUC.ListUsersWithSystemRoles(ctx.Request.Context())
	if err != nil {
		HandleError(ctx, err)
		return
	}

	responses := mapper.SystemRolesWithUserToResponses(users)
	ctx.JSON(http.StatusOK, dto.NewListResponse(responses))
}

// AddSystemRole finds or creates a user by email and assigns a system role.
// Requires SUPERADMIN role.
// @Summary Add system member
// @Tags System - Users
// @Accept json
// @Produce json
// @Param request body dto.AddSystemRoleRequest true "System member data"
// @Success 200 {object} dto.SystemRoleResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /api/v1/system/users [post]
// @Security BearerAuth
func (c *AdminController) AddSystemRole(ctx *gin.Context) {
	grantedBy, ok := middleware.GetInternalUserID(ctx)
	if !ok {
		respondError(ctx, http.StatusUnauthorized, entity.ErrUnauthorized)
		return
	}

	var req dto.AddSystemRoleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	if err := req.Validate(); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := mapper.AddSystemRoleRequestToCommand(req, grantedBy)
	assignment, err := c.systemRoleUC.AddRole(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.SystemRoleToResponse(assignment))
}

// AssignSystemRole assigns a system role to a user.
// Requires SUPERADMIN role.
// @Summary Assign system role
// @Tags System - Users
// @Accept json
// @Produce json
// @Param userId path string true "User ID"
// @Param request body dto.AssignSystemRoleRequest true "Role data"
// @Success 200 {object} dto.SystemRoleResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/system/users/{userId}/role [post]
// @Security BearerAuth
func (c *AdminController) AssignSystemRole(ctx *gin.Context) {
	userID := ctx.Param("userId")

	grantedBy, ok := middleware.GetInternalUserID(ctx)
	if !ok {
		respondError(ctx, http.StatusUnauthorized, entity.ErrUnauthorized)
		return
	}

	var req dto.AssignSystemRoleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	if err := req.Validate(); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := mapper.AssignSystemRoleRequestToCommand(userID, req, grantedBy)
	assignment, err := c.systemRoleUC.AssignRole(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.SystemRoleToResponse(assignment))
}

// RevokeSystemRole revokes a user's system role.
// Requires SUPERADMIN role.
// @Summary Revoke system role
// @Tags System - Users
// @Accept json
// @Produce json
// @Param userId path string true "User ID"
// @Success 204 "No Content"
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/system/users/{userId}/role [delete]
// @Security BearerAuth
func (c *AdminController) RevokeSystemRole(ctx *gin.Context) {
	userID := ctx.Param("userId")

	revokedBy, ok := middleware.GetInternalUserID(ctx)
	if !ok {
		respondError(ctx, http.StatusUnauthorized, entity.ErrUnauthorized)
		return
	}

	cmd := mapper.RevokeSystemRoleToCommand(userID, revokedBy)
	if err := c.systemRoleUC.RevokeRole(ctx.Request.Context(), cmd); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// --- System Injectable Handlers ---

// ListSystemInjectables lists all system injectables with their active state.
// @Summary List system injectables
// @Tags System - Injectables
// @Accept json
// @Produce json
// @Success 200 {object} dto.ListSystemInjectablesResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /api/v1/system/injectables [get]
// @Security BearerAuth
func (c *AdminController) ListSystemInjectables(ctx *gin.Context) {
	injectables, err := c.systemInjectableUC.ListAll(ctx.Request.Context())
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dto.ToListSystemInjectablesResponse(injectables))
}

// ActivateInjectable activates a system injectable globally.
// @Summary Activate system injectable
// @Tags System - Injectables
// @Accept json
// @Produce json
// @Param key path string true "Injectable key"
// @Success 204 "No Content"
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/system/injectables/{key}/activate [patch]
// @Security BearerAuth
func (c *AdminController) ActivateInjectable(ctx *gin.Context) {
	key := ctx.Param("key")

	if err := c.systemInjectableUC.Activate(ctx.Request.Context(), key); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// DeactivateInjectable deactivates a system injectable globally.
// @Summary Deactivate system injectable
// @Tags System - Injectables
// @Accept json
// @Produce json
// @Param key path string true "Injectable key"
// @Success 204 "No Content"
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/system/injectables/{key}/deactivate [patch]
// @Security BearerAuth
func (c *AdminController) DeactivateInjectable(ctx *gin.Context) {
	key := ctx.Param("key")

	if err := c.systemInjectableUC.Deactivate(ctx.Request.Context(), key); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// ListAssignments lists all assignments for a system injectable.
// @Summary List injectable assignments
// @Tags System - Injectables
// @Accept json
// @Produce json
// @Param key path string true "Injectable key"
// @Success 200 {object} dto.ListAssignmentsResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/system/injectables/{key}/assignments [get]
// @Security BearerAuth
func (c *AdminController) ListAssignments(ctx *gin.Context) {
	key := ctx.Param("key")

	assignments, err := c.systemInjectableUC.ListAssignments(ctx.Request.Context(), key)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dto.ToListAssignmentsResponse(assignments))
}

// CreateAssignment creates a new assignment for a system injectable.
// @Summary Create injectable assignment
// @Tags System - Injectables
// @Accept json
// @Produce json
// @Param key path string true "Injectable key"
// @Param request body dto.CreateAssignmentRequest true "Assignment data"
// @Success 201 {object} dto.SystemInjectableAssignmentResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/system/injectables/{key}/assignments [post]
// @Security BearerAuth
func (c *AdminController) CreateAssignment(ctx *gin.Context) {
	key := ctx.Param("key")

	var req dto.CreateAssignmentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := injectableuc.CreateAssignmentCommand{
		InjectableKey: key,
		ScopeType:     entity.InjectableScopeType(req.ScopeType),
		TenantID:      req.TenantID,
		WorkspaceID:   req.WorkspaceID,
	}

	assignment, err := c.systemInjectableUC.CreateAssignment(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, dto.ToAssignmentResponse(assignment))
}

// DeleteAssignment deletes an assignment.
// @Summary Delete injectable assignment
// @Tags System - Injectables
// @Accept json
// @Produce json
// @Param key path string true "Injectable key"
// @Param assignmentId path string true "Assignment ID"
// @Success 204 "No Content"
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/system/injectables/{key}/assignments/{assignmentId} [delete]
// @Security BearerAuth
func (c *AdminController) DeleteAssignment(ctx *gin.Context) {
	key := ctx.Param("key")
	assignmentID := ctx.Param("assignmentId")

	if err := c.systemInjectableUC.DeleteAssignment(ctx.Request.Context(), key, assignmentID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// ExcludeAssignment excludes an assignment (sets is_active=false).
// @Summary Exclude injectable assignment
// @Tags System - Injectables
// @Accept json
// @Produce json
// @Param key path string true "Injectable key"
// @Param assignmentId path string true "Assignment ID"
// @Success 204 "No Content"
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/system/injectables/{key}/assignments/{assignmentId}/exclude [patch]
// @Security BearerAuth
func (c *AdminController) ExcludeAssignment(ctx *gin.Context) {
	key := ctx.Param("key")
	assignmentID := ctx.Param("assignmentId")

	if err := c.systemInjectableUC.ExcludeAssignment(ctx.Request.Context(), key, assignmentID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// IncludeAssignment includes an assignment (sets is_active=true).
// @Summary Include injectable assignment
// @Tags System - Injectables
// @Accept json
// @Produce json
// @Param key path string true "Injectable key"
// @Param assignmentId path string true "Assignment ID"
// @Success 204 "No Content"
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/system/injectables/{key}/assignments/{assignmentId}/include [patch]
// @Security BearerAuth
func (c *AdminController) IncludeAssignment(ctx *gin.Context) {
	key := ctx.Param("key")
	assignmentID := ctx.Param("assignmentId")

	if err := c.systemInjectableUC.IncludeAssignment(ctx.Request.Context(), key, assignmentID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// BulkCreatePublicAssignments creates PUBLIC assignments for multiple injectables.
// @Summary Bulk create PUBLIC assignments
// @Tags System - Injectables
// @Accept json
// @Produce json
// @Param request body dto.BulkPublicAssignmentsRequest true "Keys to make public"
// @Success 200 {object} dto.BulkPublicAssignmentsResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /api/v1/system/injectables/bulk/public [post]
// @Security BearerAuth
func (c *AdminController) BulkCreatePublicAssignments(ctx *gin.Context) {
	var req dto.BulkPublicAssignmentsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	result, err := c.systemInjectableUC.BulkCreatePublicAssignments(ctx.Request.Context(), req.Keys)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, toBulkResponse(result))
}

// BulkDeletePublicAssignments deletes PUBLIC assignments for multiple injectables.
// @Summary Bulk delete PUBLIC assignments
// @Tags System - Injectables
// @Accept json
// @Produce json
// @Param request body dto.BulkPublicAssignmentsRequest true "Keys to remove from public"
// @Success 200 {object} dto.BulkPublicAssignmentsResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /api/v1/system/injectables/bulk/public [delete]
// @Security BearerAuth
func (c *AdminController) BulkDeletePublicAssignments(ctx *gin.Context) {
	var req dto.BulkPublicAssignmentsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	result, err := c.systemInjectableUC.BulkDeletePublicAssignments(ctx.Request.Context(), req.Keys)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, toBulkResponse(result))
}

// toBulkResponse converts a BulkAssignmentResult to a BulkPublicAssignmentsResponse.
func toBulkResponse(result *injectableuc.BulkAssignmentResult) dto.BulkPublicAssignmentsResponse {
	failed := make([]dto.BulkOperationError, len(result.Failed))
	for i, f := range result.Failed {
		failed[i] = dto.BulkOperationError{
			Key:   f.Key,
			Error: f.Error.Error(),
		}
	}
	return dto.BulkPublicAssignmentsResponse{
		Succeeded: result.Succeeded,
		Failed:    failed,
	}
}

// --- Helper Functions ---
