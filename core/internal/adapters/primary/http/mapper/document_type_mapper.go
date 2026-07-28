package mapper

import (
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	cataloguc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/catalog"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

// DocumentTypeMapper handles mapping between document type entities and DTOs.
type DocumentTypeMapper struct{}

// NewDocumentTypeMapper creates a new document type mapper.
func NewDocumentTypeMapper() *DocumentTypeMapper {
	return &DocumentTypeMapper{}
}

// ToResponse converts a DocumentType entity to a response DTO.
func (m *DocumentTypeMapper) ToResponse(dt *entity.DocumentType) *dto.DocumentTypeResponse {
	if dt == nil {
		return nil
	}
	return &dto.DocumentTypeResponse{
		ID:          dt.ID,
		TenantID:    dt.TenantID,
		Code:        dt.Code,
		Name:        dt.Name,
		Description: dt.Description,
		IsGlobal:    dt.IsGlobal,
		CreatedAt:   dt.CreatedAt,
		UpdatedAt:   dt.UpdatedAt,
	}
}

// ToListItemResponse converts a DocumentTypeListItem entity to a response DTO.
func (m *DocumentTypeMapper) ToListItemResponse(dt *entity.DocumentTypeListItem) *dto.DocumentTypeListItemResponse {
	if dt == nil {
		return nil
	}
	return &dto.DocumentTypeListItemResponse{
		DocumentTypeResponse: dto.DocumentTypeResponse{
			ID:          dt.ID,
			TenantID:    dt.TenantID,
			Code:        dt.Code,
			Name:        dt.Name,
			Description: dt.Description,
			IsGlobal:    dt.IsGlobal,
			CreatedAt:   dt.CreatedAt,
			UpdatedAt:   dt.UpdatedAt,
		},
		TemplatesCount: dt.TemplatesCount,
	}
}

// ToListItemResponses converts a slice of DocumentTypeListItem entities to response DTOs.
func (m *DocumentTypeMapper) ToListItemResponses(dts []*entity.DocumentTypeListItem) []*dto.DocumentTypeListItemResponse {
	if dts == nil {
		return []*dto.DocumentTypeListItemResponse{}
	}
	result := make([]*dto.DocumentTypeListItemResponse, len(dts))
	for i, dt := range dts {
		result[i] = m.ToListItemResponse(dt)
	}
	return result
}

// ToDeleteResponse converts a DeleteDocumentTypeResult to a response DTO.
func (m *DocumentTypeMapper) ToDeleteResponse(result *cataloguc.DeleteDocumentTypeResult) *dto.DeleteDocumentTypeResponse {
	if result == nil {
		return nil
	}
	return &dto.DeleteDocumentTypeResponse{
		Deleted:    result.Deleted,
		Templates:  m.ToTemplateInfoResponses(result.Templates),
		CanReplace: result.CanReplace,
	}
}

// ToTemplateInfoResponse converts a DocumentTypeTemplateInfo to a response DTO.
func (m *DocumentTypeMapper) ToTemplateInfoResponse(info *entity.DocumentTypeTemplateInfo) *dto.DocumentTypeTemplateInfoResponse {
	if info == nil {
		return nil
	}
	return &dto.DocumentTypeTemplateInfoResponse{
		ID:            info.ID,
		Title:         info.Title,
		WorkspaceID:   info.WorkspaceID,
		WorkspaceName: info.WorkspaceName,
	}
}

// ToTemplateInfoResponses converts a slice of DocumentTypeTemplateInfo to response DTOs.
func (m *DocumentTypeMapper) ToTemplateInfoResponses(infos []*entity.DocumentTypeTemplateInfo) []*dto.DocumentTypeTemplateInfoResponse {
	if infos == nil {
		return nil
	}
	result := make([]*dto.DocumentTypeTemplateInfoResponse, len(infos))
	for i, info := range infos {
		result[i] = m.ToTemplateInfoResponse(info)
	}
	return result
}

// ToPaginatedResponse converts a list of document types with total count to a paginated response.
func (m *DocumentTypeMapper) ToPaginatedResponse(dts []*entity.DocumentTypeListItem, total int64, page, perPage int) *dto.PaginatedDocumentTypesResponse {
	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}
	return &dto.PaginatedDocumentTypesResponse{
		Data: m.ToListItemResponses(dts),
		Pagination: dto.PaginationMeta{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	}
}

// --- Package-level functions ---

// DocumentTypeToResponse converts a DocumentType entity to a response DTO.
func DocumentTypeToResponse(dt *entity.DocumentType) *dto.DocumentTypeResponse {
	if dt == nil {
		return nil
	}
	return &dto.DocumentTypeResponse{
		ID:          dt.ID,
		TenantID:    dt.TenantID,
		Code:        dt.Code,
		Name:        dt.Name,
		Description: dt.Description,
		IsGlobal:    dt.IsGlobal,
		CreatedAt:   dt.CreatedAt,
		UpdatedAt:   dt.UpdatedAt,
	}
}

// CreateDocumentTypeRequestToCommand converts a create request to a usecase command.
func CreateDocumentTypeRequestToCommand(tenantID string, req dto.CreateDocumentTypeRequest) cataloguc.CreateDocumentTypeCommand {
	return cataloguc.CreateDocumentTypeCommand{
		TenantID:    tenantID,
		Code:        req.Code,
		Name:        entity.I18nText(req.Name),
		Description: entity.I18nText(req.Description),
	}
}

// UpdateDocumentTypeRequestToCommand converts an update request to a usecase command.
func UpdateDocumentTypeRequestToCommand(id, tenantID string, req dto.UpdateDocumentTypeRequest) cataloguc.UpdateDocumentTypeCommand {
	return cataloguc.UpdateDocumentTypeCommand{
		ID:          id,
		TenantID:    tenantID,
		Name:        entity.I18nText(req.Name),
		Description: entity.I18nText(req.Description),
	}
}

// DeleteDocumentTypeRequestToCommand converts a delete request to a usecase command.
func DeleteDocumentTypeRequestToCommand(id, tenantID string, req dto.DeleteDocumentTypeRequest) cataloguc.DeleteDocumentTypeCommand {
	return cataloguc.DeleteDocumentTypeCommand{
		ID:            id,
		TenantID:      tenantID,
		Force:         req.Force,
		ReplaceWithID: req.ReplaceWithID,
	}
}

// DocumentTypeListRequestToFilters converts a list request to repository filters.
func DocumentTypeListRequestToFilters(req dto.DocumentTypeListRequest) port.DocumentTypeFilters {
	offset := (req.Page - 1) * req.PerPage
	return port.DocumentTypeFilters{
		Search: req.Query,
		Limit:  req.PerPage,
		Offset: offset,
	}
}

// AssignDocumentTypeRequestToCommand converts an assign request to a usecase command.
func AssignDocumentTypeRequestToCommand(templateID, workspaceID string, req dto.AssignDocumentTypeRequest) templateuc.AssignDocumentTypeCommand {
	return templateuc.AssignDocumentTypeCommand{
		TemplateID:     templateID,
		WorkspaceID:    workspaceID,
		DocumentTypeID: req.DocumentTypeID,
		Force:          req.Force,
	}
}

// AssignResultToResponse converts an AssignDocumentTypeResult to a response DTO.
func AssignResultToResponse(result *templateuc.AssignDocumentTypeResult, templateMapper *TemplateMapper) *dto.AssignDocumentTypeResponse {
	if result == nil {
		return nil
	}
	resp := &dto.AssignDocumentTypeResponse{}
	if result.Template != nil {
		resp.Template = templateMapper.ToResponse(result.Template)
	}
	if result.Conflict != nil {
		resp.Conflict = &dto.TemplateConflictInfo{
			ID:    result.Conflict.ID,
			Title: result.Conflict.Title,
		}
	}
	return resp
}
