package organization

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	organizationuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/organization"
)

// NewWorkspaceMemberService creates a new workspace member service.
func NewWorkspaceMemberService(
	memberRepo port.WorkspaceMemberRepository,
	userRepo port.UserRepository,
	systemRoleRepo port.SystemRoleRepository,
	workspaceRepo port.WorkspaceRepository,
	tenantRepo port.TenantRepository,
	tenantMemberRepo port.TenantMemberRepository,
) organizationuc.WorkspaceMemberUseCase {
	return &WorkspaceMemberService{
		memberRepo:       memberRepo,
		userRepo:         userRepo,
		systemRoleRepo:   systemRoleRepo,
		workspaceRepo:    workspaceRepo,
		tenantRepo:       tenantRepo,
		tenantMemberRepo: tenantMemberRepo,
	}
}

// WorkspaceMemberService implements workspace member business logic.
type WorkspaceMemberService struct {
	memberRepo       port.WorkspaceMemberRepository
	userRepo         port.UserRepository
	systemRoleRepo   port.SystemRoleRepository
	workspaceRepo    port.WorkspaceRepository
	tenantRepo       port.TenantRepository
	tenantMemberRepo port.TenantMemberRepository
}

// ListMembers lists all members of a workspace.
func (s *WorkspaceMemberService) ListMembers(ctx context.Context, workspaceID string) ([]*entity.MemberWithUser, error) {
	members, err := s.memberRepo.FindByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("listing workspace members: %w", err)
	}
	return members, nil
}

// GetMember retrieves a specific member by ID.
func (s *WorkspaceMemberService) GetMember(ctx context.Context, memberID string) (*entity.MemberWithUser, error) {
	member, err := s.memberRepo.FindByID(ctx, memberID)
	if err != nil {
		return nil, fmt.Errorf("finding member: %w", err)
	}

	user, err := s.userRepo.FindByID(ctx, member.UserID)
	if err != nil {
		return nil, fmt.Errorf("finding user: %w", err)
	}

	return &entity.MemberWithUser{
		WorkspaceMember: *member,
		User:            user,
	}, nil
}

// InviteMember invites a user to join a workspace.
// Creates a shadow user if the email doesn't exist.
func (s *WorkspaceMemberService) InviteMember(ctx context.Context, cmd organizationuc.InviteMemberCommand) (*entity.MemberWithUser, error) {
	if cmd.Role == entity.WorkspaceRoleOwner {
		return nil, entity.ErrInvalidRole
	}

	user, err := s.findOrCreateUser(ctx, cmd.Email, cmd.FullName)
	if err != nil {
		return nil, err
	}

	// Check if user is already a member
	_, err = s.memberRepo.FindByUserAndWorkspace(ctx, user.ID, cmd.WorkspaceID)
	if err == nil {
		return nil, entity.ErrMemberAlreadyExists
	}
	if !errors.Is(err, entity.ErrMemberNotFound) {
		return nil, fmt.Errorf("checking existing membership: %w", err)
	}

	// Determine membership status based on user status
	var member *entity.WorkspaceMember
	if user.IsLinkedToIdP() {
		// User already has IdP account - activate immediately
		member = entity.NewActiveMember(cmd.WorkspaceID, user.ID, cmd.Role)
	} else {
		// Create pending membership
		member = entity.NewWorkspaceMember(cmd.WorkspaceID, user.ID, cmd.Role, &cmd.InvitedBy)
	}

	member.ID = uuid.NewString()

	if err := member.Validate(); err != nil {
		return nil, fmt.Errorf("validating membership: %w", err)
	}

	id, err := s.memberRepo.Create(ctx, member)
	if err != nil {
		return nil, fmt.Errorf("creating membership: %w", err)
	}
	member.ID = id

	slog.InfoContext(ctx, "member invited",
		slog.String("member_id", member.ID),
		slog.String("workspace_id", cmd.WorkspaceID),
		slog.String("email", cmd.Email),
		slog.String("role", string(cmd.Role)),
	)

	return &entity.MemberWithUser{
		WorkspaceMember: *member,
		User:            user,
	}, nil
}

// findOrCreateUser finds a user by email or creates a shadow user if not found.
func (s *WorkspaceMemberService) findOrCreateUser(ctx context.Context, email, fullName string) (*entity.User, error) {
	user, err := s.userRepo.FindByEmail(ctx, email)
	if err == nil {
		return user, nil
	}

	if !errors.Is(err, entity.ErrUserNotFound) {
		return nil, fmt.Errorf("finding user by email: %w", err)
	}

	user = entity.NewUser(email, fullName)
	user.ID = uuid.NewString()

	if _, err = s.userRepo.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("creating shadow user: %w", err)
	}

	slog.InfoContext(ctx, "shadow user created",
		slog.String("user_id", user.ID),
		slog.String("email", user.Email),
	)

	return user, nil
}

