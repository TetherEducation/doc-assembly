package mapper

import (
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

// TemplateMapper handles mapping between template entities and DTOs.
type TemplateMapper struct {
	versionMapper *TemplateVersionMapper
	tagMapper     *TagMapper
	folderMapper  *FolderMapper
}

// NewTemplateMapper creates a new template mapper.
func NewTemplateMapper(versionMapper *TemplateVersionMapper, tagMapper *TagMapper, folderMapper *FolderMapper) *TemplateMapper {
	return &TemplateMapper{
		versionMapper: versionMapper,
		tagMapper:     tagMapper,
		folderMapper:  folderMapper,
	}
}

// ToResponse converts a template entity to a response DTO.
func (m *TemplateMapper) ToResponse(template *entity.Template) *dto.TemplateResponse {
	if template == nil {
		return nil
	}

	return &dto.TemplateResponse{
		ID:              template.ID,
		WorkspaceID:     template.WorkspaceID,
		FolderID:        template.FolderID,
		DocumentTypeID:  template.DocumentTypeID,
		Title:           template.Title,
		IsPublicLibrary: template.IsPublicLibrary,
		Process:         template.Process,
		ProcessType:     string(template.ProcessType),
		CreatedAt:       template.CreatedAt,
		UpdatedAt:       template.UpdatedAt,
	}
}

// ToListItemResponse converts a template list item to a response DTO.
func (m *TemplateMapper) ToListItemResponse(item *entity.TemplateListItem) *dto.TemplateListItemResponse {
	if item == nil {
		return nil
	}

	return &dto.TemplateListItemResponse{
		ID:                     item.ID,
		WorkspaceID:            item.WorkspaceID,
		FolderID:               item.FolderID,
		DocumentTypeCode:       item.DocumentTypeCode,
		Title:                  item.Title,
		IsPublicLibrary:        item.IsPublicLibrary,
		Process:                item.Process,
		ProcessType:            string(item.ProcessType),
		HasPublishedVersion:    item.HasPublishedVersion,
		VersionCount:           item.VersionCount,
		ScheduledVersionCount:  item.ScheduledVersionCount,
		PublishedVersionNumber: item.PublishedVersionNumber,
		Tags:                   m.toSimpleTagList(item.Tags),
		CreatedAt:              item.CreatedAt,
		UpdatedAt:              item.UpdatedAt,
	}
}

// toSimpleTagList converts a list of tag entities to simplified tag responses.
func (m *TemplateMapper) toSimpleTagList(tags []*entity.Tag) []*dto.TagSimpleResponse {
	if tags == nil {
		return []*dto.TagSimpleResponse{}
	}

	result := make([]*dto.TagSimpleResponse, len(tags))
	for i, tag := range tags {
		result[i] = &dto.TagSimpleResponse{
			ID:    tag.ID,
			Name:  tag.Name,
			Color: tag.Color,
		}
	}
	return result
}

// ToListItemResponseList converts a list of template list items to response DTOs.
func (m *TemplateMapper) ToListItemResponseList(items []*entity.TemplateListItem) []*dto.TemplateListItemResponse {
	if items == nil {
		return []*dto.TemplateListItemResponse{}
	}

	responses := make([]*dto.TemplateListItemResponse, len(items))
	for i, item := range items {
		responses[i] = m.ToListItemResponse(item)
	}
	return responses
}

// ToListResponse converts a list of template list items to a list response DTO.
func (m *TemplateMapper) ToListResponse(items []*entity.TemplateListItem, limit, offset int) *dto.ListTemplatesResponse {
	responses := m.ToListItemResponseList(items)
	return &dto.ListTemplatesResponse{
		Items:  responses,
		Total:  len(responses),
		Limit:  limit,
		Offset: offset,
	}
}

// ToDetailsResponse converts a template with details to a response DTO.
func (m *TemplateMapper) ToDetailsResponse(details *entity.TemplateWithDetails) *dto.TemplateWithDetailsResponse {
	if details == nil {
		return nil
	}

	resp := &dto.TemplateWithDetailsResponse{
		TemplateResponse: *m.ToResponse(&details.Template),
	}

	if details.PublishedVersion != nil {
		resp.PublishedVersion = m.versionMapper.ToDetailResponse(details.PublishedVersion)
	}

	if details.Tags != nil {
		resp.Tags = m.tagMapper.ToResponseList(details.Tags)
	}

	if details.Folder != nil {
		resp.Folder = m.folderMapper.ToResponse(details.Folder)
	}

	return resp
}

// ToAllVersionsResponse converts a template with all versions to a response DTO.
func (m *TemplateMapper) ToAllVersionsResponse(details *entity.TemplateWithAllVersions) *dto.TemplateWithAllVersionsResponse {
	if details == nil {
		return nil
	}

	resp := &dto.TemplateWithAllVersionsResponse{
		TemplateResponse: *m.ToResponse(&details.Template),
	}

	if details.DocumentType != nil {
		resp.DocumentTypeName = details.DocumentType.Name
	}

	if details.Versions != nil {
		resp.Versions = m.versionMapper.ToSummaryResponseList(details.Versions)
	}

	if details.Tags != nil {
		resp.Tags = m.tagMapper.ToResponseList(details.Tags)
	}

	if details.Folder != nil {
		resp.Folder = m.folderMapper.ToResponse(details.Folder)
	}

	return resp
}

// ToCreateResponse converts a template and initial version to a create response DTO.
func (m *TemplateMapper) ToCreateResponse(template *entity.Template, version *entity.TemplateVersion) *dto.TemplateCreateResponse {
	return &dto.TemplateCreateResponse{
		Template:       m.ToResponse(template),
		InitialVersion: m.versionMapper.ToResponse(version),
	}
}

// ToCreateCommand converts a create request to a command.
func (m *TemplateMapper) ToCreateCommand(req *dto.CreateTemplateRequest, workspaceID string, userID string) templateuc.CreateTemplateCommand {
	return templateuc.CreateTemplateCommand{
		WorkspaceID:      workspaceID,
		FolderID:         req.FolderID,
		Title:            req.Title,
		ContentStructure: req.ContentStructure,
		IsPublicLibrary:  req.IsPublicLibrary,
		Process:          req.Process,
		ProcessType:      req.ProcessType,
		CreatedBy:        userID,
	}
}

// ToUpdateCommand converts an update request to a command.
func (m *TemplateMapper) ToUpdateCommand(id string, req *dto.UpdateTemplateRequest) templateuc.UpdateTemplateCommand {
	return templateuc.UpdateTemplateCommand{
		ID:              id,
		Title:           req.Title,
		FolderID:        req.FolderID,
		IsPublicLibrary: req.IsPublicLibrary,
	}
}

// ToCloneCommand converts a clone request to a command.
func (m *TemplateMapper) ToCloneCommand(sourceID string, req *dto.CloneTemplateRequest, userID string) templateuc.CloneTemplateCommand {
	return templateuc.CloneTemplateCommand{
		SourceTemplateID: sourceID,
		VersionID:        req.VersionID,
		NewTitle:         req.NewTitle,
		TargetFolderID:   req.TargetFolderID,
		ClonedBy:         userID,
	}
}

// ToFilters converts filter request parameters to port filters.
func (m *TemplateMapper) ToFilters(req *dto.TemplateFiltersRequest) port.TemplateFilters {
	filters := port.TemplateFilters{
		HasPublishedVersion: req.HasPublishedVersion,
		Search:              req.Search,
		TagIDs:              req.TagIDs,
		Limit:               req.Limit,
		Offset:              req.Offset,
	}

	if req.FolderID != nil {
		if *req.FolderID == "root" {
			filters.RootOnly = true
		} else {
			filters.FolderID = req.FolderID
		}
	}

	return filters
}
