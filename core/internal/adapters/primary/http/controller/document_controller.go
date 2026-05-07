package controller

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	documentsvc "github.com/rendis/doc-assembly/core/internal/core/service/document"
	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
)

// DocumentController handles document HTTP requests.
type DocumentController struct {
	documentUC     documentuc.DocumentUseCase
	preSigningUC   documentuc.PreSigningUseCase
	readOnlyViewUC documentuc.ReadOnlyViewUseCase
	eventEmitter   *documentsvc.EventEmitter
}

// NewDocumentController creates a new document controller.
func NewDocumentController(
	documentUC documentuc.DocumentUseCase,
	preSigningUC documentuc.PreSigningUseCase,
	readOnlyViewUC documentuc.ReadOnlyViewUseCase,
	eventEmitter *documentsvc.EventEmitter,
) *DocumentController {
	return &DocumentController{
		documentUC:     documentUC,
		preSigningUC:   preSigningUC,
		readOnlyViewUC: readOnlyViewUC,
		eventEmitter:   eventEmitter,
	}
}

// RegisterRoutes registers all document routes.
func (c *DocumentController) RegisterRoutes(api *gin.RouterGroup) {
	docs := api.Group("/documents")
	{
		// List documents in workspace
		docs.GET("", middleware.RequireViewer(), c.ListDocuments)

		// Get document statistics
		docs.GET("/statistics", middleware.RequireViewer(), c.GetStatistics)

		// Create and send document
		docs.POST("", middleware.RequireOperator(), c.CreateDocument)

		// Batch create documents
		docs.POST("/batch", middleware.RequireOperator(), c.CreateDocumentsBatch)

		// Get single document
		docs.GET("/:documentId", middleware.RequireViewer(), c.GetDocument)

		// Get document recipients
		docs.GET("/:documentId/recipients", middleware.RequireViewer(), c.GetRecipients)

		// Get document events (audit trail)
		docs.GET("/:documentId/events", middleware.RequireViewer(), c.GetDocumentEvents)

		// Get signing URL for recipient
		docs.GET("/:documentId/recipients/:recipientId/signing-url", middleware.RequireViewer(), c.GetSigningURL)

		// Download signed PDF
		docs.GET("/:documentId/pdf", middleware.RequireViewer(), c.GetDocumentPDF)

		// Create public read-only view link
		docs.POST("/:documentId/view-link", middleware.RequireViewer(), c.CreateReadOnlyViewLink)

		// Refresh document status from provider
		docs.POST("/:documentId/refresh", middleware.RequireOperator(), c.RefreshStatus)

		// Cancel document
		docs.POST("/:documentId/cancel", middleware.RequireOperator(), c.CancelDocument)

		// Deprecate completed document
		docs.POST("/:documentId/deprecate", middleware.RequireOperator(), c.DeprecateDocument)

		// Send reminder to pending recipients
		docs.POST("/:documentId/remind", middleware.RequireOperator(), c.SendReminder)

		// Regenerate pre-signing access token
		docs.POST("/:documentId/invalidate-tokens", middleware.RequireOperator(), c.InvalidateTokens)
	}
}

// ListDocuments returns all documents in the workspace.
// @Summary List documents
// @Tags Documents
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param status query string false "Filter by status"
// @Param search query string false "Search by title"
// @Param limit query int false "Limit results"
// @Param offset query int false "Offset for pagination"
// @Success 200 {array} dto.DocumentListResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents [get]
func (c *DocumentController) ListDocuments(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	var filters port.DocumentFilters
	if status := ctx.Query("status"); status != "" {
		docStatus := entity.DocumentStatus(status)
		filters.Status = &docStatus
	}
	filters.Search = ctx.Query("search")

	// Parse limit/offset with defaults
	filters.Limit = 50
	filters.Offset = 0
	if l := ctx.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			filters.Limit = parsed
		}
	}
	if o := ctx.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			filters.Offset = parsed
		}
	}

	docs, err := c.documentUC.ListDocuments(ctx.Request.Context(), workspaceID, filters)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, docs)
}

// CreateReadOnlyViewLink creates a public read-only view link for a document.
// @Summary Create read-only view link
// @Description Creates a fresh expiring public read-only link for the document.
// @Tags Documents
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param documentId path string true "Document ID"
// @Success 200 {object} dto.CreateReadOnlyViewLinkResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents/{documentId}/view-link [post]
func (c *DocumentController) CreateReadOnlyViewLink(ctx *gin.Context) {
	documentID := ctx.Param("documentId")
	workspaceID, ok := middleware.GetWorkspaceID(ctx)
	if !ok {
		HandleError(ctx, entity.ErrMissingWorkspaceID)
		return
	}

	result, err := c.readOnlyViewUC.CreateReadOnlyViewLink(ctx.Request.Context(), workspaceID, documentID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, dto.NewCreateReadOnlyViewLinkResponse(result))
}

