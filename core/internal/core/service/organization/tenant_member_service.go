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

// NewTenantMemberService creates a new tenant member service.
func NewTenantMemberService(
	memberRepo port.TenantMemberRepository,
	userRepo port.UserRepository,
	tenantRepo port.TenantRepository,
) organizationuc.TenantMemberUseCase {
	return &TenantMemberService{
		memberRepo: memberRepo,
		userRepo:   userRepo,
		tenantRepo: tenantRepo,
	}
}

// TenantMemberService implements tenant member business logic.
type TenantMemberService struct {
	memberRepo port.TenantMemberRepository
	userRepo   port.UserRepository
	tenantRepo port.TenantRepository
}

// ListMembers lists all members of a tenant.
func (s *TenantMemberService) ListMembers(ctx context.Context, tenantID string) ([]*entity.TenantMemberWithUser, error) {
	members, err := s.memberRepo.FindByTenant(ctx, tenantID)
	if err != nil {
		return nil, fmt.Errorf("listing tenant members: %w", err)
	}
	return members, nil
}

// GetMember retrieves a specific tenant member by ID.
func (s *TenantMemberService) GetMember(ctx context.Context, memberID string) (*entity.TenantMemberWithUser, error) {
	member, err := s.memberRepo.FindByID(ctx, memberID)
	if err != nil {
		return nil, fmt.Errorf("finding member: %w", err)
	}

	user, err := s.userRepo.FindByID(ctx, member.UserID)
	if err != nil {
		return nil, fmt.Errorf("finding user: %w", err)
	}

	return &entity.TenantMemberWithUser{
		TenantMember: *member,
		User:         user,
	}, nil
}

// AddMember adds a user to a tenant.
// Creates a shadow user if the email doesn't exist.
func (s *TenantMemberService) AddMember(ctx context.Context, cmd organizationuc.AddTenantMemberCommand) (*entity.TenantMemberWithUser, error) {
	// Find or create user by email
	user, err := s.userRepo.FindByEmail(ctx, cmd.Email)
	if err != nil {
		if !errors.Is(err, entity.ErrUserNotFound) {
			return nil, fmt.Errorf("finding user by email: %w", err)
		}

		// Create shadow user
		user = entity.NewUser(cmd.Email, cmd.FullName)
		user.ID = uuid.NewString()
		_, err = s.userRepo.Create(ctx, user)
		if err != nil {
			return nil, fmt.Errorf("creating shadow user: %w", err)
		}

		slog.InfoContext(ctx, "shadow user created for tenant",
			slog.String("user_id", user.ID),
			slog.String("email", user.Email),
			slog.String("tenant_id", cmd.TenantID),
		)
	}

	// Check if user is already a member
	_, err = s.memberRepo.FindByUserAndTenant(ctx, user.ID, cmd.TenantID)
	if err == nil {
		return nil, entity.ErrTenantMemberExists
	}
	if !errors.Is(err, entity.ErrTenantMemberNotFound) {
		return nil, fmt.Errorf("checking existing membership: %w", err)
	}

	// Create tenant membership (always ACTIVE for tenant members)
	member := entity.NewTenantMember(cmd.TenantID, user.ID, cmd.Role, &cmd.GrantedBy)
	member.ID = uuid.NewString()

	if err := member.Validate(); err != nil {
		return nil, fmt.Errorf("validating membership: %w", err)
	}

	id, err := s.memberRepo.Create(ctx, member)
	if err != nil {
		return nil, fmt.Errorf("creating membership: %w", err)
	}
	member.ID = id

	slog.InfoContext(ctx, "tenant member added",
		slog.String("member_id", member.ID),
		slog.String("tenant_id", cmd.TenantID),
		slog.String("email", cmd.Email),
		slog.String("role", string(cmd.Role)),
	)

	return &entity.TenantMemberWithUser{
		TenantMember: *member,
		User:         user,
	}, nil
}

// UpdateMemberRole updates a tenant member's role.
func (s *TenantMemberService) UpdateMemberRole(ctx context.Context, cmd organizationuc.UpdateTenantMemberRoleCommand) (*entity.TenantMemberWithUser, error) {
	member, err := s.memberRepo.FindByID(ctx, cmd.MemberID)
	if err != nil {
		return nil, fmt.Errorf("finding member: %w", err)
	}

	// Verify tenant match
	if member.TenantID != cmd.TenantID {
		return nil, entity.ErrTenantMemberNotFound
	}

	// Only the global system tenant must always retain a direct TENANT_OWNER.
	mustPreserveOwner, err := s.mustPreserveDirectTenantOwner(ctx, cmd.TenantID)
	if err != nil {
		return nil, err
	}

	if member.Role == entity.TenantRoleOwner &&
		cmd.NewRole != entity.TenantRoleOwner &&
		mustPreserveOwner {
		count, err := s.memberRepo.CountByRole(ctx, cmd.TenantID, entity.TenantRoleOwner)
		if err != nil {
			return nil, fmt.Errorf("counting tenant owners: %w", err)
		}
		if count <= 1 {
			return nil, entity.ErrCannotRemoveTenantOwner
		}
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

	slog.InfoContext(ctx, "tenant member role updated",
		slog.String("member_id", cmd.MemberID),
		slog.String("new_role", string(cmd.NewRole)),
		slog.String("updated_by", cmd.UpdatedBy),
	)

	return &entity.TenantMemberWithUser{
		TenantMember: *member,
		User:         user,
	}, nil
}

// RemoveMember removes a member from the tenant.
func (s *TenantMemberService) RemoveMember(ctx context.Context, cmd organizationuc.RemoveTenantMemberCommand) error {
	member, err := s.memberRepo.FindByID(ctx, cmd.MemberID)
	if err != nil {
		return fmt.Errorf("finding member: %w", err)
	}

	// Verify tenant match
	if member.TenantID != cmd.TenantID {
		return entity.ErrTenantMemberNotFound
	}

	// Only the global system tenant must always retain a direct TENANT_OWNER.
	mustPreserveOwner, err := s.mustPreserveDirectTenantOwner(ctx, cmd.TenantID)
	if err != nil {
		return err
	}

	if member.Role == entity.TenantRoleOwner && mustPreserveOwner {
		count, err := s.memberRepo.CountByRole(ctx, cmd.TenantID, entity.TenantRoleOwner)
		if err != nil {
			return fmt.Errorf("counting tenant owners: %w", err)
		}
		if count <= 1 {
			return entity.ErrCannotRemoveTenantOwner
		}
	}

	if err := s.memberRepo.Delete(ctx, cmd.MemberID); err != nil {
		return fmt.Errorf("removing member: %w", err)
	}

	slog.InfoContext(ctx, "tenant member removed",
		slog.String("member_id", cmd.MemberID),
		slog.String("tenant_id", cmd.TenantID),
		slog.String("removed_by", cmd.RemovedBy),
	)

	return nil
}

// CountOwners counts the number of TENANT_OWNER members in a tenant.
func (s *TenantMemberService) CountOwners(ctx context.Context, tenantID string) (int, error) {
	return s.memberRepo.CountByRole(ctx, tenantID, entity.TenantRoleOwner)
}

func (s *TenantMemberService) mustPreserveDirectTenantOwner(ctx context.Context, tenantID string) (bool, error) {
	tenant, err := s.tenantRepo.FindByID(ctx, tenantID)
	if err != nil {
		return false, fmt.Errorf("finding tenant: %w", err)
	}
	if tenant == nil {
		return false, entity.ErrTenantNotFound
	}

	return tenant.IsSystem, nil
}
