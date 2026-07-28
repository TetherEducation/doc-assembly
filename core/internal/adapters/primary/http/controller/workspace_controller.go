package controller

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/mapper"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	cataloguc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/catalog"
	injectableuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/injectable"
	organizationuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/organization"
)

// WorkspaceController handles workspace-related HTTP requests.
type WorkspaceController struct {
	workspaceUC           organizationuc.WorkspaceUseCase
	folderUC              cataloguc.FolderUseCase
	tagUC                 cataloguc.TagUseCase
	memberUC              organizationuc.WorkspaceMemberUseCase
	workspaceInjectableUC injectableuc.WorkspaceInjectableUseCase
	injectableMapper      *mapper.InjectableMapper
}

// NewWorkspaceController creates a new workspace controller.
func NewWorkspaceController(
	workspaceUC organizationuc.WorkspaceUseCase,
	folderUC cataloguc.FolderUseCase,
	tagUC cataloguc.TagUseCase,
	memberUC organizationuc.WorkspaceMemberUseCase,
	workspaceInjectableUC injectableuc.WorkspaceInjectableUseCase,
	injectableMapper *mapper.InjectableMapper,
) *WorkspaceController {
	return &WorkspaceController{
		workspaceUC:           workspaceUC,
		folderUC:              folderUC,
		tagUC:                 tagUC,
		memberUC:              memberUC,
		workspaceInjectableUC: workspaceInjectableUC,
		injectableMapper:      injectableMapper,
	}
}

// RegisterRoutes registers all workspace routes.
// Note: Workspace listing and creation are now handled by TenantController under /tenant/workspaces.
// This controller only handles operations within a specific workspace (requiring X-Workspace-ID).
func (c *WorkspaceController) RegisterRoutes(rg *gin.RouterGroup, middlewareProvider *middleware.Provider) {
	// Workspace-scoped routes (requires X-Workspace-ID header)
	workspace := rg.Group("/workspace")
	workspace.Use(middlewareProvider.WorkspaceContext())
	{
		// Current workspace operations (NO sandbox - always operates on parent workspace)
		workspace.GET("", c.GetWorkspace)                                   // VIEWER+
		workspace.PUT("", middleware.RequireAdmin(), c.UpdateWorkspace)     // ADMIN+
		workspace.DELETE("", middleware.RequireOwner(), c.ArchiveWorkspace) // OWNER only

		// Member routes (NO sandbox - members are shared with parent workspace)
		workspace.GET("/members", c.ListMembers)                                           // VIEWER+
		workspace.POST("/members", middleware.RequireAdmin(), c.InviteMember)              // ADMIN+
		workspace.GET("/members/:memberId", c.GetMember)                                   // VIEWER+
		workspace.PUT("/members/:memberId", middleware.RequireOwner(), c.UpdateMemberRole) // OWNER only
		workspace.DELETE("/members/:memberId", middleware.RequireAdmin(), c.RemoveMember)  // ADMIN+

		// Folder routes (WITH sandbox support - each workspace has its own folders)
		folders := workspace.Group("/folders")
		folders.Use(middlewareProvider.SandboxContext())
		{
			folders.GET("", c.ListFolders)                                             // VIEWER+
			folders.GET("/tree", c.GetFolderTree)                                      // VIEWER+
			folders.POST("", middleware.RequireEditor(), c.CreateFolder)               // EDITOR+
			folders.GET("/:folderId", c.GetFolder)                                     // VIEWER+
			folders.PUT("/:folderId", middleware.RequireEditor(), c.UpdateFolder)      // EDITOR+
			folders.PATCH("/:folderId/move", middleware.RequireEditor(), c.MoveFolder) // EDITOR+
			folders.DELETE("/:folderId", middleware.RequireAdmin(), c.DeleteFolder)    // ADMIN+
		}

		// Tag routes (NO sandbox - tags are shared with parent workspace)
		workspace.GET("/tags", c.ListTags)                                       // VIEWER+
		workspace.POST("/tags", middleware.RequireEditor(), c.CreateTag)         // EDITOR+
		workspace.GET("/tags/:tagId", c.GetTag)                                  // VIEWER+
		workspace.PUT("/tags/:tagId", middleware.RequireEditor(), c.UpdateTag)   // EDITOR+
		workspace.DELETE("/tags/:tagId", middleware.RequireAdmin(), c.DeleteTag) // ADMIN+

		// Injectable routes (NO sandbox - injectables are shared with parent workspace)
		workspace.GET("/injectables", c.ListWorkspaceInjectables)                                                   // VIEWER+
		workspace.POST("/injectables", middleware.RequireEditor(), c.CreateWorkspaceInjectable)                     // EDITOR+
		workspace.GET("/injectables/:injectableId", c.GetWorkspaceInjectable)                                       // VIEWER+
		workspace.PUT("/injectables/:injectableId", middleware.RequireEditor(), c.UpdateWorkspaceInjectable)        // EDITOR+
		workspace.DELETE("/injectables/:injectableId", middleware.RequireAdmin(), c.DeleteWorkspaceInjectable)      // ADMIN+
		workspace.POST("/injectables/:injectableId/activate", middleware.RequireEditor(), c.ActivateInjectable)     // EDITOR+
		workspace.POST("/injectables/:injectableId/deactivate", middleware.RequireEditor(), c.DeactivateInjectable) // EDITOR+
	}
}

