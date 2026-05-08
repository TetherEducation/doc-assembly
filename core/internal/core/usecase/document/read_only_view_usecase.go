package document

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
)

// ReadOnlyViewMode identifies how a read-only document view should be rendered.
type ReadOnlyViewMode string

const (
	ReadOnlyViewModeContent     ReadOnlyViewMode = "content"
	ReadOnlyViewModePDF         ReadOnlyViewMode = "pdf"
	ReadOnlyViewModeUnavailable ReadOnlyViewMode = "unavailable"
)

// CreateReadOnlyViewLinkResult contains the expiring public link details for a read-only view.
type CreateReadOnlyViewLinkResult struct {
	URL       string
	Token     string
	ExpiresAt time.Time
}

// ReadOnlyViewResponse contains the public read-only view state for a token.
type ReadOnlyViewResponse struct {
	Mode           ReadOnlyViewMode
	DocumentID     string
	DocumentTitle  string
	DocumentStatus entity.DocumentStatus
	ExpiresAt      time.Time
	Content        json.RawMessage
	PDFURL         *string
	Reason         *string
}

// ReadOnlyViewUseCase defines the input port for expiring read-only document views.
type ReadOnlyViewUseCase interface {
	CreateReadOnlyViewLink(ctx context.Context, workspaceID, documentID string) (*CreateReadOnlyViewLinkResult, error)
	GetReadOnlyView(ctx context.Context, token string) (*ReadOnlyViewResponse, error)
	GetReadOnlyViewPDF(ctx context.Context, token string) ([]byte, string, error)
}
