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
}

// NewInternalDocumentController creates a new internal document controller.
func NewInternalDocumentController(
	internalDocUC documentuc.InternalDocumentUseCase,
) *InternalDocumentController {
	return &InternalDocumentController{
		internalDocUC: internalDocUC,
	}
}

// RegisterRoutes registers all internal document routes.
// The API key is validated via middleware.
func (c *InternalDocumentController) RegisterRoutes(api *gin.RouterGroup, authMiddleware gin.HandlerFunc) {
	internal := api.Group("/internal/documents")
	internal.Use(authMiddleware)
	{
		internal.POST("/create", c.CreateDocument)
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

	result, err := c.internalDocUC.CreateDocument(ctx.Request.Context(), cmd)
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
