package workspacememberrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new workspace member repository.
func New(pool *pgxpool.Pool) port.WorkspaceMemberRepository {
	return &Repository{pool: pool}
}

// Repository implements the workspace member repository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// Create creates a new workspace membership.
func (r *Repository) Create(ctx context.Context, member *entity.WorkspaceMember) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryCreate,
		member.ID,
		member.WorkspaceID,
		member.UserID,
		member.Role,
		member.MembershipStatus,
		member.JoinedAt,
		member.InvitedBy,
		member.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("inserting workspace member: %w", err)
	}

	return id, nil
}

// FindByID finds a membership by ID.
func (r *Repository) FindByID(ctx context.Context, id string) (*entity.WorkspaceMember, error) {
	var member entity.WorkspaceMember
	err := r.pool.QueryRow(ctx, queryFindByID, id).Scan(
		&member.ID,
		&member.WorkspaceID,
		&member.UserID,
		&member.Role,
		&member.MembershipStatus,
		&member.JoinedAt,
		&member.InvitedBy,
		&member.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrMemberNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying workspace member: %w", err)
	}

	return &member, nil
}

// FindByUserAndWorkspace finds a membership for a specific user and workspace.
func (r *Repository) FindByUserAndWorkspace(ctx context.Context, userID, workspaceID string) (*entity.WorkspaceMember, error) {
	var member entity.WorkspaceMember
	err := r.pool.QueryRow(ctx, queryFindByUserAndWorkspace, userID, workspaceID).Scan(
		&member.ID,
		&member.WorkspaceID,
		&member.UserID,
		&member.Role,
		&member.MembershipStatus,
		&member.JoinedAt,
		&member.InvitedBy,
		&member.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrMemberNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying workspace member: %w", err)
	}

	return &member, nil
}

// FindByWorkspace lists all members of a workspace.
func (r *Repository) FindByWorkspace(ctx context.Context, workspaceID string) ([]*entity.MemberWithUser, error) {
	rows, err := r.pool.Query(ctx, queryFindByWorkspace, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("querying workspace members: %w", err)
	}
	defer rows.Close()

	var result []*entity.MemberWithUser
	for rows.Next() {
		var member entity.WorkspaceMember
		var user entity.User
		err := rows.Scan(
			&member.ID,
			&member.WorkspaceID,
			&member.UserID,
			&member.Role,
			&member.MembershipStatus,
			&member.JoinedAt,
			&member.InvitedBy,
			&member.CreatedAt,
			&user.ID,
			&user.Email,
			&user.FullName,
			&user.ExternalIdentityID,
			&user.Status,
			&user.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning member with user: %w", err)
		}
		result = append(result, &entity.MemberWithUser{
			WorkspaceMember: member,
			User:            &user,
		})
	}

	return result, rows.Err()
}

// FindByUser lists all workspace memberships for a user.
func (r *Repository) FindByUser(ctx context.Context, userID string) ([]*entity.WorkspaceMember, error) {
	rows, err := r.pool.Query(ctx, queryFindByUser, userID)
	if err != nil {
		return nil, fmt.Errorf("querying user memberships: %w", err)
	}
	defer rows.Close()

	var result []*entity.WorkspaceMember
	for rows.Next() {
		var member entity.WorkspaceMember
		err := rows.Scan(
			&member.ID,
			&member.WorkspaceID,
			&member.UserID,
			&member.Role,
			&member.MembershipStatus,
			&member.JoinedAt,
			&member.InvitedBy,
			&member.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning workspace member: %w", err)
		}
		result = append(result, &member)
	}

	return result, rows.Err()
}

// FindActiveByUserAndWorkspace finds an active membership.
func (r *Repository) FindActiveByUserAndWorkspace(ctx context.Context, userID, workspaceID string) (*entity.WorkspaceMember, error) {
	var member entity.WorkspaceMember
	err := r.pool.QueryRow(ctx, queryFindActiveByUserAndWorkspace, userID, workspaceID).Scan(
		&member.ID,
		&member.WorkspaceID,
		&member.UserID,
		&member.Role,
		&member.MembershipStatus,
		&member.JoinedAt,
		&member.InvitedBy,
		&member.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrMemberNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying active membership: %w", err)
	}

	return &member, nil
}

// CountActiveByWorkspace counts direct active memberships in a workspace.
func (r *Repository) CountActiveByWorkspace(ctx context.Context, workspaceID string) (int, error) {
	var count int
	if err := r.pool.QueryRow(ctx, queryCountActiveByWorkspace, workspaceID).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting active memberships: %w", err)
	}

	return count, nil
}

// Update updates a membership.
func (r *Repository) Update(ctx context.Context, member *entity.WorkspaceMember) error {
	_, err := r.pool.Exec(ctx, queryUpdate,
		member.ID,
		member.Role,
		member.MembershipStatus,
	)
	if err != nil {
		return fmt.Errorf("updating workspace member: %w", err)
	}

	return nil
}

// Delete removes a membership.
func (r *Repository) Delete(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryDelete, id)
	if err != nil {
		return fmt.Errorf("deleting workspace member: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrMemberNotFound
	}

	return nil
}

// Activate activates a pending membership.
func (r *Repository) Activate(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryActivate, id)
	if err != nil {
		return fmt.Errorf("activating membership: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrMemberNotFound
	}

	return nil
}

// UpdateRole updates a member's role.
func (r *Repository) UpdateRole(ctx context.Context, id string, role entity.WorkspaceRole) error {
	result, err := r.pool.Exec(ctx, queryUpdateRole, id, role)
	if err != nil {
		return fmt.Errorf("updating member role: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrMemberNotFound
	}

	return nil
}
