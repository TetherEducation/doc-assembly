package tenantrepo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new tenant repository.
func New(pool *pgxpool.Pool) port.TenantRepository {
	return &Repository{pool: pool}
}

// Repository implements the tenant repository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// Create creates a new tenant.
func (r *Repository) Create(ctx context.Context, tenant *entity.Tenant) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryCreate,
		tenant.ID,
		tenant.Code,
		tenant.Name,
		tenant.Description,
		tenant.Status,
		tenant.Settings,
		tenant.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("inserting tenant: %w", err)
	}

	return id, nil
}

// FindByID finds a tenant by ID.
func (r *Repository) FindByID(ctx context.Context, id string) (*entity.Tenant, error) {
	var tenant entity.Tenant
	err := r.pool.QueryRow(ctx, queryFindByID, id).Scan(
		&tenant.ID,
		&tenant.Code,
		&tenant.Name,
		&tenant.Description,
		&tenant.IsSystem,
		&tenant.Status,
		&tenant.Settings,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrTenantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying tenant: %w", err)
	}

	return &tenant, nil
}

// FindByCode finds a tenant by code.
func (r *Repository) FindByCode(ctx context.Context, code string) (*entity.Tenant, error) {
	var tenant entity.Tenant
	err := r.pool.QueryRow(ctx, queryFindByCode, code).Scan(
		&tenant.ID,
		&tenant.Code,
		&tenant.Name,
		&tenant.Description,
		&tenant.IsSystem,
		&tenant.Status,
		&tenant.Settings,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrTenantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying tenant by code: %w", err)
	}

	return &tenant, nil
}

// FindAll lists all tenants.
func (r *Repository) FindAll(ctx context.Context) ([]*entity.Tenant, error) {
	rows, err := r.pool.Query(ctx, queryFindAll)
	if err != nil {
		return nil, fmt.Errorf("querying tenants: %w", err)
	}
	defer rows.Close()

	var result []*entity.Tenant
	for rows.Next() {
		var tenant entity.Tenant
		err := rows.Scan(
			&tenant.ID,
			&tenant.Code,
			&tenant.Name,
			&tenant.Description,
			&tenant.IsSystem,
			&tenant.Status,
			&tenant.Settings,
			&tenant.CreatedAt,
			&tenant.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning tenant: %w", err)
		}
		result = append(result, &tenant)
	}

	return result, rows.Err()
}

// Update updates a tenant.
func (r *Repository) Update(ctx context.Context, tenant *entity.Tenant) error {
	_, err := r.pool.Exec(ctx, queryUpdate,
		tenant.ID,
		tenant.Name,
		tenant.Description,
		tenant.Settings,
		tenant.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("updating tenant: %w", err)
	}

	return nil
}

// Delete deletes a tenant.
func (r *Repository) Delete(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryDelete, id)
	if err != nil {
		return fmt.Errorf("deleting tenant: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrTenantNotFound
	}

	return nil
}

// ExistsByCode checks if a tenant with the given code exists.
func (r *Repository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByCode, code).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking tenant existence: %w", err)
	}

	return exists, nil
}

// FindSystemTenant finds the system tenant.
func (r *Repository) FindSystemTenant(ctx context.Context) (*entity.Tenant, error) {
	var tenant entity.Tenant
	err := r.pool.QueryRow(ctx, queryFindSystemTenant).Scan(
		&tenant.ID,
		&tenant.Code,
		&tenant.Name,
		&tenant.Description,
		&tenant.IsSystem,
		&tenant.Status,
		&tenant.Settings,
		&tenant.CreatedAt,
		&tenant.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrTenantNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying system tenant: %w", err)
	}

	return &tenant, nil
}

// FindAllPaginated lists tenants with pagination and returns total count.
func (r *Repository) FindAllPaginated(ctx context.Context, filters port.TenantFilters) ([]*entity.Tenant, int64, error) {
	var total int64
	var rows pgx.Rows
	var err error

	// System endpoint: use simple or search query based on Query filter
	if filters.UserID == "" {
		// Get total count with search filter
		err = r.pool.QueryRow(ctx, queryCountAllWithSearch, filters.Query).Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("counting tenants: %w", err)
		}
		rows, err = r.pool.Query(ctx, queryFindAllPaginatedWithSearch, filters.Query, filters.Limit, filters.Offset)
	} else {
		// User endpoint: use unified query with optional search and access history ordering
		err = r.pool.QueryRow(ctx, queryCountAllUnified, filters.Query).Scan(&total)
		if err != nil {
			return nil, 0, fmt.Errorf("counting tenants: %w", err)
		}
		rows, err = r.pool.Query(ctx, queryFindAllPaginatedUnified, filters.UserID, filters.Query, filters.Limit, filters.Offset)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("querying tenants paginated: %w", err)
	}
	defer rows.Close()

	var result []*entity.Tenant
	for rows.Next() {
		var tenant entity.Tenant
		err := rows.Scan(
			&tenant.ID,
			&tenant.Code,
			&tenant.Name,
			&tenant.Description,
			&tenant.IsSystem,
			&tenant.Status,
			&tenant.Settings,
			&tenant.CreatedAt,
			&tenant.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning tenant: %w", err)
		}
		result = append(result, &tenant)
	}

	return result, total, rows.Err()
}

// SearchByNameOrCode searches tenants by name or code similarity using trigram.
func (r *Repository) SearchByNameOrCode(ctx context.Context, query string, limit int) ([]*entity.Tenant, error) {
	rows, err := r.pool.Query(ctx, querySearchByNameOrCode, query, limit)
	if err != nil {
		return nil, fmt.Errorf("searching tenants: %w", err)
	}
	defer rows.Close()

	var result []*entity.Tenant
	for rows.Next() {
		var tenant entity.Tenant
		err := rows.Scan(
			&tenant.ID,
			&tenant.Code,
			&tenant.Name,
			&tenant.Description,
			&tenant.IsSystem,
			&tenant.Status,
			&tenant.Settings,
			&tenant.CreatedAt,
			&tenant.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning tenant: %w", err)
		}
		result = append(result, &tenant)
	}

	return result, rows.Err()
}

// UpdateStatus updates a tenant's status (cannot update system tenant status).
func (r *Repository) UpdateStatus(ctx context.Context, id string, status entity.TenantStatus, updatedAt *time.Time) error {
	result, err := r.pool.Exec(ctx, queryUpdateStatus, id, status, updatedAt)
	if err != nil {
		return fmt.Errorf("updating tenant status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrTenantNotFound
	}

	return nil
}
