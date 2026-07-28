package mapper

import (
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	accessuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/access"
)

// SystemRoleToResponse converts a SystemRoleAssignment entity to a response DTO.
func SystemRoleToResponse(r *entity.SystemRoleAssignment) *dto.SystemRoleResponse {
	if r == nil {
		return nil
	}
	return &dto.SystemRoleResponse{
		ID:        r.ID,
		UserID:    r.UserID,
		Role:      string(r.Role),
		GrantedBy: r.GrantedBy,
		CreatedAt: r.CreatedAt,
	}
}

// UserBriefToResponse converts a User entity to a brief response DTO.
func UserBriefToResponse(u *entity.User) *dto.UserBriefResponse {
	if u == nil {
		return nil
	}
	return &dto.UserBriefResponse{
		ID:       u.ID,
		Email:    u.Email,
		FullName: u.FullName,
		Status:   string(u.Status),
	}
}

// SystemRoleWithUserToResponse converts a SystemRoleWithUser entity to a response DTO.
func SystemRoleWithUserToResponse(r *entity.SystemRoleWithUser) *dto.SystemRoleWithUserResponse {
	if r == nil {
		return nil
	}
	return &dto.SystemRoleWithUserResponse{
		SystemRoleResponse: *SystemRoleToResponse(&r.SystemRoleAssignment),
		User:               UserBriefToResponse(r.User),
	}
}

// SystemRolesWithUserToResponses converts a slice of SystemRoleWithUser entities to response DTOs.
func SystemRolesWithUserToResponses(roles []*entity.SystemRoleWithUser) []*dto.SystemRoleWithUserResponse {
	result := make([]*dto.SystemRoleWithUserResponse, len(roles))
	for i, r := range roles {
		result[i] = SystemRoleWithUserToResponse(r)
	}
	return result
}

// AssignSystemRoleRequestToCommand converts a request to a command.
func AssignSystemRoleRequestToCommand(userID string, req dto.AssignSystemRoleRequest, grantedBy string) accessuc.AssignSystemRoleCommand {
	return accessuc.AssignSystemRoleCommand{
		UserID:    userID,
		Role:      entity.SystemRole(req.Role),
		GrantedBy: grantedBy,
	}
}

// AddSystemRoleRequestToCommand converts a request to an add command.
func AddSystemRoleRequestToCommand(req dto.AddSystemRoleRequest, grantedBy string) accessuc.AddSystemRoleCommand {
	return accessuc.AddSystemRoleCommand{
		Email:     req.Email,
		FullName:  req.FullName,
		Role:      entity.SystemRole(req.Role),
		GrantedBy: grantedBy,
	}
}

// RevokeSystemRoleToCommand creates a revoke command.
func RevokeSystemRoleToCommand(userID, revokedBy string) accessuc.RevokeSystemRoleCommand {
	return accessuc.RevokeSystemRoleCommand{
		UserID:    userID,
		RevokedBy: revokedBy,
	}
}
