package injectable

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

const (
	// DefaultInjectorTimeout is the default timeout for injectors.
	DefaultInjectorTimeout = 30 * time.Second
)

// ResolveResult contains the results of injector resolution.
type ResolveResult struct {
	mu sync.Mutex

	// Values contains the resolved values (code -> value).
	Values map[string]entity.InjectableValue

	// Errors contains errors from non-critical injectors.
	Errors map[string]error

	// Metadata contains additional metadata per injector.
	Metadata map[string]map[string]any
}

// InjectableResolverService resolves injector values.
type InjectableResolverService struct {
	registry port.InjectorRegistry
}

// NewInjectableResolverService creates a new resolution service.
func NewInjectableResolverService(registry port.InjectorRegistry) *InjectableResolverService {
	return &InjectableResolverService{
		registry: registry,
	}
}

// Resolve resolves the values of the referenced injectors.
// Executes Init() GLOBAL first, then resolves injectors by dependency levels.
// Codes not found in the registry are delegated to the WorkspaceInjectableProvider if one is registered.
//
//nolint:funlen // Diagnostics require explicit stage-by-stage logging.
func (s *InjectableResolverService) Resolve(
	ctx context.Context,
	injCtx *entity.InjectorContext,
	referencedCodes []string,
) (*ResolveResult, error) {
	result := &ResolveResult{
		Values:   make(map[string]entity.InjectableValue),
		Errors:   make(map[string]error),
		Metadata: make(map[string]map[string]any),
	}

	if len(referencedCodes) == 0 {
		return result, nil
	}
	providerRegistered := s.registry.GetWorkspaceInjectableProvider() != nil
	slog.InfoContext(ctx, "starting injectable resolution",
		"referenced_count", len(referencedCodes),
		"provider_registered", providerRegistered,
		"tenant_code", injCtx.TenantCode(),
		"workspace_code", injCtx.WorkspaceCode(),
		"template_id", injCtx.TemplateID(),
	)
	slog.DebugContext(ctx, "injectable resolution requested codes", "referenced_codes", referencedCodes)

	// Partition codes into registry codes and provider codes
	registryCodes, providerCodes, skippedUnknown := s.partitionCodes(referencedCodes)
	slog.InfoContext(ctx, "injectable resolution partitioned codes",
		"registry_codes_count", len(registryCodes),
		"provider_codes_count", len(providerCodes),
		"skipped_unknown_count", len(skippedUnknown),
		"provider_registered", providerRegistered,
	)
	slog.DebugContext(ctx, "injectable resolution partition details",
		"registry_codes", registryCodes,
		"provider_codes", providerCodes,
		"skipped_unknown_codes", skippedUnknown,
	)
	if len(skippedUnknown) > 0 {
		slog.WarnContext(ctx, "injectable codes skipped because workspace provider is not registered",
			"skipped_unknown_codes", skippedUnknown,
		)
	}

	// 1. Execute Init() GLOBAL if defined
	initFunc := s.registry.GetInitFunc()
	if initFunc != nil {
		slog.DebugContext(ctx, "executing global init function")
		initData, err := initFunc(ctx, injCtx)
		if err != nil {
			return nil, fmt.Errorf("global init failed: %w", err)
		}
		injCtx.SetInitData(initData)
	}

	// 2. Resolve registry injectors via dependency graph
	if len(registryCodes) > 0 {
		if err := s.resolveRegistryCodes(ctx, injCtx, registryCodes, result); err != nil {
			return nil, err
		}
	}

	// 3. Resolve provider codes via WorkspaceInjectableProvider
	if len(providerCodes) > 0 {
		if err := s.resolveProviderCodes(ctx, injCtx, providerCodes, result); err != nil {
			return nil, err
		}
	}

	slog.InfoContext(ctx, "injectable resolution finished",
		"resolved_values_count", len(result.Values),
		"errors_count", len(result.Errors),
		"metadata_count", len(result.Metadata),
	)

	return result, nil
}