// GetDocument returns a single document with recipients.
// @Summary Get document
// @Tags Documents
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param documentId path string true "Document ID"
// @Success 200 {object} dto.DocumentResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents/{documentId} [get]
func (c *DocumentController) GetDocument(ctx *gin.Context) {
	documentID := ctx.Param("documentId")

	doc, err := c.documentUC.GetDocumentWithRecipients(ctx.Request.Context(), documentID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, doc)
}

// GetRecipients returns recipients for a document.
// @Summary Get document recipients
// @Tags Documents
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param documentId path string true "Document ID"
// @Success 200 {array} dto.RecipientResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents/{documentId}/recipients [get]
func (c *DocumentController) GetRecipients(ctx *gin.Context) {
	documentID := ctx.Param("documentId")

	recipients, err := c.documentUC.GetDocumentRecipients(ctx.Request.Context(), documentID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, recipients)
}

// CreateDocument creates and sends a document for signing.
// @Summary Create and send document
// @Tags Documents
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param request body dto.CreateDocumentRequest true "Document creation request"
// @Success 201 {object} dto.DocumentResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents [post]
func (c *DocumentController) CreateDocument(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	var req dto.CreateDocumentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	// Build command
	cmd := documentuc.CreateDocumentCommand{
		WorkspaceID:               workspaceID,
		TemplateVersionID:         req.TemplateVersionID,
		Title:                     req.Title,
		ClientExternalReferenceID: req.ClientExternalReferenceID,
		InjectedValues:            req.InjectedValues,
		Recipients:                make([]documentuc.DocumentRecipientCommand, len(req.Recipients)),
		OperationType:             entity.OperationCreate,
		RelatedDocumentID:         req.RelatedDocumentID,
	}

	if req.OperationType != nil {
		cmd.OperationType = entity.OperationType(*req.OperationType)
	}

	for i, r := range req.Recipients {
		cmd.Recipients[i] = documentuc.DocumentRecipientCommand{
			RoleID: r.RoleID,
			Name:   r.Name,
			Email:  r.Email,
		}
	}

	doc, err := c.documentUC.CreateAndSendDocument(ctx.Request.Context(), cmd)
	if err != nil {
		slog.ErrorContext(ctx.Request.Context(), "failed to create document",
			slog.String("workspace_id", workspaceID),
			slog.String("error", err.Error()),
		)
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusCreated, doc)
}

// CreateDocumentsBatch creates multiple documents in a single batch.
// @Summary Batch create documents
// @Tags Documents
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param request body dto.BatchCreateDocumentRequest true "Batch document creation request"
// @Success 200 {object} dto.BatchCreateDocumentResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents/batch [post]
func (c *DocumentController) CreateDocumentsBatch(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	var req dto.BatchCreateDocumentRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, err)
		return
	}

	cmds := make([]documentuc.CreateDocumentCommand, len(req.Documents))
	for i, docReq := range req.Documents {
		cmd := documentuc.CreateDocumentCommand{
			WorkspaceID:               workspaceID,
			TemplateVersionID:         docReq.TemplateVersionID,
			Title:                     docReq.Title,
			ClientExternalReferenceID: docReq.ClientExternalReferenceID,
			InjectedValues:            docReq.InjectedValues,
			Recipients:                make([]documentuc.DocumentRecipientCommand, len(docReq.Recipients)),
			OperationType:             entity.OperationCreate,
			RelatedDocumentID:         docReq.RelatedDocumentID,
		}

		if docReq.OperationType != nil {
			cmd.OperationType = entity.OperationType(*docReq.OperationType)
		}

		for j, r := range docReq.Recipients {
			cmd.Recipients[j] = documentuc.DocumentRecipientCommand{
				RoleID: r.RoleID,
				Name:   r.Name,
				Email:  r.Email,
			}
		}

		cmds[i] = cmd
	}

	results, err := c.documentUC.CreateDocumentsBatch(ctx.Request.Context(), cmds)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	response := dto.BatchCreateDocumentResponse{
		Results: make([]dto.BatchDocumentResultResponse, len(results)),
	}
	for i, r := range results {
		result := dto.BatchDocumentResultResponse{
			Index:    r.Index,
			Success:  r.Error == nil,
			Document: r.Document,
		}
		if r.Error != nil {
			result.Error = r.Error.Error()
		}
		response.Results[i] = result
	}

	ctx.JSON(http.StatusOK, response)
}

// GetSigningURL returns the signing URL for a recipient.
// @Summary Get signing URL
// @Tags Documents
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param documentId path string true "Document ID"
// @Param recipientId path string true "Recipient ID"
// @Success 200 {object} dto.SigningURLResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents/{documentId}/recipients/{recipientId}/signing-url [get]
func (c *DocumentController) GetSigningURL(ctx *gin.Context) {
	documentID := ctx.Param("documentId")
	recipientID := ctx.Param("recipientId")

	url, err := c.documentUC.GetSigningURL(ctx.Request.Context(), documentID, recipientID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"signingUrl": url,
	})
}

