package templateversioninjectablerepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/database/postgres/common"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new template version injectable repository.
func New(pool *pgxpool.Pool) port.TemplateVersionInjectableRepository {
	return &Repository{pool: pool}
}

// Repository implements port.TemplateVersionInjectableRepository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// Create creates a new template version injectable configuration.
func (r *Repository) Create(ctx context.Context, injectable *entity.TemplateVersionInjectable) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryCreate,
		injectable.TemplateVersionID,
		injectable.InjectableDefinitionID,
		injectable.SystemInjectableKey,
		injectable.IsRequired,
		injectable.DefaultValue,
		injectable.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("creating version injectable: %w", err)
	}

	return id, nil
}

// FindByID finds a template version injectable by ID.
func (r *Repository) FindByID(ctx context.Context, id string) (*entity.TemplateVersionInjectable, error) {
	injectable := &entity.TemplateVersionInjectable{}
	err := r.pool.QueryRow(ctx, queryFindByID, id).Scan(
		&injectable.ID,
		&injectable.TemplateVersionID,
		&injectable.InjectableDefinitionID,
		&injectable.SystemInjectableKey,
		&injectable.IsRequired,
		&injectable.DefaultValue,
		&injectable.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrVersionInjectableNotFound
		}
		return nil, fmt.Errorf("finding version injectable %s: %w", id, err)
	}

	return injectable, nil
}

// FindByVersionID lists all injectables for a template version with their definitions.
func (r *Repository) FindByVersionID(ctx context.Context, versionID string) ([]*entity.VersionInjectableWithDefinition, error) {
	rows, err := r.pool.Query(ctx, queryFindByVersionID, versionID)
	if err != nil {
		return nil, fmt.Errorf("querying version injectables: %w", err)
	}
	defer rows.Close()

	var results []*entity.VersionInjectableWithDefinition
	for rows.Next() {
		iwd := &entity.VersionInjectableWithDefinition{}

		// Nullable fields for definition (LEFT JOIN may return NULLs)
		var defID, defWorkspaceID, defKey, defLabel, defDescription *string
		var defDataType *entity.InjectableDataType
		var defMetadata map[string]any
		var defFormatConfig *entity.FormatConfig
		var defCreatedAt, defUpdatedAt *string

		if err := rows.Scan(
			&iwd.ID,
			&iwd.TemplateVersionID,
			&iwd.InjectableDefinitionID,
			&iwd.SystemInjectableKey,
			&iwd.IsRequired,
			&iwd.DefaultValue,
			&iwd.CreatedAt,
			&defID,
			&defWorkspaceID,
			&defKey,
			&defLabel,
			&defDescription,
			&defDataType,
			&defMetadata,
			&defFormatConfig,
			&defCreatedAt,
			&defUpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning version injectable: %w", err)
		}

		// Build definition only if it exists (workspace injectable)
		if defID != nil {
			iwd.Definition = &entity.InjectableDefinition{
				ID:           *defID,
				WorkspaceID:  defWorkspaceID,
				Key:          common.SafeString(defKey),
				Label:        common.SafeString(defLabel),
				Description:  common.SafeString(defDescription),
				DataType:     common.SafeDataType(defDataType),
				Metadata:     defMetadata,
				FormatConfig: defFormatConfig,
				SourceType:   entity.InjectableSourceTypeInternal,
			}
		}

		results = append(results, iwd)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating version injectables: %w", err)
	}

	return results, nil
}

// Update updates a template version injectable configuration.
func (r *Repository) Update(ctx context.Context, injectable *entity.TemplateVersionInjectable) error {
	result, err := r.pool.Exec(ctx, queryUpdate,
		injectable.ID,
		injectable.IsRequired,
		injectable.DefaultValue,
	)
	if err != nil {
		return fmt.Errorf("updating version injectable: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrVersionInjectableNotFound
	}

	return nil
}

// Delete deletes a template version injectable configuration.
func (r *Repository) Delete(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryDelete, id)
	if err != nil {
		return fmt.Errorf("deleting version injectable: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrVersionInjectableNotFound
	}

	return nil
}

// DeleteByVersionID deletes all injectable configurations for a template version.
func (r *Repository) DeleteByVersionID(ctx context.Context, versionID string) error {
	_, err := r.pool.Exec(ctx, queryDeleteByVersionID, versionID)
	if err != nil {
		return fmt.Errorf("deleting version injectables: %w", err)
	}

	return nil
}

// Exists checks if an injectable definition is already linked to a version.
func (r *Repository) Exists(ctx context.Context, versionID, injectableDefID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExists, versionID, injectableDefID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking version injectable existence: %w", err)
	}

	return exists, nil
}

// ExistsSystemKey checks if a system injectable key is already linked to a version.
func (r *Repository) ExistsSystemKey(ctx context.Context, versionID, systemKey string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsSystemKey, versionID, systemKey).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking system injectable existence: %w", err)
	}

	return exists, nil
}

// CopyFromVersion copies all injectable configurations from one version to another.
func (r *Repository) CopyFromVersion(ctx context.Context, sourceVersionID, targetVersionID string) error {
	_, err := r.pool.Exec(ctx, queryCopyFromVersion, sourceVersionID, targetVersionID)
	if err != nil {
		return fmt.Errorf("copying version injectables: %w", err)
	}

	return nil
}
