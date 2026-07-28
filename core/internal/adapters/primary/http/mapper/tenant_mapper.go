package mapper

import (
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	organizationuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/organization"
)

// TenantMapper handles mapping between tenant entities and DTOs.
type TenantMapper struct{}

// NewTenantMapper creates a new tenant mapper.
func NewTenantMapper() *TenantMapper {
	return &TenantMapper{}
}

// ToResponse converts a Tenant entity to a response DTO.
func (m *TenantMapper) ToResponse(t *entity.Tenant) *dto.TenantResponse {
	if t == nil {
		return nil
	}
	return TenantToResponse(t)
}

// --- Package-level functions for backward compatibility ---

// TenantToResponse converts a Tenant entity to a response DTO.
func TenantToResponse(t *entity.Tenant) *dto.TenantResponse {
	if t == nil {
		return nil
	}

	settings := map[string]interface{}{}
	if t.Settings.Currency != "" {
		settings["currency"] = t.Settings.Currency
	}
	if t.Settings.Timezone != "" {
		settings["timezone"] = t.Settings.Timezone
	}
	if t.Settings.DateFormat != "" {
		settings["dateFormat"] = t.Settings.DateFormat
	}
	if t.Settings.Locale != "" {
		settings["locale"] = t.Settings.Locale
	}

	return &dto.TenantResponse{
		ID:          t.ID,
		Name:        t.Name,
		Code:        t.Code,
		Description: t.Description,
		IsSystem:    t.IsSystem,
		Status:      string(t.Status),
		Settings:    settings,
		CreatedAt:   t.CreatedAt,
		UpdatedAt:   t.UpdatedAt,
	}
}

// TenantsToResponses converts a slice of Tenant entities to response DTOs.
func TenantsToResponses(tenants []*entity.Tenant) []*dto.TenantResponse {
	result := make([]*dto.TenantResponse, len(tenants))
	for i, t := range tenants {
		result[i] = TenantToResponse(t)
	}
	return result
}

// TenantWithRoleToResponse converts a TenantWithRole entity to a response DTO.
func TenantWithRoleToResponse(t *entity.TenantWithRole) *dto.TenantWithRoleResponse {
	if t == nil || t.Tenant == nil {
		return nil
	}

	settings := map[string]interface{}{}
	if t.Tenant.Settings.Currency != "" {
		settings["currency"] = t.Tenant.Settings.Currency
	}
	if t.Tenant.Settings.Timezone != "" {
		settings["timezone"] = t.Tenant.Settings.Timezone
	}
	if t.Tenant.Settings.DateFormat != "" {
		settings["dateFormat"] = t.Tenant.Settings.DateFormat
	}
	if t.Tenant.Settings.Locale != "" {
		settings["locale"] = t.Tenant.Settings.Locale
	}

	return &dto.TenantWithRoleResponse{
		ID:             t.Tenant.ID,
		Name:           t.Tenant.Name,
		Code:           t.Tenant.Code,
		Description:    t.Tenant.Description,
		IsSystem:       t.Tenant.IsSystem,
		Status:         string(t.Tenant.Status),
		Role:           string(t.Role),
		Settings:       settings,
		CreatedAt:      t.Tenant.CreatedAt,
		UpdatedAt:      t.Tenant.UpdatedAt,
		LastAccessedAt: t.LastAccessedAt,
	}
}

// TenantsWithRoleToResponses converts a slice of TenantWithRole entities to response DTOs.
func TenantsWithRoleToResponses(tenants []*entity.TenantWithRole) []*dto.TenantWithRoleResponse {
	result := make([]*dto.TenantWithRoleResponse, len(tenants))
	for i, t := range tenants {
		result[i] = TenantWithRoleToResponse(t)
	}
	return result
}

// CreateTenantRequestToCommand converts a create request to a usecase command.
func CreateTenantRequestToCommand(req dto.CreateTenantRequest) organizationuc.CreateTenantCommand {
	return organizationuc.CreateTenantCommand{
		Name:        req.Name,
		Code:        req.Code,
		Description: req.Description,
	}
}

// UpdateTenantRequestToCommand converts an update request to a usecase command.
func UpdateTenantRequestToCommand(id string, req dto.UpdateTenantRequest) organizationuc.UpdateTenantCommand {
	return organizationuc.UpdateTenantCommand{
		ID:          id,
		Name:        req.Name,
		Description: req.Description,
		Settings:    req.Settings,
	}
}

// TenantListRequestToFilters converts a list request to port filters.
func TenantListRequestToFilters(req dto.TenantListRequest) port.TenantFilters {
	offset := (req.Page - 1) * req.PerPage
	return port.TenantFilters{
		Limit:  req.PerPage,
		Offset: offset,
		Query:  req.Query,
	}
}

// TenantsToPaginatedResponse converts tenants to a paginated response.
func TenantsToPaginatedResponse(tenants []*entity.Tenant, total int64, page, perPage int) *dto.PaginatedTenantsResponse {
	responses := TenantsToResponses(tenants)

	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}

	return &dto.PaginatedTenantsResponse{
		Data: responses,
		Pagination: dto.PaginationMeta{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	}
}

// TenantMemberListRequestToFilters converts a list request to tenant member filters.
func TenantMemberListRequestToFilters(req dto.TenantListRequest) port.TenantMemberFilters {
	offset := (req.Page - 1) * req.PerPage
	return port.TenantMemberFilters{
		Limit:  req.PerPage,
		Offset: offset,
		Query:  req.Query,
	}
}

// TenantsWithRoleToPaginatedResponse converts tenants with roles to a paginated response.
func TenantsWithRoleToPaginatedResponse(tenants []*entity.TenantWithRole, total int64, page, perPage int) *dto.PaginatedTenantsWithRoleResponse {
	responses := TenantsWithRoleToResponses(tenants)

	totalPages := int(total) / perPage
	if int(total)%perPage > 0 {
		totalPages++
	}

	return &dto.PaginatedTenantsWithRoleResponse{
		Data: responses,
		Pagination: dto.PaginationMeta{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	}
}
