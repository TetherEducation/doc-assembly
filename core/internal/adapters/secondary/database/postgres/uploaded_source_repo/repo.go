package uploadedsourcerepo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a repository for uploaded template PDFs.
func New(pool *pgxpool.Pool) port.UploadedSourceRepository {
	return &Repository{pool: pool}
}

// Repository implements port.UploadedSourceRepository.
type Repository struct {
	pool *pgxpool.Pool
}

const upsertPDF = `
INSERT INTO content.template_version_sources
    (template_version_id, object_key, page_count, page_sizes, fields)
VALUES ($1, $2, $3, $4, '[]'::jsonb)
ON CONFLICT (template_version_id) DO UPDATE SET
    object_key = EXCLUDED.object_key,
    page_count = EXCLUDED.page_count,
    page_sizes = EXCLUDED.page_sizes,
    updated_at = now()`

const replaceFields = `
UPDATE content.template_version_sources
SET fields = $2, updated_at = now()
WHERE template_version_id = $1`

const findByVersion = `
SELECT template_version_id, object_key, page_count, page_sizes, fields
FROM content.template_version_sources
WHERE template_version_id = $1`

// UpsertPDF stores the PDF location and page sizes without clearing fields.
func (r *Repository) UpsertPDF(ctx context.Context, source *entity.UploadedSource) error {
	sizes, err := json.Marshal(source.PageSizes)
	if err != nil {
		return fmt.Errorf("encoding page sizes: %w", err)
	}
	_, err = r.pool.Exec(ctx, upsertPDF, source.VersionID, source.ObjectKey, source.PageCount, sizes)
	if err != nil {
		return fmt.Errorf("storing uploaded source: %w", err)
	}
	return nil
}

// ReplaceFields replaces field boxes for an existing uploaded source.
func (r *Repository) ReplaceFields(ctx context.Context, versionID string, fields []entity.PlacedField) error {
	if fields == nil {
		fields = []entity.PlacedField{}
	}
	raw, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("encoding placed fields: %w", err)
	}
	tag, err := r.pool.Exec(ctx, replaceFields, versionID, raw)
	if err != nil {
		return fmt.Errorf("storing placed fields: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return entity.ErrUploadedSourceNotFound
	}
	return nil
}

// FindByVersionID loads the uploaded source for a version.
func (r *Repository) FindByVersionID(ctx context.Context, versionID string) (*entity.UploadedSource, error) {
	var source entity.UploadedSource
	var sizes, fields []byte
	err := r.pool.QueryRow(ctx, findByVersion, versionID).Scan(
		&source.VersionID, &source.ObjectKey, &source.PageCount, &sizes, &fields,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrUploadedSourceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("finding uploaded source: %w", err)
	}
	if err := json.Unmarshal(sizes, &source.PageSizes); err != nil {
		return nil, fmt.Errorf("decoding page sizes: %w", err)
	}
	if len(fields) > 0 {
		if err := json.Unmarshal(fields, &source.Fields); err != nil {
			return nil, fmt.Errorf("decoding placed fields: %w", err)
		}
	}
	return &source, nil
}
