package injectablerepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new injectable repository.
func New(pool *pgxpool.Pool) port.InjectableRepository {
	return &Repository{pool: pool}
}

// Repository implements port.InjectableRepository using PostgreSQL.
// Note: Injectables are read-only - they are managed via database migrations/seeds.
type Repository struct {
	pool *pgxpool.Pool
}

// FindByID finds an injectable definition by ID.
func (r *Repository) FindByID(ctx context.Context, id string) (*entity.InjectableDefinition, error) {
	injectable := &entity.InjectableDefinition{}
	err := r.pool.QueryRow(ctx, queryFindByID, id).Scan(
		&injectable.ID,
		&injectable.WorkspaceID,
		&injectable.Key,
		&injectable.Label,
		&injectable.Description,
		&injectable.DataType,
		&injectable.Metadata,
		&injectable.FormatConfig,
		&injectable.IsActive,
		&injectable.IsDeleted,
		&injectable.CreatedAt,
		&injectable.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrInjectableNotFound
		}
		return nil, fmt.Errorf("finding injectable definition %s: %w", id, err)
	}

	injectable.SourceType = entity.InjectableSourceTypeInternal
	return injectable, nil
}

// FindByWorkspace lists all injectable definitions for a workspace (including global).
func (r *Repository) FindByWorkspace(ctx context.Context, workspaceID string) ([]*entity.InjectableDefinition, error) {
	rows, err := r.pool.Query(ctx, queryFindByWorkspace, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("querying injectable definitions: %w", err)
	}
	defer rows.Close()

	return collectInjectableDefinitions(rows)
}

// FindByWorkspaceAndKeys lists injectable definitions for a workspace restricted to keys.
func (r *Repository) FindByWorkspaceAndKeys(ctx context.Context, workspaceID string, keys []string) ([]*entity.InjectableDefinition, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, queryFindByWorkspaceAndKeys, workspaceID, keys)
	if err != nil {
		return nil, fmt.Errorf("querying injectable definitions by keys: %w", err)
	}
	defer rows.Close()

	return collectInjectableDefinitions(rows)
}

// FindGenerationAvailableByWorkspaceAndKeys lists DB injectables and active system keys in one read model query.
func (r *Repository) FindGenerationAvailableByWorkspaceAndKeys(ctx context.Context, workspaceID string, keys []string) ([]*entity.InjectableDefinition, []string, error) {
	if len(keys) == 0 {
		return nil, nil, nil
	}
	rows, err := r.pool.Query(ctx, queryFindGenerationAvailableByWorkspaceAndKeys, workspaceID, keys)
	if err != nil {
		return nil, nil, fmt.Errorf("querying generation injectables by keys: %w", err)
	}
	defer rows.Close()

	var dbInjectables []*entity.InjectableDefinition
	var systemKeys []string
	for rows.Next() {
		var source string
		var id, workspaceID, label, description, dataType sql.NullString
		var createdAt sql.NullTime
		injectable := &entity.InjectableDefinition{}
		if err := rows.Scan(
			&source,
			&id,
			&workspaceID,
			&injectable.Key,
			&label,
			&description,
			&dataType,
			&injectable.Metadata,
			&injectable.FormatConfig,
			&injectable.IsActive,
			&injectable.IsDeleted,
			&createdAt,
			&injectable.UpdatedAt,
		); err != nil {
			return nil, nil, fmt.Errorf("scanning generation injectable: %w", err)
		}
		switch source {
		case "system":
			systemKeys = append(systemKeys, injectable.Key)
		default:
			injectable.ID = id.String
			if workspaceID.Valid {
				injectable.WorkspaceID = &workspaceID.String
			}
			injectable.Label = label.String
			injectable.Description = description.String
			injectable.DataType = entity.InjectableDataType(dataType.String)
			injectable.CreatedAt = createdAt.Time
			injectable.SourceType = entity.InjectableSourceTypeInternal
			dbInjectables = append(dbInjectables, injectable)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterating generation injectables: %w", err)
	}

	return dbInjectables, systemKeys, nil
}

func collectInjectableDefinitions(rows pgx.Rows) ([]*entity.InjectableDefinition, error) {
	var injectables []*entity.InjectableDefinition
	for rows.Next() {
		injectable := &entity.InjectableDefinition{}
		if err := rows.Scan(
			&injectable.ID,
			&injectable.WorkspaceID,
			&injectable.Key,
			&injectable.Label,
			&injectable.Description,
			&injectable.DataType,
			&injectable.Metadata,
			&injectable.FormatConfig,
			&injectable.IsActive,
			&injectable.IsDeleted,
			&injectable.CreatedAt,
			&injectable.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning injectable definition: %w", err)
		}
		injectable.SourceType = entity.InjectableSourceTypeInternal
		injectables = append(injectables, injectable)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating injectable definitions: %w", err)
	}

	return injectables, nil
}

// FindGlobal lists all global injectable definitions.
func (r *Repository) FindGlobal(ctx context.Context) ([]*entity.InjectableDefinition, error) {
	rows, err := r.pool.Query(ctx, queryFindGlobal)
	if err != nil {
		return nil, fmt.Errorf("querying global injectable definitions: %w", err)
	}
	defer rows.Close()

	var injectables []*entity.InjectableDefinition
	for rows.Next() {
		injectable := &entity.InjectableDefinition{}
		if err := rows.Scan(
			&injectable.ID,
			&injectable.WorkspaceID,
			&injectable.Key,
			&injectable.Label,
			&injectable.Description,
			&injectable.DataType,
			&injectable.Metadata,
			&injectable.FormatConfig,
			&injectable.IsActive,
			&injectable.IsDeleted,
			&injectable.CreatedAt,
			&injectable.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning injectable definition: %w", err)
		}
		injectable.SourceType = entity.InjectableSourceTypeInternal
		injectables = append(injectables, injectable)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating global injectable definitions: %w", err)
	}

	return injectables, nil
}

// FindByKey finds an injectable by key.
func (r *Repository) FindByKey(ctx context.Context, workspaceID *string, key string) (*entity.InjectableDefinition, error) {
	var query string
	var args []any

	if workspaceID == nil {
		query = queryFindByKeyGlobal
		args = []any{key}
	} else {
		query = queryFindByKeyWorkspace
		args = []any{*workspaceID, key}
	}

	injectable := &entity.InjectableDefinition{}
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&injectable.ID,
		&injectable.WorkspaceID,
		&injectable.Key,
		&injectable.Label,
		&injectable.Description,
		&injectable.DataType,
		&injectable.Metadata,
		&injectable.FormatConfig,
		&injectable.IsActive,
		&injectable.IsDeleted,
		&injectable.CreatedAt,
		&injectable.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrInjectableNotFound
		}
		return nil, fmt.Errorf("finding injectable by key: %w", err)
	}

	injectable.SourceType = entity.InjectableSourceTypeInternal
	return injectable, nil
}