// --- Workspace Handlers ---

// GetWorkspace retrieves the current workspace.
// @Summary Get current workspace
// @Tags Workspaces
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Success 200 {object} dto.WorkspaceResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace [get]
func (c *WorkspaceController) GetWorkspace(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)
	workspaceRole, _ := middleware.GetWorkspaceRole(ctx)

	workspace, err := c.workspaceUC.GetWorkspace(ctx.Request.Context(), workspaceID)
	if err != nil {
		HandleError(ctx, err)
		return
	}
	workspace.CurrentRole = workspaceRole

	ctx.JSON(http.StatusOK, mapper.WorkspaceToResponse(workspace))
}

// UpdateWorkspace updates the current workspace.
// @Summary Update current workspace
// @Tags Workspaces
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param request body dto.UpdateWorkspaceRequest true "Workspace data"
// @Success 200 {object} dto.WorkspaceResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace [put]
func (c *WorkspaceController) UpdateWorkspace(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)
	workspaceRole, _ := middleware.GetWorkspaceRole(ctx)

	var req dto.UpdateWorkspaceRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	if err := req.Validate(); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := mapper.UpdateWorkspaceRequestToCommand(workspaceID, req)
	workspace, err := c.workspaceUC.UpdateWorkspace(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}
	workspace.CurrentRole = workspaceRole

	ctx.JSON(http.StatusOK, mapper.WorkspaceToResponse(workspace))
}

