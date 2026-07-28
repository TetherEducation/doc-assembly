package organization

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	organizationuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/organization"
)

// NewWorkspaceService creates a new workspace service.
func NewWorkspaceService(
	workspaceRepo port.WorkspaceRepository,
	tenantRepo port.TenantRepository,
	memberRepo port.WorkspaceMemberRepository,
	tenantMemberRepo port.TenantMemberRepository,
	systemRoleRepo port.SystemRoleRepository,
	accessHistoryRepo port.UserAccessHistoryRepository,
) organizationuc.WorkspaceUseCase {
	return &WorkspaceService{
		workspaceRepo:     workspaceRepo,
		tenantRepo:        tenantRepo,
		memberRepo:        memberRepo,
		tenantMemberRepo:  tenantMemberRepo,
		systemRoleRepo:    systemRoleRepo,
		accessHistoryRepo: accessHistoryRepo,
	}
}

// WorkspaceService implements workspace business logic.
type WorkspaceService struct {
	workspaceRepo     port.WorkspaceRepository
	tenantRepo        port.TenantRepository
	memberRepo        port.WorkspaceMemberRepository
	tenantMemberRepo  port.TenantMemberRepository
	systemRoleRepo    port.SystemRoleRepository
	accessHistoryRepo port.UserAccessHistoryRepository
}

// CreateWorkspace creates a new workspace.
func (s *WorkspaceService) CreateWorkspace(ctx context.Context, cmd organizationuc.CreateWorkspaceCommand) (*entity.Workspace, error) {
	// For SYSTEM type, check if one already exists for the tenant
	if cmd.Type == entity.WorkspaceTypeSystem {
		exists, err := s.workspaceRepo.ExistsSystemForTenant(ctx, cmd.TenantID)
		if err != nil {
			return nil, fmt.Errorf("checking system workspace existence: %w", err)
		}
		if exists {
			return nil, entity.ErrSystemWorkspaceExists
		}
		// Auto-set code for SYSTEM workspaces
		cmd.Code = "SYS_WRKSP"
	}

	// Check code uniqueness within tenant
	if err := s.checkCodeUniqueness(ctx, cmd.TenantID, cmd.Code, ""); err != nil {
		return nil, err
	}

	workspace := &entity.Workspace{
		ID:        uuid.NewString(),
		TenantID:  cmd.TenantID,
		Name:      cmd.Name,
		Code:      cmd.Code,
		Type:      cmd.Type,
		Status:    entity.WorkspaceStatusActive,
		CreatedAt: time.Now().UTC(),
	}

	if err := workspace.Validate(); err != nil {
		return nil, fmt.Errorf("validating workspace: %w", err)
	}

	id, err := s.workspaceRepo.Create(ctx, workspace)
	if err != nil {
		return nil, fmt.Errorf("creating workspace: %w", err)
	}
	workspace.ID = id
	workspace.CurrentRole = entity.WorkspaceRoleOwner

	slog.InfoContext(ctx, "workspace created",
		slog.String("workspace_id", workspace.ID),
		slog.String("name", workspace.Name),
		slog.String("code", workspace.Code),
		slog.String("type", string(workspace.Type)),
	)

	return workspace, nil
}

// GetWorkspace retrieves a workspace by ID.
func (s *WorkspaceService) GetWorkspace(ctx context.Context, id string) (*entity.Workspace, error) {
	workspace, err := s.workspaceRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding workspace %s: %w", id, err)
	}
	return workspace, nil
}

// ListUserWorkspaces lists all workspaces a user has access to.
func (s *WorkspaceService) ListUserWorkspaces(ctx context.Context, userID string) ([]*entity.WorkspaceWithRole, error) {
	workspaces, err := s.workspaceRepo.FindByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing user workspaces: %w", err)
	}
	return workspaces, nil
}

// ListWorkspacesPaginated lists workspaces for a tenant with pagination and optional search.
func (s *WorkspaceService) ListWorkspacesPaginated(ctx context.Context, tenantID, userID string, filters port.WorkspaceFilters) ([]*entity.Workspace, int64, error) {
	filters.UserID = userID
	workspaces, total, err := s.workspaceRepo.FindByTenantPaginated(ctx, tenantID, filters)
	if err != nil {
		return nil, 0, fmt.Errorf("listing workspaces paginated: %w", err)
	}

	if err := s.enrichWorkspacesWithEffectiveRole(ctx, tenantID, userID, workspaces); err != nil {
		return nil, 0, fmt.Errorf("resolving effective workspace roles: %w", err)
	}

	// Enrich with access history
	if err := s.enrichWorkspacesWithAccessHistory(ctx, userID, workspaces); err != nil {
		slog.WarnContext(ctx, "failed to enrich workspaces with access history", slog.String("error", err.Error()))
	}

	return workspaces, total, nil
}

