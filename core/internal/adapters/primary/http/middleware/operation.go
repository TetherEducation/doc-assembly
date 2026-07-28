package middleware

import (
	"log/slog"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/TetherEducation/doc-assembly/core/internal/infra/logging"
)

const (
	// OperationIDHeader is the header name for the operation ID.
	OperationIDHeader = "X-Operation-ID"
	// operationIDKey is the context key for the operation ID.
	operationIDKey = "operation_id"
)

// Operation creates a middleware that generates a unique operation ID for each request.
// The operation ID is used for request tracing and logging.
func Operation() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if operation ID is already provided in header
		operationID := c.GetHeader(OperationIDHeader)
		if operationID == "" {
			operationID = uuid.New().String()
		}

		// Store in Gin context
		c.Set(operationIDKey, operationID)

		// Add to response headers
		c.Header(OperationIDHeader, operationID)

		// Add log attributes to request context for automatic inclusion in all logs
		ctx := logging.WithAttrs(c.Request.Context(),
			slog.String("operation_id", operationID),
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.String("client_ip", c.ClientIP()),
		)
		c.Request = c.Request.WithContext(ctx)

		slog.InfoContext(ctx, "request started")

		c.Next()

		// Log request completion
		slog.InfoContext(c.Request.Context(), "request completed",
			slog.Int("status", c.Writer.Status()),
		)
	}
}

// GetOperationID retrieves the operation ID from the Gin context.
func GetOperationID(c *gin.Context) string {
	if val, exists := c.Get(operationIDKey); exists {
		if opID, ok := val.(string); ok {
			return opID
		}
	}
	return ""
}
