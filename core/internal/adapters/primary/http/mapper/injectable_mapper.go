package mapper

import (
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// InjectableMapper handles mapping between injectable entities and DTOs.
type InjectableMapper struct{}

// NewInjectableMapper creates a new injectable mapper.
func NewInjectableMapper() *InjectableMapper {
	return &InjectableMapper{}
}

// mapFormatConfig converts entity.FormatConfig to dto.FormatConfigResponse.
func mapFormatConfig(fc *entity.FormatConfig) *dto.FormatConfigResponse {
	if fc == nil {
		return nil
	}
	return &dto.FormatConfigResponse{
		Default: fc.Default,
		Options: fc.Options,
	}
}

// ToResponse converts an injectable entity to a response DTO.
func (m *InjectableMapper) ToResponse(injectable *entity.InjectableDefinition) *dto.InjectableResponse {
	if injectable == nil {
		return nil
	}

	// Use i18n maps if available (system/provider), otherwise wrap DB string
	labels := injectable.Labels
	if labels == nil {
		labels = map[string]string{"_": injectable.Label}
	}
	descriptions := injectable.Descriptions
	if descriptions == nil && injectable.Description != "" {
		descriptions = map[string]string{"_": injectable.Description}
	}

	return &dto.InjectableResponse{
		ID:           injectable.ID,
		WorkspaceID:  injectable.WorkspaceID,
		Key:          injectable.Key,
		Label:        labels,
		Description:  descriptions,
		DataType:     string(injectable.DataType),
		SourceType:   string(injectable.SourceType),
		Metadata:     injectable.Metadata,
		FormatConfig: mapFormatConfig(injectable.FormatConfig),
		Group:        injectable.Group,
		IsGlobal:     injectable.IsGlobal(),
		CreatedAt:    injectable.CreatedAt,
		UpdatedAt:    injectable.UpdatedAt,
	}
}

// ToResponseList converts a list of injectable entities to response DTOs.
func (m *InjectableMapper) ToResponseList(injectables []*entity.InjectableDefinition) []*dto.InjectableResponse {
	if injectables == nil {
		return []*dto.InjectableResponse{}
	}

	responses := make([]*dto.InjectableResponse, len(injectables))
	for i, injectable := range injectables {
		responses[i] = m.ToResponse(injectable)
	}
	return responses
}

// ToListResponse converts a list of injectable entities to a list response DTO.
func (m *InjectableMapper) ToListResponse(injectables []*entity.InjectableDefinition, groups []port.GroupConfig) *dto.ListInjectablesResponse {
	items := m.ToResponseList(injectables)
	groupResponses := m.ToGroupResponseList(groups)
	return &dto.ListInjectablesResponse{
		Items:  items,
		Groups: groupResponses,
		Total:  len(items),
	}
}

// ToGroupResponseList converts a list of GroupConfig to GroupResponse DTOs.
func (m *InjectableMapper) ToGroupResponseList(groups []port.GroupConfig) []*dto.GroupResponse {
	if groups == nil {
		return []*dto.GroupResponse{}
	}

	responses := make([]*dto.GroupResponse, len(groups))
	for i, g := range groups {
		responses[i] = &dto.GroupResponse{
			Key:   g.Key,
			Name:  g.Names,
			Icon:  g.Icon,
			Order: g.Order,
		}
	}
	return responses
}

// VersionInjectableToResponse converts a version injectable with definition to a response DTO.
func (m *InjectableMapper) VersionInjectableToResponse(iwd *entity.VersionInjectableWithDefinition) *dto.TemplateVersionInjectableResponse {
	if iwd == nil {
		return nil
	}

	return &dto.TemplateVersionInjectableResponse{
		ID:                iwd.ID,
		TemplateVersionID: iwd.TemplateVersionID,
		IsRequired:        iwd.IsRequired,
		DefaultValue:      iwd.DefaultValue,
		Definition:        m.ToResponse(iwd.Definition),
		CreatedAt:         iwd.CreatedAt,
	}
}

// VersionInjectablesToResponse converts a list of version injectables to response DTOs.
func (m *InjectableMapper) VersionInjectablesToResponse(injectables []*entity.VersionInjectableWithDefinition) []*dto.TemplateVersionInjectableResponse {
	if injectables == nil {
		return []*dto.TemplateVersionInjectableResponse{}
	}

	responses := make([]*dto.TemplateVersionInjectableResponse, len(injectables))
	for i, iwd := range injectables {
		responses[i] = m.VersionInjectableToResponse(iwd)
	}
	return responses
}

// ToWorkspaceResponse converts a workspace injectable entity to a response DTO.
func (m *InjectableMapper) ToWorkspaceResponse(injectable *entity.InjectableDefinition) *dto.WorkspaceInjectableResponse {
	if injectable == nil {
		return nil
	}

	return &dto.WorkspaceInjectableResponse{
		ID:           injectable.ID,
		WorkspaceID:  *injectable.WorkspaceID,
		Key:          injectable.Key,
		Label:        injectable.Label,
		Description:  injectable.Description,
		DataType:     string(injectable.DataType),
		SourceType:   string(injectable.SourceType),
		Metadata:     injectable.Metadata,
		FormatConfig: mapFormatConfig(injectable.FormatConfig),
		DefaultValue: injectable.DefaultValue,
		IsActive:     injectable.IsActive,
		CreatedAt:    injectable.CreatedAt,
		UpdatedAt:    injectable.UpdatedAt,
	}
}

// ToWorkspaceResponseList converts a list of workspace injectable entities to response DTOs.
func (m *InjectableMapper) ToWorkspaceResponseList(injectables []*entity.InjectableDefinition) []*dto.WorkspaceInjectableResponse {
	if injectables == nil {
		return []*dto.WorkspaceInjectableResponse{}
	}

	responses := make([]*dto.WorkspaceInjectableResponse, len(injectables))
	for i, injectable := range injectables {
		responses[i] = m.ToWorkspaceResponse(injectable)
	}
	return responses
}

// ToWorkspaceListResponse converts a list of workspace injectable entities to a list response DTO.
func (m *InjectableMapper) ToWorkspaceListResponse(injectables []*entity.InjectableDefinition) *dto.ListWorkspaceInjectablesResponse {
	items := m.ToWorkspaceResponseList(injectables)
	return &dto.ListWorkspaceInjectablesResponse{
		Items: items,
		Total: len(items),
	}
}
