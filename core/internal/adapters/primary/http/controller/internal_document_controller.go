package controller

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/rendis/doc-assembly/core/internal/core/entity"
	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
)

// Internal API header constants.
const (
	HeaderTenantCode      = "X-Tenant-Code"
	HeaderWorkspaceCode   = "X-Workspace-Code"
	HeaderDocumentType    = "X-Document-Type"
	HeaderExternalID      = "X-External-ID"
	HeaderTransactionalID = "X-Transactional-ID"
	HeaderEnvironment     = "X-Environment"
	HeaderProcess         = "X-Process"
	HeaderProcessType     = "X-Process-Type"
)

// internalDocHeaders holds the required headers for internal document operations.
type internalDocHeaders struct {
	TenantCode      string
	WorkspaceCode   string
	DocumentType    string
	ExternalID      string
	TransactionalID string
	Environment     string
}

// InternalDocumentController handles internal API document requests.
// These endpoints are used for service-to-service communication.
type InternalDocumentController struct {
	internalDocUC documentuc.InternalDocumentUseCase
	documentUC    documentuc.DocumentUseCase
}

// NewInternalDocumentController creates a new internal document controller.
func NewInternalDocumentController(
	internalDocUC documentuc.InternalDocumentUseCase,
	documentUC ...documentuc.DocumentUseCase,
) *InternalDocumentController {
	c := &InternalDocumentController{
		internalDocUC: internalDocUC,
	}
	if len(documentUC) > 0 {
		c.documentUC = documentUC[0]
	}
	return c
}

// RegisterRoutes registers all internal document routes.
// The API key is validated via middleware.
func (c *InternalDocumentController) RegisterRoutes(api *gin.RouterGroup, authMiddleware gin.HandlerFunc) {
	internal := api.Group("/internal/documents")
	internal.Use(authMiddleware)
	{
		internal.POST("/create", c.CreateDocument)
		internal.POST("/reset", c.ResetDocument)
		internal.POST("/:documentId/deprecate", c.DeprecateDocument)
	}
}

// CreateDocument creates a document via internal API.
// @Summary Create document via internal API
// @Description Creates or replays a document using the extension system (Mapper, Init, Injectors)
// @Tags Internal
// @Accept json
// @Produce json
// @Param X-API-Key header string true "API Key for authentication"
// @Param X-Tenant-Code header string true "Tenant business code"
// @Param X-Workspace-Code header string true "Workspace business code"
// @Param X-Document-Type header string true "Document type code"
// @Param X-External-ID header string true "External ID (e.g., CRM entity ID)"
// @Param X-Transactional-ID header string true "Transactional ID for idempotency"
// @Param request body dto.InternalCreateDocumentRequest true "Internal create request"
// @Success 201 {object} dto.InternalCreateDocumentWithRecipientsResponse
// @Success 200 {object} dto.InternalCreateDocumentWithRecipientsResponse
// @Failure 400 {object} dto.InternalErrorResponse
// @Failure 401 {object} dto.InternalErrorResponse
// @Failure 404 {object} dto.InternalErrorResponse
// @Failure 500 {object} dto.InternalErrorResponse
// @Router /api/v1/internal/documents/create [post]
//
//nolint:funlen // HTTP orchestration keeps validation/mapping flow explicit.
func (c *InternalDocumentController) CreateDocument(ctx *gin.Context) {
	c.handleCreateDocument(ctx, false)
}

// ResetDocument recreates a document via internal API using force-create semantics.
// @Summary Reset unsigned document via internal API
// @Description Recreates an unsigned logical document using force-create semantics and reruns Mapper, Init, and Injectors
// @Tags Internal
// @Accept json
// @Produce json
// @Param X-API-Key header string true "API Key for authentication"
// @Param X-Tenant-Code header string true "Tenant business code"
// @Param X-Workspace-Code header string true "Workspace business code"
// @Param X-Document-Type header string true "Document type code"
// @Param X-External-ID header string true "External ID"
// @Param X-Transactional-ID header string true "Transactional ID"
// @Param request body dto.InternalCreateDocumentRequest true "Internal reset request"
// @Success 201 {object} dto.InternalCreateDocumentWithRecipientsResponse
// @Failure 400 {object} dto.InternalErrorResponse
// @Failure 401 {object} dto.InternalErrorResponse
// @Failure 409 {object} dto.InternalErrorResponse
// @Failure 500 {object} dto.InternalErrorResponse
// @Router /api/v1/internal/documents/reset [post]
func (c *InternalDocumentController) ResetDocument(ctx *gin.Context) {
	c.handleCreateDocument(ctx, true)
}

