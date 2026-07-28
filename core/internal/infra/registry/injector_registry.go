package registry

import (
	"fmt"
	"sync"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	"github.com/TetherEducation/doc-assembly/core/internal/infra/config"
)

// injectorRegistry implements port.injectorRegistry with thread-safe support.
type injectorRegistry struct {
	mu                sync.RWMutex
	injectors         map[string]port.Injector
	i18n              *config.InjectorI18nConfig
	initFunc          port.InitFunc
	workspaceProvider port.WorkspaceInjectableProvider
}

// NewInjectorRegistry creates a new InjectorRegistry instance.
func NewInjectorRegistry(i18n *config.InjectorI18nConfig) port.InjectorRegistry {
	return &injectorRegistry{
		injectors: make(map[string]port.Injector),
		i18n:      i18n,
	}
}

// Register registers an injector in the registry.
func (r *injectorRegistry) Register(injector port.Injector) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	code := injector.Code()
	if code == "" {
		return fmt.Errorf("injector code cannot be empty")
	}

	if _, exists := r.injectors[code]; exists {
		return fmt.Errorf("injector with code %q already registered", code)
	}

	r.injectors[code] = injector
	return nil
}

// Get retrieves an injector by its code.
func (r *injectorRegistry) Get(code string) (port.Injector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	injector, ok := r.injectors[code]
	return injector, ok
}

// GetAll returns all registered injectors.
func (r *injectorRegistry) GetAll() []port.Injector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]port.Injector, 0, len(r.injectors))
	for _, injector := range r.injectors {
		result = append(result, injector)
	}
	return result
}

// Codes returns all registered injector codes.
func (r *injectorRegistry) Codes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	codes := make([]string, 0, len(r.injectors))
	for code := range r.injectors {
		codes = append(codes, code)
	}
	return codes
}

// GetName returns the translated name of the injector.
func (r *injectorRegistry) GetName(code, locale string) string {
	if r.i18n == nil {
		return code
	}
	return r.i18n.GetName(code, locale)
}

// GetDescription returns the translated description of the injector.
func (r *injectorRegistry) GetDescription(code, locale string) string {
	if r.i18n == nil {
		return ""
	}
	return r.i18n.GetDescription(code, locale)
}

// GetAllNames returns all translations for the injector name.
func (r *injectorRegistry) GetAllNames(code string) map[string]string {
	if r.i18n == nil {
		return map[string]string{"en": code}
	}
	return r.i18n.GetAllNames(code)
}

// GetAllDescriptions returns all translations for the injector description.
func (r *injectorRegistry) GetAllDescriptions(code string) map[string]string {
	if r.i18n == nil {
		return map[string]string{}
	}
	return r.i18n.GetAllDescriptions(code)
}

// GetGroup returns the group the injector belongs to.
// Returns nil if the injector has no group assigned.
func (r *injectorRegistry) GetGroup(code string) *string {
	if r.i18n == nil {
		return nil
	}
	return r.i18n.GetGroup(code)
}

// SetInitFunc registers the GLOBAL initialization function.
func (r *injectorRegistry) SetInitFunc(fn port.InitFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.initFunc = fn
}

// GetInitFunc returns the registered initialization function.
func (r *injectorRegistry) GetInitFunc() port.InitFunc {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.initFunc
}

// GetAllGroups returns all groups with all locale translations.
func (r *injectorRegistry) GetAllGroups() []port.GroupConfig {
	if r.i18n == nil {
		return nil
	}
	configGroups := r.i18n.GetAllGroups()
	result := make([]port.GroupConfig, len(configGroups))
	for i, g := range configGroups {
		result[i] = port.GroupConfig{
			Key:   g.Key,
			Names: g.Names,
			Icon:  g.Icon,
			Order: g.Order,
		}
	}
	return result
}

// SetWorkspaceInjectableProvider sets the workspace injectable provider.
func (r *injectorRegistry) SetWorkspaceInjectableProvider(provider port.WorkspaceInjectableProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workspaceProvider = provider
}

// GetWorkspaceInjectableProvider returns the workspace injectable provider.
func (r *injectorRegistry) GetWorkspaceInjectableProvider() port.WorkspaceInjectableProvider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.workspaceProvider
}

// Ensure InjectorRegistry implements port.InjectorRegistry.
var _ port.InjectorRegistry = (*injectorRegistry)(nil)
