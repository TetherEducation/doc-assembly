package automation_audit_log_repo

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

const (
	queryCreate = `
INSERT INTO automation.audit_log (
    api_key_id, api_key_prefix, method, path,
    tenant_id, workspace_id, resource_type, resource_id, action,
    request_body, response_status
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

	queryListByKeyID = `
SELECT id, api_key_id, api_key_prefix, method, path,
       tenant_id, workspace_id, resource_type, resource_id, action,
       request_body, response_status, created_at
FROM automation.audit_log
WHERE api_key_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3`
)

// Repository implements port.AutomationAuditLogRepository using PostgreSQL.
type Repository struct {
	pool *pgxpool.Pool
}

// New creates a new automation audit log repository.
func New(pool *pgxpool.Pool) port.AutomationAuditLogRepository {
	return &Repository{pool: pool}
}

// Create persists a new audit log entry.
func (r *Repository) Create(ctx context.Context, log *entity.AutomationAuditLog) error {
	_, err := r.pool.Exec(ctx, queryCreate,
		log.APIKeyID,
		log.APIKeyPrefix,
		log.Method,
		log.Path,
		log.TenantID,
		log.WorkspaceID,
		log.ResourceType,
		log.ResourceID,
		log.Action,
		log.RequestBody,
		log.ResponseStatus,
	)
	if err != nil {
		return fmt.Errorf("creating automation audit log: %w", err)
	}
	return nil
}

// ListByKeyID returns audit log entries for a given API key, newest first, with pagination.
func (r *Repository) ListByKeyID(ctx context.Context, apiKeyID string, limit, offset int) ([]*entity.AutomationAuditLog, error) {
	rows, err := r.pool.Query(ctx, queryListByKeyID, apiKeyID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing automation audit logs for key %s: %w", apiKeyID, err)
	}
	defer rows.Close()

	var logs []*entity.AutomationAuditLog
	for rows.Next() {
		entry := &entity.AutomationAuditLog{}
		if err := rows.Scan(
			&entry.ID,
			&entry.APIKeyID,
			&entry.APIKeyPrefix,
			&entry.Method,
			&entry.Path,
			&entry.TenantID,
			&entry.WorkspaceID,
			&entry.ResourceType,
			&entry.ResourceID,
			&entry.Action,
			&entry.RequestBody,
			&entry.ResponseStatus,
			&entry.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning automation audit log: %w", err)
		}
		logs = append(logs, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating automation audit logs: %w", err)
	}
	return logs, nil
}