//nolint:funlen // HTTP orchestration keeps validation/mapping flow explicit.
func (c *InternalDocumentController) handleCreateDocument(ctx *gin.Context, reset bool) {
	h, missing := validateAndExtractHeaders(ctx)
	if len(missing) > 0 {
		ctx.JSON(http.StatusBadRequest, dto.InternalErrorResponse{
			Error:   "missing required headers",
			Code:    "MISSING_HEADERS",
			Details: missing,
		})
		return
	}

	env, err := entity.ParseEnvironment(h.Environment)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.InternalErrorResponse{
			Error:   "invalid X-Environment header value (must be 'dev' or 'prod')",
			Code:    "INVALID_HEADER",
			Details: []string{HeaderEnvironment},
		})
		return
	}

	rawBody, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.InternalErrorResponse{
			Error: "failed to read request body",
			Code:  "INVALID_BODY",
		})
		return
	}

	var req dto.InternalCreateDocumentRequest
	if err := json.Unmarshal(rawBody, &req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.InternalErrorResponse{
			Error: "invalid request body",
			Code:  "INVALID_BODY",
		})
		return
	}

	if len(req.Payload) == 0 || string(req.Payload) == "null" {
		ctx.JSON(http.StatusBadRequest, dto.InternalErrorResponse{
			Error: "payload is required",
			Code:  "INVALID_BODY",
		})
		return
	}

	headers := make(map[string]string)
	for key := range ctx.Request.Header {
		headers[key] = ctx.GetHeader(key)
	}

	forceCreate := false
	if req.ForceCreate != nil {
		forceCreate = *req.ForceCreate
	}
	process := ctx.GetHeader(HeaderProcess)
	processType := ctx.GetHeader(HeaderProcessType)
	slog.InfoContext(ctx.Request.Context(), "internal create request received",
		"tenant_code", h.TenantCode,
		"workspace_code", h.WorkspaceCode,
		"document_type", h.DocumentType,
		"environment", h.Environment,
		"process", process,
		"process_type", processType,
		"payload_bytes", len(req.Payload),
	)

	cmd := documentuc.InternalCreateCommand{
		TenantCode:      h.TenantCode,
		WorkspaceCode:   h.WorkspaceCode,
		DocumentType:    h.DocumentType,
		Process:         process,
		ProcessType:     processType,
		ExternalID:      h.ExternalID,
		TransactionalID: h.TransactionalID,
		Environment:     env,
		ForceCreate:     forceCreate,
		SupersedeReason: req.SupersedeReason,
		Metadata:        req.Metadata,
		Headers:         headers,
		PayloadRaw:      req.Payload,
	}

	var result *documentuc.InternalCreateResult
	if reset {
		result, err = c.internalDocUC.ResetUnsignedDocument(ctx.Request.Context(), cmd)
	} else {
		result, err = c.internalDocUC.CreateDocument(ctx.Request.Context(), cmd)
	}
	if err != nil {
		HandleError(ctx, err)
		return
	}

	status := http.StatusCreated
	if result.IdempotentReplay {
		status = http.StatusOK
	}

	ctx.JSON(status, buildCreateDocumentResponse(result, h))
}

// DeprecateDocument deprecates a completed document via internal API.
// @Summary Deprecate completed document via internal API
// @Description Invalidates a completed document and attempts best-effort cleanup in the signing provider
// @Tags Internal
// @Accept json
// @Produce json
// @Param X-API-Key header string true "API Key for authentication"
// @Param documentId path string true "Document ID"
// @Param request body dto.InternalDeprecateDocumentRequest false "Deprecation request"
// @Success 200 {object} dto.InternalDeprecateDocumentResponse
// @Failure 400 {object} dto.InternalErrorResponse
// @Failure 401 {object} dto.InternalErrorResponse
// @Failure 404 {object} dto.InternalErrorResponse
// @Failure 409 {object} dto.InternalErrorResponse
// @Failure 500 {object} dto.InternalErrorResponse
// @Router /api/v1/internal/documents/{documentId}/deprecate [post]
func (c *InternalDocumentController) DeprecateDocument(ctx *gin.Context) {
	if c.documentUC == nil {
		ctx.JSON(http.StatusInternalServerError, dto.InternalErrorResponse{
			Error: "document use case is not configured",
			Code:  "INTERNAL_ERROR",
		})
		return
	}

	documentID := ctx.Param("documentId")
	req, ok := readInternalDeprecateRequest(ctx)
	if !ok {
		return
	}

	result, err := c.documentUC.DeprecateDocument(ctx.Request.Context(), documentID, req.Reason)
	if err != nil {
		HandleError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, buildDeprecateDocumentResponse(result))
}

