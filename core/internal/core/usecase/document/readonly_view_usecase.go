package document

import (
	"context"
	"encoding/json"
	"time"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
)

// ReadonlyViewMode identifies how a read-only document view should be rendered.
type ReadonlyViewMode string

const (
	ReadonlyViewModeContent     ReadonlyViewMode = "content"
	ReadonlyViewModePDF         ReadonlyViewMode = "pdf"
	ReadonlyViewModeUnavailable ReadonlyViewMode = "unavailable"
)

// CreateReadonlyViewLinkResult contains the expiring public link details for a read-only view.
type CreateReadonlyViewLinkResult struct {
	URL       string
	Token     string
	ExpiresAt time.Time
}

// ReadonlyViewResponse contains the public read-only view state for a token.
type ReadonlyViewResponse struct {
	Mode           ReadonlyViewMode
	DocumentID     string
	DocumentTitle  string
	DocumentStatus entity.DocumentStatus
	ExpiresAt      time.Time
	Content        json.RawMessage
	PDFURL         *string
	Reason         *string
}

// ReadonlyViewUseCase defines the input port for expiring read-only document views.
type ReadonlyViewUseCase interface {
	CreateReadonlyViewLink(ctx context.Context, documentID string) (*CreateReadonlyViewLinkResult, error)
	GetReadonlyView(ctx context.Context, token string) (*ReadonlyViewResponse, error)
	GetReadonlyViewPDF(ctx context.Context, token string) ([]byte, string, error)
}
