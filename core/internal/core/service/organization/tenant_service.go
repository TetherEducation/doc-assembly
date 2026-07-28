package organization

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	organizationuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/organization"
)

// NewTenantService creates a new tenant service.
func NewTenantService(
	tenantRepo port.TenantRepository,
	workspaceRepo port.WorkspaceRepository,
	tenantMemberRepo port.TenantMemberRepository,
	systemRoleRepo port.SystemRoleRepository,
	accessHistoryRepo port.UserAccessHistoryRepository,
) organizationuc.TenantUseCase {
	return &TenantService{
		tenantRepo:        tenantRepo,
		workspaceRepo:     workspaceRepo,
		tenantMemberRepo:  tenantMemberRepo,
		systemRoleRepo:    systemRoleRepo,
		accessHistoryRepo: accessHistoryRepo,
	}
}

// TenantService implements tenant business logic.
type TenantService struct {
	tenantRepo        port.TenantRepository
	workspaceRepo     port.WorkspaceRepository
	tenantMemberRepo  port.TenantMemberRepository
	systemRoleRepo    port.SystemRoleRepository
	accessHistoryRepo port.UserAccessHistoryRepository
}

// CreateTenant creates a new tenant. System workspace is auto-created via DB trigger.
func (s *TenantService) CreateTenant(ctx context.Context, cmd organizationuc.CreateTenantCommand) (*entity.Tenant, error) {
	// Check if tenant code already exists
	exists, err := s.tenantRepo.ExistsByCode(ctx, cmd.Code)
	if err != nil {
		return nil, fmt.Errorf("checking tenant code existence: %w", err)
	}
	if exists {
		return nil, entity.ErrTenantAlreadyExists
	}

	tenant := &entity.Tenant{
		ID:          uuid.NewString(),
		Name:        cmd.Name,
		Code:        cmd.Code,
		Description: cmd.Description,
		Status:      entity.TenantStatusActive,
		Settings:    entity.TenantSettings{},
		CreatedAt:   time.Now().UTC(),
	}

	if err := tenant.Validate(); err != nil {
		return nil, fmt.Errorf("validating tenant: %w", err)
	}

	id, err := s.tenantRepo.Create(ctx, tenant)
	if err != nil {
		return nil, fmt.Errorf("creating tenant: %w", err)
	}
	tenant.ID = id

	slog.InfoContext(ctx, "tenant created",
		slog.String("tenant_id", tenant.ID),
		slog.String("code", tenant.Code),
		slog.String("name", tenant.Name),
	)

	return tenant, nil
}

// GetTenant retrieves a tenant by ID.
func (s *TenantService) GetTenant(ctx context.Context, id string) (*entity.Tenant, error) {
	tenant, err := s.tenantRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding tenant %s: %w", id, err)
	}
	return tenant, nil
}

// GetTenantByCode retrieves a tenant by its code.
func (s *TenantService) GetTenantByCode(ctx context.Context, code string) (*entity.Tenant, error) {
	tenant, err := s.tenantRepo.FindByCode(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("finding tenant by code %s: %w", code, err)
	}
	return tenant, nil
}

// SearchTenants searches tenants by name or code similarity.
func (s *TenantService) SearchTenants(ctx context.Context, query string) ([]*entity.Tenant, error) {
	const maxResults = 10
	tenants, err := s.tenantRepo.SearchByNameOrCode(ctx, query, maxResults)
	if err != nil {
		return nil, fmt.Errorf("searching tenants: %w", err)
	}
	return tenants, nil
}

// ListTenantsPaginated lists tenants with pagination.
func (s *TenantService) ListTenantsPaginated(ctx context.Context, filters port.TenantFilters) ([]*entity.Tenant, int64, error) {
	tenants, total, err := s.tenantRepo.FindAllPaginated(ctx, filters)
	if err != nil {
		return nil, 0, fmt.Errorf("listing tenants paginated: %w", err)
	}
	return tenants, total, nil
}

