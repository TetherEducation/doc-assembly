package processrepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new process repository.
func New(pool *pgxpool.Pool) port.ProcessRepository {
	return &Repository{pool: pool}
}

// Repository implements the process repository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// Create creates a new process.
func (r *Repository) Create(ctx context.Context, process *entity.Process) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryCreate,
		process.TenantID,
		process.Code,
		string(process.ProcessType),
		process.Name,
		process.Description,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("inserting process: %w", err)
	}

	return id, nil
}

// FindByID finds a process by ID.
func (r *Repository) FindByID(ctx context.Context, id string) (*entity.Process, error) {
	var process entity.Process
	var processTypeStr string
	err := r.pool.QueryRow(ctx, queryFindByID, id).Scan(
		&process.ID,
		&process.TenantID,
		&process.Code,
		&processTypeStr,
		&process.Name,
		&process.Description,
		&process.CreatedAt,
		&process.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrProcessNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying process: %w", err)
	}

	process.ProcessType = entity.ProcessType(processTypeStr)
	return &process, nil
}

// FindByCode finds a process by code within a tenant.
func (r *Repository) FindByCode(ctx context.Context, tenantID, code string) (*entity.Process, error) {
	var process entity.Process
	var processTypeStr string
	err := r.pool.QueryRow(ctx, queryFindByCode, tenantID, code).Scan(
		&process.ID,
		&process.TenantID,
		&process.Code,
		&processTypeStr,
		&process.Name,
		&process.Description,
		&process.CreatedAt,
		&process.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrProcessNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying process by code: %w", err)
	}

	process.ProcessType = entity.ProcessType(processTypeStr)
	return &process, nil
}

// FindByTenant lists all processes for a tenant with pagination.
func (r *Repository) FindByTenant(ctx context.Context, tenantID string, filters port.ProcessFilters) ([]*entity.Process, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, queryCountByTenant, tenantID, filters.Search).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting processes: %w", err)
	}

	rows, err := r.pool.Query(ctx, queryFindByTenant, tenantID, filters.Search, filters.Limit, filters.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying processes: %w", err)
	}
	defer rows.Close()

	var result []*entity.Process
	for rows.Next() {
		var process entity.Process
		var processTypeStr string
		err := rows.Scan(
			&process.ID,
			&process.TenantID,
			&process.Code,
			&processTypeStr,
			&process.Name,
			&process.Description,
			&process.CreatedAt,
			&process.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning process: %w", err)
		}
		process.ProcessType = entity.ProcessType(processTypeStr)
		result = append(result, &process)
	}

	return result, total, rows.Err()
}

// FindByTenantWithTemplateCount lists processes with template usage count.
func (r *Repository) FindByTenantWithTemplateCount(ctx context.Context, tenantID string, filters port.ProcessFilters) ([]*entity.ProcessListItem, int64, error) {
	return queryAndCollectWithCount(
		r,
		ctx,
		queryCountByTenant,
		[]any{tenantID, filters.Search},
		queryFindByTenantWithTemplateCount,
		[]any{tenantID, filters.Search, filters.Limit, filters.Offset},
		"counting processes",
		"querying processes with count",
		"iterating processes with count",
		scanProcessListItemWithTemplateCount,
		"scanning process list item",
	)
}

// Update updates a process (name and description only, code and process_type are immutable).
func (r *Repository) Update(ctx context.Context, process *entity.Process) error {
	result, err := r.pool.Exec(ctx, queryUpdate,
		process.ID,
		process.Name,
		process.Description,
	)
	if err != nil {
		return fmt.Errorf("updating process: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrProcessNotFound
	}

	return nil
}

// Delete deletes a process.
func (r *Repository) Delete(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryDelete, id)
	if err != nil {
		return fmt.Errorf("deleting process: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrProcessNotFound
	}

	return nil
}

// ExistsByCode checks if a process with the given code exists in the tenant.
func (r *Repository) ExistsByCode(ctx context.Context, tenantID, code string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByCode, tenantID, code).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking process existence: %w", err)
	}

	return exists, nil
}

// ExistsByCodeExcluding checks excluding a specific process ID.
func (r *Repository) ExistsByCodeExcluding(ctx context.Context, tenantID, code, excludeID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByCodeExcluding, tenantID, code, excludeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking process existence: %w", err)
	}

	return exists, nil
}

// CountTemplatesByProcess returns the number of templates using this process code.
// Scoped to tenant via workspace join since process is a VARCHAR column on templates.
func (r *Repository) CountTemplatesByProcess(ctx context.Context, tenantID, processCode string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, queryCountTemplatesByProcess, tenantID, processCode).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting templates by process: %w", err)
	}

	return count, nil
}

// FindTemplatesByProcess returns templates assigned to this process code.
// Scoped to tenant via workspace join since process is a VARCHAR column on templates.
func (r *Repository) FindTemplatesByProcess(ctx context.Context, tenantID, processCode string) ([]*entity.ProcessTemplateInfo, error) {
	rows, err := r.pool.Query(ctx, queryFindTemplatesByProcess, tenantID, processCode)
	if err != nil {
		return nil, fmt.Errorf("querying templates by process: %w", err)
	}
	defer rows.Close()

	var result []*entity.ProcessTemplateInfo
	for rows.Next() {
		var info entity.ProcessTemplateInfo
		err := rows.Scan(
			&info.ID,
			&info.Title,
			&info.WorkspaceID,
			&info.WorkspaceName,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning template info: %w", err)
		}
		result = append(result, &info)
	}

	return result, rows.Err()
}

