package access

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// AssignSystemRoleCommand represents the command to assign a system role.
type AssignSystemRoleCommand struct {
	UserID    string
	Role      entity.SystemRole
	GrantedBy string
}

// AddSystemRoleCommand represents the command to add a system role by email.
type AddSystemRoleCommand struct {
	Email     string
	FullName  string
	Role      entity.SystemRole
	GrantedBy string
}

// RevokeSystemRoleCommand represents the command to revoke a system role.
type RevokeSystemRoleCommand struct {
	UserID    string
	RevokedBy string
}

// SystemRoleUseCase defines the input port for system role operations.
type SystemRoleUseCase interface {
	// ListUsersWithSystemRoles lists all users that have system roles.
	ListUsersWithSystemRoles(ctx context.Context) ([]*entity.SystemRoleWithUser, error)

	// AssignRole assigns a system role to a user.
	AssignRole(ctx context.Context, cmd AssignSystemRoleCommand) (*entity.SystemRoleAssignment, error)

	// AddRole finds or creates a user by email and assigns a system role.
	AddRole(ctx context.Context, cmd AddSystemRoleCommand) (*entity.SystemRoleAssignment, error)

	// RevokeRole revokes a user's system role.
	RevokeRole(ctx context.Context, cmd RevokeSystemRoleCommand) error

	// GetUserSystemRole gets a user's system role.
	GetUserSystemRole(ctx context.Context, userID string) (*entity.SystemRoleAssignment, error)
}
