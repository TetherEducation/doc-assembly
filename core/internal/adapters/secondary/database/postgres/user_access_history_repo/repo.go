package useraccesshistoryrepo

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

// New creates a new user access history repository.
func New(pool *pgxpool.Pool) port.UserAccessHistoryRepository {
	return &Repository{pool: pool}
}

// Repository implements the user access history repository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// RecordAccess records or updates an access entry using UPSERT.
func (r *Repository) RecordAccess(ctx context.Context, userID string, entityType entity.AccessEntityType, entityID string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryRecordAccess, userID, entityType, entityID).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("recording access: %w", err)
	}
	return id, nil
}

// GetRecentAccessIDs returns recently accessed entity IDs.
func (r *Repository) GetRecentAccessIDs(ctx context.Context, userID string, entityType entity.AccessEntityType, limit int) ([]string, error) {
	rows, err := r.pool.Query(ctx, queryGetRecentAccessIDs, userID, entityType, limit)
	if err != nil {
		return nil, fmt.Errorf("querying recent access IDs: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scanning entity ID: %w", err)
		}
		ids = append(ids, id)
	}

	return ids, rows.Err()
}

// GetRecentAccesses returns full access records.
func (r *Repository) GetRecentAccesses(ctx context.Context, userID string, entityType entity.AccessEntityType, limit int) ([]*entity.UserAccessHistory, error) {
	rows, err := r.pool.Query(ctx, queryGetRecentAccesses, userID, entityType, limit)
	if err != nil {
		return nil, fmt.Errorf("querying recent accesses: %w", err)
	}
	defer rows.Close()

	var result []*entity.UserAccessHistory
	for rows.Next() {
		var h entity.UserAccessHistory
		if err := rows.Scan(&h.ID, &h.UserID, &h.EntityType, &h.EntityID, &h.AccessedAt); err != nil {
			return nil, fmt.Errorf("scanning access history: %w", err)
		}
		result = append(result, &h)
	}

	return result, rows.Err()
}

// GetAccessTimesForEntities returns access times for multiple entities.
func (r *Repository) GetAccessTimesForEntities(ctx context.Context, userID string, entityType entity.AccessEntityType, entityIDs []string) (map[string]time.Time, error) {
	if len(entityIDs) == 0 {
		return make(map[string]time.Time), nil
	}

	rows, err := r.pool.Query(ctx, queryGetAccessTimesForEntities, userID, entityType, entityIDs)
	if err != nil {
		return nil, fmt.Errorf("querying access times: %w", err)
	}
	defer rows.Close()

	result := make(map[string]time.Time, len(entityIDs))
	for rows.Next() {
		var entityID string
		var accessedAt time.Time
		if err := rows.Scan(&entityID, &accessedAt); err != nil {
			return nil, fmt.Errorf("scanning access time: %w", err)
		}
		result[entityID] = accessedAt
	}

	return result, rows.Err()
}

// DeleteOldAccesses removes entries beyond keepCount.
func (r *Repository) DeleteOldAccesses(ctx context.Context, userID string, entityType entity.AccessEntityType, keepCount int) error {
	_, err := r.pool.Exec(ctx, queryDeleteOldAccesses, userID, entityType, keepCount)
	if err != nil {
		return fmt.Errorf("deleting old accesses: %w", err)
	}
	return nil
}

// DeleteByEntity removes all history for an entity.
func (r *Repository) DeleteByEntity(ctx context.Context, entityType entity.AccessEntityType, entityID string) error {
	_, err := r.pool.Exec(ctx, queryDeleteByEntity, entityType, entityID)
	if err != nil {
		return fmt.Errorf("deleting access history by entity: %w", err)
	}
	return nil
}

// RecordTenantAccessIfAllowed records tenant access only if user has membership or system role.
func (r *Repository) RecordTenantAccessIfAllowed(ctx context.Context, userID, tenantID string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryRecordTenantAccessIfAllowed, userID, tenantID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", entity.ErrForbidden
		}
		return "", fmt.Errorf("recording tenant access: %w", err)
	}
	return id, nil
}

// RecordWorkspaceAccessIfAllowed records workspace access only if user has membership or system role.
func (r *Repository) RecordWorkspaceAccessIfAllowed(ctx context.Context, userID, workspaceID string) (string, error) {
	var id string
	err := r.pool.QueryRow(ctx, queryRecordWorkspaceAccessIfAllowed, userID, workspaceID).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", entity.ErrForbidden
		}
		return "", fmt.Errorf("recording workspace access: %w", err)
	}
	return id, nil
}