func (s *WorkspaceService) enrichWorkspacesWithEffectiveRole(ctx context.Context, tenantID, userID string, workspaces []*entity.Workspace) error {
	if len(workspaces) == 0 {
		return nil
	}

	isSuperAdmin, err := s.isUserSuperAdmin(ctx, userID)
	if err != nil {
		return err
	}
	if isSuperAdmin {
		for _, workspace := range workspaces {
			workspace.CurrentRole = entity.WorkspaceRoleOwner
		}
		return nil
	}

	isTenantOwner, err := s.isUserTenantOwner(ctx, tenantID, userID)
	if err != nil {
		return err
	}
	if isTenantOwner {
		for _, workspace := range workspaces {
			workspace.CurrentRole = entity.WorkspaceRoleOwner
		}
		return nil
	}

	directRoles, err := s.loadActiveWorkspaceRoles(ctx, userID)
	if err != nil {
		return err
	}

	for _, workspace := range workspaces {
		if role, ok := directRoles[workspace.ID]; ok {
			workspace.CurrentRole = role
			continue
		}
		workspace.CurrentRole = ""
	}
	return nil
}

func (s *WorkspaceService) isUserSuperAdmin(ctx context.Context, userID string) (bool, error) {
	systemRole, err := s.systemRoleRepo.FindByUserID(ctx, userID)
	if err == nil {
		return systemRole.Role.HasPermission(entity.SystemRoleSuperAdmin), nil
	}
	if errors.Is(err, entity.ErrSystemRoleNotFound) {
		return false, nil
	}
	return false, fmt.Errorf("finding system role: %w", err)
}

func (s *WorkspaceService) isUserTenantOwner(ctx context.Context, tenantID, userID string) (bool, error) {
	tenantMember, err := s.tenantMemberRepo.FindActiveByUserAndTenant(ctx, userID, tenantID)
	if err == nil {
		return tenantMember.Role.HasPermission(entity.TenantRoleOwner), nil
	}
	if errors.Is(err, entity.ErrTenantMemberNotFound) {
		return false, nil
	}
	return false, fmt.Errorf("finding tenant membership: %w", err)
}

func (s *WorkspaceService) loadActiveWorkspaceRoles(ctx context.Context, userID string) (map[string]entity.WorkspaceRole, error) {
	memberships, err := s.memberRepo.FindByUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("listing user workspace memberships: %w", err)
	}

	directRoles := make(map[string]entity.WorkspaceRole, len(memberships))
	for _, membership := range memberships {
		if membership.MembershipStatus != entity.MembershipStatusActive {
			continue
		}
		directRoles[membership.WorkspaceID] = membership.Role
	}

	return directRoles, nil
}

// UpdateWorkspace updates a workspace's details.
func (s *WorkspaceService) UpdateWorkspace(ctx context.Context, cmd organizationuc.UpdateWorkspaceCommand) (*entity.Workspace, error) {
	workspace, err := s.workspaceRepo.FindByID(ctx, cmd.ID)
	if err != nil {
		return nil, fmt.Errorf("finding workspace: %w", err)
	}

	if cmd.Name != nil {
		workspace.Name = *cmd.Name
	}

	// Handle code update if requested
	if err := s.applyCodeUpdate(ctx, workspace, cmd.Code); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	workspace.UpdatedAt = &now

	if err := workspace.Validate(); err != nil {
		return nil, fmt.Errorf("validating workspace: %w", err)
	}

	if err := s.workspaceRepo.Update(ctx, workspace); err != nil {
		return nil, fmt.Errorf("updating workspace: %w", err)
	}

	slog.InfoContext(ctx, "workspace updated",
		slog.String("workspace_id", workspace.ID),
		slog.String("name", workspace.Name),
		slog.String("code", workspace.Code),
	)

	return workspace, nil
}

// applyCodeUpdate validates and applies a code change to the workspace.
// Returns nil if code is nil or unchanged.
func (s *WorkspaceService) applyCodeUpdate(ctx context.Context, workspace *entity.Workspace, code *string) error {
	if code == nil || *code == workspace.Code {
		return nil
	}
	if workspace.Type == entity.WorkspaceTypeSystem {
		return entity.ErrCannotModifySystemWorkspace
	}
	if err := s.checkCodeUniqueness(ctx, workspace.TenantID, *code, workspace.ID); err != nil {
		return err
	}
	workspace.Code = *code
	return nil
}

