package controller

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/rendis/doc-assembly/core/internal/adapters/primary/http/middleware"
	"github.com/rendis/doc-assembly/core/internal/core/entity"
	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
)

// SigningSessionController handles authenticated signing-session creation for
// embedded iframe flows.
type SigningSessionController struct {
	signingSessionUC documentuc.SigningSessionUseCase
}

// NewSigningSessionController creates a new signing session controller.
func NewSigningSessionController(signingSessionUC documentuc.SigningSessionUseCase) *SigningSessionController {
	return &SigningSessionController{signingSessionUC: signingSessionUC}
}

// RegisterRoutes registers signing session routes under /api/v1.
func (c *SigningSessionController) RegisterRoutes(router gin.IRouter, authMiddlewares ...gin.HandlerFunc) {
	group := router.Group("/signing-sessions")
	handlers := make([]gin.HandlerFunc, 0, len(authMiddlewares)+1)
	handlers = append(handlers, authMiddlewares...)
	handlers = append(handlers, c.CreateOrGetSession)
	group.POST("/:documentId", handlers...)
}

// CreateOrGetSession creates or reuses a tokenized signing session URL for the
// authenticated recipient.
// @Summary Create or get signing session
// @Description Returns a reusable /public/sign/{token} URL and signing page state for an authenticated recipient.
// @Tags Signing Sessions
// @Produce json
// @Param documentId path string true "Document ID"
// @Param language query string false "Public signing UI language (en or es)"
// @Success 200 {object} documentuc.SigningSessionResponse
// @Failure 401 {object} map[string]string
// @Failure 403 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 429 {object} map[string]string
// @Router /api/v1/signing-sessions/{documentId} [post]
func (c *SigningSessionController) CreateOrGetSession(ctx *gin.Context) {
	documentID := strings.TrimSpace(ctx.Param("documentId"))
	if documentID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "documentId is required"})
		return
	}

	claims, ok := middleware.GetSigningSessionAuthClaims(ctx)
	if !ok {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": entity.ErrUnauthorized.Error()})
		return
	}

	resp, err := c.signingSessionUC.CreateOrGetSession(ctx.Request.Context(), documentID, &documentuc.SigningSessionPrincipal{
		Email:    strings.TrimSpace(claims.Email),
		Subject:  strings.TrimSpace(claims.Subject),
		Provider: strings.TrimSpace(claims.Provider),
		Language: explicitPublicLanguage(ctx.Query("language")),
		Extra:    claims.Extra,
	})
	if err != nil {
		handleSigningSessionError(ctx, err)
		return
	}

	ctx.JSON(http.StatusOK, resp)
}

func handleSigningSessionError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, entity.ErrUnauthorized):
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	case errors.Is(err, entity.ErrForbidden):
		ctx.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	case errors.Is(err, entity.ErrTooManyRequests):
		ctx.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
		return
	case errors.Is(err, entity.ErrInvalidDocumentState):
		ctx.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	default:
		HandleError(ctx, err)
	}
}
