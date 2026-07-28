package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// SystemRoleRepository defines the interface for system role data access.
type SystemRoleRepository interface {
	// Create creates a new system role assignment.
	Create(ctx context.Context, assignment *entity.SystemRoleAssignment) (string, error)

	// FindByUserID finds a system role assignment by user ID.
	FindByUserID(ctx context.Context, userID string) (*entity.SystemRoleAssignment, error)

	// FindAll lists all system role assignments.
	FindAll(ctx context.Context) ([]*entity.SystemRoleAssignment, error)

	// Delete removes a system role assignment by user ID.
	Delete(ctx context.Context, userID string) error

	// UpdateRole updates a user's system role.
	UpdateRole(ctx context.Context, userID string, role entity.SystemRole) error
}
