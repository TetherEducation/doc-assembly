package injectable

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// SystemInjectableUseCase defines the input port for system injectable management operations.
// System injectables are code-defined injectors whose availability is controlled via database.
type SystemInjectableUseCase interface {
	// ListAll returns all system injectors from the registry with their active state.
	// If an injector exists in DB, uses is_active from DB. Otherwise, returns as is_active=false.
	ListAll(ctx context.Context) ([]*entity.SystemInjectableInfo, error)

	// Activate enables a system injectable globally.
	// If the key doesn't exist in DB, creates it with is_active=true.
	Activate(ctx context.Context, key string) error

	// Deactivate disables a system injectable globally.
	// If the key doesn't exist in DB, creates it with is_active=false.
	Deactivate(ctx context.Context, key string) error

	// ListAssignments returns all assignments for a given system injectable key.
	ListAssignments(ctx context.Context, key string) ([]*entity.SystemInjectableAssignment, error)

	// CreateAssignment creates a new assignment for a system injectable.
	CreateAssignment(ctx context.Context, cmd CreateAssignmentCommand) (*entity.SystemInjectableAssignment, error)

	// DeleteAssignment removes an assignment.
	DeleteAssignment(ctx context.Context, key, assignmentID string) error

	// ExcludeAssignment sets an assignment's is_active to false (exclusion).
	ExcludeAssignment(ctx context.Context, key, assignmentID string) error

	// IncludeAssignment sets an assignment's is_active to true (undo exclusion).
	IncludeAssignment(ctx context.Context, key, assignmentID string) error

	// BulkCreatePublicAssignments creates PUBLIC assignments for multiple injectable keys.
	// Returns a result with succeeded keys and any failures (e.g., key not in registry, already has PUBLIC).
	BulkCreatePublicAssignments(ctx context.Context, keys []string) (*BulkAssignmentResult, error)

	// BulkDeletePublicAssignments deletes PUBLIC assignments for multiple injectable keys.
	// Returns a result with succeeded keys and any failures (e.g., key not in registry, no PUBLIC assignment).
	BulkDeletePublicAssignments(ctx context.Context, keys []string) (*BulkAssignmentResult, error)
}

// CreateAssignmentCommand holds the data needed to create a system injectable assignment.
type CreateAssignmentCommand struct {
	InjectableKey string
	ScopeType     entity.InjectableScopeType
	TenantID      *string
	WorkspaceID   *string
}

// BulkAssignmentResult holds the result of a bulk assignment operation.
type BulkAssignmentResult struct {
	Succeeded []string
	Failed    []BulkAssignmentError
}

// BulkAssignmentError represents an error for a specific key in a bulk operation.
type BulkAssignmentError struct {
	Key   string
	Error error
}
