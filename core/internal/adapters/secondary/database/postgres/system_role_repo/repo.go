package systemrolerepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new system role repository.
func New(pool *pgxpool.Pool) port.SystemRoleRepository {
	return &Repository{pool: pool}
}

// Repository implements the system role repository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// Create creates a new system role assignment.
func (r *Repository) Create(ctx context.Context, assignment *entity.SystemRoleAssignment) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryCreate,
		assignment.ID,
		assignment.UserID,
		assignment.Role,
		assignment.GrantedBy,
		assignment.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("inserting system role: %w", err)
	}

	return id, nil
}

// FindByUserID finds a system role assignment by user ID.
func (r *Repository) FindByUserID(ctx context.Context, userID string) (*entity.SystemRoleAssignment, error) {
	var assignment entity.SystemRoleAssignment
	err := r.pool.QueryRow(ctx, queryFindByUserID, userID).Scan(
		&assignment.ID,
		&assignment.UserID,
		&assignment.Role,
		&assignment.GrantedBy,
		&assignment.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrSystemRoleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying system role: %w", err)
	}

	return &assignment, nil
}

// FindAll lists all system role assignments.
func (r *Repository) FindAll(ctx context.Context) ([]*entity.SystemRoleAssignment, error) {
	rows, err := r.pool.Query(ctx, queryFindAll)
	if err != nil {
		return nil, fmt.Errorf("querying system roles: %w", err)
	}
	defer rows.Close()

	var result []*entity.SystemRoleAssignment
	for rows.Next() {
		var assignment entity.SystemRoleAssignment
		err := rows.Scan(
			&assignment.ID,
			&assignment.UserID,
			&assignment.Role,
			&assignment.GrantedBy,
			&assignment.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning system role: %w", err)
		}
		result = append(result, &assignment)
	}

	return result, rows.Err()
}

// Delete removes a system role assignment by user ID.
func (r *Repository) Delete(ctx context.Context, userID string) error {
	result, err := r.pool.Exec(ctx, queryDelete, userID)
	if err != nil {
		return fmt.Errorf("deleting system role: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrSystemRoleNotFound
	}

	return nil
}

// UpdateRole updates a user's system role.
func (r *Repository) UpdateRole(ctx context.Context, userID string, role entity.SystemRole) error {
	result, err := r.pool.Exec(ctx, queryUpdateRole, userID, role)
	if err != nil {
		return fmt.Errorf("updating system role: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrSystemRoleNotFound
	}

	return nil
}
