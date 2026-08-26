package controller

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	documentsvc "github.com/TetherEducation/doc-assembly/core/internal/core/service/document"
	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
)

// DocumentController handles document HTTP requests.
type DocumentController struct {
	documentUC     documentuc.DocumentUseCase
	preSigningUC   documentuc.PreSigningUseCase
	readOnlyViewUC documentuc.ReadOnlyViewUseCase
	eventEmitter   *documentsvc.EventEmitter
}

type documentRouteOptions struct {
	skipReadOnlyViewLink bool
}

// DocumentRouteOption configures document route registration.
type DocumentRouteOption func(*documentRouteOptions)

// WithoutReadOnlyViewLinkRoute skips the authenticated panel read-only link route.
func WithoutReadOnlyViewLinkRoute() DocumentRouteOption {
	return func(opts *documentRouteOptions) {
		opts.skipReadOnlyViewLink = true
	}
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
func (c *DocumentController) RegisterRoutes(api *gin.RouterGroup, opts ...DocumentRouteOption) {
	routeOptions := documentRouteOptions{}
	for _, opt := range opts {
		if opt != nil {
			opt(&routeOptions)
		}
	}

	docs := api.Group("/documents")
	{
		// List documents in workspace
		docs.GET("", middleware.RequireViewer(), c.ListDocuments)

		// Get document statistics
		docs.GET("/statistics", middleware.RequireViewer(), c.GetStatistics)

		// Distinct document types on workspace documents (signing filter dropdown)
		docs.GET("/document-type-options", middleware.RequireViewer(), c.ListDocumentTypeOptions)

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

		if !routeOptions.skipReadOnlyViewLink {
			// Create public read-only view link
			docs.POST("/:documentId/view-link", middleware.RequireViewer(), c.CreateReadOnlyViewLink)

			// Render the unsigned (or blank) PDF for in-person signing.
			// Registered externally with custom auth when the read-only view
			// link authenticator is set, mirroring view-link.
			docs.GET("/:documentId/print-pdf", middleware.RequireViewer(), c.GetDocumentPrintPDF)
		}

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
// @Param status query string false "Filter by document status; comma-separated for multiple (OR)"
// @Param documentTypeIds query string false "Comma-separated document type UUIDs (multi-select filter)"
// @Param search query string false "Search by document title or signer recipient email (substring, case-insensitive)"
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
		statuses, err := parseCommaSeparatedDocumentStatuses(status)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
			return
		}
		filters.Statuses = statuses
	}
	filters.Search = ctx.Query("search")

	if raw := ctx.Query("documentTypeIds"); raw != "" {
		ids, err := parseCommaSeparatedUUIDs(raw)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, dto.NewErrorResponse(err))
			return
		}
		filters.DocumentTypeIDs = ids
	}

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

