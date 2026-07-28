package mapper

import (
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	organizationuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/organization"
)

// WorkspaceMemberMapper handles mapping between member entities and DTOs.
type WorkspaceMemberMapper struct{}

// NewWorkspaceMemberMapper creates a new workspace member mapper.
func NewWorkspaceMemberMapper() *WorkspaceMemberMapper {
	return &WorkspaceMemberMapper{}
}

// ToResponse converts a MemberWithUser entity to a response DTO.
func (m *WorkspaceMemberMapper) ToResponse(member *entity.MemberWithUser) *dto.MemberResponse {
	if member == nil {
		return nil
	}

	resp := &dto.MemberResponse{
		ID:               member.ID,
		WorkspaceID:      member.WorkspaceID,
		Role:             string(member.Role),
		MembershipStatus: string(member.MembershipStatus),
		JoinedAt:         member.JoinedAt,
		CreatedAt:        member.CreatedAt,
	}

	if member.User != nil {
		resp.User = &dto.MemberUserResponse{
			ID:       member.User.ID,
			Email:    member.User.Email,
			FullName: member.User.FullName,
			Status:   string(member.User.Status),
		}
	}

	return resp
}

// ToResponseList converts a slice of MemberWithUser entities to response DTOs.
func (m *WorkspaceMemberMapper) ToResponseList(members []*entity.MemberWithUser) []*dto.MemberResponse {
	result := make([]*dto.MemberResponse, len(members))
	for i, member := range members {
		result[i] = m.ToResponse(member)
	}
	return result
}

// --- Package-level functions for backward compatibility ---

// MemberToResponse converts a MemberWithUser entity to a response DTO.
func MemberToResponse(member *entity.MemberWithUser) *dto.MemberResponse {
	if member == nil {
		return nil
	}

	resp := &dto.MemberResponse{
		ID:               member.ID,
		WorkspaceID:      member.WorkspaceID,
		Role:             string(member.Role),
		MembershipStatus: string(member.MembershipStatus),
		JoinedAt:         member.JoinedAt,
		CreatedAt:        member.CreatedAt,
	}

	if member.User != nil {
		resp.User = &dto.MemberUserResponse{
			ID:       member.User.ID,
			Email:    member.User.Email,
			FullName: member.User.FullName,
			Status:   string(member.User.Status),
		}
	}

	return resp
}

// MembersToResponses converts a slice of MemberWithUser entities to response DTOs.
func MembersToResponses(members []*entity.MemberWithUser) []*dto.MemberResponse {
	result := make([]*dto.MemberResponse, len(members))
	for i, member := range members {
		result[i] = MemberToResponse(member)
	}
	return result
}

// InviteMemberRequestToCommand converts an invite request to a usecase command.
func InviteMemberRequestToCommand(workspaceID string, req dto.InviteMemberRequest, invitedBy string) organizationuc.InviteMemberCommand {
	return organizationuc.InviteMemberCommand{
		WorkspaceID: workspaceID,
		Email:       req.Email,
		FullName:    req.FullName,
		Role:        entity.WorkspaceRole(req.Role),
		InvitedBy:   invitedBy,
	}
}

// UpdateMemberRoleRequestToCommand converts an update role request to a usecase command.
func UpdateMemberRoleRequestToCommand(memberID, workspaceID string, req dto.UpdateMemberRoleRequest, updatedBy string) organizationuc.UpdateMemberRoleCommand {
	return organizationuc.UpdateMemberRoleCommand{
		MemberID:    memberID,
		WorkspaceID: workspaceID,
		NewRole:     entity.WorkspaceRole(req.Role),
		UpdatedBy:   updatedBy,
	}
}

// RemoveMemberToCommand creates a remove member command.
func RemoveMemberToCommand(memberID, workspaceID, removedBy string) organizationuc.RemoveMemberCommand {
	return organizationuc.RemoveMemberCommand{
		MemberID:    memberID,
		WorkspaceID: workspaceID,
		RemovedBy:   removedBy,
	}
}
