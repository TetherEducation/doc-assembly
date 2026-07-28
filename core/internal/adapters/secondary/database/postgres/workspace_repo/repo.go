package workspacerepo

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new workspace repository.
func New(pool *pgxpool.Pool) port.WorkspaceRepository {
	return &Repository{pool: pool}
}

// Repository implements the workspace repository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// Create creates a new workspace.
func (r *Repository) Create(ctx context.Context, workspace *entity.Workspace) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryCreate,
		workspace.ID,
		workspace.TenantID,
		workspace.Name,
		workspace.Code,
		workspace.Type,
		workspace.Status,
		workspace.CreatedAt,
	).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("inserting workspace: %w", err)
	}

	return id, nil
}

// CreateSandbox creates a sandbox workspace with is_sandbox=true and sandbox_of_id set.
func (r *Repository) CreateSandbox(ctx context.Context, sandbox *entity.Workspace) (string, error) {
	if err := validateSandboxWorkspace(sandbox); err != nil {
		return "", err
	}

	var id string
	err := r.pool.QueryRow(ctx, queryCreateSandbox,
		sandbox.ID,
		sandbox.TenantID,
		sandbox.Name,
		sandbox.Code,
		sandbox.Type,
		sandbox.Status,
		sandbox.SandboxOfID,
		sandbox.CreatedAt,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", entity.ErrSandboxNotSupported
	}
	if err != nil {
		return "", fmt.Errorf("inserting sandbox workspace: %w", err)
	}

	return id, nil
}

func validateSandboxWorkspace(sandbox *entity.Workspace) error {
	if sandbox == nil || sandbox.SandboxOfID == nil || *sandbox.SandboxOfID == "" {
		return entity.ErrRequiredField
	}
	if !sandbox.IsSandbox || sandbox.Type != entity.WorkspaceTypeClient {
		return entity.ErrSandboxNotSupported
	}
	return nil
}

// FindByID finds a workspace by ID.
func (r *Repository) FindByID(ctx context.Context, id string) (*entity.Workspace, error) {
	var ws entity.Workspace
	err := r.pool.QueryRow(ctx, queryFindByID, id).Scan(
		&ws.ID,
		&ws.TenantID,
		&ws.Name,
		&ws.Code,
		&ws.Type,
		&ws.Status,
		&ws.IsSandbox,
		&ws.SandboxOfID,
		&ws.CreatedAt,
		&ws.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrWorkspaceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying workspace: %w", err)
	}

	return &ws, nil
}

// FindByCode finds a workspace by tenant and business code.
func (r *Repository) FindByCode(ctx context.Context, tenantID, code string) (*entity.Workspace, error) {
	var ws entity.Workspace
	err := r.pool.QueryRow(ctx, queryFindByCode, tenantID, code).Scan(
		&ws.ID,
		&ws.TenantID,
		&ws.Name,
		&ws.Code,
		&ws.Type,
		&ws.Status,
		&ws.IsSandbox,
		&ws.SandboxOfID,
		&ws.CreatedAt,
		&ws.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrWorkspaceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying workspace by code: %w", err)
	}

	return &ws, nil
}

// FindByCodeIncludingSandbox finds a workspace by tenant and business code, including sandbox workspaces.
func (r *Repository) FindByCodeIncludingSandbox(ctx context.Context, tenantID, code string) (*entity.Workspace, error) {
	var ws entity.Workspace
	err := r.pool.QueryRow(ctx, queryFindByCodeIncludingSandbox, tenantID, code).Scan(
		&ws.ID,
		&ws.TenantID,
		&ws.Name,
		&ws.Code,
		&ws.Type,
		&ws.Status,
		&ws.IsSandbox,
		&ws.SandboxOfID,
		&ws.CreatedAt,
		&ws.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrWorkspaceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying workspace by code including sandbox: %w", err)
	}

	return &ws, nil
}

// FindSandboxByParentID finds the sandbox workspace for a given parent workspace ID.
func (r *Repository) FindSandboxByParentID(ctx context.Context, parentID string) (*entity.Workspace, error) {
	var ws entity.Workspace
	err := r.pool.QueryRow(ctx, queryFindSandboxByParentID, parentID).Scan(
		&ws.ID,
		&ws.TenantID,
		&ws.Name,
		&ws.Code,
		&ws.Type,
		&ws.Status,
		&ws.IsSandbox,
		&ws.SandboxOfID,
		&ws.CreatedAt,
		&ws.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrSandboxNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying sandbox workspace: %w", err)
	}

	return &ws, nil
}