// partitionCodes separates codes into registry-known and provider-bound codes.
func (s *InjectableResolverService) partitionCodes(codes []string) (
	registryCodes, providerCodes, skippedUnknown []string,
) {
	provider := s.registry.GetWorkspaceInjectableProvider()

	for _, code := range codes {
		if _, ok := s.registry.Get(code); ok {
			registryCodes = append(registryCodes, code)
		} else if provider != nil {
			providerCodes = append(providerCodes, code)
		} else {
			skippedUnknown = append(skippedUnknown, code)
		}
	}

	return registryCodes, providerCodes, skippedUnknown
}

// resolveRegistryCodes resolves codes through the injector registry using the dependency graph.
func (s *InjectableResolverService) resolveRegistryCodes(
	ctx context.Context,
	injCtx *entity.InjectorContext,
	codes []string,
	result *ResolveResult,
) error {
	// Build dependency graph
	graph := NewDependencyGraph()
	err := graph.BuildFromInjectors(
		func(code string) ([]string, bool) {
			inj, ok := s.registry.Get(code)
			if !ok {
				return nil, false
			}
			_, deps := inj.Resolve()
			return deps, true
		},
		codes,
	)
	if err != nil {
		return fmt.Errorf("building dependency graph: %w", err)
	}

	// Get execution order (by levels)
	levels, err := graph.TopologicalSort()
	if err != nil {
		return fmt.Errorf("topological sort: %w", err)
	}

	// Execute injectors by levels
	for levelIdx, level := range levels {
		slog.DebugContext(ctx, "executing injector level",
			"level", levelIdx,
			"injectors", level,
		)

		if err := s.executeLevel(ctx, injCtx, level, result); err != nil {
			return err
		}
	}

	return nil
}

// resolveProviderCodes delegates resolution of provider-bound codes to the WorkspaceInjectableProvider.
//
//nolint:funlen // Structured provider diagnostics are intentionally explicit.
func (s *InjectableResolverService) resolveProviderCodes(
	ctx context.Context,
	injCtx *entity.InjectorContext,
	codes []string,
	result *ResolveResult,
) error {
	provider := s.registry.GetWorkspaceInjectableProvider()
	if provider == nil {
		return nil
	}

	req := &port.ResolveInjectablesRequest{
		TenantCode:      injCtx.TenantCode(),
		WorkspaceCode:   injCtx.WorkspaceCode(),
		TemplateID:      injCtx.TemplateID(),
		Codes:           codes,
		SelectedFormats: injCtx.GetSelectedFormats(),
		Headers:         injCtx.GetHeaders(),
		Payload:         injCtx.RequestPayload(),
		InitData:        injCtx.InitData(),
		Environment:     injCtx.Environment(),
	}
	slog.InfoContext(ctx, "resolving provider injectable codes",
		"tenant_code", req.TenantCode,
		"workspace_code", req.WorkspaceCode,
		"template_id", req.TemplateID,
		"environment", req.Environment,
		"codes", req.Codes,
	)

	providerResult, err := provider.ResolveInjectables(ctx, req)
	if err != nil {
		return fmt.Errorf("workspace injectable provider resolution failed: %w", err)
	}

	if providerResult == nil {
		slog.WarnContext(ctx, "provider returned nil resolve result",
			"tenant_code", req.TenantCode,
			"workspace_code", req.WorkspaceCode,
			"template_id", req.TemplateID,
			"requested_codes_count", len(codes),
		)
		return nil
	}

	// Merge provider values into result
	for code, val := range providerResult.Values {
		if val != nil {
			result.Values[code] = *val
			injCtx.SetResolved(code, val.AsAny())
		}
	}

	// Merge provider errors as non-critical
	for code, errMsg := range providerResult.Errors {
		result.Errors[code] = fmt.Errorf("%s", errMsg)
	}

	missingRequested := findMissingRequestedProviderCodes(codes, providerResult)
	slog.InfoContext(ctx, "provider injectable resolution completed",
		"tenant_code", req.TenantCode,
		"workspace_code", req.WorkspaceCode,
		"template_id", req.TemplateID,
		"requested_codes_count", len(codes),
		"provider_values_count", len(providerResult.Values),
		"provider_errors_count", len(providerResult.Errors),
		"requested_but_missing_count", len(missingRequested),
	)
	for _, missingCode := range missingRequested {
		slog.WarnContext(ctx, "provider did not return value or error for requested code",
			"code", missingCode,
			"tenant_code", req.TenantCode,
			"workspace_code", req.WorkspaceCode,
			"template_id", req.TemplateID,
		)
	}
	slog.DebugContext(ctx, "provider injectable resolution details",
		"requested_codes", codes,
		"provider_returned_codes", mapKeysInjectableValue(providerResult.Values),
		"provider_error_codes", mapKeysString(providerResult.Errors),
		"requested_but_missing_codes", missingRequested,
	)

	return nil
}

