package injectable

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	injectableuc "github.com/rendis/doc-assembly/core/internal/core/usecase/injectable"
)

// Ensure InjectableService implements InjectableUseCase.
var _ injectableuc.InjectableUseCase = (*InjectableService)(nil)

// NewInjectableService creates a new injectable service.
func NewInjectableService(
	injectableRepo port.InjectableRepository,
	systemInjectableRepo port.SystemInjectableRepository,
	injectorRegistry port.InjectorRegistry,
	workspaceRepo port.WorkspaceRepository,
	tenantRepo port.TenantRepository,
	workspaceProvider port.WorkspaceInjectableProvider, // can be nil
) injectableuc.InjectableUseCase {
	return &InjectableService{
		injectableRepo:       injectableRepo,
		systemInjectableRepo: systemInjectableRepo,
		injectorRegistry:     injectorRegistry,
		workspaceRepo:        workspaceRepo,
		tenantRepo:           tenantRepo,
		workspaceProvider:    workspaceProvider,
	}
}

// InjectableService implements injectable definition business logic.
// Note: Injectables are read-only - they are managed via database migrations/seeds.
type InjectableService struct {
	injectableRepo       port.InjectableRepository
	systemInjectableRepo port.SystemInjectableRepository
	injectorRegistry     port.InjectorRegistry
	workspaceRepo        port.WorkspaceRepository
	tenantRepo           port.TenantRepository
	workspaceProvider    port.WorkspaceInjectableProvider // can be nil
}

// GetInjectable retrieves an injectable definition by ID.
func (s *InjectableService) GetInjectable(ctx context.Context, id string) (*entity.InjectableDefinition, error) {
	injectable, err := s.injectableRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding injectable %s: %w", id, err)
	}
	return injectable, nil
}

// ListInjectables lists all injectable definitions for a workspace (including global, system, and provider).
func (s *InjectableService) ListInjectables(
	ctx context.Context, req *injectableuc.ListInjectablesRequest,
) (*injectableuc.ListInjectablesResult, error) {
	requestedCodes := uniqueNonEmptyCodes(req.Codes)
	dbInjectables, activeSystemKeys, err := s.findBaseInjectables(ctx, req.WorkspaceID, requestedCodes)
	if err != nil {
		return nil, fmt.Errorf("listing injectables: %w", err)
	}

	systemInjectables, err := s.getSystemInjectables(ctx, req.WorkspaceID, requestedCodes, activeSystemKeys)
	if err != nil {
		return nil, fmt.Errorf("listing system injectables: %w", err)
	}

	// Get provider injectables if provider is registered
	providerInjectables, providerGroups, err := s.getProviderInjectablesAndGroups(
		ctx,
		req.WorkspaceID,
		req.Environment,
		requestedCodes,
		dbInjectables,
		systemInjectables,
	)
	if err != nil {
		return nil, err
	}

	// Merge all injectables
	allInjectables := s.mergeInjectables(dbInjectables, systemInjectables)
	allInjectables = append(allInjectables, providerInjectables...)

	// Merge groups (registry groups + provider groups)
	var registryGroups []port.GroupConfig
	if s.injectorRegistry != nil {
		registryGroups = s.injectorRegistry.GetAllGroups()
	}
	allGroups := make([]port.GroupConfig, 0, len(registryGroups)+len(providerGroups))
	allGroups = append(allGroups, registryGroups...)
	allGroups = append(allGroups, providerGroups...)

	return &injectableuc.ListInjectablesResult{
		Injectables: allInjectables,
		Groups:      allGroups,
	}, nil
}

