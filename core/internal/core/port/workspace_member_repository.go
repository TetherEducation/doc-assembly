package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// WorkspaceMemberRepository defines the interface for workspace membership data access.
type WorkspaceMemberRepository interface {
	// Create creates a new workspace membership.
	Create(ctx context.Context, member *entity.WorkspaceMember) (string, error)

	// FindByID finds a membership by ID.
	FindByID(ctx context.Context, id string) (*entity.WorkspaceMember, error)

	// FindByUserAndWorkspace finds a membership for a specific user and workspace.
	FindByUserAndWorkspace(ctx context.Context, userID, workspaceID string) (*entity.WorkspaceMember, error)

	// FindByWorkspace lists all members of a workspace.
	FindByWorkspace(ctx context.Context, workspaceID string) ([]*entity.MemberWithUser, error)

	// FindByUser lists all workspace memberships for a user.
	FindByUser(ctx context.Context, userID string) ([]*entity.WorkspaceMember, error)

	// FindActiveByUserAndWorkspace finds an active membership.
	FindActiveByUserAndWorkspace(ctx context.Context, userID, workspaceID string) (*entity.WorkspaceMember, error)

	// CountActiveByWorkspace counts direct active memberships in a workspace.
	CountActiveByWorkspace(ctx context.Context, workspaceID string) (int, error)

	// Update updates a membership.
	Update(ctx context.Context, member *entity.WorkspaceMember) error

	// Delete removes a membership.
	Delete(ctx context.Context, id string) error

	// Activate activates a pending membership.
	Activate(ctx context.Context, id string) error

	// UpdateRole updates a member's role.
	UpdateRole(ctx context.Context, id string, role entity.WorkspaceRole) error
}