// RefreshStatus refreshes document status from the signing provider.
// @Summary Refresh document status
// @Tags Documents
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param documentId path string true "Document ID"
// @Success 200 {object} dto.DocumentResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents/{documentId}/refresh [post]
func (c *DocumentController) RefreshStatus(ctx *gin.Context) {
	documentID := ctx.Param("documentId")

	doc, err := c.documentUC.RefreshDocumentStatus(ctx.Request.Context(), documentID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, doc)
}

// CancelDocument cancels a pending document.
// @Summary Cancel document
// @Tags Documents
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param documentId path string true "Document ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents/{documentId}/cancel [post]
func (c *DocumentController) CancelDocument(ctx *gin.Context) {
	documentID := ctx.Param("documentId")

	if err := c.documentUC.CancelDocument(ctx.Request.Context(), documentID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"status": "cancelled",
	})
}

// DeprecateDocument deprecates a completed document.
// @Summary Deprecate completed document
// @Tags Documents
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param documentId path string true "Document ID"
// @Param request body dto.InternalDeprecateDocumentRequest false "Deprecation request"
// @Success 200 {object} dto.InternalDeprecateDocumentResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents/{documentId}/deprecate [post]
func (c *DocumentController) DeprecateDocument(ctx *gin.Context) {
	documentID := ctx.Param("documentId")

	var req dto.InternalDeprecateDocumentRequest
	if ctx.Request.ContentLength > 0 {
		if err := ctx.ShouldBindJSON(&req); err != nil {
			ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
			return
		}
	}

	result, err := c.documentUC.DeprecateDocument(ctx.Request.Context(), documentID, req.Reason)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, buildDeprecateDocumentResponse(result))
}

// GetDocumentPDF returns the signed PDF for a completed document.
// @Summary Download signed PDF
// @Tags Documents
// @Produce application/pdf
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param documentId path string true "Document ID"
// @Success 200 {file} file
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents/{documentId}/pdf [get]
func (c *DocumentController) GetDocumentPDF(ctx *gin.Context) {
	documentID := ctx.Param("documentId")

	pdfData, filename, err := c.documentUC.GetDocumentPDF(ctx.Request.Context(), documentID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	ctx.Data(http.StatusOK, "application/pdf", pdfData)
}

// GetStatistics returns document statistics for the workspace.
// @Summary Get document statistics
// @Tags Documents
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Success 200 {object} documentuc.DocumentStatistics
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents/statistics [get]
func (c *DocumentController) GetStatistics(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	stats, err := c.documentUC.GetDocumentStatistics(ctx.Request.Context(), workspaceID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, stats)
}

// SendReminder sends reminder notifications to pending recipients of a document.
// @Summary Send document reminder
// @Tags Documents
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param documentId path string true "Document ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents/{documentId}/remind [post]
func (c *DocumentController) SendReminder(ctx *gin.Context) {
	documentID := ctx.Param("documentId")

	if err := c.documentUC.SendReminder(ctx.Request.Context(), documentID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"status": "reminders_sent",
	})
}

// GetDocumentEvents returns the audit event trail for a document.
// @Summary Get document events
// @Tags Documents
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param documentId path string true "Document ID"
// @Param limit query int false "Limit results" default(50)
// @Param offset query int false "Offset for pagination" default(0)
// @Success 200 {array} dto.DocumentEventResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents/{documentId}/events [get]
func (c *DocumentController) GetDocumentEvents(ctx *gin.Context) {
	documentID := ctx.Param("documentId")

	limit := 50
	offset := 0
	if l := ctx.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if o := ctx.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil && parsed >= 0 {
			offset = parsed
		}
	}

	events, err := c.eventEmitter.GetDocumentEvents(ctx.Request.Context(), documentID, limit, offset)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	responses := make([]dto.DocumentEventResponse, 0, len(events))
	for _, e := range events {
		resp := dto.DocumentEventResponse{
			ID:          e.ID,
			DocumentID:  e.DocumentID,
			EventType:   e.EventType,
			ActorType:   e.ActorType,
			ActorID:     e.ActorID,
			OldStatus:   e.OldStatus,
			NewStatus:   e.NewStatus,
			RecipientID: e.RecipientID,
			CreatedAt:   e.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		if e.Metadata != nil {
			var meta any
			if err := json.Unmarshal(e.Metadata, &meta); err == nil {
				resp.Metadata = meta
			}
		}
		responses = append(responses, resp)
	}

	ctx.JSON(http.StatusOK, responses)
}

// InvalidateTokens invalidates all active access tokens for a document.
// @Summary Invalidate all access tokens for a document
// @Tags Documents
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param documentId path string true "Document ID"
// @Success 200 {object} map[string]any
// @Failure 400 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents/{documentId}/invalidate-tokens [post]
func (c *DocumentController) InvalidateTokens(ctx *gin.Context) {
	documentID := ctx.Param("documentId")

	if err := c.preSigningUC.InvalidateTokens(ctx.Request.Context(), documentID); err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Tokens invalidated",
	})
}
