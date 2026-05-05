package document

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/rendis/doc-assembly/core/internal/core/port"
)

// TemplateVersionSearchAdapter provides read-only template version search for custom resolvers.
type TemplateVersionSearchAdapter struct {
	tenantRepo    port.TenantRepository
	workspaceRepo port.WorkspaceRepository
	docTypeRepo   port.DocumentTypeRepository
	templateRepo  port.TemplateRepository
	versionRepo   port.TemplateVersionRepository
}

// NewTemplateVersionSearchAdapter builds a new read-only search adapter.
func NewTemplateVersionSearchAdapter(
	tenantRepo port.TenantRepository,
	workspaceRepo port.WorkspaceRepository,
	docTypeRepo port.DocumentTypeRepository,
	templateRepo port.TemplateRepository,
	versionRepo port.TemplateVersionRepository,
) port.TemplateVersionSearchAdapter {
	return &TemplateVersionSearchAdapter{
		tenantRepo:    tenantRepo,
		workspaceRepo: workspaceRepo,
		docTypeRepo:   docTypeRepo,
		templateRepo:  templateRepo,
		versionRepo:   versionRepo,
	}
}

// SearchTemplateVersions returns deterministic candidates by tenant/workspace/document type.
func (a *TemplateVersionSearchAdapter) SearchTemplateVersions(ctx context.Context, params port.TemplateVersionSearchParams) ([]port.TemplateVersionSearchItem, error) {
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

	queryParams := params
	if queryParams.Published == nil {
		published := true
		queryParams.Published = &published
	}

	queryStartedAt := time.Now()
	candidates, err := a.templateRepo.FindResolutionCandidates(ctx, port.TemplateResolutionQuery(queryParams))
	slog.InfoContext(ctx, "template search timing",
		slog.String("stage", "aggregator_query"),
		slog.Int("workspace_count", len(params.WorkspaceCodes)),
		slog.Int("tag_filter_count", len(params.Tags)),
		slog.Duration("duration", time.Since(queryStartedAt)),
	)
	if err != nil {
		return nil, fmt.Errorf("finding template resolution candidates: %w", err)
	}

	results := make([]port.TemplateVersionSearchItem, 0, len(candidates))
	published := *queryParams.Published
	for _, candidate := range candidates {
		results = append(results, port.TemplateVersionSearchItem{
			Published:     published,
			TenantCode:    candidate.TenantCode,
			WorkspaceCode: candidate.WorkspaceCode,
			VersionID:     candidate.VersionID,
			Tags:          candidate.Tags,
		})
	}

	slog.InfoContext(ctx, "template search timing",
		slog.String("stage", "total"),
		slog.Int("workspace_count", len(params.WorkspaceCodes)),
		slog.Int("candidates", len(results)),
		slog.Duration("duration", time.Since(startedAt)),
	)
	return results, nil
}

// ResolveInternalTemplateContext returns the full internal-create template context in one read model query.
func (a *TemplateVersionSearchAdapter) ResolveInternalTemplateContext(
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

	result, err := a.templateRepo.FindInternalTemplateContext(ctx, port.InternalTemplateContextQuery(params))
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