// ListTenantWorkspaces lists workspaces for a tenant with optional search (system admin use).
func (s *TenantService) ListTenantWorkspaces(ctx context.Context, tenantID string, filters port.WorkspaceFilters) ([]*entity.Workspace, int64, error) {
	// Verify tenant exists
	if _, err := s.tenantRepo.FindByID(ctx, tenantID); err != nil {
		return nil, 0, err
	}
	return s.workspaceRepo.FindByTenantPaginated(ctx, tenantID, filters)
}

// ListUserTenants lists all tenants a user belongs to with their roles.
func (s *TenantService) ListUserTenants(ctx context.Context, userID string) ([]*entity.TenantWithRole, error) {
	tenants, err := s.tenantMemberRepo.FindTenantsWithRoleByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing user tenants: %w", err)
	}
	return tenants, nil
}

// ListUserTenantsPaginated lists tenants a user belongs to with pagination and optional search.
// If the user has a system role (SUPERADMIN or PLATFORM_ADMIN), returns all tenants.
func (s *TenantService) ListUserTenantsPaginated(ctx context.Context, userID string, filters port.TenantMemberFilters) ([]*entity.TenantWithRole, int64, error) {
	var tenants []*entity.TenantWithRole
	var total int64

	// Check if user has a system role
	systemRoleAssignment, err := s.systemRoleRepo.FindByUserID(ctx, userID)
	if err == nil && systemRoleAssignment != nil {
		virtualRole := s.getVirtualTenantRole(systemRoleAssignment.Role)
		if virtualRole != "" {
			tenantFilters := port.TenantFilters{
				Limit:  filters.Limit,
				Offset: filters.Offset,
				UserID: userID,
				Query:  filters.Query,
			}
			allTenants, t, err := s.tenantRepo.FindAllPaginated(ctx, tenantFilters)
			if err != nil {
				return nil, 0, fmt.Errorf("listing all tenants paginated: %w", err)
			}
			tenants = s.mapTenantsWithRole(allTenants, virtualRole)
			total = t
		}
	}

	// Regular users: list only their tenants
	if tenants == nil {
		var err error
		tenants, total, err = s.tenantMemberRepo.FindTenantsWithRoleByUserPaginated(ctx, userID, filters)
		if err != nil {
			return nil, 0, fmt.Errorf("listing user tenants paginated: %w", err)
		}
	}

	// Enrich with access history
	if err := s.enrichTenantsWithAccessHistory(ctx, userID, tenants); err != nil {
		slog.WarnContext(ctx, "failed to enrich tenants with access history", slog.String("error", err.Error()))
	}

	return tenants, total, nil
}

// UpdateTenant updates a tenant's details.
func (s *TenantService) UpdateTenant(ctx context.Context, cmd organizationuc.UpdateTenantCommand) (*entity.Tenant, error) {
	tenant, err := s.tenantRepo.FindByID(ctx, cmd.ID)
	if err != nil {
		return nil, fmt.Errorf("finding tenant: %w", err)
	}

	tenant.Name = cmd.Name
	tenant.Description = cmd.Description

	// Update settings if provided
	if cmd.Settings != nil {
		applySettingsUpdates(cmd.Settings, &tenant.Settings)
	}

	now := time.Now().UTC()
	tenant.UpdatedAt = &now

	if err := tenant.Validate(); err != nil {
		return nil, fmt.Errorf("validating tenant: %w", err)
	}

	if err := s.tenantRepo.Update(ctx, tenant); err != nil {
		return nil, fmt.Errorf("updating tenant: %w", err)
	}

	slog.InfoContext(ctx, "tenant updated",
		slog.String("tenant_id", tenant.ID),
		slog.String("name", tenant.Name),
	)

	return tenant, nil
}