// UpdateMemberRole updates a member's role within the workspace.
func (s *WorkspaceMemberService) UpdateMemberRole(ctx context.Context, cmd organizationuc.UpdateMemberRoleCommand) (*entity.MemberWithUser, error) {
	member, err := s.memberRepo.FindByID(ctx, cmd.MemberID)
	if err != nil {
		return nil, fmt.Errorf("finding member: %w", err)
	}

	// Verify workspace match
	if member.WorkspaceID != cmd.WorkspaceID {
		return nil, entity.ErrMemberNotFound
	}

	// Cannot change role to or from OWNER
	if cmd.NewRole == entity.WorkspaceRoleOwner || member.Role == entity.WorkspaceRoleOwner {
		return nil, entity.ErrInvalidRole
	}

	// Update role
	if err := s.memberRepo.UpdateRole(ctx, cmd.MemberID, cmd.NewRole); err != nil {
		return nil, fmt.Errorf("updating member role: %w", err)
	}

	member.Role = cmd.NewRole

	user, err := s.userRepo.FindByID(ctx, member.UserID)
	if err != nil {
		return nil, fmt.Errorf("finding user: %w", err)
	}

	slog.InfoContext(ctx, "member role updated",
		slog.String("member_id", cmd.MemberID),
		slog.String("new_role", string(cmd.NewRole)),
		slog.String("updated_by", cmd.UpdatedBy),
	)

	return &entity.MemberWithUser{
		WorkspaceMember: *member,
		User:            user,
	}, nil
}

// RemoveMember removes a member from the workspace.
func (s *WorkspaceMemberService) RemoveMember(ctx context.Context, cmd organizationuc.RemoveMemberCommand) error {
	member, err := s.memberRepo.FindByID(ctx, cmd.MemberID)
	if err != nil {
		return fmt.Errorf("finding member: %w", err)
	}

	// Verify workspace match
	if member.WorkspaceID != cmd.WorkspaceID {
		return entity.ErrMemberNotFound
	}

	workspace, err := s.workspaceRepo.FindByID(ctx, cmd.WorkspaceID)
	if err != nil {
		return fmt.Errorf("finding workspace: %w", err)
	}

	if err := s.ensureGlobalSystemWorkspaceCanRemoveMember(ctx, workspace, member); err != nil {
		return err
	}

	// Cannot remove owner
	if member.Role == entity.WorkspaceRoleOwner {
		canRemoveOwner, err := s.canRemoveOwnerMembership(ctx, workspace, cmd.RemovedBy)
		if err != nil {
			return err
		}
		if !canRemoveOwner {
			return entity.ErrCannotRemoveOwner
		}
	}

	if err := s.memberRepo.Delete(ctx, cmd.MemberID); err != nil {
		return fmt.Errorf("removing member: %w", err)
	}

	slog.InfoContext(ctx, "member removed",
		slog.String("member_id", cmd.MemberID),
		slog.String("workspace_id", cmd.WorkspaceID),
		slog.String("removed_by", cmd.RemovedBy),
	)

	return nil
}

func (s *WorkspaceMemberService) ensureGlobalSystemWorkspaceCanRemoveMember(
	ctx context.Context,
	workspace *entity.Workspace,
	member *entity.WorkspaceMember,
) error {
	if member.MembershipStatus != entity.MembershipStatusActive {
		return nil
	}
	if !workspace.IsSystem() {
		return nil
	}

	isGlobalSystem, err := s.isGlobalSystemWorkspace(ctx, workspace)
	if err != nil {
		return err
	}
	if !isGlobalSystem {
		return nil
	}

	activeMembers, err := s.memberRepo.CountActiveByWorkspace(ctx, workspace.ID)
	if err != nil {
		return fmt.Errorf("counting workspace members: %w", err)
	}
	if activeMembers <= 1 {
		return entity.ErrCannotRemoveLastSystemMember
	}

	return nil
}

func (s *WorkspaceMemberService) canRemoveOwnerMembership(
	ctx context.Context,
	workspace *entity.Workspace,
	removedBy string,
) (bool, error) {
	systemRole, err := s.systemRoleRepo.FindByUserID(ctx, removedBy)
	if err != nil && !errors.Is(err, entity.ErrSystemRoleNotFound) {
		return false, fmt.Errorf("finding remover system role: %w", err)
	}
	if err == nil && systemRole.Role.HasPermission(entity.SystemRoleSuperAdmin) {
		return true, nil
	}

	if workspace.TenantID == nil {
		return false, nil
	}

	tenantMember, err := s.tenantMemberRepo.FindActiveByUserAndTenant(ctx, removedBy, *workspace.TenantID)
	if err != nil {
		if errors.Is(err, entity.ErrTenantMemberNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("finding remover tenant membership: %w", err)
	}

	return tenantMember.Role == entity.TenantRoleOwner, nil
}

func (s *WorkspaceMemberService) isGlobalSystemWorkspace(ctx context.Context, workspace *entity.Workspace) (bool, error) {
	if !workspace.IsSystem() || workspace.TenantID == nil || *workspace.TenantID == "" {
		return false, nil
	}

	tenant, err := s.tenantRepo.FindByID(ctx, *workspace.TenantID)
	if err != nil {
		return false, fmt.Errorf("finding tenant: %w", err)
	}

	return tenant.IsSystem, nil
}
