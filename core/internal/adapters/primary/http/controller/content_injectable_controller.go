package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	_ "github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto" // for swagger
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/mapper"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	injectableuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/injectable"
)

// ContentInjectableController handles injectable-related HTTP requests.
type ContentInjectableController struct {
	injectableUC     injectableuc.InjectableUseCase
	injectableMapper *mapper.InjectableMapper
}

// NewContentInjectableController creates a new injectable controller.
func NewContentInjectableController(
	injectableUC injectableuc.InjectableUseCase,
	injectableMapper *mapper.InjectableMapper,
) *ContentInjectableController {
	return &ContentInjectableController{
		injectableUC:     injectableUC,
		injectableMapper: injectableMapper,
	}
}

// RegisterRoutes registers all injectable routes.
// All injectable routes require X-Workspace-ID header.
// Note: Injectables are read-only - they are managed via database migrations/seeds.
func (c *ContentInjectableController) RegisterRoutes(rg *gin.RouterGroup, middlewareProvider *middleware.Provider) {
	// Content group requires X-Workspace-ID header
	content := rg.Group("/content")
	content.Use(middlewareProvider.WorkspaceContext())
	{
		// Injectable routes (read-only)
		injectables := content.Group("/injectables")
		{
			injectables.GET("", c.ListInjectables)             // VIEWER+
			injectables.GET("/:injectableId", c.GetInjectable) // VIEWER+
		}
	}
}

// ListInjectables lists all injectable definitions for a workspace.
// @Summary List injectables
// @Tags Injectables
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Success 200 {object} dto.ListInjectablesResponse
// @Failure 401 {object} dto.ErrorResponse
// @Router /api/v1/content/injectables [get]
func (c *ContentInjectableController) ListInjectables(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	result, err := c.injectableUC.ListInjectables(ctx.Request.Context(), &injectableuc.ListInjectablesRequest{
		WorkspaceID: workspaceID,
	})
	if err != nil {
		respondError(ctx, http.StatusInternalServerError, err)
		return
	}

	ctx.JSON(http.StatusOK, c.injectableMapper.ToListResponse(result.Injectables, result.Groups))
}

// GetInjectable retrieves an injectable by ID.
// @Summary Get injectable
// @Tags Injectables
// @Accept json
// @Produce json
// @Param injectableId path string true "Injectable ID"
// @Success 200 {object} dto.InjectableResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/content/injectables/{injectableId} [get]
func (c *ContentInjectableController) GetInjectable(ctx *gin.Context) {
	injectableID := ctx.Param("injectableId")

	injectable, err := c.injectableUC.GetInjectable(ctx.Request.Context(), injectableID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.injectableMapper.ToResponse(injectable))
}
