package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// SystemInjectableRepository defines the interface for system injectable data access.
// System injectables are code-defined injectors whose availability is controlled via database.
type SystemInjectableRepository interface {
	// FindActiveKeysForWorkspace returns the keys of system injectables that are active
	// for a given workspace. Respects priority: WORKSPACE > TENANT > PUBLIC.
	// Both the definition and assignment must be active (is_active = true).
	FindActiveKeysForWorkspace(ctx context.Context, workspaceID string) ([]string, error)

	// FindActiveKeysForWorkspaceAndKeys returns active system injectable keys restricted to the requested keys.
	FindActiveKeysForWorkspaceAndKeys(ctx context.Context, workspaceID string, keys []string) ([]string, error)

	// FindAllDefinitions returns a map of all definition keys to their is_active status.
	FindAllDefinitions(ctx context.Context) (map[string]bool, error)

	// UpsertDefinition creates or updates a system injectable definition.
	// If the key doesn't exist, creates it. If it exists, updates is_active.
	UpsertDefinition(ctx context.Context, key string, isActive bool) error

	// EnsureDefinition creates an active definition when the key is missing.
	// If the key already exists, it does not modify is_active or updated_at.
	EnsureDefinition(ctx context.Context, key string) error

	// FindAssignmentsByKey returns all assignments for a given injectable key.
	FindAssignmentsByKey(ctx context.Context, key string) ([]*entity.SystemInjectableAssignment, error)

	// CreateAssignment creates a new assignment.
	CreateAssignment(ctx context.Context, assignment *entity.SystemInjectableAssignment) error

	// DeleteAssignment removes an assignment by ID.
	DeleteAssignment(ctx context.Context, id string) error

	// SetAssignmentActive updates the is_active flag for an assignment.
	SetAssignmentActive(ctx context.Context, id string, isActive bool) error

	// FindPublicActiveKeys returns a set of injectable keys that have an active PUBLIC assignment.
	FindPublicActiveKeys(ctx context.Context) (map[string]bool, error)

	// CreatePublicAssignments creates PUBLIC assignments for multiple keys in batch.
	// Uses ON CONFLICT DO NOTHING to skip keys that already have a PUBLIC assignment.
	// Returns the number of actually created assignments.
	CreatePublicAssignments(ctx context.Context, keys []string) (int, error)

	// DeletePublicAssignments deletes PUBLIC assignments for multiple keys.
	// Returns the number of actually deleted assignments.
	DeletePublicAssignments(ctx context.Context, keys []string) (int, error)

	// FindPublicAssignmentsByKeys returns a map of key -> assignmentID for PUBLIC assignments.
	FindPublicAssignmentsByKeys(ctx context.Context, keys []string) (map[string]string, error)
}
