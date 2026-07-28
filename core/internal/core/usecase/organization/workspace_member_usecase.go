package organization

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// InviteMemberCommand contains data for inviting a user to a workspace.
type InviteMemberCommand struct {
	WorkspaceID string
	Email       string
	FullName    string
	Role        entity.WorkspaceRole
	InvitedBy   string
}

// UpdateMemberRoleCommand contains data for updating a member's role.
type UpdateMemberRoleCommand struct {
	MemberID    string
	WorkspaceID string
	NewRole     entity.WorkspaceRole
	UpdatedBy   string // The user performing the update
}

// RemoveMemberCommand contains data for removing a member.
type RemoveMemberCommand struct {
	MemberID    string
	WorkspaceID string
	RemovedBy   string // The user performing the removal
}

// WorkspaceMemberUseCase defines the interface for workspace member operations.
type WorkspaceMemberUseCase interface {
	// ListMembers lists all members of a workspace.
	ListMembers(ctx context.Context, workspaceID string) ([]*entity.MemberWithUser, error)

	// GetMember retrieves a specific member by ID.
	GetMember(ctx context.Context, memberID string) (*entity.MemberWithUser, error)

	// InviteMember invites a user to join a workspace.
	// Creates a shadow user if the email doesn't exist.
	InviteMember(ctx context.Context, cmd InviteMemberCommand) (*entity.MemberWithUser, error)

	// UpdateMemberRole updates a member's role within the workspace.
	UpdateMemberRole(ctx context.Context, cmd UpdateMemberRoleCommand) (*entity.MemberWithUser, error)

	// RemoveMember removes a member from the workspace.
	RemoveMember(ctx context.Context, cmd RemoveMemberCommand) error
}
