package controller

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	templatesvc "github.com/TetherEducation/doc-assembly/core/internal/core/service/template"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

// InternalCommentController exposes the CRM-only comment thread on templates to
// service-to-service callers (crm-admission).
//
// Separate from InternalTemplateController for the same reason approvals are: that
// controller is read-only by construction, and these endpoints write.
type InternalCommentController struct {
	commentUC templateuc.TemplateCommentUseCase
}

// NewInternalCommentController creates the controller.
func NewInternalCommentController(
	commentUC templateuc.TemplateCommentUseCase,
) *InternalCommentController {
	return &InternalCommentController{commentUC: commentUC}
}

// RegisterRoutes registers the internal comment routes.
//
// Comments are written against a version and read per template, so the routes sit
// under both prefixes. Both groups share their wildcard names with the existing
// internal template and approval routes, which gin requires.
func (c *InternalCommentController) RegisterRoutes(
	api *gin.RouterGroup,
	authMiddleware gin.HandlerFunc,
) {
	versions := api.Group("/internal/template-versions")
	versions.Use(authMiddleware)
	versions.POST("/:versionId/comments", c.AddComment)

	templates := api.Group("/internal/templates")
	templates.Use(authMiddleware)
	templates.GET("/:templateId/comments", c.ListComments)
}

// AddComment adds a comment to the template a version belongs to.
// @Summary Add a CRM-only comment to a template
// @Description Appends a comment to the version's template, tagged with that version. The template is derived from the version. Comments are append-only and never part of the template content.
// @Tags Internal
// @Accept json
// @Produce json
// @Param X-API-Key header string true "API Key for authentication"
// @Param X-Workspace-Code header string true "Workspace business code"
// @Param versionId path string true "Template version ID"
// @Param request body dto.InternalAddCommentRequest true "Comment"
// @Success 201 {object} dto.InternalCommentResponse
// @Failure 400 {object} dto.InternalErrorResponse
// @Failure 401 {object} dto.InternalErrorResponse
// @Failure 403 {object} dto.InternalErrorResponse
// @Failure 404 {object} dto.InternalErrorResponse
// @Router /api/v1/internal/template-versions/{versionId}/comments [post]
func (c *InternalCommentController) AddComment(ctx *gin.Context) {
	var req dto.InternalAddCommentRequest
	if !readInternalJSON(ctx, &req) {
		return
	}

	workspace, ok := requireWorkspace(ctx)
	if !ok {
		return
	}

	versionID := ctx.Param("versionId")
	if _, err := uuid.Parse(versionID); err != nil {
		HandleError(ctx, entity.ErrVersionNotFound)
		return
	}

	comment, err := c.commentUC.AddComment(ctx.Request.Context(), templateuc.AddCommentCommand{
		TemplateVersionID: versionID,
		WorkspaceCode:     workspace,
		Audience:          req.Audience,
		Body:              req.Body,
		Email:             req.Email,
		Name:              req.Name,
		Campus:            req.Campus,
	})
	if err != nil {
		writeCommentError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, commentToResponse(comment))
}

// ListComments returns one audience's comments on a template, oldest first.
// @Summary List CRM-only comments on a template
// @Description Lists the comments of one audience on a template, oldest first, each tagged with the version it was written against.
// @Tags Internal
// @Produce json
// @Param X-API-Key header string true "API Key for authentication"
// @Param X-Workspace-Code header string true "Workspace business code"
// @Param templateId path string true "Template ID"
// @Param audience query string true "Audience the thread belongs to"
// @Success 200 {object} dto.InternalCommentListResponse
// @Failure 400 {object} dto.InternalErrorResponse
// @Failure 401 {object} dto.InternalErrorResponse
// @Failure 403 {object} dto.InternalErrorResponse
// @Failure 404 {object} dto.InternalErrorResponse
// @Router /api/v1/internal/templates/{templateId}/comments [get]
func (c *InternalCommentController) ListComments(ctx *gin.Context) {
	audience := strings.TrimSpace(ctx.Query("audience"))
	if audience == "" {
		ctx.JSON(http.StatusBadRequest, dto.InternalErrorResponse{
			Error: "audience query parameter is required",
			Code:  "INVALID_QUERY",
		})
		return
	}

	workspace, ok := requireWorkspace(ctx)
	if !ok {
		return
	}

	templateID := ctx.Param("templateId")
	if _, err := uuid.Parse(templateID); err != nil {
		HandleError(ctx, entity.ErrTemplateNotFound)
		return
	}

	comments, err := c.commentUC.ListComments(ctx.Request.Context(), templateuc.ListCommentsQuery{
		TemplateID:    templateID,
		WorkspaceCode: workspace,
		Audience:      audience,
	})
	if err != nil {
		writeCommentError(ctx, err)
		return
	}

	// An empty thread is an empty list, never null.
	items := make([]dto.InternalCommentResponse, 0, len(comments))
	for _, comment := range comments {
		items = append(items, commentToResponse(comment))
	}
	ctx.JSON(http.StatusOK, dto.InternalCommentListResponse{Items: items})
}

// writeCommentError maps comment errors onto status codes. Workspace scope errors
// answer exactly as the approval endpoints do.
func writeCommentError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, entity.ErrTemplateCommentInvalid):
		ctx.JSON(http.StatusBadRequest, dto.InternalErrorResponse{
			Error: err.Error(), Code: "INVALID_BODY",
		})
	case errors.Is(err, templatesvc.ErrWorkspaceMismatch),
		errors.Is(err, templatesvc.ErrWorkspaceScopeRequired):
		ctx.JSON(http.StatusForbidden, dto.InternalErrorResponse{
			Error: err.Error(), Code: "WORKSPACE_SCOPE",
		})
	default:
		HandleError(ctx, err)
	}
}

func commentToResponse(c *entity.TemplateComment) dto.InternalCommentResponse {
	return dto.InternalCommentResponse{
		ID:                c.ID,
		TemplateID:        c.TemplateID,
		TemplateVersionID: c.TemplateVersionID,
		VersionNumber:     c.VersionNumber,
		Audience:          c.Audience,
		Body:              c.Body,
		AuthorEmail:       c.AuthorEmail,
		AuthorName:        c.AuthorName,
		AuthorCampus:      c.AuthorCampus,
		CreatedAt:         c.CreatedAt,
	}
}
