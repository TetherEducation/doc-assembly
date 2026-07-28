package injectable

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	injectableuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/injectable"
)

// NewSystemInjectableService creates a new system injectable service.
func NewSystemInjectableService(
	repo port.SystemInjectableRepository,
	registry port.InjectorRegistry,
) injectableuc.SystemInjectableUseCase {
	return &SystemInjectableService{
		repo:     repo,
		registry: registry,
	}
}

// SystemInjectableService implements system injectable management business logic.
type SystemInjectableService struct {
	repo     port.SystemInjectableRepository
	registry port.InjectorRegistry
}

// ListAll returns all system injectors from the registry with their active state.
func (s *SystemInjectableService) ListAll(ctx context.Context) ([]*entity.SystemInjectableInfo, error) {
	injectors := s.registry.GetAll()
	if len(injectors) == 0 {
		return nil, nil
	}

	definitions, err := s.repo.FindAllDefinitions(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading definitions: %w", err)
	}

	publicActiveKeys, err := s.repo.FindPublicActiveKeys(ctx)
	if err != nil {
		return nil, fmt.Errorf("loading public active keys: %w", err)
	}

	result := make([]*entity.SystemInjectableInfo, 0, len(injectors))
	for _, inj := range injectors {
		code := inj.Code()
		isActive := definitions[code]      // false if not found
		isPublic := publicActiveKeys[code] // false if not found

		result = append(result, &entity.SystemInjectableInfo{
			Key:         code,
			Label:       s.registry.GetAllNames(code),
			Description: s.registry.GetAllDescriptions(code),
			DataType:    convertValueTypeToDataType(inj.DataType()),
			Group:       s.registry.GetGroup(code),
			IsActive:    isActive,
			IsPublic:    isPublic,
		})
	}

	return result, nil
}

// Activate enables a system injectable globally.
func (s *SystemInjectableService) Activate(ctx context.Context, key string) error {
	if err := s.validateKeyExists(key); err != nil {
		return err
	}
	return s.repo.UpsertDefinition(ctx, key, true)
}

// Deactivate disables a system injectable globally.
func (s *SystemInjectableService) Deactivate(ctx context.Context, key string) error {
	if err := s.validateKeyExists(key); err != nil {
		return err
	}
	return s.repo.UpsertDefinition(ctx, key, false)
}

// ListAssignments returns all assignments for a given system injectable key.
func (s *SystemInjectableService) ListAssignments(ctx context.Context, key string) ([]*entity.SystemInjectableAssignment, error) {
	if err := s.validateKeyExists(key); err != nil {
		return nil, err
	}
	return s.repo.FindAssignmentsByKey(ctx, key)
}

// CreateAssignment creates a new assignment for a system injectable.
func (s *SystemInjectableService) CreateAssignment(ctx context.Context, cmd injectableuc.CreateAssignmentCommand) (*entity.SystemInjectableAssignment, error) {
	if err := s.validateKeyExists(cmd.InjectableKey); err != nil {
		return nil, err
	}

	assignment := &entity.SystemInjectableAssignment{
		ID:            uuid.New().String(),
		InjectableKey: cmd.InjectableKey,
		ScopeType:     cmd.ScopeType,
		TenantID:      cmd.TenantID,
		WorkspaceID:   cmd.WorkspaceID,
		IsActive:      true,
		CreatedAt:     time.Now().UTC(),
	}

	if err := assignment.Validate(); err != nil {
		return nil, err
	}

	if err := s.repo.CreateAssignment(ctx, assignment); err != nil {
		return nil, err
	}

	return assignment, nil
}

// DeleteAssignment removes an assignment.
func (s *SystemInjectableService) DeleteAssignment(ctx context.Context, key, assignmentID string) error {
	if err := s.validateKeyExists(key); err != nil {
		return err
	}
	return s.repo.DeleteAssignment(ctx, assignmentID)
}

// ExcludeAssignment sets an assignment's is_active to false (exclusion).
func (s *SystemInjectableService) ExcludeAssignment(ctx context.Context, key, assignmentID string) error {
	if err := s.validateKeyExists(key); err != nil {
		return err
	}
	return s.repo.SetAssignmentActive(ctx, assignmentID, false)
}

// IncludeAssignment sets an assignment's is_active to true (undo exclusion).
func (s *SystemInjectableService) IncludeAssignment(ctx context.Context, key, assignmentID string) error {
	if err := s.validateKeyExists(key); err != nil {
		return err
	}
	return s.repo.SetAssignmentActive(ctx, assignmentID, true)
}

// validateKeyExists checks if the key exists in the registry.
func (s *SystemInjectableService) validateKeyExists(key string) error {
	if _, found := s.registry.Get(key); !found {
		return entity.ErrSystemInjectableNotFound
	}
	return nil
}