// UpdateTenantStatus updates a tenant's status (ACTIVE, SUSPENDED, ARCHIVED).
func (s *TenantService) UpdateTenantStatus(ctx context.Context, cmd organizationuc.UpdateTenantStatusCommand) (*entity.Tenant, error) {
	tenant, err := s.tenantRepo.FindByID(ctx, cmd.ID)
	if err != nil {
		return nil, fmt.Errorf("finding tenant: %w", err)
	}

	// System tenant cannot have its status changed
	if tenant.IsSystem {
		return nil, entity.ErrCannotModifySystemTenant
	}

	// Validate status
	if !cmd.Status.IsValid() {
		return nil, entity.ErrInvalidTenantStatus
	}

	now := time.Now().UTC()
	if err := s.tenantRepo.UpdateStatus(ctx, cmd.ID, cmd.Status, &now); err != nil {
		return nil, fmt.Errorf("updating tenant status: %w", err)
	}

	tenant.Status = cmd.Status
	tenant.UpdatedAt = &now

	slog.InfoContext(ctx, "tenant status updated",
		slog.String("tenant_id", tenant.ID),
		slog.String("status", string(cmd.Status)),
	)

	return tenant, nil
}

// DeleteTenant deletes a tenant and all its data.
func (s *TenantService) DeleteTenant(ctx context.Context, id string) error {
	// Check if tenant exists
	_, err := s.tenantRepo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("finding tenant: %w", err)
	}

	// Delete tenant (cascade should handle workspaces)
	if err := s.tenantRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("deleting tenant: %w", err)
	}

	slog.InfoContext(ctx, "tenant deleted", slog.String("tenant_id", id))
	return nil
}

// getVirtualTenantRole returns the virtual tenant role for a system role.
func (s *TenantService) getVirtualTenantRole(role entity.SystemRole) entity.TenantRole {
	switch role {
	case entity.SystemRoleSuperAdmin:
		return entity.TenantRoleOwner
	case entity.SystemRolePlatformAdmin:
		return entity.TenantRoleAdmin
	default:
		return ""
	}
}

// mapTenantsWithRole converts Tenant slice to TenantWithRole with virtual role.
func (s *TenantService) mapTenantsWithRole(tenants []*entity.Tenant, role entity.TenantRole) []*entity.TenantWithRole {
	result := make([]*entity.TenantWithRole, len(tenants))
	for i, t := range tenants {
		result[i] = &entity.TenantWithRole{
			Tenant: t,
			Role:   role,
		}
	}
	return result
}

// enrichTenantsWithAccessHistory adds LastAccessedAt to tenants.
func (s *TenantService) enrichTenantsWithAccessHistory(ctx context.Context, userID string, tenants []*entity.TenantWithRole) error {
	if len(tenants) == 0 {
		return nil
	}

	// Extract tenant IDs
	ids := make([]string, len(tenants))
	for i, t := range tenants {
		ids[i] = t.Tenant.ID
	}

	// Get access times
	accessTimes, err := s.accessHistoryRepo.GetAccessTimesForEntities(ctx, userID, entity.AccessEntityTypeTenant, ids)
	if err != nil {
		return fmt.Errorf("getting access times: %w", err)
	}

	// Enrich tenants
	for _, t := range tenants {
		if accessedAt, ok := accessTimes[t.Tenant.ID]; ok {
			t.LastAccessedAt = &accessedAt
		}
	}

	return nil
}

// applySettingsUpdates updates tenant settings from a map of values.
func applySettingsUpdates(settings map[string]any, tenantSettings *entity.TenantSettings) {
	if currency, ok := settings["currency"].(string); ok {
		tenantSettings.Currency = currency
	}
	if timezone, ok := settings["timezone"].(string); ok {
		tenantSettings.Timezone = timezone
	}
	if dateFormat, ok := settings["dateFormat"].(string); ok {
		tenantSettings.DateFormat = dateFormat
	}
	if locale, ok := settings["locale"].(string); ok {
		tenantSettings.Locale = locale
	}
}
