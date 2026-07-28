package access

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	accessuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/access"
)

// NewSystemRoleService creates a new system role service.
func NewSystemRoleService(
	systemRoleRepo port.SystemRoleRepository,
	userRepo port.UserRepository,
) accessuc.SystemRoleUseCase {
	return &SystemRoleService{
		systemRoleRepo: systemRoleRepo,
		userRepo:       userRepo,
	}
}

// SystemRoleService implements the SystemRoleUseCase interface.
type SystemRoleService struct {
	systemRoleRepo port.SystemRoleRepository
	userRepo       port.UserRepository
}

const usersEmailUniqueConstraint = "users_email_key"

// ListUsersWithSystemRoles lists all users that have system roles.
func (s *SystemRoleService) ListUsersWithSystemRoles(ctx context.Context) ([]*entity.SystemRoleWithUser, error) {
	assignments, err := s.systemRoleRepo.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing system roles: %w", err)
	}

	result := make([]*entity.SystemRoleWithUser, 0, len(assignments))
	for _, assignment := range assignments {
		user, err := s.userRepo.FindByID(ctx, assignment.UserID)
		if err != nil {
			slog.WarnContext(ctx, "user not found for system role",
				slog.String("user_id", assignment.UserID),
				slog.Any("error", err),
			)
			continue
		}
		result = append(result, &entity.SystemRoleWithUser{
			SystemRoleAssignment: *assignment,
			User:                 user,
		})
	}

	return result, nil
}

// AssignRole assigns a system role to a user.
func (s *SystemRoleService) AssignRole(ctx context.Context, cmd accessuc.AssignSystemRoleCommand) (*entity.SystemRoleAssignment, error) {
	// Check if user exists
	_, err := s.userRepo.FindByID(ctx, cmd.UserID)
	if err != nil {
		return nil, fmt.Errorf("finding user: %w", err)
	}

	// Check if user already has a system role
	existing, err := s.systemRoleRepo.FindByUserID(ctx, cmd.UserID)
	if err == nil && existing != nil {
		// Update existing role
		if err := s.systemRoleRepo.UpdateRole(ctx, cmd.UserID, cmd.Role); err != nil {
			return nil, fmt.Errorf("updating system role: %w", err)
		}
		existing.Role = cmd.Role
		slog.InfoContext(ctx, "system role updated",
			slog.String("user_id", cmd.UserID),
			slog.String("role", string(cmd.Role)),
		)
		return existing, nil
	}

	// Create new assignment
	assignment := &entity.SystemRoleAssignment{
		ID:        uuid.NewString(),
		UserID:    cmd.UserID,
		Role:      cmd.Role,
		GrantedBy: &cmd.GrantedBy,
		CreatedAt: time.Now().UTC(),
	}

	id, err := s.systemRoleRepo.Create(ctx, assignment)
	if err != nil {
		return nil, fmt.Errorf("creating system role: %w", err)
	}
	assignment.ID = id

	slog.InfoContext(ctx, "system role assigned",
		slog.String("user_id", cmd.UserID),
		slog.String("role", string(cmd.Role)),
		slog.String("granted_by", cmd.GrantedBy),
	)

	return assignment, nil
}

// AddRole finds or creates a user and assigns a system role.
//
//nolint:gocognit,nestif
func (s *SystemRoleService) AddRole(ctx context.Context, cmd accessuc.AddSystemRoleCommand) (*entity.SystemRoleAssignment, error) {
	normalizedEmail := normalizeEmail(cmd.Email)
	fullName := strings.TrimSpace(cmd.FullName)
	createdUser := false

	user, err := s.userRepo.FindByEmail(ctx, normalizedEmail)
	if err != nil {
		if !errors.Is(err, entity.ErrUserNotFound) {
			return nil, fmt.Errorf("finding user by email: %w", err)
		}

		user = entity.NewUser(normalizedEmail, fullName)
		user.ID = uuid.NewString()
		if err := user.Validate(); err != nil {
			return nil, fmt.Errorf("validating user: %w", err)
		}
		if _, err := s.userRepo.Create(ctx, user); err != nil {
			if !isUsersEmailUniqueViolation(err) {
				return nil, fmt.Errorf("creating shadow user: %w", err)
			}

			user, err = s.userRepo.FindByEmail(ctx, normalizedEmail)
			if err != nil {
				return nil, fmt.Errorf("reloading user after duplicate email: %w", err)
			}
		} else {
			createdUser = true
		}

		if createdUser {
			slog.InfoContext(ctx, "shadow user created for system role",
				slog.String("user_id", user.ID),
				slog.String("email", user.Email),
			)
		}
	} else if fullName != "" && user.FullName == "" {
		user.FullName = fullName
		if err := s.userRepo.Update(ctx, user); err != nil {
			return nil, fmt.Errorf("updating user full name: %w", err)
		}
	}

	return s.AssignRole(ctx, accessuc.AssignSystemRoleCommand{
		UserID:    user.ID,
		Role:      cmd.Role,
		GrantedBy: cmd.GrantedBy,
	})
}

// RevokeRole revokes a user's system role.
func (s *SystemRoleService) RevokeRole(ctx context.Context, cmd accessuc.RevokeSystemRoleCommand) error {
	if err := s.systemRoleRepo.Delete(ctx, cmd.UserID); err != nil {
		return fmt.Errorf("revoking system role: %w", err)
	}

	slog.InfoContext(ctx, "system role revoked",
		slog.String("user_id", cmd.UserID),
		slog.String("revoked_by", cmd.RevokedBy),
	)

	return nil
}

// GetUserSystemRole gets a user's system role.
func (s *SystemRoleService) GetUserSystemRole(ctx context.Context, userID string) (*entity.SystemRoleAssignment, error) {
	return s.systemRoleRepo.FindByUserID(ctx, userID)
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func isUsersEmailUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == "23505" && pgErr.ConstraintName == usersEmailUniqueConstraint
}
