package mapper

import (
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	organizationuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/organization"
)

// TenantMemberMapper handles mapping between tenant member entities and DTOs.
type TenantMemberMapper struct{}

// NewTenantMemberMapper creates a new tenant member mapper.
func NewTenantMemberMapper() *TenantMemberMapper {
	return &TenantMemberMapper{}
}

// ToResponse converts a TenantMemberWithUser entity to a response DTO.
func (m *TenantMemberMapper) ToResponse(member *entity.TenantMemberWithUser) *dto.TenantMemberResponse {
	return TenantMemberToResponse(member)
}

// ToResponseList converts a slice of TenantMemberWithUser entities to response DTOs.
func (m *TenantMemberMapper) ToResponseList(members []*entity.TenantMemberWithUser) []*dto.TenantMemberResponse {
	return TenantMembersToResponses(members)
}

// --- Package-level functions ---

// TenantMemberToResponse converts a TenantMemberWithUser entity to a response DTO.
func TenantMemberToResponse(member *entity.TenantMemberWithUser) *dto.TenantMemberResponse {
	if member == nil {
		return nil
	}

	resp := &dto.TenantMemberResponse{
		ID:               member.ID,
		TenantID:         member.TenantID,
		Role:             string(member.Role),
		MembershipStatus: string(member.MembershipStatus),
		CreatedAt:        member.CreatedAt,
	}

	if member.User != nil {
		resp.User = &dto.TenantMemberUserResponse{
			ID:       member.User.ID,
			Email:    member.User.Email,
			FullName: member.User.FullName,
			Status:   string(member.User.Status),
		}
	}

	return resp
}

// TenantMembersToResponses converts a slice of TenantMemberWithUser entities to response DTOs.
func TenantMembersToResponses(members []*entity.TenantMemberWithUser) []*dto.TenantMemberResponse {
	result := make([]*dto.TenantMemberResponse, len(members))
	for i, member := range members {
		result[i] = TenantMemberToResponse(member)
	}
	return result
}

// AddTenantMemberRequestToCommand converts an add member request to a usecase command.
func AddTenantMemberRequestToCommand(tenantID string, req dto.AddTenantMemberRequest, grantedBy string) organizationuc.AddTenantMemberCommand {
	return organizationuc.AddTenantMemberCommand{
		TenantID:  tenantID,
		Email:     req.Email,
		FullName:  req.FullName,
		Role:      entity.TenantRole(req.Role),
		GrantedBy: grantedBy,
	}
}

// UpdateTenantMemberRoleRequestToCommand converts an update role request to a usecase command.
func UpdateTenantMemberRoleRequestToCommand(memberID, tenantID string, req dto.UpdateTenantMemberRoleRequest, updatedBy string) organizationuc.UpdateTenantMemberRoleCommand {
	return organizationuc.UpdateTenantMemberRoleCommand{
		MemberID:  memberID,
		TenantID:  tenantID,
		NewRole:   entity.TenantRole(req.Role),
		UpdatedBy: updatedBy,
	}
}

// RemoveTenantMemberToCommand creates a remove tenant member command.
func RemoveTenantMemberToCommand(memberID, tenantID, removedBy string) organizationuc.RemoveTenantMemberCommand {
	return organizationuc.RemoveTenantMemberCommand{
		MemberID:  memberID,
		TenantID:  tenantID,
		RemovedBy: removedBy,
	}
}
