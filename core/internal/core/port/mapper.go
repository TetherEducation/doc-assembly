package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// MapperContext contains the context for request mapping.
// The system extracts this information from the request and provides it to the mapper.
type MapperContext struct {
	ExternalID        string               // external ID of the request
	TemplateID        string               // template ID to use
	TemplateVersionID string               // template version ID selected by resolver
	DocumentTypeID    string               // resolved document type ID
	TenantCode        string               // tenant business code
	WorkspaceCode     string               // workspace business code
	DocumentTypeCode  string               // document type business code
	TransactionalID   string               // transactional ID for traceability
	Operation         entity.OperationType // operation type
	Environment       entity.Environment   // dev or prod
	Headers           map[string]string    // HTTP request headers
	RawBody           []byte               // unparsed body
}

// RequestMapper defines the interface that users implement to map requests.
// The user only needs to parse RawBody and return the typed payload.
// If multiple document types are needed, the user handles routing internally.
// The system handles building InjectorContext from MapperContext + payload.
type RequestMapper interface {
	// Map parses the raw body and returns the business-specific payload.
	// The system handles building InjectorContext from MapperContext + payload.
	Map(ctx context.Context, mapCtx *MapperContext) (any, error)
}
