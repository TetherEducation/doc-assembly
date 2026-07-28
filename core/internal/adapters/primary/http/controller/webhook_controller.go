package controller

import (
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	_ "github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
)

// WebhookController handles incoming webhooks from signing providers.
type WebhookController struct {
	documentUC      documentuc.DocumentUseCase
	webhookHandlers map[string]port.WebhookHandler
}

// NewWebhookController creates a new webhook controller.
func NewWebhookController(
	documentUC documentuc.DocumentUseCase,
	webhookHandlers map[string]port.WebhookHandler,
) *WebhookController {
	return &WebhookController{
		documentUC:      documentUC,
		webhookHandlers: webhookHandlers,
	}
}

// RegisterRoutes registers webhook routes.
// Webhooks are not protected by auth middleware as they come from external providers.
func (c *WebhookController) RegisterRoutes(router gin.IRouter) {
	webhooks := router.Group("/webhooks")
	{
		// Provider-specific webhook endpoints
		webhooks.POST("/signing/:provider", c.HandleSigningWebhook)
	}
}

// HandleSigningWebhook processes incoming webhooks from signing providers.
// @Summary Handle signing provider webhook
// @Tags Webhooks
// @Accept json
// @Produce json
// @Param provider path string true "Provider name (e.g., documenso)"
// @Param X-Documenso-Secret header string false "Webhook signature (for Documenso)"
// @Param X-Webhook-Signature header string false "Webhook signature (generic)"
// @Success 200 {object} map[string]string
// @Failure 400 {object} dto.ErrorResponse
// @Failure 401 {object} dto.ErrorResponse
// @Failure 500 {object} dto.ErrorResponse
// @Router /webhooks/signing/{provider} [post]
func (c *WebhookController) HandleSigningWebhook(ctx *gin.Context) {
	provider := ctx.Param("provider")

	handler, ok := c.webhookHandlers[provider]
	if !ok {
		slog.WarnContext(ctx.Request.Context(), "webhook received for unknown provider", slog.String("provider", provider))
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "unknown provider"})
		return
	}

	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		slog.ErrorContext(ctx.Request.Context(), "failed to read webhook body", slog.String("provider", provider), slog.String("error", err.Error()))
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return
	}

	signature := extractWebhookSignature(ctx)
	slog.InfoContext(ctx.Request.Context(), "processing signing webhook",
		slog.String("provider", provider),
		slog.Int("body_length", len(body)),
		slog.Bool("has_signature", signature != ""),
	)

	event, ok := c.parseWebhook(ctx, handler, body, signature, provider)
	if !ok {
		return
	}

	if !c.processWebhookEvent(ctx, event, provider) {
		return
	}

	slog.InfoContext(ctx.Request.Context(), "webhook processed successfully",
		slog.String("provider", provider),
		slog.String("event_type", event.EventType),
		slog.String("document_id", event.ProviderDocumentID),
	)
	ctx.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// extractWebhookSignature extracts the webhook signature from request headers.
// Different providers use different header names.
func extractWebhookSignature(ctx *gin.Context) string {
	if sig := ctx.GetHeader("X-Documenso-Secret"); sig != "" {
		return sig
	}
	if sig := ctx.GetHeader("X-Webhook-Signature"); sig != "" {
		return sig
	}
	return ctx.GetHeader("X-Signature")
}

// parseWebhook parses and validates the webhook payload.
// Returns the parsed event and true on success, or false if an error response was sent.
func (c *WebhookController) parseWebhook(ctx *gin.Context, handler port.WebhookHandler, body []byte, signature, provider string) (*port.WebhookEvent, bool) {
	event, err := handler.ParseWebhook(ctx.Request.Context(), &port.ParseWebhookRequest{
		Body:        body,
		Signature:   signature,
		Environment: entity.EnvironmentProd,
	})
	if err != nil {
		if err == entity.ErrInvalidWebhookSignature {
			slog.WarnContext(ctx.Request.Context(), "invalid webhook signature",
				slog.String("provider", provider),
			)
			ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
			return nil, false
		}

		slog.ErrorContext(ctx.Request.Context(), "failed to parse webhook",
			slog.String("provider", provider),
			slog.String("error", err.Error()),
		)
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "failed to parse webhook"})
		return nil, false
	}
	return event, true
}

// processWebhookEvent processes the webhook event through the document use case.
// Returns true on success, or false if an error response was sent.
func (c *WebhookController) processWebhookEvent(ctx *gin.Context, event *port.WebhookEvent, provider string) bool {
	if err := c.documentUC.HandleWebhookEvent(ctx.Request.Context(), event); err != nil {
		slog.ErrorContext(ctx.Request.Context(), "failed to process webhook event",
			slog.String("provider", provider),
			slog.String("event_type", event.EventType),
			slog.String("document_id", event.ProviderDocumentID),
			slog.String("error", err.Error()),
		)
		// Return 200 anyway to prevent retries for business logic errors
		ctx.JSON(http.StatusOK, gin.H{
			"status":  "error",
			"message": "event processing failed",
		})
		return false
	}
	return true
}
