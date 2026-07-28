package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// LegacyDocumentRequest contains the minimum doc-assembly context plus the
// raw request data a host application needs to resolve a legacy document.
type LegacyDocumentRequest struct {
	WorkspaceCode string
	Environment   entity.Environment
	Headers       map[string][]string
	RawBody       []byte
}

// LegacyDocumentResponse is a host-provided JSON response for a legacy
// document request. Body is serialized as JSON by doc-assembly.
type LegacyDocumentResponse struct {
	StatusCode int
	Headers    map[string][]string
	Body       any
}

// LegacyDocumentHandler resolves requests for documents outside the current
// doc-assembly document lifecycle.
type LegacyDocumentHandler interface {
	HandleLegacyDocument(ctx context.Context, req *LegacyDocumentRequest) (*LegacyDocumentResponse, error)
}
