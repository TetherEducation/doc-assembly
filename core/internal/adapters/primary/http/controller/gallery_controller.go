package controller

import (
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	galleryuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/gallery"
)

const galleryMaxUploadBytes = 11 * 1024 * 1024 // 11 MB multipart limit (10 MB file + overhead)

// GalleryController handles image gallery HTTP requests.
type GalleryController struct {
	galleryUC galleryuc.GalleryUseCase
}

// NewGalleryController creates a new gallery controller.
func NewGalleryController(galleryUC galleryuc.GalleryUseCase) *GalleryController {
	return &GalleryController{galleryUC: galleryUC}
}

// RegisterRoutes registers all gallery routes under /workspace/gallery.
func (c *GalleryController) RegisterRoutes(rg *gin.RouterGroup, middlewareProvider *middleware.Provider) {
	gallery := rg.Group("/workspace/gallery")
	gallery.Use(middlewareProvider.WorkspaceContext())
	{
		gallery.GET("", c.ListAssets)                                // VIEWER+
		gallery.GET("/search", c.SearchAssets)                       // VIEWER+
		gallery.POST("", middleware.RequireEditor(), c.UploadAsset)  // EDITOR+
		gallery.DELETE("", middleware.RequireAdmin(), c.DeleteAsset) // ADMIN+
		gallery.GET("/url", c.GetAssetURL)                           // VIEWER+
		gallery.GET("/serve", c.ServeAsset)                          // VIEWER+ (local storage fallback)
	}
}

// ListAssets returns a paginated list of gallery assets.
// @Summary List gallery assets
// @Tags Gallery
// @Produce json
// @Security BearerAuth
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param page query int false "Page number (default 1)"
// @Param perPage query int false "Items per page (default 20)"
// @Success 200 {object} dto.GalleryListResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/workspace/gallery [get]
func (c *GalleryController) ListAssets(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)
	page, perPage := parsePaginationParams(ctx)

	result, err := c.galleryUC.ListAssets(ctx.Request.Context(), galleryuc.ListAssetsCmd{
		WorkspaceID: workspaceID,
		Page:        page,
		PerPage:     perPage,
	})
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, galleryPageToResponse(result))
}

// SearchAssets returns gallery assets matching a query string.
// @Summary Search gallery assets
// @Tags Gallery
// @Produce json
// @Security BearerAuth
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param q query string true "Search query"
// @Param page query int false "Page number (default 1)"
// @Param perPage query int false "Items per page (default 20)"
// @Success 200 {object} dto.GalleryListResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/workspace/gallery/search [get]
func (c *GalleryController) SearchAssets(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)
	page, perPage := parsePaginationParams(ctx)

	result, err := c.galleryUC.SearchAssets(ctx.Request.Context(), galleryuc.SearchAssetsCmd{
		WorkspaceID: workspaceID,
		Query:       ctx.Query("q"),
		Page:        page,
		PerPage:     perPage,
	})
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, galleryPageToResponse(result))
}

// UploadAsset accepts a multipart file upload and stores it in the gallery.
// @Summary Upload gallery asset
// @Tags Gallery
// @Accept mpfd
// @Produce json
// @Security BearerAuth
// @Param X-Tenant-ID header string true "Tenant ID"
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param file formData file true "Image file"
// @Success 201 {object} dto.GalleryAssetResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/workspace/gallery [post]
func (c *GalleryController) UploadAsset(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)
	tenantID, _ := middleware.GetTenantIDFromHeader(ctx)
	userID, _ := middleware.GetInternalUserID(ctx)

	if err := ctx.Request.ParseMultipartForm(galleryMaxUploadBytes); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	file, header, err := ctx.Request.FormFile("file")
	if err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		respondError(ctx, http.StatusInternalServerError, err)
		return
	}

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	asset, err := c.galleryUC.UploadAsset(ctx.Request.Context(), galleryuc.UploadAssetCmd{
		TenantID:    tenantID,
		WorkspaceID: workspaceID,
		UserID:      userID,
		Filename:    header.Filename,
		ContentType: contentType,
		Data:        data,
	})
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, galleryAssetToResponse(asset))
}

// DeleteAsset removes an asset from the gallery.
// @Summary Delete gallery asset
// @Tags Gallery
// @Produce json
// @Security BearerAuth
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param key query string true "Asset key"
// @Success 204 "No Content"
// @Failure 400 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/workspace/gallery [delete]
func (c *GalleryController) DeleteAsset(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	if err := c.galleryUC.DeleteAsset(ctx.Request.Context(), galleryuc.DeleteAssetCmd{
		WorkspaceID: workspaceID,
		Key:         ctx.Query("key"),
	}); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Status(http.StatusNoContent)
}

// GetAssetURL resolves a gallery key to an HTTP URL.
// @Summary Get gallery asset URL
// @Tags Gallery
// @Produce json
// @Security BearerAuth
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param key query string true "Asset key"
// @Success 200 {object} dto.GalleryURLResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/workspace/gallery/url [get]
func (c *GalleryController) GetAssetURL(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	resolvedURL, err := c.galleryUC.GetAssetURL(ctx.Request.Context(), galleryuc.GetAssetURLCmd{
		WorkspaceID: workspaceID,
		Key:         ctx.Query("key"),
	})
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dto.GalleryURLResponse{URL: resolvedURL})
}

// ServeAsset streams a gallery asset directly from storage.
// Used as fallback when local storage (file://) is configured instead of S3.
// @Summary Serve gallery asset bytes
// @Tags Gallery
// @Produce application/octet-stream
// @Security BearerAuth
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param key query string true "Asset key"
// @Success 200 {file} binary
// @Failure 400 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/workspace/gallery/serve [get]
func (c *GalleryController) ServeAsset(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	payload, err := c.galleryUC.ServeAsset(ctx.Request.Context(), galleryuc.ServeAssetCmd{
		WorkspaceID: workspaceID,
		Key:         ctx.Query("key"),
	})
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Data(http.StatusOK, payload.ContentType, payload.Data)
}

func galleryAssetToResponse(a *entity.GalleryAsset) *dto.GalleryAssetResponse {
	return &dto.GalleryAssetResponse{
		ID:          a.ID,
		Key:         a.Key,
		Filename:    a.Filename,
		ContentType: a.ContentType,
		Size:        a.Size,
		CreatedAt:   a.CreatedAt,
	}
}

func galleryPageToResponse(page *galleryuc.AssetsPage) *dto.GalleryListResponse {
	items := make([]*dto.GalleryAssetResponse, 0, len(page.Assets))
	for _, a := range page.Assets {
		items = append(items, galleryAssetToResponse(a))
	}
	return &dto.GalleryListResponse{
		Items:   items,
		Total:   page.Total,
		Page:    page.Page,
		PerPage: page.PerPage,
	}
}

func parsePaginationParams(ctx *gin.Context) (int, int) {
	page, _ := strconv.Atoi(ctx.Query("page"))
	perPage, _ := strconv.Atoi(ctx.Query("perPage"))
	return page, perPage
}
