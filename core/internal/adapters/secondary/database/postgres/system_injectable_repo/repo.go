package systeminjectablerepo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// New creates a new system injectable repository.
func New(pool *pgxpool.Pool) port.SystemInjectableRepository {
	return &Repository{pool: pool}
}

// Repository implements port.SystemInjectableRepository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// FindActiveKeysForWorkspace returns the keys of active system injectables for a workspace.
func (r *Repository) FindActiveKeysForWorkspace(ctx context.Context, workspaceID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, queryFindActiveKeysForWorkspace, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("querying active system injectable keys: %w", err)
	}
	defer rows.Close()

	return collectSystemInjectableKeys(rows)
}

// FindActiveKeysForWorkspaceAndKeys returns active system injectable keys restricted to requested keys.
func (r *Repository) FindActiveKeysForWorkspaceAndKeys(ctx context.Context, workspaceID string, keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	rows, err := r.pool.Query(ctx, queryFindActiveKeysForWorkspaceAndKeys, workspaceID, keys)
	if err != nil {
		return nil, fmt.Errorf("querying active system injectable keys by keys: %w", err)
	}
	defer rows.Close()

	return collectSystemInjectableKeys(rows)
}

func collectSystemInjectableKeys(rows pgx.Rows) ([]string, error) {
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scanning system injectable key: %w", err)
		}
		keys = append(keys, key)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating system injectable keys: %w", err)
	}

	return keys, nil
}

// FindAllDefinitions returns a map of all definition keys to their is_active status.
func (r *Repository) FindAllDefinitions(ctx context.Context) (map[string]bool, error) {
	rows, err := r.pool.Query(ctx, queryFindAllDefinitions)
	if err != nil {
		return nil, fmt.Errorf("querying all definitions: %w", err)
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var key string
		var isActive bool
		if err := rows.Scan(&key, &isActive); err != nil {
			return nil, fmt.Errorf("scanning definition: %w", err)
		}
		result[key] = isActive
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating definitions: %w", err)
	}

	return result, nil
}

// UpsertDefinition creates or updates a system injectable definition.
func (r *Repository) UpsertDefinition(ctx context.Context, key string, isActive bool) error {
	_, err := r.pool.Exec(ctx, queryUpsertDefinition, key, isActive)
	if err != nil {
		return fmt.Errorf("upserting definition %s: %w", key, err)
	}
	return nil
}

// EnsureDefinition creates an active system injectable definition only when it is missing.
func (r *Repository) EnsureDefinition(ctx context.Context, key string) error {
	_, err := r.pool.Exec(ctx, queryEnsureDefinition, key)
	if err != nil {
		return fmt.Errorf("ensuring definition %s: %w", key, err)
	}
	return nil
}

// FindAssignmentsByKey returns all assignments for a given injectable key with tenant/workspace names.
func (r *Repository) FindAssignmentsByKey(ctx context.Context, key string) ([]*entity.SystemInjectableAssignment, error) {
	rows, err := r.pool.Query(ctx, queryFindAssignmentsByKey, key)
	if err != nil {
		return nil, fmt.Errorf("querying assignments for key %s: %w", key, err)
	}
	defer rows.Close()

	var assignments []*entity.SystemInjectableAssignment
	for rows.Next() {
		var a entity.SystemInjectableAssignment
		var scopeType string
		var workspaceTenantID *string
		var workspaceTenantName *string
		if err := rows.Scan(
			&a.ID,
			&a.InjectableKey,
			&scopeType,
			&a.TenantID,
			&a.TenantName,
			&a.WorkspaceID,
			&a.WorkspaceName,
			&workspaceTenantID,
			&workspaceTenantName,
			&a.IsActive,
			&a.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning assignment: %w", err)
		}
		a.ScopeType = entity.InjectableScopeType(scopeType)

		// For WORKSPACE scope, set TenantID and TenantName from workspace's tenant
		if a.ScopeType == entity.InjectableScopeWorkspace {
			a.TenantID = workspaceTenantID
			a.TenantName = workspaceTenantName
		}

		assignments = append(assignments, &a)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating assignments: %w", err)
	}

	return assignments, nil
}

