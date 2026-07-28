package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// AutomationAPIKeyRepository defines the interface for automation API key data access.
type AutomationAPIKeyRepository interface {
	// Create persists a new API key and returns the created entity.
	Create(ctx context.Context, key *entity.AutomationAPIKey) (*entity.AutomationAPIKey, error)

	// FindByHash looks up an active API key by its SHA-256 hash.
	FindByHash(ctx context.Context, keyHash string) (*entity.AutomationAPIKey, error)

	// FindByID retrieves an API key by its UUID.
	FindByID(ctx context.Context, id string) (*entity.AutomationAPIKey, error)

	// List returns all API keys (active and revoked), ordered by created_at DESC.
	List(ctx context.Context) ([]*entity.AutomationAPIKey, error)

	// Update applies name and allowed_tenants changes to an existing key.
	Update(ctx context.Context, key *entity.AutomationAPIKey) (*entity.AutomationAPIKey, error)

	// Revoke sets revoked_at = NOW() and is_active = false for the given key ID.
	Revoke(ctx context.Context, id string) error

	// TouchLastUsed updates last_used_at = NOW() for the given key ID.
	TouchLastUsed(ctx context.Context, id string) error
}

// AutomationAuditLogRepository defines the interface for automation audit log data access.
type AutomationAuditLogRepository interface {
	// Create persists a new audit log entry.
	Create(ctx context.Context, log *entity.AutomationAuditLog) error

	// ListByKeyID returns audit log entries for a given API key, newest first, with pagination.
	ListByKeyID(ctx context.Context, apiKeyID string, limit, offset int) ([]*entity.AutomationAuditLog, error)
}
