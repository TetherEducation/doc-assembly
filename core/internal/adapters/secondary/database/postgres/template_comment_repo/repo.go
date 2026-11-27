package templatecommentrepo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new template comment repository.
func New(pool *pgxpool.Pool) port.TemplateCommentRepository {
	return &Repository{pool: pool}
}

// Repository stores schools' comments on contract templates. Append-only.
type Repository struct {
	pool *pgxpool.Pool
}

// Create inserts a comment and fills in its ID, CreatedAt and VersionNumber.
func (r *Repository) Create(ctx context.Context, comment *entity.TemplateComment) error {
	err := r.pool.QueryRow(ctx, queryCreate,
		comment.TemplateID,
		comment.TemplateVersionID,
		comment.Audience,
		comment.Body,
		comment.AuthorEmail,
		comment.AuthorName,
		comment.AuthorCampus,
	).Scan(&comment.ID, &comment.CreatedAt, &comment.VersionNumber)
	if err != nil {
		return fmt.Errorf("creating template comment: %w", err)
	}
	return nil
}

// ListForTemplate returns one audience's comments on a template, oldest first.
func (r *Repository) ListForTemplate(
	ctx context.Context,
	templateID, audience string,
) ([]*entity.TemplateComment, error) {
	rows, err := r.pool.Query(ctx, queryListForTemplate, templateID, audience)
	if err != nil {
		return nil, fmt.Errorf("listing template comments: %w", err)
	}
	defer rows.Close()

	comments := make([]*entity.TemplateComment, 0)
	for rows.Next() {
		var c entity.TemplateComment
		if err := rows.Scan(
			&c.ID, &c.TemplateID, &c.TemplateVersionID, &c.VersionNumber,
			&c.Audience, &c.Body, &c.AuthorEmail, &c.AuthorName, &c.AuthorCampus,
			&c.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning template comment: %w", err)
		}
		comments = append(comments, &c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating template comments: %w", err)
	}
	return comments, nil
}
