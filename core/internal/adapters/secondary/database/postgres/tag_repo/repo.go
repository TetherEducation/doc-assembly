package tagrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new tag repository.
func New(pool *pgxpool.Pool) port.TagRepository {
	return &Repository{pool: pool}
}

// Repository implements the tag repository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// Create creates a new tag.
func (r *Repository) Create(ctx context.Context, tag *entity.Tag) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryCreate,
		tag.ID,
		tag.WorkspaceID,
		tag.Name,
		tag.Color,
		tag.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("inserting tag: %w", err)
	}

	return id, nil
}

// FindByID finds a tag by ID.
func (r *Repository) FindByID(ctx context.Context, id string) (*entity.Tag, error) {
	var tag entity.Tag
	err := r.pool.QueryRow(ctx, queryFindByID, id).Scan(
		&tag.ID,
		&tag.WorkspaceID,
		&tag.Name,
		&tag.Color,
		&tag.CreatedAt,
		&tag.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrTagNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying tag: %w", err)
	}

	return &tag, nil
}

// FindByWorkspace lists all tags in a workspace.
func (r *Repository) FindByWorkspace(ctx context.Context, workspaceID string) ([]*entity.Tag, error) {
	rows, err := r.pool.Query(ctx, queryFindByWorkspace, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("querying tags: %w", err)
	}
	defer rows.Close()

	return scanTags(rows)
}

// FindByWorkspaceWithCount lists all tags with template counts.
func (r *Repository) FindByWorkspaceWithCount(ctx context.Context, workspaceID string) ([]*entity.TagWithCount, error) {
	rows, err := r.pool.Query(ctx, queryFindByWorkspaceWithCount, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("querying tags with count from cache: %w", err)
	}
	defer rows.Close()

	var result []*entity.TagWithCount
	for rows.Next() {
		var tag entity.Tag
		var count int
		err := rows.Scan(
			&tag.ID,
			&tag.WorkspaceID,
			&tag.Name,
			&tag.Color,
			&count,
			&tag.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning tag with count from cache: %w", err)
		}
		result = append(result, &entity.TagWithCount{
			Tag:           tag,
			TemplateCount: count,
		})
	}

	return result, rows.Err()
}

// FindByName finds a tag by name within a workspace.
func (r *Repository) FindByName(ctx context.Context, workspaceID, name string) (*entity.Tag, error) {
	var tag entity.Tag
	err := r.pool.QueryRow(ctx, queryFindByName, workspaceID, name).Scan(
		&tag.ID,
		&tag.WorkspaceID,
		&tag.Name,
		&tag.Color,
		&tag.CreatedAt,
		&tag.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrTagNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying tag by name: %w", err)
	}

	return &tag, nil
}

// Update updates a tag.
func (r *Repository) Update(ctx context.Context, tag *entity.Tag) error {
	_, err := r.pool.Exec(ctx, queryUpdate,
		tag.ID,
		tag.Name,
		tag.Color,
		tag.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("updating tag: %w", err)
	}

	return nil
}

// Delete deletes a tag.
func (r *Repository) Delete(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryDelete, id)
	if err != nil {
		return fmt.Errorf("deleting tag: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrTagNotFound
	}

	return nil
}

// ExistsByName checks if a tag with the given name exists in the workspace.
func (r *Repository) ExistsByName(ctx context.Context, workspaceID, name string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByName, workspaceID, name).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking tag existence: %w", err)
	}

	return exists, nil
}

// ExistsByNameExcluding checks excluding a specific tag ID.
func (r *Repository) ExistsByNameExcluding(ctx context.Context, workspaceID, name, excludeID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByNameExcluding, workspaceID, name, excludeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking tag existence: %w", err)
	}

	return exists, nil
}

// IsInUse checks if a tag is attached to any templates.
func (r *Repository) IsInUse(ctx context.Context, id string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryIsInUse, id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking tag usage: %w", err)
	}

	return exists, nil
}

// GetTemplateCount returns the number of templates using this tag.
func (r *Repository) GetTemplateCount(ctx context.Context, id string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, queryGetTemplateCount, id).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting templates for tag: %w", err)
	}

	return count, nil
}

// scanTags scans tag rows into a slice.
func scanTags(rows pgx.Rows) ([]*entity.Tag, error) {
	var result []*entity.Tag
	for rows.Next() {
		var tag entity.Tag
		err := rows.Scan(
			&tag.ID,
			&tag.WorkspaceID,
			&tag.Name,
			&tag.Color,
			&tag.CreatedAt,
			&tag.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning tag: %w", err)
		}
		result = append(result, &tag)
	}
	return result, rows.Err()
}
