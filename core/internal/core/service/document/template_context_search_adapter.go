package document

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/rendis/doc-assembly/core/internal/core/port"
)

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
