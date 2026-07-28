package workspaceinjectablerepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new workspace injectable repository.
func New(pool *pgxpool.Pool) port.WorkspaceInjectableRepository {
	return &Repository{pool: pool}
}

// Repository implements the workspace injectable repository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// Create creates a new workspace-owned injectable.
func (r *Repository) Create(ctx context.Context, injectable *entity.InjectableDefinition) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryCreate,
		injectable.ID,
		injectable.WorkspaceID,
		injectable.Key,
		injectable.Label,
		injectable.Description,
		injectable.DataType,
		injectable.Metadata,
		injectable.FormatConfig,
		injectable.DefaultValue,
		injectable.IsActive,
		injectable.IsDeleted,
		injectable.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("inserting injectable: %w", err)
	}

	return id, nil
}

// FindByID finds an injectable by ID, ensuring it belongs to the workspace and is not deleted.
func (r *Repository) FindByID(ctx context.Context, id, workspaceID string) (*entity.InjectableDefinition, error) {
	injectable := &entity.InjectableDefinition{}
	err := r.pool.QueryRow(ctx, queryFindByID, id, workspaceID).Scan(
		&injectable.ID,
		&injectable.WorkspaceID,
		&injectable.Key,
		&injectable.Label,
		&injectable.Description,
		&injectable.DataType,
		&injectable.Metadata,
		&injectable.FormatConfig,
		&injectable.DefaultValue,
		&injectable.IsActive,
		&injectable.IsDeleted,
		&injectable.CreatedAt,
		&injectable.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrInjectableNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying injectable: %w", err)
	}

	injectable.SourceType = entity.InjectableSourceTypeInternal
	return injectable, nil
}

// FindByWorkspaceOwned lists injectables owned by a workspace (excluding deleted).
func (r *Repository) FindByWorkspaceOwned(ctx context.Context, workspaceID string) ([]*entity.InjectableDefinition, error) {
	rows, err := r.pool.Query(ctx, queryFindByWorkspaceOwned, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("querying workspace injectables: %w", err)
	}
	defer rows.Close()

	return scanInjectables(rows)
}

// Update updates a workspace-owned injectable.
func (r *Repository) Update(ctx context.Context, injectable *entity.InjectableDefinition) error {
	result, err := r.pool.Exec(ctx, queryUpdate,
		injectable.ID,
		injectable.Key,
		injectable.Label,
		injectable.Description,
		injectable.Metadata,
		injectable.FormatConfig,
		injectable.DefaultValue,
		injectable.UpdatedAt,
		injectable.WorkspaceID,
	)
	if err != nil {
		return fmt.Errorf("updating injectable: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrInjectableNotFound
	}

	return nil
}

// SoftDelete marks an injectable as deleted (is_deleted=true).
func (r *Repository) SoftDelete(ctx context.Context, id, workspaceID string) error {
	result, err := r.pool.Exec(ctx, querySoftDelete, id, workspaceID)
	if err != nil {
		return fmt.Errorf("soft deleting injectable: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrInjectableNotFound
	}

	return nil
}

// SetActive sets the is_active flag for an injectable.
func (r *Repository) SetActive(ctx context.Context, id, workspaceID string, isActive bool) error {
	result, err := r.pool.Exec(ctx, querySetActive, id, workspaceID, isActive)
	if err != nil {
		return fmt.Errorf("setting injectable active status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrInjectableNotFound
	}

	return nil
}

// ExistsByKey checks if an injectable with the given key exists for the workspace.
func (r *Repository) ExistsByKey(ctx context.Context, workspaceID, key string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByKey, workspaceID, key).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking injectable existence: %w", err)
	}

	return exists, nil
}

// ExistsByKeyExcluding checks if an injectable with the given key exists, excluding a specific ID.
func (r *Repository) ExistsByKeyExcluding(ctx context.Context, workspaceID, key, excludeID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByKeyExcluding, workspaceID, key, excludeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking injectable existence: %w", err)
	}

	return exists, nil
}

// scanInjectables scans injectable rows into a slice.
func scanInjectables(rows pgx.Rows) ([]*entity.InjectableDefinition, error) {
	var result []*entity.InjectableDefinition
	for rows.Next() {
		injectable := &entity.InjectableDefinition{}
		err := rows.Scan(
			&injectable.ID,
			&injectable.WorkspaceID,
			&injectable.Key,
			&injectable.Label,
			&injectable.Description,
			&injectable.DataType,
			&injectable.Metadata,
			&injectable.FormatConfig,
			&injectable.DefaultValue,
			&injectable.IsActive,
			&injectable.IsDeleted,
			&injectable.CreatedAt,
			&injectable.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning injectable: %w", err)
		}
		injectable.SourceType = entity.InjectableSourceTypeInternal
		result = append(result, injectable)
	}
	return result, rows.Err()
}