func readInternalDeprecateRequest(ctx *gin.Context) (dto.InternalDeprecateDocumentRequest, bool) {
	var req dto.InternalDeprecateDocumentRequest
	if ctx.Request.Body == nil {
		return req, true
	}
	rawBody, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, dto.InternalErrorResponse{Error: "failed to read request body", Code: "INVALID_BODY"})
		return req, false
	}
	if len(rawBody) == 0 {
		return req, true
	}
	if err := json.Unmarshal(rawBody, &req); err != nil {
		ctx.JSON(http.StatusBadRequest, dto.InternalErrorResponse{Error: "invalid request body", Code: "INVALID_BODY"})
		return req, false
	}
	return req, true
}

// validateAndExtractHeaders validates and extracts required headers from the request.
// Returns the headers struct and a list of missing header names (empty if all present).
func validateAndExtractHeaders(ctx *gin.Context) (*internalDocHeaders, []string) {
	h := &internalDocHeaders{
		TenantCode:      ctx.GetHeader(HeaderTenantCode),
		WorkspaceCode:   ctx.GetHeader(HeaderWorkspaceCode),
		DocumentType:    ctx.GetHeader(HeaderDocumentType),
		ExternalID:      ctx.GetHeader(HeaderExternalID),
		TransactionalID: ctx.GetHeader(HeaderTransactionalID),
		Environment:     ctx.GetHeader(HeaderEnvironment),
	}

	var missing []string
	if h.TenantCode == "" {
		missing = append(missing, HeaderTenantCode)
	}
	if h.WorkspaceCode == "" {
		missing = append(missing, HeaderWorkspaceCode)
	}
	if h.DocumentType == "" {
		missing = append(missing, HeaderDocumentType)
	}
	if h.ExternalID == "" {
		missing = append(missing, HeaderExternalID)
	}
	if h.TransactionalID == "" {
		missing = append(missing, HeaderTransactionalID)
	}
	if h.Environment == "" {
		missing = append(missing, HeaderEnvironment)
	}

	return h, missing
}

// buildCreateDocumentResponse builds the response DTO from the create result.
func buildCreateDocumentResponse(
	result *documentuc.InternalCreateResult,
	h *internalDocHeaders,
) dto.InternalCreateDocumentWithRecipientsResponse {
	doc := result.Document
	response := dto.InternalCreateDocumentWithRecipientsResponse{
		InternalCreateDocumentResponse: dto.InternalCreateDocumentResponse{
			ID:                           doc.ID,
			WorkspaceID:                  doc.WorkspaceID,
			TemplateVersionID:            doc.TemplateVersionID,
			ExternalID:                   h.ExternalID,
			TransactionalID:              h.TransactionalID,
			Status:                       string(doc.Status),
			IdempotentReplay:             result.IdempotentReplay,
			SupersededPreviousDocumentID: result.SupersededPreviousDocumentID,
		},
	}

	for _, r := range doc.Recipients {
		response.Recipients = append(response.Recipients, dto.InternalDocumentRecipientResponse{
			ID:         r.ID,
			Name:       r.Name,
			Email:      r.Email,
			SigningURL: r.SigningURL,
		})
	}

	return response
}

func buildDeprecateDocumentResponse(result *documentuc.DeprecateDocumentResult) dto.InternalDeprecateDocumentResponse {
	response := dto.InternalDeprecateDocumentResponse{
		ID:     result.Document.ID,
		Status: string(result.Document.Status),
	}
	if result.ProviderCleanup != nil {
		response.ProviderCleanup = &dto.ProviderCleanupResponse{
			Action: result.ProviderCleanup.Action,
			Status: result.ProviderCleanup.Status,
			Reason: result.ProviderCleanup.Reason,
		}
	}
	return response
}