// CompleteGenerationInjectables hydrates preloaded DB/system injectables and only calls
// the optional workspace provider when one is registered. It avoids the DB listing query
// on hot generation paths that already loaded the read model.
func (s *InjectableService) CompleteGenerationInjectables(
	ctx context.Context,
	req *injectableuc.CompleteGenerationInjectablesRequest,
) (*injectableuc.ListInjectablesResult, error) {
	requestedCodes := uniqueNonEmptyCodes(req.Codes)
	systemInjectables, err := s.getSystemInjectables(ctx, req.WorkspaceID, requestedCodes, req.ActiveSystemKeys)
	if err != nil {
		return nil, fmt.Errorf("listing system injectables: %w", err)
	}

	providerInjectables, providerGroups, err := s.getProviderInjectablesAndGroups(
		ctx,
		req.WorkspaceID,
		req.Environment,
		requestedCodes,
		req.DBInjectables,
		systemInjectables,
	)
	if err != nil {
		return nil, err
	}

	allInjectables := s.mergeInjectables(req.DBInjectables, systemInjectables)
	allInjectables = append(allInjectables, providerInjectables...)

	var registryGroups []port.GroupConfig
	if s.injectorRegistry != nil {
		registryGroups = s.injectorRegistry.GetAllGroups()
	}
	allGroups := make([]port.GroupConfig, 0, len(registryGroups)+len(providerGroups))
	allGroups = append(allGroups, registryGroups...)
	allGroups = append(allGroups, providerGroups...)

	return &injectableuc.ListInjectablesResult{
		Injectables: allInjectables,
		Groups:      allGroups,
	}, nil
}

func (s *InjectableService) findBaseInjectables(
	ctx context.Context,
	workspaceID string,
	codes []string,
) ([]*entity.InjectableDefinition, []string, error) {
	if len(codes) > 0 {
		return s.injectableRepo.FindGenerationAvailableByWorkspaceAndKeys(ctx, workspaceID, codes)
	}
	dbInjectables, err := s.injectableRepo.FindByWorkspace(ctx, workspaceID)
	return dbInjectables, nil, err
}

// getProviderInjectablesAndGroups fetches and validates provider injectables and groups.
func (s *InjectableService) getProviderInjectablesAndGroups(
	ctx context.Context,
	workspaceID string,
	env entity.Environment,
	codes []string,
	dbInjectables, systemInjectables []*entity.InjectableDefinition,
) ([]*entity.InjectableDefinition, []port.GroupConfig, error) {
	if s.workspaceProvider == nil {
		return nil, nil, nil
	}

	providerResult, err := s.fetchProviderResult(ctx, workspaceID, env, codes)
	if err != nil {
		return nil, nil, fmt.Errorf("getting provider injectables: %w", err)
	}

	if providerResult == nil {
		return nil, nil, nil
	}

	providerInjectables := s.convertProviderInjectables(providerResult.Injectables)
	providerGroups := convertProviderGroups(providerResult.Groups)

	if err := validateNoDuplicateCodes(dbInjectables, systemInjectables, providerInjectables); err != nil {
		return nil, nil, err
	}

	return providerInjectables, providerGroups, nil
}

// getSystemInjectables returns system injectables filtered by active assignments for the workspace.
func (s *InjectableService) getSystemInjectables(
	ctx context.Context, workspaceID string, codes []string, activeKeys []string,
) ([]*entity.InjectableDefinition, error) {
	if s.injectorRegistry == nil || s.systemInjectableRepo == nil {
		return nil, nil
	}

	if len(codes) == 0 {
		var err error
		activeKeys, err = s.systemInjectableRepo.FindActiveKeysForWorkspace(ctx, workspaceID)
		if err != nil {
			return nil, err
		}
	}

	activeKeySet := make(map[string]bool, len(activeKeys))
	for _, key := range activeKeys {
		activeKeySet[key] = true
	}

	injectors := s.injectorRegistry.GetAll()
	result := make([]*entity.InjectableDefinition, 0, len(activeKeys))
	for _, inj := range injectors {
		if activeKeySet[inj.Code()] {
			result = append(result, s.injectorToDefinition(inj))
		}
	}

	return result, nil
}

// injectorToDefinition converts a port.Injector to entity.InjectableDefinition.
func (s *InjectableService) injectorToDefinition(inj port.Injector) *entity.InjectableDefinition {
	code := inj.Code()

	labels := s.injectorRegistry.GetAllNames(code)
	descriptions := s.injectorRegistry.GetAllDescriptions(code)

	// Convert DataType
	dataType := convertValueTypeToDataType(inj.DataType())

	// Convert FormatConfig
	var formatConfig *entity.FormatConfig
	if formats := inj.Formats(); formats != nil {
		formatConfig = &entity.FormatConfig{
			Default: formats.Default,
			Options: formats.Options,
		}
	}

	// Convert DefaultValue
	var defaultValue *string
	if defVal := inj.DefaultValue(); defVal != nil {
		if str, ok := defVal.String(); ok {
			defaultValue = &str
		}
	}

	// Build metadata - for TABLE types, include column schema; for LIST types, include list schema
	metadata := buildInjectorMetadata(inj)

	return &entity.InjectableDefinition{
		ID:           code,
		WorkspaceID:  nil,
		Key:          code,
		Labels:       labels,
		Descriptions: descriptions,
		DataType:     dataType,
		SourceType:   entity.InjectableSourceTypeInternal,
		Metadata:     metadata,
		FormatConfig: formatConfig,
		Group:        s.injectorRegistry.GetGroup(code),
		DefaultValue: defaultValue,
		IsActive:     true,
		IsDeleted:    false,
		CreatedAt:    time.Time{},
		UpdatedAt:    nil,
	}
}

