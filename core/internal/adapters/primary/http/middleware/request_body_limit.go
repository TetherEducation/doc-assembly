package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequestBodyLimit caps how much of a request body downstream code can read. Reads past
// maxBytes fail with *http.MaxBytesError, which body consumers map to 413 Request Entity
// Too Large (see AutomationAuditLogger). A non-positive limit disables the cap.
func RequestBodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maxBytes > 0 && c.Request.Body != nil && c.Request.Body != http.NoBody {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}
