package document

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

type templateResolutionEnvironmentKey struct{}

func withTemplateResolutionEnvironment(ctx context.Context, env entity.Environment) context.Context {
	if env == "" {
		env = entity.EnvironmentProd
	}
	return context.WithValue(ctx, templateResolutionEnvironmentKey{}, env)
}

func templateResolutionEnvironmentFromContext(ctx context.Context) entity.Environment {
	env, ok := ctx.Value(templateResolutionEnvironmentKey{}).(entity.Environment)
	if !ok || env == "" {
		return entity.EnvironmentProd
	}
	return env
}

// TemplateContextSearchAdapter provides read-only template context search for custom resolvers.
type TemplateContextSearchAdapter struct {
	tenantRepo    port.TenantRepository
	workspaceRepo port.WorkspaceRepository
	docTypeRepo   port.DocumentTypeRepository
	templateRepo  port.TemplateRepository
	versionRepo   port.TemplateVersionRepository
}

// NewTemplateContextSearchAdapter builds a new read-only search adapter.
func NewTemplateContextSearchAdapter(
	tenantRepo port.TenantRepository,
	workspaceRepo port.WorkspaceRepository,
	docTypeRepo port.DocumentTypeRepository,
	templateRepo port.TemplateRepository,
	versionRepo port.TemplateVersionRepository,
) port.InternalTemplateContextSearchAdapter {
	return &TemplateContextSearchAdapter{
		tenantRepo:    tenantRepo,
		workspaceRepo: workspaceRepo,
		docTypeRepo:   docTypeRepo,
		templateRepo:  templateRepo,
		versionRepo:   versionRepo,
	}
}

// ResolveInternalTemplateContext returns the full internal-create template context in one read model query.
func (a *TemplateContextSearchAdapter) ResolveInternalTemplateContext(
	ctx context.Context,
	params port.InternalTemplateContextSearchParams,
) (*port.InternalTemplateContext, error) {
	startedAt := time.Now()
	if params.TenantCode == "" {
		return nil, fmt.Errorf("tenantCode is required")
	}
	if len(params.WorkspaceCodes) == 0 {
		return nil, fmt.Errorf("workspaceCodes is required")
	}
	if params.DocumentType == "" {
		return nil, fmt.Errorf("documentType is required")
	}

	query := port.InternalTemplateContextQuery{
		TenantCode:             params.TenantCode,
		RequestedWorkspaceCode: params.RequestedWorkspaceCode,
		WorkspaceCodes:         params.WorkspaceCodes,
		DocumentType:           params.DocumentType,
		Process:                params.Process,
		Tags:                   params.Tags,
		Published:              params.Published,
		Environment:            templateResolutionEnvironmentFromContext(ctx),
	}
	result, err := a.templateRepo.FindInternalTemplateContext(ctx, query)
	slog.InfoContext(ctx, "template context aggregator timing",
		slog.Int("workspace_count", len(params.WorkspaceCodes)),
		slog.Int("tag_filter_count", len(params.Tags)),
		slog.Duration("duration", time.Since(startedAt)),
	)
	if err != nil {
		return nil, fmt.Errorf("finding internal template context: %w", err)
	}
	return result, nil
}
