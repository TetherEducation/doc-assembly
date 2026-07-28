package port

import (
	"context"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// UserAccessHistoryRepository defines the interface for user access history data access.
type UserAccessHistoryRepository interface {
	// RecordAccess records or updates an access entry using UPSERT semantics.
	// If the entry exists, updates accessed_at. Returns the record ID.
	RecordAccess(ctx context.Context, userID string, entityType entity.AccessEntityType, entityID string) (string, error)

	// GetRecentAccessIDs returns the IDs of recently accessed entities of a given type.
	// Returns up to `limit` entity IDs, ordered by most recent first.
	GetRecentAccessIDs(ctx context.Context, userID string, entityType entity.AccessEntityType, limit int) ([]string, error)

	// GetRecentAccesses returns full access records for a user and entity type.
	GetRecentAccesses(ctx context.Context, userID string, entityType entity.AccessEntityType, limit int) ([]*entity.UserAccessHistory, error)

	// GetAccessTimesForEntities returns the last access time for multiple entities.
	// Returns a map of entityID -> accessedAt. Missing entries mean no access recorded.
	GetAccessTimesForEntities(ctx context.Context, userID string, entityType entity.AccessEntityType, entityIDs []string) (map[string]time.Time, error)

	// DeleteOldAccesses removes entries beyond the most recent N for cleanup.
	// This is called after recording access to maintain the limit.
	DeleteOldAccesses(ctx context.Context, userID string, entityType entity.AccessEntityType, keepCount int) error

	// DeleteByEntity removes all access history for a specific entity.
	// Useful when entity is deleted.
	DeleteByEntity(ctx context.Context, entityType entity.AccessEntityType, entityID string) error

	// RecordTenantAccessIfAllowed records tenant access only if user has membership or system role.
	// Returns ErrForbidden if user has no access to the tenant.
	RecordTenantAccessIfAllowed(ctx context.Context, userID, tenantID string) (string, error)

	// RecordWorkspaceAccessIfAllowed records workspace access only if user has membership or system role.
	// Returns ErrForbidden if user has no access to the workspace.
	RecordWorkspaceAccessIfAllowed(ctx context.Context, userID, workspaceID string) (string, error)
}