func findMissingRequestedProviderCodes(
	requested []string,
	result *port.ResolveInjectablesResult,
) []string {
	if len(requested) == 0 || result == nil {
		return nil
	}
	missing := make([]string, 0)
	for _, code := range requested {
		if _, ok := result.Values[code]; ok {
			continue
		}
		if _, ok := result.Errors[code]; ok {
			continue
		}
		missing = append(missing, code)
	}
	return missing
}

func mapKeysInjectableValue(input map[string]*entity.InjectableValue) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	return keys
}

func mapKeysString(input map[string]string) []string {
	keys := make([]string, 0, len(input))
	for key := range input {
		keys = append(keys, key)
	}
	return keys
}

// executeLevel executes all injectors in a level in parallel.
func (s *InjectableResolverService) executeLevel(
	ctx context.Context,
	injCtx *entity.InjectorContext,
	codes []string,
	result *ResolveResult,
) error {
	g, gCtx := errgroup.WithContext(ctx)

	for _, code := range codes {
		g.Go(func() error {
			return s.executeInjector(gCtx, injCtx, code, result)
		})
	}

	return g.Wait()
}

// executeInjector executes an individual injector.
func (s *InjectableResolverService) executeInjector(
	ctx context.Context,
	injCtx *entity.InjectorContext,
	code string,
	result *ResolveResult,
) error {
	inj, ok := s.registry.Get(code)
	if !ok {
		slog.WarnContext(ctx, "injector not found", "code", code)
		return nil
	}

	// Get the resolution function
	resolveFunc, _ := inj.Resolve()
	if resolveFunc == nil {
		slog.WarnContext(ctx, "injector has nil resolve function", "code", code)
		return nil
	}

	// Determine timeout
	timeout := inj.Timeout()
	if timeout <= 0 {
		timeout = DefaultInjectorTimeout
	}

	// Create context with timeout
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Execute injector
	slog.DebugContext(ctx, "executing injector", "code", code, "timeout", timeout)

	injResult, err := resolveFunc(timeoutCtx, injCtx)
	if err != nil {
		slog.ErrorContext(ctx, "injector failed",
			"code", code,
			"error", err,
			"critical", inj.IsCritical(),
		)

		if inj.IsCritical() {
			return fmt.Errorf("critical injector %q failed: %w", code, err)
		}

		// Non-critical error, save and continue
		result.mu.Lock()
		result.Errors[code] = err
		result.mu.Unlock()
		return nil
	}

	// Save result
	if injResult != nil {
		result.mu.Lock()
		result.Values[code] = injResult.Value
		if injResult.Metadata != nil {
			result.Metadata[code] = injResult.Metadata
		}
		result.mu.Unlock()

		injCtx.SetResolved(code, injResult.Value.AsAny())
	}

	slog.DebugContext(ctx, "injector completed", "code", code)
	return nil
}

// MergeWithPayloadValues combines injector values with values extracted from the payload.
// Payload values have priority (they overwrite injector values).
func (s *InjectableResolverService) MergeWithPayloadValues(
	resolved *ResolveResult,
	payloadValues map[string]entity.InjectableValue,
) map[string]any {
	merged := make(map[string]any)

	// First add injector values
	for code, value := range resolved.Values {
		merged[code] = value.AsAny()
	}

	// Then overwrite with payload values
	for key, value := range payloadValues {
		merged[key] = value.AsAny()
	}

	return merged
}
