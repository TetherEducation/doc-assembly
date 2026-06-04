package controller

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

const legacyDocumentsProxyPath = "/legacy-documents/proxy"

// LegacyDocumentController delegates legacy document access negotiation to a
// host-provided handler.
type LegacyDocumentController struct {
	handler      port.LegacyDocumentHandler
	maxBodyBytes int64
}

// NewLegacyDocumentController creates a controller for the legacy document proxy.
func NewLegacyDocumentController(handler port.LegacyDocumentHandler, maxBodyBytes int64) *LegacyDocumentController {
	return &LegacyDocumentController{
		handler:      handler,
		maxBodyBytes: maxBodyBytes,
	}
}

// RegisterRoutes registers the legacy document proxy route. The route is
// intentionally registered for all methods so non-POST calls return 405.
func (c *LegacyDocumentController) RegisterRoutes(rg *gin.RouterGroup) {
	if c == nil || c.handler == nil || rg == nil {
		return
	}

	rg.Any(legacyDocumentsProxyPath, c.Proxy)
}

// Proxy handles legacy document access negotiation.
//
// @Summary      Proxy legacy document access negotiation
// @Description  Delegates legacy document access to a host-registered LegacyDocumentHandler. This endpoint is only for documents outside the current doc-assembly document lifecycle and is mounted only when a handler is registered.
// @Tags         legacy-documents
// @Accept       json
// @Produce      json
// @Param        X-Workspace-Code header string true "Workspace business code"
// @Param        X-Environment header string true "Runtime environment: dev or prod"
// @Param        request body object false "Host-defined JSON payload"
// @Success      200 {object} map[string]interface{} "Host-defined JSON response"
// @Failure      400 {object} map[string]string "Missing or invalid required header"
// @Failure      405 {object} map[string]string "Method not allowed"
// @Failure      413 {object} map[string]string "Request body too large"
// @Failure      500 {object} map[string]string "Handler or JSON serialization failure"
// @Router       /legacy-documents/proxy [post]
func (c *LegacyDocumentController) Proxy(ctx *gin.Context) {
	if ctx.Request.Method != http.MethodPost {
		ctx.JSON(http.StatusMethodNotAllowed, gin.H{"error": "method not allowed"})
		return
	}

	workspaceCode := strings.TrimSpace(ctx.GetHeader(HeaderWorkspaceCode))
	if workspaceCode == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "missing X-Workspace-Code"})
		return
	}

	environment, ok := parseRequiredLegacyEnvironment(ctx)
	if !ok {
		return
	}

	rawBody, ok := c.readRawBody(ctx)
	if !ok {
		return
	}

	resp, err := c.handler.HandleLegacyDocument(ctx.Request.Context(), &port.LegacyDocumentRequest{
		WorkspaceCode: workspaceCode,
		Environment:   environment,
		Headers:       cloneLegacyHeaders(ctx.Request.Header),
		RawBody:       rawBody,
	})
	if err != nil {
		slog.ErrorContext(ctx.Request.Context(), "legacy document handler failed",
			slog.String("workspace_code", workspaceCode),
			slog.String("environment", string(environment)),
			slog.String("operation_id", middleware.GetOperationID(ctx)),
			slog.Any("error", err),
		)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "legacy document handler failed"})
		return
	}
	if resp == nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "legacy document handler failed"})
		return
	}

	applyLegacyResponseHeaders(ctx, resp.Headers)
	writeLegacyJSON(ctx, resp.StatusCode, resp.Body)
}

func parseRequiredLegacyEnvironment(ctx *gin.Context) (entity.Environment, bool) {
	raw := strings.TrimSpace(ctx.GetHeader(HeaderEnvironment))
	if raw == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "missing X-Environment"})
		return "", false
	}

	environment, err := entity.ParseEnvironment(raw)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid X-Environment header value (must be 'dev' or 'prod')"})
		return "", false
	}

	return environment, true
}

func (c *LegacyDocumentController) readRawBody(ctx *gin.Context) ([]byte, bool) {
	limit := c.maxBodyBytes
	if limit <= 0 {
		limit = 64 * 1024
	}

	reader := http.MaxBytesReader(ctx.Writer, ctx.Request.Body, limit)
	body, err := io.ReadAll(reader)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "request body too large"})
			return nil, false
		}

		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read request body"})
		return nil, false
	}

	return body, true
}

func cloneLegacyHeaders(headers http.Header) map[string][]string {
	cloned := make(map[string][]string, len(headers))
	for key, values := range headers {
		cloned[key] = append([]string(nil), values...)
	}

	return cloned
}

func applyLegacyResponseHeaders(ctx *gin.Context, headers map[string][]string) {
	for key, values := range headers {
		if isProtectedLegacyResponseHeader(key) {
			continue
		}

		for _, value := range values {
			ctx.Writer.Header().Add(key, value)
		}
	}
}

func isProtectedLegacyResponseHeader(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if normalized == "" {
		return true
	}
	if strings.HasPrefix(normalized, "access-control-") {
		return true
	}

	switch normalized {
	case "content-type",
		"x-operation-id",
		"vary",
		"cache-control",
		"pragma",
		"expires",
		"set-cookie":
		return true
	default:
		return false
	}
}

func writeLegacyJSON(ctx *gin.Context, statusCode int, body any) {
	if statusCode < http.StatusContinue || statusCode > 599 {
		statusCode = http.StatusOK
	}

	ctx.Header("Content-Type", "application/json; charset=utf-8")

	if raw, ok := body.(json.RawMessage); ok {
		if !json.Valid(raw) {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "legacy document response serialization failed"})
			return
		}

		ctx.Status(statusCode)
		_, _ = ctx.Writer.Write(raw)
		return
	}

	payload, err := json.Marshal(body)
	if err != nil {
		slog.ErrorContext(ctx.Request.Context(), "legacy document response serialization failed",
			slog.String("operation_id", middleware.GetOperationID(ctx)),
			slog.Any("error", err),
		)
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "legacy document response serialization failed"})
		return
	}

	ctx.Status(statusCode)
	_, _ = ctx.Writer.Write(payload)
}