// ArchiveWorkspace archives the current workspace.
// @Summary Archive current workspace
// @Tags Workspaces
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Success 204 "No Content"
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace [delete]
func (c *WorkspaceController) ArchiveWorkspace(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	if err := c.workspaceUC.ArchiveWorkspace(ctx.Request.Context(), workspaceID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// --- Member Handlers ---

// ListMembers lists all members of the current workspace.
// @Summary List workspace members
// @Tags Members
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Success 200 {object} dto.ListResponse[dto.MemberResponse]
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/members [get]
func (c *WorkspaceController) ListMembers(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	members, err := c.memberUC.ListMembers(ctx.Request.Context(), workspaceID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	responses := mapper.MembersToResponses(members)
	ctx.JSON(http.StatusOK, dto.NewListResponse(responses))
}

// InviteMember invites a user to the current workspace.
// @Summary Invite member
// @Tags Members
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param request body dto.InviteMemberRequest true "Member invitation data"
// @Success 201 {object} dto.MemberResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Router /api/v1/workspace/members [post]
func (c *WorkspaceController) InviteMember(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)
	userID, _ := middleware.GetUserID(ctx)

	var req dto.InviteMemberRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	if err := req.Validate(); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := mapper.InviteMemberRequestToCommand(workspaceID, req, userID)
	member, err := c.memberUC.InviteMember(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, mapper.MemberToResponse(member))
}

// GetMember retrieves a member by ID.
// @Summary Get member
// @Tags Members
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param memberId path string true "Member ID"
// @Success 200 {object} dto.MemberResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/members/{memberId} [get]
func (c *WorkspaceController) GetMember(ctx *gin.Context) {
	memberID := ctx.Param("memberId")

	member, err := c.memberUC.GetMember(ctx.Request.Context(), memberID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.MemberToResponse(member))
}

// UpdateMemberRole updates a member's role.
// @Summary Update member role
// @Tags Members
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param memberId path string true "Member ID"
// @Param request body dto.UpdateMemberRoleRequest true "Role update data"
// @Success 200 {object} dto.MemberResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/members/{memberId} [put]
func (c *WorkspaceController) UpdateMemberRole(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)
	userID, _ := middleware.GetUserID(ctx)
	memberID := ctx.Param("memberId")

	var req dto.UpdateMemberRoleRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	if err := req.Validate(); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := mapper.UpdateMemberRoleRequestToCommand(memberID, workspaceID, req, userID)
	member, err := c.memberUC.UpdateMemberRole(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.MemberToResponse(member))
}

// RemoveMember removes a member from the workspace.
// @Summary Remove member
// @Tags Members
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param memberId path string true "Member ID"
// @Success 204 "No Content"
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/members/{memberId} [delete]
func (c *WorkspaceController) RemoveMember(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)
	userID, _ := middleware.GetInternalUserID(ctx)
	memberID := ctx.Param("memberId")

	cmd := mapper.RemoveMemberToCommand(memberID, workspaceID, userID)
	if err := c.memberUC.RemoveMember(ctx.Request.Context(), cmd); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// --- Folder Handlers ---

// ListFolders lists all folders in the current workspace.
// @Summary List folders
// @Tags Folders
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Success 200 {object} dto.ListResponse[dto.FolderResponse]
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/folders [get]
func (c *WorkspaceController) ListFolders(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	folders, err := c.folderUC.ListFoldersWithCounts(ctx.Request.Context(), workspaceID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	responses := mapper.FoldersWithCountsToResponses(folders)
	ctx.JSON(http.StatusOK, dto.NewListResponse(responses))
}

// GetFolderTree gets the folder tree for the current workspace.
// @Summary Get folder tree
// @Tags Folders
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Success 200 {array} dto.FolderTreeResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/folders/tree [get]
func (c *WorkspaceController) GetFolderTree(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	tree, err := c.folderUC.GetFolderTree(ctx.Request.Context(), workspaceID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	responses := mapper.FolderTreesToResponses(tree)
	ctx.JSON(http.StatusOK, responses)
}

// CreateFolder creates a new folder in the current workspace.
// @Summary Create folder
// @Tags Folders
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param request body dto.CreateFolderRequest true "Folder data"
// @Success 201 {object} dto.FolderResponse
// @Failure 400 {object} dto.ErrorResponse
// @Router /api/v1/workspace/folders [post]
func (c *WorkspaceController) CreateFolder(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)
	userID, _ := middleware.GetUserID(ctx)

	var req dto.CreateFolderRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	if err := req.Validate(); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := mapper.CreateFolderRequestToCommand(workspaceID, req, userID)
	folder, err := c.folderUC.CreateFolder(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, mapper.FolderToResponse(folder))
}

// GetFolder retrieves a folder by ID.
// @Summary Get folder
// @Tags Folders
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param folderId path string true "Folder ID"
// @Success 200 {object} dto.FolderResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/folders/{folderId} [get]
func (c *WorkspaceController) GetFolder(ctx *gin.Context) {
	folderID := ctx.Param("folderId")

	folder, err := c.folderUC.GetFolderWithCounts(ctx.Request.Context(), folderID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.FolderWithCountsToResponse(folder))
}

// UpdateFolder updates a folder.
// @Summary Update folder
// @Tags Folders
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param folderId path string true "Folder ID"
// @Param request body dto.UpdateFolderRequest true "Folder data"
// @Success 200 {object} dto.FolderResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/folders/{folderId} [put]
func (c *WorkspaceController) UpdateFolder(ctx *gin.Context) {
	folderID := ctx.Param("folderId")

	var req dto.UpdateFolderRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	if err := req.Validate(); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := mapper.UpdateFolderRequestToCommand(folderID, req)
	folder, err := c.folderUC.UpdateFolder(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.FolderToResponse(folder))
}

// MoveFolder moves a folder to a new parent.
// @Summary Move folder
// @Tags Folders
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param folderId path string true "Folder ID"
// @Param request body dto.MoveFolderRequest true "Move data"
// @Success 200 {object} dto.FolderResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/folders/{folderId}/move [patch]
func (c *WorkspaceController) MoveFolder(ctx *gin.Context) {
	folderID := ctx.Param("folderId")

	var req dto.MoveFolderRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := mapper.MoveFolderRequestToCommand(folderID, req)
	folder, err := c.folderUC.MoveFolder(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.FolderToResponse(folder))
}

// DeleteFolder deletes a folder.
// @Summary Delete folder
// @Tags Folders
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param X-Sandbox-Mode header string false "Enable sandbox mode (operates on sandbox workspace)"
// @Param folderId path string true "Folder ID"
// @Success 204 "No Content"
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/folders/{folderId} [delete]
func (c *WorkspaceController) DeleteFolder(ctx *gin.Context) {
	folderID := ctx.Param("folderId")

	if err := c.folderUC.DeleteFolder(ctx.Request.Context(), folderID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// --- Tag Handlers ---

// ListTags lists all tags in the current workspace.
// @Summary List tags
// @Tags Tags
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Success 200 {object} dto.ListResponse[dto.TagWithCountResponse]
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/tags [get]
func (c *WorkspaceController) ListTags(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	tags, err := c.tagUC.ListTagsWithCount(ctx.Request.Context(), workspaceID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	responses := mapper.TagsWithCountToResponses(tags)
	ctx.JSON(http.StatusOK, dto.NewListResponse(responses))
}

// CreateTag creates a new tag in the current workspace.
// @Summary Create tag
// @Tags Tags
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param request body dto.CreateTagRequest true "Tag data"
// @Success 201 {object} dto.TagResponse
// @Failure 400 {object} dto.ErrorResponse
// @Router /api/v1/workspace/tags [post]
func (c *WorkspaceController) CreateTag(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)
	userID, _ := middleware.GetUserID(ctx)

	var req dto.CreateTagRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	if err := req.Validate(); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := mapper.CreateTagRequestToCommand(workspaceID, req, userID)
	tag, err := c.tagUC.CreateTag(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, mapper.TagToResponse(tag))
}

// GetTag retrieves a tag by ID.
// @Summary Get tag
// @Tags Tags
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param tagId path string true "Tag ID"
// @Success 200 {object} dto.TagResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/tags/{tagId} [get]
func (c *WorkspaceController) GetTag(ctx *gin.Context) {
	tagID := ctx.Param("tagId")

	tag, err := c.tagUC.GetTag(ctx.Request.Context(), tagID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.TagToResponse(tag))
}

// UpdateTag updates a tag.
// @Summary Update tag
// @Tags Tags
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param tagId path string true "Tag ID"
// @Param request body dto.UpdateTagRequest true "Tag data"
// @Success 200 {object} dto.TagResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/tags/{tagId} [put]
func (c *WorkspaceController) UpdateTag(ctx *gin.Context) {
	tagID := ctx.Param("tagId")

	var req dto.UpdateTagRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	if err := req.Validate(); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := mapper.UpdateTagRequestToCommand(tagID, req)
	tag, err := c.tagUC.UpdateTag(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, mapper.TagToResponse(tag))
}

// DeleteTag deletes a tag.
// @Summary Delete tag
// @Tags Tags
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param tagId path string true "Tag ID"
// @Success 204 "No Content"
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/tags/{tagId} [delete]
func (c *WorkspaceController) DeleteTag(ctx *gin.Context) {
	tagID := ctx.Param("tagId")

	if err := c.tagUC.DeleteTag(ctx.Request.Context(), tagID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// --- Injectable Handlers ---

// ListWorkspaceInjectables lists all injectables owned by the current workspace.
// @Summary List workspace injectables
// @Tags Injectables
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Success 200 {object} dto.ListWorkspaceInjectablesResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/injectables [get]
func (c *WorkspaceController) ListWorkspaceInjectables(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	injectables, err := c.workspaceInjectableUC.ListInjectables(ctx.Request.Context(), workspaceID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.injectableMapper.ToWorkspaceListResponse(injectables))
}

// CreateWorkspaceInjectable creates a new injectable in the current workspace.
// @Summary Create workspace injectable
// @Tags Injectables
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param request body dto.CreateWorkspaceInjectableRequest true "Injectable data"
// @Success 201 {object} dto.WorkspaceInjectableResponse
// @Failure 400 {object} dto.ErrorResponse
// @Router /api/v1/workspace/injectables [post]
func (c *WorkspaceController) CreateWorkspaceInjectable(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	var req dto.CreateWorkspaceInjectableRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := injectableuc.CreateWorkspaceInjectableCommand{
		WorkspaceID:  workspaceID,
		Key:          req.Key,
		Label:        req.Label,
		Description:  req.Description,
		DefaultValue: req.DefaultValue,
		Metadata:     req.Metadata,
	}

	injectable, err := c.workspaceInjectableUC.CreateInjectable(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, c.injectableMapper.ToWorkspaceResponse(injectable))
}

// GetWorkspaceInjectable retrieves an injectable by ID.
// @Summary Get workspace injectable
// @Tags Injectables
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param injectableId path string true "Injectable ID"
// @Success 200 {object} dto.WorkspaceInjectableResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/injectables/{injectableId} [get]
func (c *WorkspaceController) GetWorkspaceInjectable(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)
	injectableID := ctx.Param("injectableId")

	injectable, err := c.workspaceInjectableUC.GetInjectable(ctx.Request.Context(), injectableID, workspaceID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.injectableMapper.ToWorkspaceResponse(injectable))
}

// UpdateWorkspaceInjectable updates an injectable.
// @Summary Update workspace injectable
// @Tags Injectables
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param injectableId path string true "Injectable ID"
// @Param request body dto.UpdateWorkspaceInjectableRequest true "Injectable data"
// @Success 200 {object} dto.WorkspaceInjectableResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/injectables/{injectableId} [put]
func (c *WorkspaceController) UpdateWorkspaceInjectable(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)
	injectableID := ctx.Param("injectableId")

	var req dto.UpdateWorkspaceInjectableRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmd := injectableuc.UpdateWorkspaceInjectableCommand{
		ID:           injectableID,
		WorkspaceID:  workspaceID,
		Key:          req.Key,
		Label:        req.Label,
		Description:  req.Description,
		DefaultValue: req.DefaultValue,
		Metadata:     req.Metadata,
	}

	injectable, err := c.workspaceInjectableUC.UpdateInjectable(ctx.Request.Context(), cmd)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.injectableMapper.ToWorkspaceResponse(injectable))
}

// DeleteWorkspaceInjectable soft-deletes an injectable.
// @Summary Delete workspace injectable
// @Tags Injectables
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param injectableId path string true "Injectable ID"
// @Success 204 "No Content"
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/injectables/{injectableId} [delete]
func (c *WorkspaceController) DeleteWorkspaceInjectable(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)
	injectableID := ctx.Param("injectableId")

	if err := c.workspaceInjectableUC.DeleteInjectable(ctx.Request.Context(), injectableID, workspaceID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// ActivateInjectable activates an injectable.
// @Summary Activate injectable
// @Tags Injectables
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param injectableId path string true "Injectable ID"
// @Success 200 {object} dto.WorkspaceInjectableResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/injectables/{injectableId}/activate [post]
func (c *WorkspaceController) ActivateInjectable(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)
	injectableID := ctx.Param("injectableId")

	injectable, err := c.workspaceInjectableUC.ActivateInjectable(ctx.Request.Context(), injectableID, workspaceID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.injectableMapper.ToWorkspaceResponse(injectable))
}

// DeactivateInjectable deactivates an injectable.
// @Summary Deactivate injectable
// @Tags Injectables
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param injectableId path string true "Injectable ID"
// @Success 200 {object} dto.WorkspaceInjectableResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Router /api/v1/workspace/injectables/{injectableId}/deactivate [post]
func (c *WorkspaceController) DeactivateInjectable(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)
	injectableID := ctx.Param("injectableId")

	injectable, err := c.workspaceInjectableUC.DeactivateInjectable(ctx.Request.Context(), injectableID, workspaceID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, c.injectableMapper.ToWorkspaceResponse(injectable))
}
