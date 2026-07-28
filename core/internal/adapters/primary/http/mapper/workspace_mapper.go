package mapper

import (
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	organizationuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/organization"
)

// WorkspaceToResponse converts a Workspace entity to a response DTO.
func WorkspaceToResponse(ws *entity.Workspace) dto.WorkspaceResponse {
	return dto.WorkspaceResponse{
		ID:             ws.ID,
		TenantID:       ws.TenantID,
		Name:           ws.Name,
		Code:           ws.Code,
		Type:           string(ws.Type),
		Status:         string(ws.Status),
		Role:           string(ws.CurrentRole),
		CreatedAt:      ws.CreatedAt,
		UpdatedAt:      ws.UpdatedAt,
		LastAccessedAt: ws.LastAccessedAt,
	}
}

// WorkspacesToResponses converts a slice of Workspace entities to response DTOs.
func WorkspacesToResponses(workspaces []*entity.Workspace) []dto.WorkspaceResponse {
	result := make([]dto.WorkspaceResponse, len(workspaces))
	for i, ws := range workspaces {
		result[i] = WorkspaceToResponse(ws)
	}
	return result
}

// CreateWorkspaceRequestToCommand converts a create request to a usecase command.
func CreateWorkspaceRequestToCommand(req dto.CreateWorkspaceRequest, createdBy string) organizationuc.CreateWorkspaceCommand {
	return organizationuc.CreateWorkspaceCommand{
		TenantID:  req.TenantID,
		Name:      req.Name,
		Code:      req.Code,
		Type:      entity.WorkspaceType(req.Type),
		CreatedBy: createdBy,
	}
}

// UpdateWorkspaceRequestToCommand converts an update request to a usecase command.
func UpdateWorkspaceRequestToCommand(id string, req dto.UpdateWorkspaceRequest) organizationuc.UpdateWorkspaceCommand {
	return organizationuc.UpdateWorkspaceCommand{
		ID:   id,
		Name: &req.Name,
		Code: req.Code,
	}
}

// WorkspaceListRequestToFilters converts a list request to port filters.
func WorkspaceListRequestToFilters(req dto.WorkspaceListRequest) port.WorkspaceFilters {
	offset := (req.Page - 1) * req.PerPage
	return port.WorkspaceFilters{
		Limit:          req.PerPage,
		Offset:         offset,
		Query:          req.Query,
		Status:         req.Status,
		AccessibleOnly: req.AccessibleOnly,
	}
}

// WorkspacesToPaginatedResponse converts workspaces to a paginated response.
func WorkspacesToPaginatedResponse(workspaces []*entity.Workspace, total int64, page, perPage int) *dto.PaginatedWorkspacesResponse {
	responses := make([]*dto.WorkspaceResponse, len(workspaces))
	for i, ws := range workspaces {
		resp := WorkspaceToResponse(ws)
		responses[i] = &resp
	}

	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}

	return &dto.PaginatedWorkspacesResponse{
		Data: responses,
		Pagination: dto.PaginationMeta{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	}
}
