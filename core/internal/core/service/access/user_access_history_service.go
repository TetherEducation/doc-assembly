package access

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	accessuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/access"
)

const (
	// maxRecentAccesses is the maximum number of recent accesses to keep per entity type.
	maxRecentAccesses = 10
)

// NewUserAccessHistoryService creates a new user access history service.
func NewUserAccessHistoryService(
	accessHistoryRepo port.UserAccessHistoryRepository,
) accessuc.UserAccessHistoryUseCase {
	return &UserAccessHistoryService{
		accessHistoryRepo: accessHistoryRepo,
	}
}

// UserAccessHistoryService implements user access history business logic.
type UserAccessHistoryService struct {
	accessHistoryRepo port.UserAccessHistoryRepository
}

// RecordTenantAccess records that a user accessed a tenant.
// Access is validated in the database: user must have tenant membership OR a system role.
func (s *UserAccessHistoryService) RecordTenantAccess(ctx context.Context, userID, tenantID string) error {
	// Record access with DB-level validation (membership OR system role)
	_, err := s.accessHistoryRepo.RecordTenantAccessIfAllowed(ctx, userID, tenantID)
	if err != nil {
		return err
	}

	// Cleanup old entries (keep only last 10)
	if err := s.accessHistoryRepo.DeleteOldAccesses(ctx, userID, entity.AccessEntityTypeTenant, maxRecentAccesses); err != nil {
		// Log but don't fail - cleanup is not critical
		slog.WarnContext(ctx, "failed to cleanup old tenant accesses",
			slog.String("user_id", userID),
			slog.String("error", err.Error()))
	}

	slog.DebugContext(ctx, "recorded tenant access",
		slog.String("user_id", userID),
		slog.String("tenant_id", tenantID))

	return nil
}

// RecordWorkspaceAccess records that a user accessed a workspace.
// Access is validated in the database: user must have workspace membership OR a system role.
func (s *UserAccessHistoryService) RecordWorkspaceAccess(ctx context.Context, userID, workspaceID string) error {
	// Record access with DB-level validation (membership OR system role)
	_, err := s.accessHistoryRepo.RecordWorkspaceAccessIfAllowed(ctx, userID, workspaceID)
	if err != nil {
		return err
	}

	// Cleanup old entries
	if err := s.accessHistoryRepo.DeleteOldAccesses(ctx, userID, entity.AccessEntityTypeWorkspace, maxRecentAccesses); err != nil {
		slog.WarnContext(ctx, "failed to cleanup old workspace accesses",
			slog.String("user_id", userID),
			slog.String("error", err.Error()))
	}

	slog.DebugContext(ctx, "recorded workspace access",
		slog.String("user_id", userID),
		slog.String("workspace_id", workspaceID))

	return nil
}

// GetRecentTenantIDs returns the IDs of recently accessed tenants.
func (s *UserAccessHistoryService) GetRecentTenantIDs(ctx context.Context, userID string) ([]string, error) {
	ids, err := s.accessHistoryRepo.GetRecentAccessIDs(ctx, userID, entity.AccessEntityTypeTenant, maxRecentAccesses)
	if err != nil {
		return nil, fmt.Errorf("getting recent tenant IDs: %w", err)
	}
	return ids, nil
}

// GetRecentWorkspaceIDs returns the IDs of recently accessed workspaces.
func (s *UserAccessHistoryService) GetRecentWorkspaceIDs(ctx context.Context, userID string) ([]string, error) {
	ids, err := s.accessHistoryRepo.GetRecentAccessIDs(ctx, userID, entity.AccessEntityTypeWorkspace, maxRecentAccesses)
	if err != nil {
		return nil, fmt.Errorf("getting recent workspace IDs: %w", err)
	}
	return ids, nil
}
