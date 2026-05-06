package document

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

// DefaultTemplateResolver resolves template context by deterministic fallback.
type DefaultTemplateResolver struct{}

// NewDefaultTemplateResolver creates a new default resolver instance.
func NewDefaultTemplateResolver() port.TemplateResolver {
	return &DefaultTemplateResolver{}
}

// Resolve applies tenant/workspace/documentType resolution and returns a published template context.
// When Environment==dev, only the sandbox workspace is eligible. When Environment==prod,
// only the requested non-sandbox workspace is eligible.
func (r *DefaultTemplateResolver) Resolve(
	ctx context.Context,
	req *port.TemplateResolverRequest,
	adapter port.InternalTemplateContextSearchAdapter,
) (*port.InternalTemplateContext, error) {
	if req == nil {
		return nil, fmt.Errorf("template resolver request is nil")
	}

	return r.resolveWithFallback(ctx, req, adapter)
}

// resolveWithFallback builds the candidate workspace chain depending on environment.
func (r *DefaultTemplateResolver) resolveWithFallback(
	ctx context.Context,
	req *port.TemplateResolverRequest,
	adapter port.InternalTemplateContextSearchAdapter,
) (*port.InternalTemplateContext, error) {
	published := true
	process := req.Process
	if process == "" {
		process = entity.DefaultProcess
	}

	for _, step := range fallbackSteps(req) {
		resolved, err := adapter.ResolveInternalTemplateContext(ctx, port.InternalTemplateContextSearchParams{
			TenantCode:             step.tenantCode,
			RequestedWorkspaceCode: req.WorkspaceCode,
			WorkspaceCodes:         step.workspaceCodes,
			DocumentType:           req.DocumentType,
			Process:                process,
			Published:              &published,
		})
		if err != nil {
			return nil, fmt.Errorf("default template resolution failed at stage %s: %w", step.stage, err)
		}
		if resolved == nil {
			slog.DebugContext(ctx, "default template resolver stage miss",
				"stage", step.stage,
				"tenantCode", step.tenantCode,
				"workspaceCode", step.workspaceCodes[0],
				"documentType", req.DocumentType,
			)
			continue
		}

		slog.InfoContext(ctx, "default template resolver hit",
			"stage", step.stage,
			"tenantCode", step.tenantCode,
			"workspaceCode", step.workspaceCodes[0],
			"documentType", req.DocumentType,
			"templateVersionID", resolved.Version.ID,
		)
		return resolved, nil
	}

	return nil, entity.ErrInternalTemplateResolutionNotFound
}

type fallbackStep struct {
	tenantCode     string
	workspaceCodes []string
	stage          string
}

func fallbackSteps(req *port.TemplateResolverRequest) []fallbackStep {
	if req.Environment == entity.EnvironmentDev && req.SandboxWorkspaceCode != "" {
		return []fallbackStep{{
			tenantCode:     req.TenantCode,
			workspaceCodes: []string{req.SandboxWorkspaceCode},
			stage:          "tenant_sandbox_workspace",
		}}
	}
	if req.Environment == entity.EnvironmentDev {
		return nil
	}

	return []fallbackStep{
		{tenantCode: req.TenantCode, workspaceCodes: []string{req.WorkspaceCode}, stage: "tenant_workspace"},
	}
}
