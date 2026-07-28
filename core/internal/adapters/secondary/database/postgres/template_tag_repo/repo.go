package templatetagrepo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new template tag repository.
func New(pool *pgxpool.Pool) port.TemplateTagRepository {
	return &Repository{pool: pool}
}

// Repository implements port.TemplateTagRepository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// AddTag adds a tag to a template.
func (r *Repository) AddTag(ctx context.Context, templateID, tagID string) error {
	_, err := r.pool.Exec(ctx, queryAddTag, templateID, tagID)
	if err != nil {
		return fmt.Errorf("adding tag to template: %w", err)
	}

	return nil
}

// RemoveTag removes a tag from a template.
func (r *Repository) RemoveTag(ctx context.Context, templateID, tagID string) error {
	_, err := r.pool.Exec(ctx, queryRemoveTag, templateID, tagID)
	if err != nil {
		return fmt.Errorf("removing tag from template: %w", err)
	}

	return nil
}

// FindTagsByTemplate lists all tags for a template.
func (r *Repository) FindTagsByTemplate(ctx context.Context, templateID string) ([]*entity.Tag, error) {
	rows, err := r.pool.Query(ctx, queryFindTagsByTemplate, templateID)
	if err != nil {
		return nil, fmt.Errorf("querying template tags: %w", err)
	}
	defer rows.Close()

	var tags []*entity.Tag
	for rows.Next() {
		tag := &entity.Tag{}
		if err := rows.Scan(
			&tag.ID,
			&tag.WorkspaceID,
			&tag.Name,
			&tag.Color,
			&tag.CreatedAt,
			&tag.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning tag: %w", err)
		}
		tags = append(tags, tag)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating tags: %w", err)
	}

	return tags, nil
}

// FindTemplatesByTag lists all template IDs with a specific tag.
func (r *Repository) FindTemplatesByTag(ctx context.Context, tagID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, queryFindTemplatesByTag, tagID)
	if err != nil {
		return nil, fmt.Errorf("querying templates by tag: %w", err)
	}
	defer rows.Close()

	var templateIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning template ID: %w", err)
		}
		templateIDs = append(templateIDs, id)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating template IDs: %w", err)
	}

	return templateIDs, nil
}

// Exists checks if a tag is already linked to a template.
func (r *Repository) Exists(ctx context.Context, templateID, tagID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExists, templateID, tagID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking template tag existence: %w", err)
	}

	return exists, nil
}

// DeleteByTemplate removes all tag associations for a template.
func (r *Repository) DeleteByTemplate(ctx context.Context, templateID string) error {
	_, err := r.pool.Exec(ctx, queryDeleteByTemplate, templateID)
	if err != nil {
		return fmt.Errorf("deleting template tags: %w", err)
	}

	return nil
}