// IsSysTenant checks if the given tenant is the system tenant.
func (r *Repository) IsSysTenant(ctx context.Context, tenantID string) (bool, error) {
	var isSystem bool
	err := r.pool.QueryRow(ctx, queryIsSysTenant, tenantID).Scan(&isSystem)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, entity.ErrTenantNotFound
	}
	if err != nil {
		return false, fmt.Errorf("checking if tenant is system: %w", err)
	}
	return isSystem, nil
}

// FindByTenantWithGlobalFallback lists processes including global (SYS tenant) processes.
// Tenant's own processes take priority over global processes with the same code.
func (r *Repository) FindByTenantWithGlobalFallback(ctx context.Context, tenantID string, filters port.ProcessFilters) ([]*entity.Process, int64, error) {
	return queryAndCollectWithCount(
		r,
		ctx,
		queryCountByTenantWithGlobalFallback,
		[]any{tenantID, filters.Search},
		queryFindByTenantWithGlobalFallback,
		[]any{tenantID, filters.Search, filters.Limit, filters.Offset},
		"counting processes with global",
		"querying processes with global",
		"iterating processes with global",
		scanProcessWithGlobalFallback,
		"scanning process",
	)
}

// FindByTenantWithTemplateCountAndGlobal lists processes with template count, including global processes.
func (r *Repository) FindByTenantWithTemplateCountAndGlobal(ctx context.Context, tenantID string, filters port.ProcessFilters) ([]*entity.ProcessListItem, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, queryCountByTenantWithGlobalFallback, tenantID, filters.Search).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting processes with global: %w", err)
	}

	rows, err := r.pool.Query(ctx, queryFindByTenantWithTemplateCountAndGlobal, tenantID, filters.Search, filters.Limit, filters.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying processes with count and global: %w", err)
	}
	defer rows.Close()

	var result []*entity.ProcessListItem
	for rows.Next() {
		var item entity.ProcessListItem
		var processTypeStr string
		err := rows.Scan(
			&item.ID,
			&item.TenantID,
			&item.Code,
			&processTypeStr,
			&item.Name,
			&item.Description,
			&item.IsGlobal,
			&item.TemplatesCount,
			&item.CreatedAt,
			&item.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning process list item: %w", err)
		}
		item.ProcessType = entity.ProcessType(processTypeStr)
		result = append(result, &item)
	}

	return result, total, rows.Err()
}

// FindByCodeWithGlobalFallback finds a process by code, checking tenant first then SYS tenant.
func (r *Repository) FindByCodeWithGlobalFallback(ctx context.Context, tenantID, code string) (*entity.Process, error) {
	var process entity.Process
	var processTypeStr string
	err := r.pool.QueryRow(ctx, queryFindByCodeWithGlobalFallback, tenantID, code).Scan(
		&process.ID,
		&process.TenantID,
		&process.Code,
		&processTypeStr,
		&process.Name,
		&process.Description,
		&process.IsGlobal,
		&process.CreatedAt,
		&process.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrProcessNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying process by code with global: %w", err)
	}

	process.ProcessType = entity.ProcessType(processTypeStr)
	return &process, nil
}

func (r *Repository) queryWithCount(
	ctx context.Context,
	countQuery string,
	countArgs []any,
	listQuery string,
	listArgs []any,
	countErrMsg string,
	listErrMsg string,
	iterateErrMsg string,
	scanRows func(rows pgx.Rows) error,
) (int64, error) {
	var total int64
	if err := r.pool.QueryRow(ctx, countQuery, countArgs...).Scan(&total); err != nil {
		return 0, fmt.Errorf("%s: %w", countErrMsg, err)
	}

	rows, err := r.pool.Query(ctx, listQuery, listArgs...)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", listErrMsg, err)
	}
	defer rows.Close()

	if err := scanRows(rows); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("%s: %w", iterateErrMsg, err)
	}

	return total, nil
}

func queryAndCollectWithCount[T any](
	repo *Repository,
	ctx context.Context,
	countQuery string,
	countArgs []any,
	listQuery string,
	listArgs []any,
	countErrMsg string,
	listErrMsg string,
	iterateErrMsg string,
	scanItem func(rows pgx.Rows) (*T, error),
	scanErrMsg string,
) ([]*T, int64, error) {
	var result []*T
	total, err := repo.queryWithCount(
		ctx,
		countQuery,
		countArgs,
		listQuery,
		listArgs,
		countErrMsg,
		listErrMsg,
		iterateErrMsg,
		func(rows pgx.Rows) error {
			for rows.Next() {
				item, scanErr := scanItem(rows)
				if scanErr != nil {
					return fmt.Errorf("%s: %w", scanErrMsg, scanErr)
				}
				result = append(result, item)
			}
			return nil
		},
	)
	if err != nil {
		return nil, 0, err
	}

	return result, total, nil
}

func scanProcessListItemWithTemplateCount(rows pgx.Rows) (*entity.ProcessListItem, error) {
	var item entity.ProcessListItem
	var processTypeStr string
	if err := rows.Scan(
		&item.ID,
		&item.TenantID,
		&item.Code,
		&processTypeStr,
		&item.Name,
		&item.Description,
		&item.TemplatesCount,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, err
	}

	item.ProcessType = entity.ProcessType(processTypeStr)
	return &item, nil
}

func scanProcessWithGlobalFallback(rows pgx.Rows) (*entity.Process, error) {
	var process entity.Process
	var processTypeStr string
	if err := rows.Scan(
		&process.ID,
		&process.TenantID,
		&process.Code,
		&processTypeStr,
		&process.Name,
		&process.Description,
		&process.IsGlobal,
		&process.CreatedAt,
		&process.UpdatedAt,
	); err != nil {
		return nil, err
	}

	process.ProcessType = entity.ProcessType(processTypeStr)
	return &process, nil
}
