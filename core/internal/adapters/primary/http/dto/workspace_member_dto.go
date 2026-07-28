package dto

import (
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// MemberResponse represents a workspace member in API responses.
type MemberResponse struct {
	ID               string              `json:"id"`
	WorkspaceID      string              `json:"workspaceId"`
	Role             string              `json:"role"`
	MembershipStatus string              `json:"membershipStatus"`
	JoinedAt         *time.Time          `json:"joinedAt,omitempty"`
	CreatedAt        time.Time           `json:"createdAt"`
	User             *MemberUserResponse `json:"user"`
}

// MemberUserResponse represents the user data within a member response.
type MemberUserResponse struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	FullName string `json:"fullName"`
	Status   string `json:"status"`
}

// InviteMemberRequest represents a request to invite a user to a workspace.
type InviteMemberRequest struct {
	Email    string `json:"email" binding:"required,email"`
	FullName string `json:"fullName,omitempty" binding:"max=255"`
	Role     string `json:"role" binding:"required"`
}

// UpdateMemberRoleRequest represents a request to update a member's role.
type UpdateMemberRoleRequest struct {
	Role string `json:"role" binding:"required"`
}

// Validate validates the InviteMemberRequest.
func (r *InviteMemberRequest) Validate() error {
	if r.Email == "" {
		return ErrEmailRequired
	}
	if !isValidInviteRole(r.Role) {
		return ErrInvalidRole
	}
	return nil
}

// Validate validates the UpdateMemberRoleRequest.
func (r *UpdateMemberRoleRequest) Validate() error {
	if !isValidInviteRole(r.Role) {
		return ErrInvalidRole
	}
	return nil
}

// isValidInviteRole checks if the role is valid for invitation/update (excludes OWNER).
func isValidInviteRole(role string) bool {
	switch entity.WorkspaceRole(role) {
	case entity.WorkspaceRoleAdmin, entity.WorkspaceRoleEditor,
		entity.WorkspaceRoleOperator, entity.WorkspaceRoleViewer:
		return true
	}
	return false
}
