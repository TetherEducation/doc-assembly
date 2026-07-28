package documenttyperepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new document type repository.
func New(pool *pgxpool.Pool) port.DocumentTypeRepository {
	return &Repository{pool: pool}
}

// Repository implements the document type repository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// Create creates a new document type.
func (r *Repository) Create(ctx context.Context, docType *entity.DocumentType) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryCreate,
		docType.TenantID,
		docType.Code,
		docType.Name,
		docType.Description,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("inserting document type: %w", err)
	}

	return id, nil
}

// FindByID finds a document type by ID.
func (r *Repository) FindByID(ctx context.Context, id string) (*entity.DocumentType, error) {
	var docType entity.DocumentType
	err := r.pool.QueryRow(ctx, queryFindByID, id).Scan(
		&docType.ID,
		&docType.TenantID,
		&docType.Code,
		&docType.Name,
		&docType.Description,
		&docType.CreatedAt,
		&docType.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrDocumentTypeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying document type: %w", err)
	}

	return &docType, nil
}

// FindByCode finds a document type by code within a tenant.
func (r *Repository) FindByCode(ctx context.Context, tenantID, code string) (*entity.DocumentType, error) {
	var docType entity.DocumentType
	err := r.pool.QueryRow(ctx, queryFindByCode, tenantID, code).Scan(
		&docType.ID,
		&docType.TenantID,
		&docType.Code,
		&docType.Name,
		&docType.Description,
		&docType.CreatedAt,
		&docType.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrDocumentTypeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying document type by code: %w", err)
	}

	return &docType, nil
}

// FindByTenant lists all document types for a tenant with pagination.
func (r *Repository) FindByTenant(ctx context.Context, tenantID string, filters port.DocumentTypeFilters) ([]*entity.DocumentType, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, queryCountByTenant, tenantID, filters.Search).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting document types: %w", err)
	}

	rows, err := r.pool.Query(ctx, queryFindByTenant, tenantID, filters.Search, filters.Limit, filters.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying document types: %w", err)
	}
	defer rows.Close()

	var result []*entity.DocumentType
	for rows.Next() {
		var docType entity.DocumentType
		err := rows.Scan(
			&docType.ID,
			&docType.TenantID,
			&docType.Code,
			&docType.Name,
			&docType.Description,
			&docType.CreatedAt,
			&docType.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning document type: %w", err)
		}
		result = append(result, &docType)
	}

	return result, total, rows.Err()
}

// FindByTenantWithTemplateCount lists document types with template usage count.
func (r *Repository) FindByTenantWithTemplateCount(ctx context.Context, tenantID string, filters port.DocumentTypeFilters) ([]*entity.DocumentTypeListItem, int64, error) {
	return queryAndCollectWithCount(
		r,
		ctx,
		queryCountByTenant,
		[]any{tenantID, filters.Search},
		queryFindByTenantWithTemplateCount,
		[]any{tenantID, filters.Search, filters.Limit, filters.Offset},
		"counting document types",
		"querying document types with count",
		"iterating document types with count",
		scanDocumentTypeListItemWithTemplateCount,
		"scanning document type list item",
	)
}

// Update updates a document type (name and description only, code is immutable).
func (r *Repository) Update(ctx context.Context, docType *entity.DocumentType) error {
	result, err := r.pool.Exec(ctx, queryUpdate,
		docType.ID,
		docType.Name,
		docType.Description,
	)
	if err != nil {
		return fmt.Errorf("updating document type: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrDocumentTypeNotFound
	}

	return nil
}

// Delete deletes a document type.
func (r *Repository) Delete(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryDelete, id)
	if err != nil {
		return fmt.Errorf("deleting document type: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrDocumentTypeNotFound
	}

	return nil
}

// ExistsByCode checks if a document type with the given code exists in the tenant.
func (r *Repository) ExistsByCode(ctx context.Context, tenantID, code string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByCode, tenantID, code).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking document type existence: %w", err)
	}

	return exists, nil
}

// ExistsByCodeExcluding checks excluding a specific document type ID.
func (r *Repository) ExistsByCodeExcluding(ctx context.Context, tenantID, code, excludeID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByCodeExcluding, tenantID, code, excludeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking document type existence: %w", err)
	}

	return exists, nil
}

