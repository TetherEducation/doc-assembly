package port

import (
	"context"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
)

// WorkspaceInjectableProvider defines the interface for dynamic workspace-specific injectables.
// Implementations are provided by users to supply custom injectables per workspace.
// The provider handles all i18n internally - returned labels and descriptions should be pre-translated.
type WorkspaceInjectableProvider interface {
	// GetInjectables returns available injectables for a workspace.
	// Called when editor opens to populate the injectable list.
	// Provider is responsible for i18n - return labels/descriptions already translated for the requested locale.
	// Use injCtx.TenantCode() and injCtx.WorkspaceCode() to identify the workspace.
	GetInjectables(ctx context.Context, injCtx *entity.InjectorContext) (*GetInjectablesResult, error)

	// ResolveInjectables resolves a batch of injectable codes.
	// Called during render for workspace-specific injectables.
	//
	// Error handling:
	//   - Return (nil, error) for CRITICAL failures that should stop the render.
	//   - Return (result, nil) with result.Errors[code] for NON-CRITICAL failures (render continues).
	ResolveInjectables(ctx context.Context, req *ResolveInjectablesRequest) (*ResolveInjectablesResult, error)
}

// FilteredWorkspaceInjectableProvider can be optionally implemented by providers that can
// list only requested injectable codes. Providers that don't implement it keep using
// WorkspaceInjectableProvider.GetInjectables.
type FilteredWorkspaceInjectableProvider interface {
	GetInjectablesByCodes(ctx context.Context, injCtx *entity.InjectorContext, codes []string) (*GetInjectablesResult, error)
}

// ─────────────────────────────────────────────────────────────────────────────
// GET INJECTABLES (listing for editor)
// ─────────────────────────────────────────────────────────────────────────────

// GetInjectablesResult contains the list of available injectables and groups.
type GetInjectablesResult struct {
	// Injectables is the list of available injectables for the workspace.
	Injectables []ProviderInjectable

	// Groups contains custom groups defined by the provider (optional).
	// These are merged with YAML-defined groups. Provider groups appear at the end.
	Groups []ProviderGroup
}

// ProviderInjectable represents an injectable definition from the provider.
type ProviderInjectable struct {
	// Code is the unique identifier for this injectable.
	// REQUIRED. Must not collide with registry-defined injector codes.
	Code string `json:"code"`

	// Label is the display name shown in the editor.
	// REQUIRED. Map of locale -> translated label (e.g., {"es": "Nombre", "en": "Name"}).
	Label map[string]string `json:"label"`

	// Description is optional help text shown in the editor.
	// Map of locale -> translated description.
	Description map[string]string `json:"description,omitempty"`

	// DataType indicates the type of value this injectable produces.
	// REQUIRED. Use InjectableDataType constants.
	DataType entity.InjectableDataType `json:"dataType"`

	// GroupKey is the key of the group to assign this injectable to (optional).
	GroupKey string `json:"groupKey,omitempty"`

	// Formats defines available format options for this injectable (optional).
	Formats []ProviderFormat `json:"formats,omitempty"`
}

// ProviderFormat represents a format option for an injectable.
type ProviderFormat struct {
	// Key is the format identifier (e.g., "DD/MM/YYYY").
	Key string `json:"key"`

	// Label is the display label shown in the format selector.
	// Map of locale -> translated label.
	Label map[string]string `json:"label"`
}

// ProviderGroup represents a custom group for organizing injectables.
type ProviderGroup struct {
	// Key is the unique group identifier.
	Key string `json:"key"`

	// Name is the display name shown in the editor.
	// Map of locale -> translated name.
	Name map[string]string `json:"name"`

	// Icon is the optional icon name (e.g., "calendar", "user", "database").
	Icon string `json:"icon,omitempty"`
}

// ─────────────────────────────────────────────────────────────────────────────
// RESOLVE INJECTABLES (resolution during render)
// ─────────────────────────────────────────────────────────────────────────────

// ResolveInjectablesRequest contains parameters for resolving injectable values.
type ResolveInjectablesRequest struct {
	// TenantCode is the tenant identifier.
	TenantCode string

	// WorkspaceCode is the workspace identifier.
	WorkspaceCode string

	// TemplateID is the ID of the template being rendered.
	TemplateID string

	// Codes is the list of injectable codes to resolve.
	Codes []string

	// SelectedFormats maps injectable codes to their selected format keys.
	SelectedFormats map[string]string

	// Headers contains HTTP headers from the original request.
	Headers map[string]string

	// Payload contains the request body data.
	Payload any

	// InitData contains shared initialization data from InitFunc.
	InitData any

	// Environment indicates dev or prod.
	Environment entity.Environment
}

// ResolveInjectablesResult contains the resolved values and any non-critical errors.
type ResolveInjectablesResult struct {
	// Values maps injectable codes to their resolved values.
	Values map[string]*entity.InjectableValue

	// Errors maps injectable codes to error messages for non-critical failures.
	Errors map[string]string
}
