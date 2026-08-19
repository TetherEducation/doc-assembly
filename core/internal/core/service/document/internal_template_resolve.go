package document

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
)

// ResolveTemplate answers "which template version would a document created right now use?"
// without creating anything.
//
// It deliberately goes through resolveTemplateContextWithResolvers — the same call the
// create path makes — so the configured custom resolver runs with its own fallback order
// (for Tether that is campus -> network -> DEFAULT, implemented in tools-doc-assembly).
// Reimplementing that chain here would produce an answer that drifts from the one families
// actually receive, and that is the only answer worth reporting.
func (s *InternalDocumentService) ResolveTemplate(
	ctx context.Context,
	cmd documentuc.InternalResolveTemplateCommand,
) (*documentuc.InternalResolveTemplateResult, error) {
	tenantCode := strings.ToUpper(strings.TrimSpace(cmd.TenantCode))
	workspaceCode := strings.ToUpper(strings.TrimSpace(cmd.WorkspaceCode))
	documentTypeCode := strings.ToUpper(strings.TrimSpace(cmd.DocumentType))

	if tenantCode == "" || workspaceCode == "" || documentTypeCode == "" {
		return nil, fmt.Errorf("resolve template: tenantCode, workspaceCode and documentType are required")
	}

	process := strings.ToUpper(strings.TrimSpace(cmd.Process))
	if process == "" {
		process = entity.DefaultProcess
	}
	processType := cmd.ProcessType
	if processType == "" {
		processType = string(entity.DefaultProcessType)
	}

	resolverReq := &port.TemplateResolverRequest{
		TenantCode:    tenantCode,
		WorkspaceCode: workspaceCode,
		DocumentType:  documentTypeCode,
		Process:       process,
		ProcessType:   processType,
		Environment:   cmd.Environment,
	}
	// Sandbox rewriting is part of resolution in dev, so it has to happen here too, or a
	// dev answer would name a workspace the create path would never have used.
	if err := s.applySandboxWorkspaceCode(ctx, resolverReq); err != nil {
		return nil, err
	}

	resolutionCtx := withTemplateResolutionEnvironment(ctx, resolverReq.Environment)
	resolved, err := s.resolveTemplateContextWithResolvers(resolutionCtx, resolverReq)
	if err != nil {
		return nil, err
	}
	if resolved == nil || resolved.Version == nil || resolved.Template == nil {
		return nil, entity.ErrInternalTemplateResolutionNotFound
	}

	slog.InfoContext(ctx, "internal template resolve",
		slog.String("requested_workspace", resolverReq.WorkspaceCode),
		slog.String("resolved_workspace", resolvedWorkspaceCodeOf(resolved)),
		slog.String("document_type", documentTypeCode),
		slog.String("version_id", resolved.Version.ID),
	)

	return buildResolveTemplateResult(resolverReq, resolved), nil
}

func resolvedWorkspaceCodeOf(resolved *port.InternalTemplateContext) string {
	if resolved == nil || resolved.Workspace == nil {
		return ""
	}
	return resolved.Workspace.Code
}

// buildResolveTemplateResult projects the resolved context into the read model.
//
// It reports the requested and resolved workspace codes but does not classify the
// relationship between them. Whether a resolved workspace is "the campus's own", "its
// network's" or "the shared baseline" is tenant vocabulary the engine does not model;
// the caller owns that judgement, and encoding it here would bake one tenant's hierarchy
// into core.
func buildResolveTemplateResult(
	req *port.TemplateResolverRequest,
	resolved *port.InternalTemplateContext,
) *documentuc.InternalResolveTemplateResult {
	out := &documentuc.InternalResolveTemplateResult{
		TenantCode:             req.TenantCode,
		RequestedWorkspaceCode: req.WorkspaceCode,
		ResolvedWorkspaceCode:  resolvedWorkspaceCodeOf(resolved),
		DocumentType:           req.DocumentType,
		Process:                resolved.Template.Process,
		TemplateID:             resolved.Template.ID,
		VersionID:              resolved.Version.ID,
		VersionNumber:          resolved.Version.VersionNumber,
		VersionStatus:          string(resolved.Version.Status),
		UpdatedAt:              resolved.Version.UpdatedAt,
	}
	if resolved.DocumentType != nil {
		out.DocumentType = resolved.DocumentType.Code
	}

	for _, role := range resolved.Version.SignerRoles {
		if role == nil {
			continue
		}
		out.SignerRoles = append(out.SignerRoles, documentuc.InternalResolvedSignerRole{
			RoleName:     role.RoleName,
			SignerOrder:  role.SignerOrder,
			AnchorString: role.AnchorString,
		})
	}

	for _, injectable := range resolved.Version.Injectables {
		key, label := injectableIdentity(injectable)
		if key == "" {
			continue
		}
		out.Injectables = append(out.Injectables, documentuc.InternalResolvedInjectable{
			Key:        key,
			Label:      label,
			IsRequired: injectable.IsRequired,
		})
	}

	return out
}

// injectableIdentity resolves an injectable's key and label from whichever of the two
// shapes it carries: workspace injectables hang off a definition, system injectables are
// named inline by key and have no definition row to take a label from.
func injectableIdentity(injectable *entity.VersionInjectableWithDefinition) (key, label string) {
	if injectable == nil {
		return "", ""
	}
	if injectable.Definition != nil && strings.TrimSpace(injectable.Definition.Key) != "" {
		return injectable.Definition.Key, injectable.Definition.Label
	}
	if injectable.SystemInjectableKey != nil {
		return *injectable.SystemInjectableKey, ""
	}
	return "", ""
}