// CountTemplatesByType returns the number of templates using this document type.
func (r *Repository) CountTemplatesByType(ctx context.Context, documentTypeID string) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, queryCountTemplatesByType, documentTypeID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting templates by type: %w", err)
	}

	return count, nil
}

// FindTemplatesByType returns templates assigned to this document type.
func (r *Repository) FindTemplatesByType(ctx context.Context, documentTypeID string) ([]*entity.DocumentTypeTemplateInfo, error) {
	rows, err := r.pool.Query(ctx, queryFindTemplatesByType, documentTypeID)
	if err != nil {
		return nil, fmt.Errorf("querying templates by type: %w", err)
	}
	defer rows.Close()

	var result []*entity.DocumentTypeTemplateInfo
	for rows.Next() {
		var info entity.DocumentTypeTemplateInfo
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

// FindByTenantWithGlobalFallback lists document types including global (SYS tenant) types.
// Tenant's own types take priority over global types with the same code.
func (r *Repository) FindByTenantWithGlobalFallback(ctx context.Context, tenantID string, filters port.DocumentTypeFilters) ([]*entity.DocumentType, int64, error) {
	return queryAndCollectWithCount(
		r,
		ctx,
		queryCountByTenantWithGlobalFallback,
		[]any{tenantID, filters.Search},
		queryFindByTenantWithGlobalFallback,
		[]any{tenantID, filters.Search, filters.Limit, filters.Offset},
		"counting document types with global",
		"querying document types with global",
		"iterating document types with global",
		scanDocumentTypeWithGlobalFallback,
		"scanning document type",
	)
}

// FindByTenantWithTemplateCountAndGlobal lists document types with template count, including global types.
func (r *Repository) FindByTenantWithTemplateCountAndGlobal(ctx context.Context, tenantID string, filters port.DocumentTypeFilters) ([]*entity.DocumentTypeListItem, int64, error) {
	var total int64
	err := r.pool.QueryRow(ctx, queryCountByTenantWithGlobalFallback, tenantID, filters.Search).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("counting document types with global: %w", err)
	}

	rows, err := r.pool.Query(ctx, queryFindByTenantWithTemplateCountAndGlobal, tenantID, filters.Search, filters.Limit, filters.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("querying document types with count and global: %w", err)
	}
	defer rows.Close()

	var result []*entity.DocumentTypeListItem
	for rows.Next() {
		var item entity.DocumentTypeListItem
		err := rows.Scan(
			&item.ID,
			&item.TenantID,
			&item.Code,
			&item.Name,
			&item.Description,
			&item.IsGlobal,
			&item.TemplatesCount,
			&item.CreatedAt,
			&item.UpdatedAt,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scanning document type list item: %w", err)
		}
		result = append(result, &item)
	}

	return result, total, rows.Err()
}

// FindByCodeWithGlobalFallback finds a document type by code, checking tenant first then SYS tenant.
func (r *Repository) FindByCodeWithGlobalFallback(ctx context.Context, tenantID, code string) (*entity.DocumentType, error) {
	var docType entity.DocumentType
	err := r.pool.QueryRow(ctx, queryFindByCodeWithGlobalFallback, tenantID, code).Scan(
		&docType.ID,
		&docType.TenantID,
		&docType.Code,
		&docType.Name,
		&docType.Description,
		&docType.IsGlobal,
		&docType.CreatedAt,
		&docType.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrDocumentTypeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying document type by code with global: %w", err)
	}

	return &docType, nil
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

func scanDocumentTypeListItemWithTemplateCount(rows pgx.Rows) (*entity.DocumentTypeListItem, error) {
	var item entity.DocumentTypeListItem
	if err := rows.Scan(
		&item.ID,
		&item.TenantID,
		&item.Code,
		&item.Name,
		&item.Description,
		&item.TemplatesCount,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return nil, err
	}

	return &item, nil
}

func scanDocumentTypeWithGlobalFallback(rows pgx.Rows) (*entity.DocumentType, error) {
	var docType entity.DocumentType
	if err := rows.Scan(
		&docType.ID,
		&docType.TenantID,
		&docType.Code,
		&docType.Name,
		&docType.Description,
		&docType.IsGlobal,
		&docType.CreatedAt,
		&docType.UpdatedAt,
	); err != nil {
		return nil, err
	}

	return &docType, nil
}
