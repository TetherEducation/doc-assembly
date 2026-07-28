package userrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new user repository.
func New(pool *pgxpool.Pool) port.UserRepository {
	return &Repository{pool: pool}
}

// Repository implements the user repository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// Create creates a new user.
func (r *Repository) Create(ctx context.Context, user *entity.User) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryCreate,
		user.ID,
		user.ExternalIdentityID,
		user.Email,
		user.FullName,
		user.Status,
		user.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("inserting user: %w", err)
	}

	return id, nil
}

// FindByID finds a user by their internal ID.
func (r *Repository) FindByID(ctx context.Context, id string) (*entity.User, error) {
	var user entity.User
	err := r.pool.QueryRow(ctx, queryFindByID, id).Scan(
		&user.ID,
		&user.ExternalIdentityID,
		&user.Email,
		&user.FullName,
		&user.Status,
		&user.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying user: %w", err)
	}

	return &user, nil
}

// FindByEmail finds a user by email.
func (r *Repository) FindByEmail(ctx context.Context, email string) (*entity.User, error) {
	var user entity.User
	err := r.pool.QueryRow(ctx, queryFindByEmail, email).Scan(
		&user.ID,
		&user.ExternalIdentityID,
		&user.Email,
		&user.FullName,
		&user.Status,
		&user.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying user by email: %w", err)
	}

	return &user, nil
}

// FindByExternalID finds a user by their external IdP ID.
func (r *Repository) FindByExternalID(ctx context.Context, externalID string) (*entity.User, error) {
	var user entity.User
	err := r.pool.QueryRow(ctx, queryFindByExternalID, externalID).Scan(
		&user.ID,
		&user.ExternalIdentityID,
		&user.Email,
		&user.FullName,
		&user.Status,
		&user.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying user by external ID: %w", err)
	}

	return &user, nil
}

// Update updates a user's information.
func (r *Repository) Update(ctx context.Context, user *entity.User) error {
	_, err := r.pool.Exec(ctx, queryUpdate,
		user.ID,
		user.Email,
		user.FullName,
		user.Status,
	)
	if err != nil {
		return fmt.Errorf("updating user: %w", err)
	}

	return nil
}

// LinkToIdP links an existing user to an IdP account.
func (r *Repository) LinkToIdP(ctx context.Context, id, externalID string) error {
	_, err := r.pool.Exec(ctx, queryLinkToIdP, id, externalID)
	if err != nil {
		return fmt.Errorf("linking user to IdP: %w", err)
	}

	return nil
}
