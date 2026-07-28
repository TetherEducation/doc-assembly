package controller

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	automationuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/automation"
)

// AutomationKeyController handles SUPERADMIN routes for API key management.
type AutomationKeyController struct {
	apiKeyUseCase automationuc.APIKeyUseCase
}

// NewAutomationKeyController creates a new AutomationKeyController.
func NewAutomationKeyController(apiKeyUseCase automationuc.APIKeyUseCase) *AutomationKeyController {
	return &AutomationKeyController{apiKeyUseCase: apiKeyUseCase}
}

// RegisterRoutes registers SUPERADMIN routes for API key management.
// The group passed in should already have JWT auth applied.
// This method adds RequireSuperAdmin guard internally.
func (ctrl *AutomationKeyController) RegisterRoutes(adminGroup *gin.RouterGroup) {
	g := adminGroup.Group("/automation-keys", middleware.RequireSuperAdmin())
	g.POST("/", ctrl.createKey)
	g.GET("/", ctrl.listKeys)
	g.GET("/:id", ctrl.getKey)
	g.PATCH("/:id", ctrl.updateKey)
	g.DELETE("/:id", ctrl.revokeKey)
	g.GET("/:id/audit", ctrl.getAuditLog)
}

// createKey creates a new API key.
// @Summary Create automation API key
// @Description Creates a new API key. The raw key is returned ONLY in this response.
// @Tags Automation Keys
// @Accept json
// @Produce json
// @Param request body dto.CreateAutomationKeyRequest true "API key data"
// @Success 201 {object} dto.CreateAutomationKeyResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /admin/automation-keys/ [post]
// @Security BearerAuth
func (ctrl *AutomationKeyController) createKey(ctx *gin.Context) {
	var req dto.CreateAutomationKeyRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	createdBy, _ := middleware.GetInternalUserID(ctx)

	result, err := ctrl.apiKeyUseCase.CreateKey(ctx.Request.Context(), req.Name, req.AllowedTenants, createdBy, req.KeyType)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, dto.CreateAutomationKeyResponse{
		ID:             result.Key.ID,
		Name:           result.Key.Name,
		KeyPrefix:      result.Key.KeyPrefix,
		KeyType:        result.Key.KeyType,
		AllowedTenants: result.Key.AllowedTenants,
		IsActive:       result.Key.IsActive,
		CreatedBy:      result.Key.CreatedBy,
		CreatedAt:      result.Key.CreatedAt,
		RawKey:         result.RawKey,
	})
}

// listKeys lists all API keys.
// @Summary List automation API keys
// @Tags Automation Keys
// @Produce json
// @Success 200 {object} dto.ListResponse[dto.AutomationKeyResponse]
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Router /admin/automation-keys/ [get]
// @Security BearerAuth
func (ctrl *AutomationKeyController) listKeys(ctx *gin.Context) {
	keys, err := ctrl.apiKeyUseCase.ListKeys(ctx.Request.Context())
	if err != nil {
		HandleError(ctx, err)
		return
	}

	responses := make([]dto.AutomationKeyResponse, 0, len(keys))
	for _, k := range keys {
		responses = append(responses, mapKeyToResponse(k))
	}

	ctx.JSON(http.StatusOK, dto.NewListResponse(responses))
}

// getKey retrieves a single API key by ID.
// @Summary Get automation API key
// @Tags Automation Keys
// @Produce json
// @Param id path string true "API Key ID"
// @Success 200 {object} dto.AutomationKeyResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /admin/automation-keys/{id} [get]
// @Security BearerAuth
func (ctrl *AutomationKeyController) getKey(ctx *gin.Context) {
	id := ctx.Param("id")

	key, err := ctrl.apiKeyUseCase.GetKey(ctx.Request.Context(), id)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapKeyToResponse(key))
}