// CreateReadOnlyViewLink creates a public read-only view link for a document from panel workspace context.
func (c *DocumentController) CreateReadOnlyViewLink(ctx *gin.Context) {
	documentID := ctx.Param("documentId")
	workspaceID, ok := middleware.GetWorkspaceID(ctx)
	if !ok || strings.TrimSpace(workspaceID) == "" {
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

// CreateReadOnlyViewLinkByWorkspaceCode creates a public read-only view link for
// external callers that identify the workspace by business code.
// @Summary Create read-only view link
// @Description Creates a fresh expiring public read-only link for the document.
// @Tags Documents
// @Accept json
// @Produce json
// @Param X-Workspace-Code header string true "Workspace business code"
// @Param documentId path string true "Document ID"
// @Success 200 {object} dto.CreateReadOnlyViewLinkResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents/{documentId}/view-link [post]
func (c *DocumentController) CreateReadOnlyViewLinkByWorkspaceCode(ctx *gin.Context) {
	documentID := ctx.Param("documentId")
	workspaceCode := strings.TrimSpace(ctx.GetHeader(middleware.ReadOnlyViewLinkWorkspaceCodeHeader))
	if workspaceCode == "" {
		HandleError(ctx, entity.ErrMissingWorkspaceID)
		return
	}

	for _, candidateCode := range readOnlyViewWorkspaceCodeCandidates(ctx, workspaceCode) {
		result, err := c.readOnlyViewUC.CreateReadOnlyViewLinkByWorkspaceCode(ctx.Request.Context(), candidateCode, documentID)
		if err == nil {
			ctx.JSON(http.StatusOK, dto.NewCreateReadOnlyViewLinkResponse(result))
			return
		}
		if errors.Is(err, entity.ErrForbidden) {
			continue
		}
		HandleError(ctx, err)
		return
	}

	HandleError(ctx, entity.ErrForbidden)
}

// GetDocumentPrintPDF renders the unsigned (or blank) document PDF for
// in-person signing, for authenticated panel users.
// @Summary Download unsigned document PDF for printing
// @Description Renders the document's current unsigned PDF, or the blank template when blank=true, so campuses can print it for in-person signing.
// @Tags Documents
// @Produce application/pdf
// @Param X-Workspace-ID header string true "Workspace ID"
// @Param documentId path string true "Document ID"
// @Param blank query boolean false "Render the blank template without injected values"
// @Success 200 {file} application/pdf
// @Failure 400 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents/{documentId}/print-pdf [get]
func (c *DocumentController) GetDocumentPrintPDF(ctx *gin.Context) {
	documentID := ctx.Param("documentId")
	workspaceID, ok := middleware.GetWorkspaceID(ctx)
	if !ok || strings.TrimSpace(workspaceID) == "" {
		HandleError(ctx, entity.ErrMissingWorkspaceID)
		return
	}

	pdf, filename, err := c.readOnlyViewUC.GetPrintPDF(ctx.Request.Context(), workspaceID, documentID, printPDFBlankParam(ctx))
	if err != nil {
		HandleError(ctx, err)
		return
	}

	writePrintPDF(ctx, pdf, filename)
}

// GetDocumentPrintPDFByWorkspaceCode renders the unsigned (or blank) document
// PDF for external callers that identify the workspace by business code.
// @Summary Download unsigned document PDF for printing
// @Description Renders the document's current unsigned PDF, or the blank template when blank=true, so campuses can print it for in-person signing.
// @Tags Documents
// @Produce application/pdf
// @Param X-Workspace-Code header string true "Workspace business code"
// @Param documentId path string true "Document ID"
// @Param blank query boolean false "Render the blank template without injected values"
// @Success 200 {file} application/pdf
// @Failure 400 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 404 {object} dto.ErrorResponse
// @Failure 409 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents/{documentId}/print-pdf [get]
func (c *DocumentController) GetDocumentPrintPDFByWorkspaceCode(ctx *gin.Context) {
	documentID := ctx.Param("documentId")
	workspaceCode := strings.TrimSpace(ctx.GetHeader(middleware.ReadOnlyViewLinkWorkspaceCodeHeader))
	if workspaceCode == "" {
		HandleError(ctx, entity.ErrMissingWorkspaceID)
		return
	}
	blank := printPDFBlankParam(ctx)

	for _, candidateCode := range readOnlyViewWorkspaceCodeCandidates(ctx, workspaceCode) {
		pdf, filename, err := c.readOnlyViewUC.GetPrintPDFByWorkspaceCode(ctx.Request.Context(), candidateCode, documentID, blank)
		if err == nil {
			writePrintPDF(ctx, pdf, filename)
			return
		}
		if errors.Is(err, entity.ErrForbidden) {
			continue
		}
		HandleError(ctx, err)
		return
	}

	HandleError(ctx, entity.ErrForbidden)
}

// maxSigningStateDocumentIDs bounds one batch. Each ID costs two queries (document
// + recipients), so this caps a single request's fan-out; callers reconciling a
// whole cohort are expected to page rather than send one enormous batch.
const maxSigningStateDocumentIDs = 200

// GetDocumentsSigningStateByWorkspaceCode reports the real signing state of a batch
// of documents for external callers identifying the workspace by business code.
// @Summary Get documents signing state
// @Description Reports, per document, whether it was actually signed, plus per-recipient progress. Lets a caller tell "nobody ever signed" apart from "signed" instead of trusting its own workflow state.
// @Tags Documents
// @Accept json
// @Produce json
// @Param X-Workspace-Code header string true "Workspace business code"
// @Param request body dto.SigningStateRequest true "Document IDs to report on"
// @Success 200 {object} dto.SigningStateResponse
// @Failure 400 {object} dto.ErrorResponse
// @Failure 403 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents/signing-state [post]
func (c *DocumentController) GetDocumentsSigningStateByWorkspaceCode(ctx *gin.Context) {
	workspaceCode := strings.TrimSpace(ctx.GetHeader(middleware.ReadOnlyViewLinkWorkspaceCodeHeader))
	if workspaceCode == "" {
		HandleError(ctx, entity.ErrMissingWorkspaceID)
		return
	}

	var req dto.SigningStateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "documentIds is required"})
		return
	}

	pending := normalizeSigningStateDocumentIDs(req.DocumentIDs)
	if len(pending) == 0 {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse{Error: "documentIds must contain at least one id"})
		return
	}
	if len(pending) > maxSigningStateDocumentIDs {
		ctx.JSON(http.StatusBadRequest, dto.ErrorResponse{
			Error: fmt.Sprintf("documentIds must contain at most %d ids", maxSigningStateDocumentIDs),
		})
		return
	}

	// The single-document endpoints treat the candidate codes as "try until one is
	// allowed". A batch cannot: documents in one request may sit under different
	// authorized codes (campus, network, sandbox parent), so each pass resolves what
	// it can and hands the rest to the next candidate. Anything no authorized code
	// resolves stays unavailable.
	documents := make([]documentuc.SigningStateDocument, 0, len(pending))
	for _, candidateCode := range readOnlyViewWorkspaceCodeCandidates(ctx, workspaceCode) {
		if len(pending) == 0 {
			break
		}
		result, err := c.readOnlyViewUC.GetSigningStateByWorkspaceCode(ctx.Request.Context(), candidateCode, pending)
		if err != nil {
			if errors.Is(err, entity.ErrForbidden) {
				continue
			}
			HandleError(ctx, err)
			return
		}
		documents = append(documents, result.Documents...)
		pending = result.Unavailable
	}

	ctx.JSON(http.StatusOK, dto.NewSigningStateResponse(&documentuc.SigningStateResult{
		Documents:   documents,
		Unavailable: pending,
	}))
}

