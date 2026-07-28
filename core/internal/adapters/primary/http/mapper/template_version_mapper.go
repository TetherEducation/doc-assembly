package mapper

import (
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

// TemplateVersionMapper handles mapping between template version entities and DTOs.
type TemplateVersionMapper struct {
	injectableMapper *InjectableMapper
}

// NewTemplateVersionMapper creates a new template version mapper.
func NewTemplateVersionMapper(injectableMapper *InjectableMapper) *TemplateVersionMapper {
	return &TemplateVersionMapper{
		injectableMapper: injectableMapper,
	}
}

// ToResponse converts a template version entity to a response DTO (without content).
func (m *TemplateVersionMapper) ToResponse(version *entity.TemplateVersion) *dto.TemplateVersionResponse {
	if version == nil {
		return nil
	}

	return &dto.TemplateVersionResponse{
		ID:                 version.ID,
		TemplateID:         version.TemplateID,
		VersionNumber:      version.VersionNumber,
		Name:               version.Name,
		Description:        version.Description,
		Status:             string(version.Status),
		ScheduledPublishAt: version.ScheduledPublishAt,
		ScheduledArchiveAt: version.ScheduledArchiveAt,
		PublishedAt:        version.PublishedAt,
		ArchivedAt:         version.ArchivedAt,
		PublishedBy:        version.PublishedBy,
		ArchivedBy:         version.ArchivedBy,
		CreatedBy:          version.CreatedBy,
		CreatedAt:          version.CreatedAt,
		UpdatedAt:          version.UpdatedAt,
	}
}

// ToResponseList converts a list of template versions to response DTOs.
func (m *TemplateVersionMapper) ToResponseList(versions []*entity.TemplateVersion) []*dto.TemplateVersionResponse {
	if versions == nil {
		return []*dto.TemplateVersionResponse{}
	}

	responses := make([]*dto.TemplateVersionResponse, len(versions))
	for i, version := range versions {
		responses[i] = m.ToResponse(version)
	}
	return responses
}

// ToListResponse converts a list of template versions to a list response DTO.
func (m *TemplateVersionMapper) ToListResponse(versions []*entity.TemplateVersion) *dto.ListTemplateVersionsResponse {
	items := m.ToResponseList(versions)
	return &dto.ListTemplateVersionsResponse{
		Items: items,
		Total: len(items),
	}
}

// ToDetailResponse converts a template version with details to a response DTO.
func (m *TemplateVersionMapper) ToDetailResponse(details *entity.TemplateVersionWithDetails) *dto.TemplateVersionDetailResponse {
	if details == nil {
		return nil
	}

	resp := &dto.TemplateVersionDetailResponse{
		TemplateVersionResponse: *m.ToResponse(&details.TemplateVersion),
		ContentStructure:        details.ContentStructure,
	}

	if details.Injectables != nil {
		resp.Injectables = m.injectableMapper.VersionInjectablesToResponse(details.Injectables)
	}

	if details.SignerRoles != nil {
		resp.SignerRoles = m.SignerRolesToResponse(details.SignerRoles)
	}

	return resp
}

// ToDetailResponseList converts a list of template versions with details to response DTOs.
func (m *TemplateVersionMapper) ToDetailResponseList(details []*entity.TemplateVersionWithDetails) []*dto.TemplateVersionDetailResponse {
	if details == nil {
		return []*dto.TemplateVersionDetailResponse{}
	}

	responses := make([]*dto.TemplateVersionDetailResponse, len(details))
	for i, d := range details {
		responses[i] = m.ToDetailResponse(d)
	}
	return responses
}

// ToSummaryResponse converts a template version with details to a summary response DTO (without content).
func (m *TemplateVersionMapper) ToSummaryResponse(details *entity.TemplateVersionWithDetails) *dto.TemplateVersionSummaryResponse {
	if details == nil {
		return nil
	}

	resp := &dto.TemplateVersionSummaryResponse{
		TemplateVersionResponse: *m.ToResponse(&details.TemplateVersion),
	}

	if details.Injectables != nil {
		resp.Injectables = m.injectableMapper.VersionInjectablesToResponse(details.Injectables)
	}

	if details.SignerRoles != nil {
		resp.SignerRoles = m.SignerRolesToResponse(details.SignerRoles)
	}

	return resp
}

// ToSummaryResponseList converts a list of template versions with details to summary response DTOs.
func (m *TemplateVersionMapper) ToSummaryResponseList(details []*entity.TemplateVersionWithDetails) []*dto.TemplateVersionSummaryResponse {
	if details == nil {
		return []*dto.TemplateVersionSummaryResponse{}
	}

	responses := make([]*dto.TemplateVersionSummaryResponse, len(details))
	for i, d := range details {
		responses[i] = m.ToSummaryResponse(d)
	}
	return responses
}

// SignerRoleToResponse converts a version signer role to a response DTO.
func (m *TemplateVersionMapper) SignerRoleToResponse(role *entity.TemplateVersionSignerRole) *dto.TemplateVersionSignerRoleResponse {
	if role == nil {
		return nil
	}

	return &dto.TemplateVersionSignerRoleResponse{
		ID:                role.ID,
		TemplateVersionID: role.TemplateVersionID,
		RoleName:          role.RoleName,
		AnchorString:      role.AnchorString,
		SignerOrder:       role.SignerOrder,
		CreatedAt:         role.CreatedAt,
		UpdatedAt:         role.UpdatedAt,
	}
}

// SignerRolesToResponse converts a list of version signer roles to response DTOs.
func (m *TemplateVersionMapper) SignerRolesToResponse(roles []*entity.TemplateVersionSignerRole) []*dto.TemplateVersionSignerRoleResponse {
	if roles == nil {
		return []*dto.TemplateVersionSignerRoleResponse{}
	}

	responses := make([]*dto.TemplateVersionSignerRoleResponse, len(roles))
	for i, role := range roles {
		responses[i] = m.SignerRoleToResponse(role)
	}
	return responses
}

// ToCreateCommand converts a create version request to a command.
func (m *TemplateVersionMapper) ToCreateCommand(templateID string, req *dto.CreateVersionRequest, userID string) templateuc.CreateVersionCommand {
	return templateuc.CreateVersionCommand{
		TemplateID:  templateID,
		Name:        req.Name,
		Description: req.Description,
		CreatedBy:   &userID,
	}
}

// ToUpdateCommand converts an update version request to a command.
func (m *TemplateVersionMapper) ToUpdateCommand(versionID string, req *dto.UpdateVersionRequest) templateuc.UpdateVersionCommand {
	return templateuc.UpdateVersionCommand{
		ID:               versionID,
		Name:             req.Name,
		Description:      req.Description,
		ContentStructure: req.ContentStructure,
	}
}

// ToAddInjectableCommand converts an add injectable request to a command.
func (m *TemplateVersionMapper) ToAddInjectableCommand(versionID string, req *dto.AddVersionInjectableRequest) templateuc.AddVersionInjectableCommand {
	return templateuc.AddVersionInjectableCommand{
		VersionID:              versionID,
		InjectableDefinitionID: req.InjectableDefinitionID,
		IsRequired:             req.IsRequired,
		DefaultValue:           req.DefaultValue,
	}
}

// ToSchedulePublishCommand converts a schedule publish request to a command.
func (m *TemplateVersionMapper) ToSchedulePublishCommand(versionID string, req *dto.SchedulePublishRequest) templateuc.SchedulePublishCommand {
	return templateuc.SchedulePublishCommand{
		VersionID: versionID,
		PublishAt: req.PublishAt,
	}
}

// ToScheduleArchiveCommand converts a schedule archive request to a command.
func (m *TemplateVersionMapper) ToScheduleArchiveCommand(versionID string, req *dto.ScheduleArchiveRequest) templateuc.ScheduleArchiveCommand {
	return templateuc.ScheduleArchiveCommand{
		VersionID: versionID,
		ArchiveAt: req.ArchiveAt,
	}
}

// ToPromoteCommand converts a promote version request to a command.
func (m *TemplateVersionMapper) ToPromoteCommand(
	templateID, versionID string,
	req *dto.PromoteVersionRequest,
	targetWorkspaceID, userID string,
) templateuc.PromoteVersionCommand {
	return templateuc.PromoteVersionCommand{
		SourceVersionID:   versionID,
		SourceTemplateID:  templateID,
		TargetWorkspaceID: targetWorkspaceID,
		Mode:              string(req.Mode),
		TargetTemplateID:  req.TargetTemplateID,
		TargetFolderID:    req.TargetFolderID,
		VersionName:       req.VersionName,
		PromotedBy:        userID,
	}
}
