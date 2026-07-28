package galleryassetrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// Repository implements port.GalleryRepository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// New creates a new gallery asset repository.
func New(pool *pgxpool.Pool) port.GalleryRepository {
	return &Repository{pool: pool}
}

// Save persists a new gallery asset and sets its generated ID.
func (r *Repository) Save(ctx context.Context, asset *entity.GalleryAsset) error {
	err := r.pool.QueryRow(ctx, querySave,
		asset.TenantID,
		asset.WorkspaceID,
		asset.Key,
		asset.Filename,
		asset.ContentType,
		asset.Size,
		asset.SHA256,
		asset.CreatedBy,
		asset.CreatedAt,
	).Scan(&asset.ID)
	if err != nil {
		return fmt.Errorf("inserting gallery asset: %w", err)
	}

	return nil
}

// FindBySHA256 looks up an asset by content hash within a workspace.
func (r *Repository) FindBySHA256(ctx context.Context, workspaceID, sha256 string) (*entity.GalleryAsset, error) {
	var a entity.GalleryAsset
	err := r.pool.QueryRow(ctx, queryFindBySHA256, workspaceID, sha256).Scan(
		&a.ID, &a.TenantID, &a.WorkspaceID, &a.Key, &a.Filename,
		&a.ContentType, &a.Size, &a.SHA256, &a.CreatedBy, &a.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("querying gallery asset by sha256: %w", err)
	}

	return &a, nil
}

// FindByKey looks up an asset by its storage key within a workspace.
func (r *Repository) FindByKey(ctx context.Context, workspaceID, key string) (*entity.GalleryAsset, error) {
	var a entity.GalleryAsset
	err := r.pool.QueryRow(ctx, queryFindByKey, workspaceID, key).Scan(
		&a.ID, &a.TenantID, &a.WorkspaceID, &a.Key, &a.Filename,
		&a.ContentType, &a.Size, &a.SHA256, &a.CreatedBy, &a.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrGalleryAssetNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying gallery asset by key: %w", err)
	}

	return &a, nil
}

// List returns a paginated list of assets ordered by creation date (newest first).
func (r *Repository) List(ctx context.Context, workspaceID string, page, perPage int) ([]*entity.GalleryAsset, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, queryListCount, workspaceID).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting gallery assets: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := r.pool.Query(ctx, queryList, workspaceID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("listing gallery assets: %w", err)
	}
	defer rows.Close()

	assets, err := scanAssets(rows)
	if err != nil {
		return nil, 0, err
	}

	return assets, total, nil
}

// Search returns assets whose filename matches the query string (case-insensitive).
func (r *Repository) Search(ctx context.Context, workspaceID, query string, page, perPage int) ([]*entity.GalleryAsset, int, error) {
	pattern := "%" + query + "%"

	var total int
	if err := r.pool.QueryRow(ctx, querySearchCount, workspaceID, pattern).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting gallery search results: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := r.pool.Query(ctx, querySearch, workspaceID, pattern, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("searching gallery assets: %w", err)
	}
	defer rows.Close()

	assets, err := scanAssets(rows)
	if err != nil {
		return nil, 0, err
	}

	return assets, total, nil
}

// Delete removes an asset record by workspace and key.
func (r *Repository) Delete(ctx context.Context, workspaceID, key string) error {
	_, err := r.pool.Exec(ctx, queryDelete, workspaceID, key)
	if err != nil {
		return fmt.Errorf("deleting gallery asset: %w", err)
	}

	return nil
}

func scanAssets(rows pgx.Rows) ([]*entity.GalleryAsset, error) {
	var result []*entity.GalleryAsset
	for rows.Next() {
		var a entity.GalleryAsset
		err := rows.Scan(
			&a.ID, &a.TenantID, &a.WorkspaceID, &a.Key, &a.Filename,
			&a.ContentType, &a.Size, &a.SHA256, &a.CreatedBy, &a.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning gallery asset: %w", err)
		}
		result = append(result, &a)
	}

	return result, rows.Err()
}

// Verify Repository implements GalleryRepository.
var _ port.GalleryRepository = (*Repository)(nil)
