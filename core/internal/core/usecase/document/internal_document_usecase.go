package document

import (
	"context"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
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

// InternalResolveTemplateCommand asks which template version a document created now
// would use, without creating one.
type InternalResolveTemplateCommand struct {
	TenantCode    string             // From header X-Tenant-Code
	WorkspaceCode string             // From header X-Workspace-Code
	DocumentType  string             // From header X-Document-Type
	Process       string             // From header X-Process (default: "default")
	ProcessType   string             // From header X-Process-Type (default: "CANONICAL_NAME")
	Environment   entity.Environment // From header X-Environment
}

// InternalResolvedSignerRole is one signer the resolved version expects.
type InternalResolvedSignerRole struct {
	RoleName     string
	SignerOrder  int
	AnchorString string
}

// InternalResolvedInjectable is one value the resolved version fills in.
type InternalResolvedInjectable struct {
	Key        string
	Label      string
	IsRequired bool
}

// InternalResolveTemplateResult describes the version resolution landed on.
//
// RequestedWorkspaceCode and ResolvedWorkspaceCode are reported separately and left
// unclassified on purpose: a resolved workspace that differs from the requested one means
// the caller's own hierarchy took over (a network, a shared baseline), and only the caller
// knows what those names mean.
type InternalResolveTemplateResult struct {
	TenantCode             string
	RequestedWorkspaceCode string
	ResolvedWorkspaceCode  string
	DocumentType           string
	Process                string
	TemplateID             string
	VersionID              string
	VersionNumber          int
	VersionStatus          string
	UpdatedAt              *time.Time
	SignerRoles            []InternalResolvedSignerRole
	Injectables            []InternalResolvedInjectable
}

// InternalDocumentUseCase defines the input port for internal document operations.
// These operations are used for service-to-service communication.
type InternalDocumentUseCase interface {
	// CreateDocument creates or replays a document using the extension system (Mapper, Init, Injectors).
	CreateDocument(ctx context.Context, cmd InternalCreateCommand) (*InternalCreateResult, error)

	// ResetUnsignedDocument recreates a logical document with force-create semantics when the active document is unsigned.
	ResetUnsignedDocument(ctx context.Context, cmd InternalCreateCommand) (*InternalCreateResult, error)

	// ResolveTemplate reports which template version a create would use, without creating one.
	ResolveTemplate(ctx context.Context, cmd InternalResolveTemplateCommand) (*InternalResolveTemplateResult, error)
}