// checkCodeUniqueness verifies that no other workspace in the tenant uses the given code.
// excludeID allows excluding a workspace from the check (for updates).
func (s *WorkspaceService) checkCodeUniqueness(ctx context.Context, tenantID *string, code, excludeID string) error {
	if tenantID == nil {
		return nil
	}
	exists, err := s.workspaceRepo.ExistsByCodeForTenant(ctx, *tenantID, code, excludeID)
	if err != nil {
		return fmt.Errorf("checking workspace code: %w", err)
	}
	if exists {
		return entity.ErrWorkspaceCodeExists
	}
	return nil
}

// ArchiveWorkspace archives a workspace (soft delete).
func (s *WorkspaceService) ArchiveWorkspace(ctx context.Context, id string) error {
	workspace, err := s.workspaceRepo.FindByID(ctx, id)
	if err != nil {
		return fmt.Errorf("finding workspace: %w", err)
	}

	if workspace.Type == entity.WorkspaceTypeSystem {
		return entity.ErrCannotArchiveSystem
	}

	if err := s.workspaceRepo.UpdateStatus(ctx, id, entity.WorkspaceStatusArchived); err != nil {
		return fmt.Errorf("archiving workspace: %w", err)
	}

	slog.InfoContext(ctx, "workspace archived", slog.String("workspace_id", id))
	return nil
}

// ActivateWorkspace activates a workspace.
func (s *WorkspaceService) ActivateWorkspace(ctx context.Context, id string) error {
	if err := s.workspaceRepo.UpdateStatus(ctx, id, entity.WorkspaceStatusActive); err != nil {
		return fmt.Errorf("activating workspace: %w", err)
	}

	slog.InfoContext(ctx, "workspace activated", slog.String("workspace_id", id))
	return nil
}

// GetSystemWorkspace retrieves the system workspace for a tenant.
func (s *WorkspaceService) GetSystemWorkspace(ctx context.Context, tenantID *string) (*entity.Workspace, error) {
	workspace, err := s.workspaceRepo.FindSystemByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("finding system workspace: %w", err)
	}
	return workspace, nil
}

// UpdateWorkspaceStatus updates a workspace's status (ACTIVE, SUSPENDED, ARCHIVED).
func (s *WorkspaceService) UpdateWorkspaceStatus(ctx context.Context, cmd organizationuc.UpdateWorkspaceStatusCommand) (*entity.Workspace, error) {
	workspace, err := s.workspaceRepo.FindByID(ctx, cmd.ID)
	if err != nil {
		return nil, fmt.Errorf("finding workspace: %w", err)
	}

	// Cannot change status of SYSTEM workspace
	if workspace.Type == entity.WorkspaceTypeSystem {
		return nil, entity.ErrCannotModifySystemWorkspace
	}

	if err := s.workspaceRepo.UpdateStatus(ctx, cmd.ID, cmd.Status); err != nil {
		return nil, fmt.Errorf("updating workspace status: %w", err)
	}

	workspace.Status = cmd.Status
	now := time.Now().UTC()
	workspace.UpdatedAt = &now

	slog.InfoContext(ctx, "workspace status updated",
		slog.String("workspace_id", cmd.ID),
		slog.String("status", string(cmd.Status)),
	)

	return workspace, nil
}

// enrichWorkspacesWithAccessHistory adds LastAccessedAt to workspaces.
func (s *WorkspaceService) enrichWorkspacesWithAccessHistory(ctx context.Context, userID string, workspaces []*entity.Workspace) error {
	if len(workspaces) == 0 {
		return nil
	}

	// Extract workspace IDs
	ids := make([]string, len(workspaces))
	for i, w := range workspaces {
		ids[i] = w.ID
	}

	// Get access times
	accessTimes, err := s.accessHistoryRepo.GetAccessTimesForEntities(ctx, userID, entity.AccessEntityTypeWorkspace, ids)
	if err != nil {
		return fmt.Errorf("getting access times: %w", err)
	}

	// Enrich workspaces
	for _, w := range workspaces {
		if accessedAt, ok := accessTimes[w.ID]; ok {
			w.LastAccessedAt = &accessedAt
		}
	}

	return nil
}