func uniqueNonEmptyCodes(codes []string) []string {
	if len(codes) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(codes))
	result := make([]string, 0, len(codes))
	for _, code := range codes {
		if code == "" {
			continue
		}
		if _, ok := seen[code]; ok {
			continue
		}
		seen[code] = struct{}{}
		result = append(result, code)
	}
	sort.Strings(result)
	return result
}

// buildInjectorMetadata builds metadata for table and list schema providers.
func buildInjectorMetadata(inj port.Injector) map[string]any {
	var metadata map[string]any

	if tableProvider, ok := inj.(port.TableSchemaProvider); ok {
		columns := tableProvider.ColumnSchema()
		if len(columns) > 0 {
			metadata = map[string]any{
				"columns": columns,
			}
		}
	}

	if listProvider, ok := inj.(port.ListSchemaProvider); ok {
		schema := listProvider.ListSchema()
		if metadata == nil {
			metadata = make(map[string]any)
		}
		metadata["listSchema"] = schema
	}

	return metadata
}

// convertValueTypeToDataType converts entity.ValueType to entity.InjectableDataType.
func convertValueTypeToDataType(vt entity.ValueType) entity.InjectableDataType {
	switch vt {
	case entity.ValueTypeString:
		return entity.InjectableDataTypeText
	case entity.ValueTypeNumber:
		return entity.InjectableDataTypeNumber
	case entity.ValueTypeBool:
		return entity.InjectableDataTypeBoolean
	case entity.ValueTypeTime:
		return entity.InjectableDataTypeDate
	case entity.ValueTypeTable:
		return entity.InjectableDataTypeTable
	case entity.ValueTypeImage:
		return entity.InjectableDataTypeImage
	case entity.ValueTypeList:
		return entity.InjectableDataTypeList
	default:
		return entity.InjectableDataTypeText
	}
}

// mergeInjectables merges DB and extension injectables.
// DB injectables take priority (can override extension keys).
func (s *InjectableService) mergeInjectables(db, ext []*entity.InjectableDefinition) []*entity.InjectableDefinition {
	// Build set of DB keys
	dbKeys := make(map[string]bool)
	for _, inj := range db {
		dbKeys[inj.Key] = true
	}

	// Start with DB injectables
	result := make([]*entity.InjectableDefinition, 0, len(db)+len(ext))
	result = append(result, db...)

	// Add extension injectables that don't conflict with DB keys
	for _, inj := range ext {
		if !dbKeys[inj.Key] {
			result = append(result, inj)
		}
	}

	return result
}

// fetchProviderResult retrieves provider injectables by resolving workspace codes.
func (s *InjectableService) fetchProviderResult(
	ctx context.Context,
	workspaceID string,
	env entity.Environment,
	codes []string,
) (*port.GetInjectablesResult, error) {
	tenantCode, workspaceCode, err := s.getWorkspaceCodes(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("getting workspace codes: %w", err)
	}
	if env == "" {
		env = entity.EnvironmentProd
	}
	slog.InfoContext(ctx, "fetching provider injectables",
		"workspace_id", workspaceID,
		"tenant_code", tenantCode,
		"workspace_code", workspaceCode,
		"environment", env,
		"requested_codes_count", len(codes),
	)

	injCtx := entity.NewInjectorContextWithCodes("", "", "", "list", tenantCode, workspaceCode, env, nil, nil)
	result, err := s.fetchProviderInjectables(ctx, injCtx, codes)
	if err != nil {
		return nil, err
	}
	if result == nil {
		slog.WarnContext(ctx, "provider injectables request returned nil result",
			"workspace_id", workspaceID,
			"tenant_code", tenantCode,
			"workspace_code", workspaceCode,
			"environment", env,
		)
		return nil, nil
	}

	slog.InfoContext(ctx, "provider injectables fetched",
		"workspace_id", workspaceID,
		"tenant_code", tenantCode,
		"workspace_code", workspaceCode,
		"environment", env,
		"requested_codes_count", len(codes),
		"injectables_count", len(result.Injectables),
		"groups_count", len(result.Groups),
	)

	resultCodes := make([]string, 0, len(result.Injectables))
	for _, injectable := range result.Injectables {
		if injectable.Code != "" {
			resultCodes = append(resultCodes, injectable.Code)
		}
	}
	slog.DebugContext(ctx, "provider injectable codes",
		"workspace_id", workspaceID,
		"codes", resultCodes,
	)

	return result, nil
}

