package mapper

import (
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	cataloguc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/catalog"
)

// TagMapper handles mapping between tag entities and DTOs.
type TagMapper struct{}

// NewTagMapper creates a new tag mapper.
func NewTagMapper() *TagMapper {
	return &TagMapper{}
}

// ToResponse converts a Tag entity to a response DTO.
func (m *TagMapper) ToResponse(t *entity.Tag) *dto.TagResponse {
	if t == nil {
		return nil
	}
	return &dto.TagResponse{
		ID:          t.ID,
		WorkspaceID: t.WorkspaceID,
		Name:        t.Name,
		Color:       t.Color,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

// ToResponseList converts a slice of Tag entities to response DTOs.
func (m *TagMapper) ToResponseList(tags []*entity.Tag) []*dto.TagResponse {
	if tags == nil {
		return []*dto.TagResponse{}
	}
	result := make([]*dto.TagResponse, len(tags))
	for i, t := range tags {
		result[i] = m.ToResponse(t)
	}
	return result
}

// --- Package-level functions for backward compatibility ---

// TagToResponse converts a Tag entity to a response DTO.
func TagToResponse(t *entity.Tag) dto.TagResponse {
	return dto.TagResponse{
		ID:          t.ID,
		WorkspaceID: t.WorkspaceID,
		Name:        t.Name,
		Color:       t.Color,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

// TagsToResponses converts a slice of Tag entities to response DTOs.
func TagsToResponses(tags []*entity.Tag) []dto.TagResponse {
	result := make([]dto.TagResponse, len(tags))
	for i, t := range tags {
		result[i] = TagToResponse(t)
	}
	return result
}

// TagWithCountToResponse converts a TagWithCount entity to a response DTO.
func TagWithCountToResponse(t *entity.TagWithCount) dto.TagWithCountResponse {
	return dto.TagWithCountResponse{
		TagResponse:   TagToResponse(&t.Tag),
		TemplateCount: t.TemplateCount,
	}
}

// TagsWithCountToResponses converts a slice of TagWithCount entities to response DTOs.
func TagsWithCountToResponses(tags []*entity.TagWithCount) []dto.TagWithCountResponse {
	result := make([]dto.TagWithCountResponse, len(tags))
	for i, t := range tags {
		result[i] = TagWithCountToResponse(t)
	}
	return result
}

// CreateTagRequestToCommand converts a create request to a usecase command.
func CreateTagRequestToCommand(workspaceID string, req dto.CreateTagRequest, createdBy string) cataloguc.CreateTagCommand {
	return cataloguc.CreateTagCommand{
		WorkspaceID: workspaceID,
		Name:        req.Name,
		Color:       req.Color,
		CreatedBy:   createdBy,
	}
}

// UpdateTagRequestToCommand converts an update request to a usecase command.
func UpdateTagRequestToCommand(id string, req dto.UpdateTagRequest) cataloguc.UpdateTagCommand {
	return cataloguc.UpdateTagCommand{
		ID:    id,
		Name:  req.Name,
		Color: req.Color,
	}
}
