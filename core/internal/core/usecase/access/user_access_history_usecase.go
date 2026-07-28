package access

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// RecordAccessCommand contains data for recording a resource access.
type RecordAccessCommand struct {
	UserID     string
	EntityType entity.AccessEntityType
	EntityID   string
}

// UserAccessHistoryUseCase defines the interface for user access history operations.
type UserAccessHistoryUseCase interface {
	// RecordTenantAccess records that a user accessed a tenant.
	RecordTenantAccess(ctx context.Context, userID, tenantID string) error

	// RecordWorkspaceAccess records that a user accessed a workspace.
	RecordWorkspaceAccess(ctx context.Context, userID, workspaceID string) error

	// GetRecentTenantIDs returns the IDs of recently accessed tenants for a user.
	GetRecentTenantIDs(ctx context.Context, userID string) ([]string, error)

	// GetRecentWorkspaceIDs returns the IDs of recently accessed workspaces for a user.
	GetRecentWorkspaceIDs(ctx context.Context, userID string) ([]string, error)
}