// CreateAssignment creates a new assignment.
func (r *Repository) CreateAssignment(ctx context.Context, assignment *entity.SystemInjectableAssignment) error {
	_, err := r.pool.Exec(ctx, queryCreateAssignment,
		assignment.ID,
		assignment.InjectableKey,
		string(assignment.ScopeType),
		assignment.TenantID,
		assignment.WorkspaceID,
		assignment.IsActive,
	)
	if err != nil {
		return fmt.Errorf("creating assignment: %w", err)
	}
	return nil
}

// DeleteAssignment removes an assignment by ID.
func (r *Repository) DeleteAssignment(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, queryDeleteAssignment, id)
	if err != nil {
		return fmt.Errorf("deleting assignment %s: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return entity.ErrAssignmentNotFound
	}
	return nil
}

// SetAssignmentActive updates the is_active flag for an assignment.
func (r *Repository) SetAssignmentActive(ctx context.Context, id string, isActive bool) error {
	result, err := r.pool.Exec(ctx, querySetAssignmentActive, isActive, id)
	if err != nil {
		return fmt.Errorf("updating assignment %s is_active: %w", id, err)
	}
	if result.RowsAffected() == 0 {
		return entity.ErrAssignmentNotFound
	}
	return nil
}

// FindPublicActiveKeys returns a set of injectable keys that have an active PUBLIC assignment.
func (r *Repository) FindPublicActiveKeys(ctx context.Context) (map[string]bool, error) {
	rows, err := r.pool.Query(ctx, queryFindPublicActiveKeys)
	if err != nil {
		return nil, fmt.Errorf("querying public active keys: %w", err)
	}
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, fmt.Errorf("scanning public active key: %w", err)
		}
		result[key] = true
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating public active keys: %w", err)
	}

	return result, nil
}

// CreatePublicAssignments creates PUBLIC assignments for multiple keys using batch.
func (r *Repository) CreatePublicAssignments(ctx context.Context, keys []string) (int, error) {
	if len(keys) == 0 {
		return 0, nil
	}

	batch := &pgx.Batch{}
	for _, key := range keys {
		batch.Queue(queryCreatePublicAssignment, key)
	}

	results := r.pool.SendBatch(ctx, batch)
	defer func() { _ = results.Close() }()

	created := 0
	for i := 0; i < len(keys); i++ {
		result, err := results.Exec()
		if err != nil {
			return created, fmt.Errorf("creating PUBLIC assignment for key %s: %w", keys[i], err)
		}
		if result.RowsAffected() > 0 {
			created++
		}
	}

	return created, nil
}

// DeletePublicAssignments deletes PUBLIC assignments for multiple keys.
func (r *Repository) DeletePublicAssignments(ctx context.Context, keys []string) (int, error) {
	if len(keys) == 0 {
		return 0, nil
	}

	result, err := r.pool.Exec(ctx, queryDeletePublicAssignments, keys)
	if err != nil {
		return 0, fmt.Errorf("deleting PUBLIC assignments: %w", err)
	}

	return int(result.RowsAffected()), nil
}

// FindPublicAssignmentsByKeys returns a map of key -> assignmentID for PUBLIC assignments.
func (r *Repository) FindPublicAssignmentsByKeys(ctx context.Context, keys []string) (map[string]string, error) {
	if len(keys) == 0 {
		return make(map[string]string), nil
	}

	rows, err := r.pool.Query(ctx, queryFindPublicAssignmentsByKeys, keys)
	if err != nil {
		return nil, fmt.Errorf("querying PUBLIC assignments by keys: %w", err)
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var key, id string
		if err := rows.Scan(&key, &id); err != nil {
			return nil, fmt.Errorf("scanning PUBLIC assignment: %w", err)
		}
		result[key] = id
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating PUBLIC assignments: %w", err)
	}

	return result, nil
}