// updateKey updates the name and/or allowed tenants of a key.
// @Summary Update automation API key
// @Tags Automation Keys
// @Accept json
// @Produce json
// @Param id path string true "API Key ID"
// @Param request body dto.UpdateAutomationKeyRequest true "Key data"
// @Success 200 {object} dto.AutomationKeyResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /admin/automation-keys/{id} [patch]
// @Security BearerAuth
func (ctrl *AutomationKeyController) updateKey(ctx *gin.Context) {
	id := ctx.Param("id")

	var req dto.UpdateAutomationKeyRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	// Determine the name to use: if nil, load current key name.
	name := ""
	if req.Name != nil {
		name = *req.Name
	} else {
		current, err := ctrl.apiKeyUseCase.GetKey(ctx.Request.Context(), id)
		if err != nil {
			HandleError(ctx, err)
			return
		}
		name = current.Name
	}

	key, err := ctrl.apiKeyUseCase.UpdateKey(ctx.Request.Context(), id, name, req.AllowedTenants)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapKeyToResponse(key))
}

// revokeKey revokes an API key.
// @Summary Revoke automation API key
// @Tags Automation Keys
// @Param id path string true "API Key ID"
// @Success 204 "No Content"
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /admin/automation-keys/{id} [delete]
// @Security BearerAuth
func (ctrl *AutomationKeyController) revokeKey(ctx *gin.Context) {
	id := ctx.Param("id")

	if err := ctrl.apiKeyUseCase.RevokeKey(ctx.Request.Context(), id); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// getAuditLog returns paginated audit log entries for a specific API key.
// @Summary Get audit log for API key
// @Tags Automation Keys
// @Produce json
// @Param id path string true "API Key ID"
// @Param limit query int false "Limit results" default(20)
// @Param offset query int false "Offset results" default(0)
// @Success 200 {object} dto.ListResponse[dto.AutomationAuditLogResponse]
// @Failure 401 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /admin/automation-keys/{id}/audit [get]
// @Security BearerAuth
func (ctrl *AutomationKeyController) getAuditLog(ctx *gin.Context) {
	id := ctx.Param("id")

	limit := 20
	offset := 0

	if v := ctx.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			limit = parsed
		}
	}
	if v := ctx.Query("offset"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			offset = parsed
		}
	}

	logs, err := ctrl.apiKeyUseCase.ListAuditLog(ctx.Request.Context(), id, limit, offset)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	responses := make([]dto.AutomationAuditLogResponse, 0, len(logs))
	for _, l := range logs {
		responses = append(responses, mapAuditLogToResponse(l))
	}

	ctx.JSON(http.StatusOK, dto.NewListResponse(responses))
}

// mapKeyToResponse converts an AutomationAPIKey entity to its DTO representation.
func mapKeyToResponse(k *entity.AutomationAPIKey) dto.AutomationKeyResponse {
	return dto.AutomationKeyResponse{
		ID:             k.ID,
		Name:           k.Name,
		KeyPrefix:      k.KeyPrefix,
		KeyType:        k.KeyType,
		AllowedTenants: k.AllowedTenants,
		IsActive:       k.IsActive,
		CreatedBy:      k.CreatedBy,
		LastUsedAt:     k.LastUsedAt,
		CreatedAt:      k.CreatedAt,
		RevokedAt:      k.RevokedAt,
	}
}

// mapAuditLogToResponse converts an AutomationAuditLog entity to its DTO representation.
func mapAuditLogToResponse(l *entity.AutomationAuditLog) dto.AutomationAuditLogResponse {
	return dto.AutomationAuditLogResponse{
		ID:             l.ID,
		APIKeyID:       l.APIKeyID,
		APIKeyPrefix:   l.APIKeyPrefix,
		Method:         l.Method,
		Path:           l.Path,
		TenantID:       l.TenantID,
		WorkspaceID:    l.WorkspaceID,
		ResourceType:   l.ResourceType,
		ResourceID:     l.ResourceID,
		Action:         l.Action,
		RequestBody:    l.RequestBody,
		ResponseStatus: l.ResponseStatus,
		CreatedAt:      l.CreatedAt,
	}
}
