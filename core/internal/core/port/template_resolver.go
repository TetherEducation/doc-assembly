package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
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

// TemplateResolver resolves the full internal template context for document creation.
// Return nil to let the engine continue with the default resolver fallback.
type TemplateResolver interface {
	Resolve(
		ctx context.Context,
		req *TemplateResolverRequest,
		adapter InternalTemplateContextSearchAdapter,
	) (*InternalTemplateContext, error)
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
