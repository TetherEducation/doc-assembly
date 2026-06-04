package bootstrap

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rendis/doc-assembly/core/internal/core/port"
)

type testLegacyDocumentHandler struct{}

func (testLegacyDocumentHandler) HandleLegacyDocument(
	context.Context,
	*port.LegacyDocumentRequest,
) (*port.LegacyDocumentResponse, error) {
	return &port.LegacyDocumentResponse{StatusCode: 200, Body: map[string]any{"ok": true}}, nil
}

func TestEngineLegacyDocumentHandler_SetAndGet(t *testing.T) {
	handler := &testLegacyDocumentHandler{}

	engine := New().SetLegacyDocumentHandler(handler)

	assert.Same(t, handler, engine.GetLegacyDocumentHandler())
}