// ExistsByKey checks if an injectable with the given key exists.
func (r *Repository) ExistsByKey(ctx context.Context, workspaceID *string, key string) (bool, error) {
	var query string
	var args []any

	if workspaceID == nil {
		query = queryExistsByKeyGlobal
		args = []any{key}
	} else {
		query = queryExistsByKeyWorkspace
		args = []any{*workspaceID, key}
	}

	var exists bool
	err := r.pool.QueryRow(ctx, query, args...).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking injectable existence: %w", err)
	}

	return exists, nil
}

// ExistsByKeyExcluding checks if an injectable with the given key exists, excluding a specific ID.
func (r *Repository) ExistsByKeyExcluding(ctx context.Context, workspaceID *string, key, excludeID string) (bool, error) {
	var query string
	var args []any

	if workspaceID == nil {
		query = queryExistsByKeyGlobalExcluding
		args = []any{key, excludeID}
	} else {
		query = queryExistsByKeyWorkspaceExcluding
		args = []any{*workspaceID, key, excludeID}
	}

	var exists bool
	err := r.pool.QueryRow(ctx, query, args...).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking injectable existence: %w", err)
	}

	return exists, nil
}

// IsInUse checks if the injectable is in use by any template version.
func (r *Repository) IsInUse(ctx context.Context, id string) (bool, error) {
	var inUse bool
	err := r.pool.QueryRow(ctx, queryIsInUse, id).Scan(&inUse)
	if err != nil {
		return false, fmt.Errorf("checking injectable usage: %w", err)
	}

	return inUse, nil
}

// GetVersionCount returns the number of template versions using this injectable.
func (r *Repository) GetVersionCount(ctx context.Context, id string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, queryGetVersionCount, id).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting version usage: %w", err)
	}

	return count, nil
}

// ExistsByKeysForWorkspace returns a set of keys that are accessible to the workspace.
// It checks both workspace-specific and global injectables.
func (r *Repository) ExistsByKeysForWorkspace(ctx context.Context, workspaceID string, keys []string) (map[string]bool, error) {
	if len(keys) == 0 {
		return make(map[string]bool), nil
	}

	rows, err := r.pool.Query(ctx, queryFindKeysByWorkspace, workspaceID, keys)
	if err != nil {
		return nil, fmt.Errorf("querying accessible keys: %w", err)
	}
	defer rows.Close()

	result := make(map[string]bool, len(keys))
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scanning key: %w", err)
		}
		result[key] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating keys: %w", err)
	}

	return result, nil
}
