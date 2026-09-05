package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	"github.com/gin-gonic/gin"
)

const (
	// bodyCaptureLimitBytes caps the request body persisted with an audit entry. It does not
	// limit what handlers read: the middleware always hands the complete body downstream.
	bodyCaptureLimitBytes = 64 * 1024 // 64 KB
	auditWriteTimeout     = 3 * time.Second
)

// AutomationAuditLogger is a Gin middleware that asynchronously records all automation API
// calls to the audit log table. It must run AFTER AutomationKeyAuth so that the key ID
// is available in the context.
func AutomationAuditLogger(auditRepo port.AutomationAuditLogRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Buffer the whole request body for the handler; the audit entry keeps at most
		// bodyCaptureLimitBytes of it.
		requestBody := captureRequestBody(c)

		// Wrap ResponseWriter to capture status code
		rw := &responseStatusWriter{ResponseWriter: c.Writer, status: http.StatusOK}
		c.Writer = rw

		c.Next()

		// Read context values after handler ran
		keyID, _ := GetAutomationKeyID(c)
		keyPrefix, _ := GetAutomationKeyPrefix(c)
		if keyID == "" {
			return // no key in context, nothing to audit
		}

		// Extract optional tenant/workspace from path params
		tenantID := extractOptionalParam(c, "tenantId")
		workspaceID := extractOptionalParam(c, "workspaceId")

		// Infer resource type and action from method + path
		resourceType, resourceID, action := inferResourceAction(c.Request.Method, c.FullPath(), c.Params)

		logEntry := &entity.AutomationAuditLog{
			APIKeyID:       keyID,
			APIKeyPrefix:   keyPrefix,
			Method:         c.Request.Method,
			Path:           c.Request.URL.Path,
			TenantID:       tenantID,
			WorkspaceID:    workspaceID,
			ResourceType:   resourceType,
			ResourceID:     resourceID,
			Action:         action,
			RequestBody:    requestBody,
			ResponseStatus: rw.status,
		}

		// Write asynchronously — do not block the response
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), auditWriteTimeout)
			defer cancel()
			_ = auditRepo.Create(ctx, logEntry)
		}()
	}
}

// captureRequestBody buffers the whole request body so downstream handlers see every byte,
// and returns what the audit entry should persist: the body itself when it fits within
// bodyCaptureLimitBytes, otherwise a small marker object. A truncated prefix is never stored:
// it is not valid JSON, and the JSONB column would reject the whole audit row.
func captureRequestBody(c *gin.Context) json.RawMessage {
	if c.Request.Body == nil || c.Request.Body == http.NoBody {
		return nil
	}
	raw, err := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewReader(raw))
	switch {
	case err != nil || len(raw) == 0:
		return nil
	case len(raw) > bodyCaptureLimitBytes:
		return truncatedBodyMarker(len(raw))
	default:
		return raw
	}
}

// truncatedBodyMarker is stored in place of bodies larger than bodyCaptureLimitBytes.
func truncatedBodyMarker(originalBytes int) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(
		`{"truncated":true,"originalBytes":%d,"limitBytes":%d}`, originalBytes, bodyCaptureLimitBytes))
}

// responseStatusWriter wraps gin.ResponseWriter to capture the HTTP status code.
type responseStatusWriter struct {
	gin.ResponseWriter
	status int
}

func (w *responseStatusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseStatusWriter) WriteHeaderNow() {
	w.ResponseWriter.WriteHeaderNow()
	w.status = w.Status()
}

// extractOptionalParam returns a pointer to the path param value, or nil if not present.
func extractOptionalParam(c *gin.Context, paramName string) *string {
	v := c.Param(paramName)
	if v == "" {
		return nil
	}
	return &v
}

// inferResourceAction infers resource type, resource ID, and action from the HTTP method and path.
func inferResourceAction(method, path string, params gin.Params) (resourceType, resourceID *string, action *string) {
	lower := strings.ToLower(path)
	act := inferAction(method, lower)
	rt := inferResourceType(lower)

	if rt != "" {
		resourceType = &rt
	}
	action = &act
	resourceID = inferResourceID(params)
	return
}

// inferAction maps HTTP method and path suffix to an audit action string.
func inferAction(method, lowerPath string) string {
	var act string
	switch method {
	case http.MethodPost:
		act = "CREATE"
	case http.MethodPut, http.MethodPatch:
		act = "UPDATE"
	case http.MethodDelete:
		act = "DELETE"
	case http.MethodGet:
		act = "READ"
	default:
		act = method
	}

	switch {
	case strings.HasSuffix(lowerPath, "/publish"):
		act = "PUBLISH"
	case strings.HasSuffix(lowerPath, "/archive"):
		act = "ARCHIVE"
	case strings.HasSuffix(lowerPath, "/content") && method == http.MethodPut:
		act = "UPDATE_CONTENT"
	}
	return act
}

// inferResourceType determines the resource type from the path.
func inferResourceType(lowerPath string) string {
	switch {
	case strings.Contains(lowerPath, "/versions"):
		return "VERSION"
	case strings.Contains(lowerPath, "/templates"):
		return "TEMPLATE"
	case strings.Contains(lowerPath, "/workspaces"):
		return "WORKSPACE"
	case strings.Contains(lowerPath, "/tenants"):
		return "TENANT"
	case strings.Contains(lowerPath, "/injectables"):
		return "INJECTABLE"
	case strings.Contains(lowerPath, "/document-types"):
		return "DOCUMENT_TYPE"
	default:
		return ""
	}
}

// inferResourceID extracts the most specific resource ID from path params.
// Priority: versionId > templateId > workspaceId > tenantId
func inferResourceID(params gin.Params) *string {
	for _, paramName := range []string{"versionId", "templateId", "workspaceId", "tenantId"} {
		for _, p := range params {
			if p.Key == paramName && p.Value != "" {
				v := p.Value
				return &v
			}
		}
	}
	return nil
}