// FindByTenantPaginated lists workspaces for a tenant with pagination and optional search.
// When filters.Query is provided, orders by similarity. Otherwise, orders by access history.
func (r *Repository) FindByTenantPaginated(ctx context.Context, tenantID string, filters port.WorkspaceFilters) ([]*entity.Workspace, int64, error) {
	var total int64

	// Convert empty UserID to nil so the queries receive NULL (not invalid UUID).
	var userID interface{}
	if filters.UserID != "" {
		userID = filters.UserID
	}

	// Get total count with search and status filter
	if err := r.pool.QueryRow(ctx, queryCountByTenant, tenantID, filters.Query, filters.Status, userID, filters.AccessibleOnly).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting workspaces: %w", err)
	}

	// Query with unified ordering (similarity when Query provided, access history otherwise)
	rows, err := r.pool.Query(ctx, queryFindByTenantPaginated, tenantID, userID, filters.Query, filters.Limit, filters.Offset, filters.Status, filters.AccessibleOnly)
	if err != nil {
		return nil, 0, fmt.Errorf("querying workspaces: %w", err)
	}
	defer rows.Close()

	workspaces, err := scanWorkspaces(rows)
	if err != nil {
		return nil, 0, err
	}

	return workspaces, total, nil
}

// FindByUser lists all workspaces a user has access to.
func (r *Repository) FindByUser(ctx context.Context, userID string) ([]*entity.WorkspaceWithRole, error) {
	rows, err := r.pool.Query(ctx, queryFindByUser, userID)
	if err != nil {
		return nil, fmt.Errorf("querying user workspaces: %w", err)
	}
	defer rows.Close()

	return scanWorkspacesWithRole(rows)
}

// FindSystemByTenant finds the system workspace for a tenant.
func (r *Repository) FindSystemByTenant(ctx context.Context, tenantID *string) (*entity.Workspace, error) {
	var query string
	var args []any

	if tenantID == nil {
		query = queryFindSystemByTenantNull
	} else {
		query = queryFindSystemByTenant
		args = append(args, *tenantID)
	}

	var ws entity.Workspace
	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&ws.ID,
		&ws.TenantID,
		&ws.Name,
		&ws.Code,
		&ws.Type,
		&ws.Status,
		&ws.IsSandbox,
		&ws.SandboxOfID,
		&ws.CreatedAt,
		&ws.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, entity.ErrWorkspaceNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying system workspace: %w", err)
	}

	return &ws, nil
}

// Update updates a workspace.
func (r *Repository) Update(ctx context.Context, workspace *entity.Workspace) error {
	_, err := r.pool.Exec(ctx, queryUpdate,
		workspace.ID,
		workspace.Name,
		workspace.Code,
		workspace.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("updating workspace: %w", err)
	}

	return nil
}

// UpdateStatus updates a workspace's status.
func (r *Repository) UpdateStatus(ctx context.Context, id string, status entity.WorkspaceStatus) error {
	result, err := r.pool.Exec(ctx, queryUpdateStatus, id, status)
	if err != nil {
		return fmt.Errorf("updating workspace status: %w", err)
	}

	if result.RowsAffected() == 0 {
		return entity.ErrWorkspaceNotFound
	}

	return nil
}

// Delete deletes a workspace (soft delete by archiving).
func (r *Repository) Delete(ctx context.Context, id string) error {
	return r.UpdateStatus(ctx, id, entity.WorkspaceStatusArchived)
}

// ExistsSystemForTenant checks if a system workspace exists for a tenant.
func (r *Repository) ExistsSystemForTenant(ctx context.Context, tenantID *string) (bool, error) {
	var query string
	var args []any

	if tenantID == nil {
		query = queryExistsSystemForTenantNull
	} else {
		query = queryExistsSystemForTenant
		args = append(args, *tenantID)
	}

	var exists bool
	err := r.pool.QueryRow(ctx, query, args...).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking system workspace existence: %w", err)
	}

	return exists, nil
}

// ExistsByCodeForTenant checks if a workspace with the given code exists in a tenant.
func (r *Repository) ExistsByCodeForTenant(ctx context.Context, tenantID, code, excludeID string) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, queryExistsByCodeForTenant, tenantID, code, excludeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("checking workspace code existence: %w", err)
	}
	return exists, nil
}

// scanWorkspaces scans workspace rows into a slice.
func scanWorkspaces(rows pgx.Rows) ([]*entity.Workspace, error) {
	var result []*entity.Workspace
	for rows.Next() {
		var ws entity.Workspace
		err := rows.Scan(
			&ws.ID,
			&ws.TenantID,
			&ws.Name,
			&ws.Code,
			&ws.Type,
			&ws.Status,
			&ws.IsSandbox,
			&ws.SandboxOfID,
			&ws.CreatedAt,
			&ws.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning workspace: %w", err)
		}
		result = append(result, &ws)
	}
	return result, rows.Err()
}

// scanWorkspacesWithRole scans workspace rows with role into a slice.
func scanWorkspacesWithRole(rows pgx.Rows) ([]*entity.WorkspaceWithRole, error) {
	var result []*entity.WorkspaceWithRole
	for rows.Next() {
		var ws entity.Workspace
		var role entity.WorkspaceRole
		err := rows.Scan(
			&ws.ID,
			&ws.TenantID,
			&ws.Name,
			&ws.Code,
			&ws.Type,
			&ws.Status,
			&ws.IsSandbox,
			&ws.SandboxOfID,
			&ws.CreatedAt,
			&ws.UpdatedAt,
			&role,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning workspace with role: %w", err)
		}
		result = append(result, &entity.WorkspaceWithRole{
			Workspace: ws,
			Role:      role,
		})
	}
	return result, rows.Err()
}
