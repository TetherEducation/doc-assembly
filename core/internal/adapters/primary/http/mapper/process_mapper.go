package mapper

import (
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	cataloguc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/catalog"
)

// ProcessMapper handles mapping between process entities and DTOs.
type ProcessMapper struct{}

// NewProcessMapper creates a new process mapper.
func NewProcessMapper() *ProcessMapper {
	return &ProcessMapper{}
}

// ToResponse converts a Process entity to a response DTO.
func (m *ProcessMapper) ToResponse(p *entity.Process) *dto.ProcessResponse {
	if p == nil {
		return nil
	}
	return &dto.ProcessResponse{
		ID:          p.ID,
		TenantID:    p.TenantID,
		Code:        p.Code,
		ProcessType: string(p.ProcessType),
		Name:        p.Name,
		Description: p.Description,
		IsGlobal:    p.IsGlobal,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

// ToListItemResponse converts a ProcessListItem entity to a response DTO.
func (m *ProcessMapper) ToListItemResponse(p *entity.ProcessListItem) *dto.ProcessListItemResponse {
	if p == nil {
		return nil
	}
	return &dto.ProcessListItemResponse{
		ProcessResponse: dto.ProcessResponse{
			ID:          p.ID,
			TenantID:    p.TenantID,
			Code:        p.Code,
			ProcessType: string(p.ProcessType),
			Name:        p.Name,
			Description: p.Description,
			IsGlobal:    p.IsGlobal,
			CreatedAt:   p.CreatedAt,
			UpdatedAt:   p.UpdatedAt,
		},
		TemplatesCount: p.TemplatesCount,
	}
}

// ToListItemResponses converts a slice of ProcessListItem entities to response DTOs.
func (m *ProcessMapper) ToListItemResponses(ps []*entity.ProcessListItem) []*dto.ProcessListItemResponse {
	if ps == nil {
		return []*dto.ProcessListItemResponse{}
	}
	result := make([]*dto.ProcessListItemResponse, len(ps))
	for i, p := range ps {
		result[i] = m.ToListItemResponse(p)
	}
	return result
}

// ToDeleteResponse converts a DeleteProcessResult to a response DTO.
func (m *ProcessMapper) ToDeleteResponse(result *cataloguc.DeleteProcessResult) *dto.DeleteProcessResponse {
	if result == nil {
		return nil
	}
	return &dto.DeleteProcessResponse{
		Deleted:    result.Deleted,
		Templates:  m.ToTemplateInfoResponses(result.Templates),
		CanReplace: result.CanReplace,
	}
}

// ToTemplateInfoResponse converts a ProcessTemplateInfo to a response DTO.
func (m *ProcessMapper) ToTemplateInfoResponse(info *entity.ProcessTemplateInfo) *dto.ProcessTemplateInfoResponse {
	if info == nil {
		return nil
	}
	return &dto.ProcessTemplateInfoResponse{
		ID:            info.ID,
		Title:         info.Title,
		WorkspaceID:   info.WorkspaceID,
		WorkspaceName: info.WorkspaceName,
	}
}

// ToTemplateInfoResponses converts a slice of ProcessTemplateInfo to response DTOs.
func (m *ProcessMapper) ToTemplateInfoResponses(infos []*entity.ProcessTemplateInfo) []*dto.ProcessTemplateInfoResponse {
	if infos == nil {
		return nil
	}
	result := make([]*dto.ProcessTemplateInfoResponse, len(infos))
	for i, info := range infos {
		result[i] = m.ToTemplateInfoResponse(info)
	}
	return result
}

// ToPaginatedResponse converts a list of processes with total count to a paginated response.
func (m *ProcessMapper) ToPaginatedResponse(ps []*entity.ProcessListItem, total int64, page, perPage int) *dto.PaginatedProcessesResponse {
	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}
	return &dto.PaginatedProcessesResponse{
		Data: m.ToListItemResponses(ps),
		Pagination: dto.PaginationMeta{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	}
}

// --- Package-level functions ---

// ProcessToResponse converts a Process entity to a response DTO.
func ProcessToResponse(p *entity.Process) *dto.ProcessResponse {
	if p == nil {
		return nil
	}
	return &dto.ProcessResponse{
		ID:          p.ID,
		TenantID:    p.TenantID,
		Code:        p.Code,
		ProcessType: string(p.ProcessType),
		Name:        p.Name,
		Description: p.Description,
		IsGlobal:    p.IsGlobal,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

// CreateProcessRequestToCommand converts a create request to a usecase command.
func CreateProcessRequestToCommand(tenantID string, req dto.CreateProcessRequest) cataloguc.CreateProcessCommand {
	return cataloguc.CreateProcessCommand{
		TenantID:    tenantID,
		Code:        req.Code,
		ProcessType: entity.ProcessType(req.ProcessType),
		Name:        entity.I18nText(req.Name),
		Description: entity.I18nText(req.Description),
	}
}

// UpdateProcessRequestToCommand converts an update request to a usecase command.
func UpdateProcessRequestToCommand(id, tenantID string, req dto.UpdateProcessRequest) cataloguc.UpdateProcessCommand {
	return cataloguc.UpdateProcessCommand{
		ID:          id,
		TenantID:    tenantID,
		Name:        entity.I18nText(req.Name),
		Description: entity.I18nText(req.Description),
	}
}

// DeleteProcessRequestToCommand converts a delete request to a usecase command.
func DeleteProcessRequestToCommand(id, tenantID string, req dto.DeleteProcessRequest) cataloguc.DeleteProcessCommand {
	return cataloguc.DeleteProcessCommand{
		ID:              id,
		TenantID:        tenantID,
		Force:           req.Force,
		ReplaceWithCode: req.ReplaceWithCode,
	}
}

// ProcessListRequestToFilters converts a list request to repository filters.
func ProcessListRequestToFilters(req dto.ProcessListRequest) port.ProcessFilters {
	offset := (req.Page - 1) * req.PerPage
	return port.ProcessFilters{
		Search: req.Query,
		Limit:  req.PerPage,
		Offset: offset,
	}
}
