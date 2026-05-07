package document

import (
	"context"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
)

// InternalCreateCommand contains the data for creating a document via internal API.
type InternalCreateCommand struct {
	TenantCode      string             // From header X-Tenant-Code
	WorkspaceCode   string             // From header X-Workspace-Code
	DocumentType    string             // From header X-Document-Type
	Process         string             // From header X-Process (default: "default")
	ProcessType     string             // From header X-Process-Type (default: "CANONICAL_NAME")
	ExternalID      string             // From header X-External-ID
	TransactionalID string             // From header X-Transactional-ID
	Environment     entity.Environment // From header X-Environment
	ForceCreate     bool               // Optional body field. Defaults to false.
	SupersedeReason *string            // Optional body field.
	Metadata        map[string]string  // Optional body field. Round-trip metadata returned in completion event.
	Headers         map[string]string  // All HTTP headers
	PayloadRaw      []byte             // Unparsed payload object (passed to Mapper)
}

// InternalCreateResult contains the result of an internal create request.
type InternalCreateResult struct {
	Document                     *entity.DocumentWithRecipients
	IdempotentReplay             bool
	SupersededPreviousDocumentID *string
}

// InternalDocumentUseCase defines the input port for internal document operations.
// These operations are used for service-to-service communication.
type InternalDocumentUseCase interface {
	// CreateDocument creates or replays a document using the extension system (Mapper, Init, Injectors).
	CreateDocument(ctx context.Context, cmd InternalCreateCommand) (*InternalCreateResult, error)

	// ResetUnsignedDocument recreates a logical document with force-create semantics when the active document is unsigned.
	ResetUnsignedDocument(ctx context.Context, cmd InternalCreateCommand) (*InternalCreateResult, error)
}
