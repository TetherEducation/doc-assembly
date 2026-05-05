package port

import (
	"context"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
)

// TemplateResolverRequest is the context passed to custom template resolvers.
type TemplateResolverRequest struct {
	TenantCode           string
	WorkspaceCode        string
	SandboxWorkspaceCode string // populated when Environment==dev, empty for prod
	DocumentType         string
	Process              string
	ProcessType          string
	ExternalID           string
	TransactionalID      string
	ForceCreate          bool
	SupersedeReason      *string
	Headers              map[string]string
	RawBody              []byte
	Environment          entity.Environment
}

// TemplateResolver allows custom template version selection before default fallback.
type TemplateResolver interface {
	// Resolve returns:
	//   - non-nil version ID: use this version
	//   - nil version ID: use default resolver fallback
	//   - error: abort request
	Resolve(ctx context.Context, req *TemplateResolverRequest, adapter TemplateVersionSearchAdapter) (*string, error)
}

// InternalTemplateContextResolver can resolve the full internal template context.
// Implementers should return nil to let the standard resolver fallback run.
type InternalTemplateContextResolver interface {
	ResolveInternalTemplateContext(
		ctx context.Context,
		req *TemplateResolverRequest,
		adapter InternalTemplateContextSearchAdapter,
	) (*InternalTemplateContext, error)
}

// TemplateVersionSearchAdapter exposes read-only template version search for custom resolvers.
type TemplateVersionSearchAdapter interface {
	SearchTemplateVersions(ctx context.Context, params TemplateVersionSearchParams) ([]TemplateVersionSearchItem, error)
}

// InternalTemplateContextSearchAdapter exposes a full template context read model
// for resolvers that can provide the candidate workspace codes.
type InternalTemplateContextSearchAdapter interface {
	ResolveInternalTemplateContext(ctx context.Context, params InternalTemplateContextSearchParams) (*InternalTemplateContext, error)
}

// InternalTemplateContextSearchParams filters the full template context search.
type InternalTemplateContextSearchParams struct {
	TenantCode             string
	RequestedWorkspaceCode string
	WorkspaceCodes         []string
	DocumentType           string
	Process                string
	Tags                   []string
	Published              *bool
}

// TemplateVersionSearchParams filters the read-only search.
type TemplateVersionSearchParams struct {
	TenantCode     string
	WorkspaceCodes []string
	DocumentType   string
	Process        string
	Tags           []string
	Published      *bool
}

// TemplateVersionSearchItem is one candidate returned by SearchTemplateVersions.
type TemplateVersionSearchItem struct {
	Published     bool
	TenantCode    string
	WorkspaceCode string
	VersionID     string
	Tags          []string
}
