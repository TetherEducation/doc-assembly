package templateversionsignerrolerepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new template version signer role repository.
func New(pool *pgxpool.Pool) port.TemplateVersionSignerRoleRepository {
	return &Repository{pool: pool}
}

// Repository implements port.TemplateVersionSignerRoleRepository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// Create creates a new signer role for a template version.
func (r *Repository) Create(ctx context.Context, role *entity.TemplateVersionSignerRole) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryCreate,
		role.TemplateVersionID,
		role.RoleName,
		role.AnchorString,
		role.SignerOrder,
		role.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("creating version signer role: %w", err)
	}

	return id, nil
}

// FindByID finds a signer role by ID.
func (r *Repository) FindByID(ctx context.Context, id string) (*entity.TemplateVersionSignerRole, error) {
	role := &entity.TemplateVersionSignerRole{}
	err := r.pool.QueryRow(ctx, queryFindByID, id).Scan(
		&role.ID,
		&role.TemplateVersionID,
		&role.RoleName,
		&role.AnchorString,
		&role.SignerOrder,
		&role.CreatedAt,
		&role.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, entity.ErrSignerRoleNotFound
		}
		return nil, fmt.Errorf("finding version signer role %s: %w", id, err)
	}

	return role, nil
}

// FindByVersionID lists all signer roles for a template version ordered by signer_order.
func (r *Repository) FindByVersionID(ctx context.Context, versionID string) ([]*entity.TemplateVersionSignerRole, error) {
	rows, err := r.pool.Query(ctx, queryFindByVersionID, versionID)
	if err != nil {
		return nil, fmt.Errorf("querying version signer roles: %w", err)
	}
	defer rows.Close()

	var roles []*entity.TemplateVersionSignerRole
	for rows.Next() {
		role := &entity.TemplateVersionSignerRole{}
		if err := rows.Scan(
			&role.ID,
			&role.TemplateVersionID,
			&role.RoleName,
			&role.AnchorString,
			&role.SignerOrder,
			&role.CreatedAt,
			&role.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning version signer role: %w", err)
		}
		roles = append(roles, role)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating version signer roles: %w", err)
	}

	return roles, nil
}

// Update updates a signer role.
func (r *Repository) Update(ctx context.Context, role *entity.TemplateVersionSignerRole) error {
	result, err := r.pool.Exec(ctx, queryUpdate,
		role.ID,
		role.RoleName,
		role.AnchorString,
		role.SignerOrder,
	)
	if err != nil {
		return fmt.Errorf("updating version signer role: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrSignerRoleNotFound
	}

	return nil
}

// Delete deletes a signer role.
func (r *Repository) Delete(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryDelete, id)
	if err != nil {
		return fmt.Errorf("deleting version signer role: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrSignerRoleNotFound
	}

	return nil
}

// DeleteByVersionID deletes all signer roles for a template version.
func (r *Repository) DeleteByVersionID(ctx context.Context, versionID string) error {
	_, err := r.pool.Exec(ctx, queryDeleteByVersionID, versionID)
	if err != nil {
		return fmt.Errorf("deleting version signer roles: %w", err)
	}

	return nil
}

// ExistsByAnchor checks if an anchor string already exists for the version.
func (r *Repository) ExistsByAnchor(ctx context.Context, versionID, anchorString string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByAnchor, versionID, anchorString).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking anchor existence: %w", err)
	}

	return exists, nil
}

// ExistsByAnchorExcluding checks if an anchor exists excluding a specific role ID.
func (r *Repository) ExistsByAnchorExcluding(ctx context.Context, versionID, anchorString, excludeID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByAnchorExcluding, versionID, anchorString, excludeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking anchor existence: %w", err)
	}

	return exists, nil
}

// ExistsByOrder checks if a signer order already exists for the version.
func (r *Repository) ExistsByOrder(ctx context.Context, versionID string, order int) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByOrder, versionID, order).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking signer order existence: %w", err)
	}

	return exists, nil
}

// ExistsByOrderExcluding checks if an order exists excluding a specific role ID.
func (r *Repository) ExistsByOrderExcluding(ctx context.Context, versionID string, order int, excludeID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByOrderExcluding, versionID, order, excludeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking signer order existence: %w", err)
	}

	return exists, nil
}

// CopyFromVersion copies all signer roles from one version to another.
func (r *Repository) CopyFromVersion(ctx context.Context, sourceVersionID, targetVersionID string) error {
	_, err := r.pool.Exec(ctx, queryCopyFromVersion, sourceVersionID, targetVersionID)
	if err != nil {
		return fmt.Errorf("copying version signer roles: %w", err)
	}

	return nil
}