func normalizeSigningStateDocumentIDs(ids []string) []string {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func printPDFBlankParam(ctx *gin.Context) bool {
	value := strings.ToLower(strings.TrimSpace(ctx.Query("blank")))
	return value == "1" || value == "true"
}

func writePrintPDF(ctx *gin.Context, pdf []byte, filename string) {
	ctx.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	ctx.Data(http.StatusOK, "application/pdf", pdf)
}

func readOnlyViewWorkspaceCodeCandidates(ctx *gin.Context, headerWorkspaceCode string) []string {
	codes := []string{headerWorkspaceCode}
	if claims, ok := middleware.GetReadOnlyViewLinkAuthClaims(ctx); ok {
		codes = append(codes, claims.AuthorizedWorkspaceCodes...)
	}

	seen := make(map[string]struct{}, len(codes))
	out := make([]string, 0, len(codes))
	for _, code := range codes {
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		key := strings.ToUpper(code)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, code)
	}
	return out
}

// ListDocumentTypeOptions returns distinct document types that appear on documents in the workspace.
// @Summary List document type filter options
// @Tags Documents
// @Accept json
// @Produce json
// @Param X-Workspace-ID header string true "Workspace ID"
// @Success 200 {array} entity.DocumentTypeFilterOption
// @Failure 400 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /api/v1/documents/document-type-options [get]
func (c *DocumentController) ListDocumentTypeOptions(ctx *gin.Context) {
	workspaceID, _ := middleware.GetWorkspaceID(ctx)

	opts, err := c.documentUC.ListDocumentTypeFilterOptions(ctx.Request.Context(), workspaceID)
	if err != nil {
		HandleError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, opts)
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

func parseCommaSeparatedDocumentStatuses(raw string) ([]entity.DocumentStatus, error) {
	parts := strings.Split(raw, ",")
	var out []entity.DocumentStatus
	seen := make(map[entity.DocumentStatus]struct{})
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		s := entity.DocumentStatus(p)
		if !s.IsValid() {
			return nil, fmt.Errorf("invalid document status %q", p)
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out, nil
}

func parseCommaSeparatedUUIDs(raw string) ([]string, error) {
	parts := strings.Split(raw, ",")
	var ids []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if _, err := uuid.Parse(p); err != nil {
			return nil, fmt.Errorf("invalid document type id %q: %w", p, err)
		}
		ids = append(ids, p)
	}
	return ids, nil
}