// validateKeys validates keys exist in registry, returns valid keys and failed entries.
func (s *SystemInjectableService) validateKeys(keys []string) ([]string, []injectableuc.BulkAssignmentError) {
	validKeys := make([]string, 0, len(keys))
	failed := make([]injectableuc.BulkAssignmentError, 0)

	for _, key := range keys {
		if err := s.validateKeyExists(key); err != nil {
			failed = append(failed, injectableuc.BulkAssignmentError{Key: key, Error: err})
		} else {
			validKeys = append(validKeys, key)
		}
	}

	return validKeys, failed
}

// filterExistingAssignments filters keys, marking existing as succeeded, returns keys to create.
func filterExistingAssignments(
	validKeys []string,
	existing map[string]string,
	result *injectableuc.BulkAssignmentResult,
) []string {
	keysToCreate := make([]string, 0, len(validKeys))

	for _, key := range validKeys {
		if _, exists := existing[key]; exists {
			result.Succeeded = append(result.Succeeded, key)
		} else {
			keysToCreate = append(keysToCreate, key)
		}
	}

	return keysToCreate
}

// BulkCreatePublicAssignments creates PUBLIC assignments for multiple injectable keys.
// Keys that already have PUBLIC assignments are considered successful (idempotent).
func (s *SystemInjectableService) BulkCreatePublicAssignments(ctx context.Context, keys []string) (*injectableuc.BulkAssignmentResult, error) {
	result := &injectableuc.BulkAssignmentResult{
		Succeeded: []string{},
		Failed:    []injectableuc.BulkAssignmentError{},
	}

	if len(keys) == 0 {
		return result, nil
	}

	// 1. Validate all keys exist in registry
	validKeys, failed := s.validateKeys(keys)
	result.Failed = failed

	if len(validKeys) == 0 {
		return result, nil
	}

	// 2. Find which keys already have PUBLIC assignments
	existing, err := s.repo.FindPublicAssignmentsByKeys(ctx, validKeys)
	if err != nil {
		return nil, fmt.Errorf("checking existing PUBLIC assignments: %w", err)
	}

	// 3. Filter keys - already existing are considered successful (idempotent)
	keysToCreate := filterExistingAssignments(validKeys, existing, result)

	if len(keysToCreate) == 0 {
		return result, nil
	}

	// 4. Ensure definitions exist for keys to create (required by FK constraint)
	for _, key := range keysToCreate {
		if err := s.repo.UpsertDefinition(ctx, key, true); err != nil {
			return nil, fmt.Errorf("ensuring definition for key %s: %w", key, err)
		}
	}

	// 5. Create assignments for remaining keys
	_, err = s.repo.CreatePublicAssignments(ctx, keysToCreate)
	if err != nil {
		return nil, fmt.Errorf("creating PUBLIC assignments: %w", err)
	}

	result.Succeeded = append(result.Succeeded, keysToCreate...)
	return result, nil
}

// filterMissingAssignments filters keys, marking missing as succeeded, returns keys to delete.
func filterMissingAssignments(
	validKeys []string,
	existing map[string]string,
	result *injectableuc.BulkAssignmentResult,
) []string {
	keysToDelete := make([]string, 0, len(validKeys))

	for _, key := range validKeys {
		if _, exists := existing[key]; !exists {
			result.Succeeded = append(result.Succeeded, key)
		} else {
			keysToDelete = append(keysToDelete, key)
		}
	}

	return keysToDelete
}

// BulkDeletePublicAssignments deletes PUBLIC assignments for multiple injectable keys.
// Keys that don't have PUBLIC assignments are considered successful (idempotent).
func (s *SystemInjectableService) BulkDeletePublicAssignments(ctx context.Context, keys []string) (*injectableuc.BulkAssignmentResult, error) {
	result := &injectableuc.BulkAssignmentResult{
		Succeeded: []string{},
		Failed:    []injectableuc.BulkAssignmentError{},
	}

	if len(keys) == 0 {
		return result, nil
	}

	// 1. Validate all keys exist in registry
	validKeys, failed := s.validateKeys(keys)
	result.Failed = failed

	if len(validKeys) == 0 {
		return result, nil
	}

	// 2. Find which keys have PUBLIC assignments
	existing, err := s.repo.FindPublicAssignmentsByKeys(ctx, validKeys)
	if err != nil {
		return nil, fmt.Errorf("checking existing PUBLIC assignments: %w", err)
	}

	// 3. Filter keys - non-existing are considered successful (idempotent)
	keysToDelete := filterMissingAssignments(validKeys, existing, result)

	if len(keysToDelete) == 0 {
		return result, nil
	}

	// 4. Delete PUBLIC assignments
	_, err = s.repo.DeletePublicAssignments(ctx, keysToDelete)
	if err != nil {
		return nil, fmt.Errorf("deleting PUBLIC assignments: %w", err)
	}

	result.Succeeded = append(result.Succeeded, keysToDelete...)
	return result, nil
}
