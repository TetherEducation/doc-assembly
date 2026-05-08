package dto

import (
	"encoding/json"
	"time"

	documentuc "github.com/rendis/doc-assembly/core/internal/core/usecase/document"
)

// CreateReadOnlyViewLinkResponse is returned when creating a public read-only view link.
type CreateReadOnlyViewLinkResponse struct {
	URL       string    `json:"url"`
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// NewCreateReadOnlyViewLinkResponse maps a use case result to an HTTP DTO.
func NewCreateReadOnlyViewLinkResponse(result *documentuc.CreateReadOnlyViewLinkResult) CreateReadOnlyViewLinkResponse {
	if result == nil {
		return CreateReadOnlyViewLinkResponse{}
	}
	return CreateReadOnlyViewLinkResponse{
		URL:       result.URL,
		Token:     result.Token,
		ExpiresAt: result.ExpiresAt,
	}
}

// ReadOnlyViewResponse describes a public read-only document view.
type ReadOnlyViewResponse struct {
	Mode           documentuc.ReadOnlyViewMode `json:"mode"`
	DocumentID     string                      `json:"documentId"`
	DocumentTitle  string                      `json:"documentTitle"`
	DocumentStatus string                      `json:"documentStatus"`
	ExpiresAt      time.Time                   `json:"expiresAt"`
	Content        json.RawMessage             `json:"content,omitempty" swaggertype:"object"`
	PDFURL         *string                     `json:"pdfUrl,omitempty"`
	Reason         *string                     `json:"reason,omitempty"`
}

// NewReadOnlyViewResponse maps a use case response to an HTTP DTO.
func NewReadOnlyViewResponse(resp *documentuc.ReadOnlyViewResponse) ReadOnlyViewResponse {
	if resp == nil {
		return ReadOnlyViewResponse{}
	}
	return ReadOnlyViewResponse{
		Mode:           resp.Mode,
		DocumentID:     resp.DocumentID,
		DocumentTitle:  resp.DocumentTitle,
		DocumentStatus: string(resp.DocumentStatus),
		ExpiresAt:      resp.ExpiresAt,
		Content:        resp.Content,
		PDFURL:         resp.PDFURL,
		Reason:         resp.Reason,
	}
}
