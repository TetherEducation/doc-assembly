package registry

import (
	"errors"
	"sync"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// mapperRegistry implements port.MapperRegistry with thread-safe support.
type mapperRegistry struct {
	mu     sync.RWMutex
	mapper port.RequestMapper
}

// NewMapperRegistry creates a new MapperRegistry instance.
func NewMapperRegistry() port.MapperRegistry {
	return &mapperRegistry{}
}

// Set registers the request mapper.
func (r *mapperRegistry) Set(mapper port.RequestMapper) error {
	if mapper == nil {
		return errors.New("mapper cannot be nil")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.mapper != nil {
		return errors.New("mapper already registered")
	}

	r.mapper = mapper
	return nil
}

// Get returns the registered mapper.
func (r *mapperRegistry) Get() (port.RequestMapper, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.mapper, r.mapper != nil
}

// Ensure mapperRegistry implements port.MapperRegistry.
var _ port.MapperRegistry = (*mapperRegistry)(nil)