func (s *InjectableService) fetchProviderInjectables(
	ctx context.Context,
	injCtx *entity.InjectorContext,
	codes []string,
) (*port.GetInjectablesResult, error) {
	if len(codes) > 0 {
		if provider, ok := s.workspaceProvider.(port.FilteredWorkspaceInjectableProvider); ok {
			return provider.GetInjectablesByCodes(ctx, injCtx, codes)
		}
	}
	return s.workspaceProvider.GetInjectables(ctx, injCtx)
}

// getWorkspaceCodes retrieves tenant code and workspace code from workspace ID.
func (s *InjectableService) getWorkspaceCodes(
	ctx context.Context, workspaceID string,
) (tenantCode, workspaceCode string, err error) {
	workspace, err := s.workspaceRepo.FindByID(ctx, workspaceID)
	if err != nil {
		return "", "", fmt.Errorf("finding workspace: %w", err)
	}
	workspaceCode = workspace.Code

	if workspace.TenantID != nil {
		tenant, err := s.tenantRepo.FindByID(ctx, *workspace.TenantID)
		if err != nil {
			return "", "", fmt.Errorf("finding tenant: %w", err)
		}
		tenantCode = tenant.Code
	}

	return tenantCode, workspaceCode, nil
}

// convertProviderInjectables converts provider injectables to entity definitions.
func (s *InjectableService) convertProviderInjectables(
	injectables []port.ProviderInjectable,
) []*entity.InjectableDefinition {
	result := make([]*entity.InjectableDefinition, 0, len(injectables))
	for _, inj := range injectables {
		def := &entity.InjectableDefinition{
			ID:           inj.Code,
			WorkspaceID:  nil,
			Key:          inj.Code,
			Labels:       inj.Label,
			Descriptions: inj.Description,
			DataType:     inj.DataType,
			SourceType:   entity.InjectableSourceTypeExternal,
			Metadata:     nil,
			IsActive:     true,
			IsDeleted:    false,
			CreatedAt:    time.Time{},
		}
		if inj.GroupKey != "" {
			def.Group = &inj.GroupKey
		}
		if len(inj.Formats) > 0 {
			options := make([]string, len(inj.Formats))
			for i, f := range inj.Formats {
				options[i] = f.Key
			}
			def.FormatConfig = &entity.FormatConfig{
				Default: inj.Formats[0].Key,
				Options: options,
			}
		}
		result = append(result, def)
	}
	return result
}

// convertProviderGroups converts provider groups to GroupConfig.
func convertProviderGroups(groups []port.ProviderGroup) []port.GroupConfig {
	result := make([]port.GroupConfig, 0, len(groups))
	for i, g := range groups {
		names := g.Name
		if names == nil {
			names = map[string]string{"en": g.Key}
		}
		result = append(result, port.GroupConfig{
			Key:   g.Key,
			Names: names,
			Icon:  g.Icon,
			Order: 1000 + i,
		})
	}
	return result
}

// validateNoDuplicateCodes checks that provider codes don't conflict with existing codes.
func validateNoDuplicateCodes(db, system, provider []*entity.InjectableDefinition) error {
	existingCodes := make(map[string]bool)
	for _, inj := range db {
		existingCodes[inj.Key] = true
	}
	for _, inj := range system {
		existingCodes[inj.Key] = true
	}

	for _, inj := range provider {
		if existingCodes[inj.Key] {
			return fmt.Errorf("provider injectable code %q conflicts with existing injectable", inj.Key)
		}
	}
	return nil
}
