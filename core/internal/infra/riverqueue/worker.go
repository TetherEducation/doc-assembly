package riverqueue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// buildCompletedEvent loads fresh document and active-attempt data from the database.
func buildCompletedEvent(ctx context.Context, pool *pgxpool.Pool, documentID string, attemptID string) (port.DocumentCompletedEvent, error) {
	event, err := loadCompletedDocument(ctx, pool, documentID, attemptID)
	if err != nil {
		return event, err
	}
	recipients, err := loadCompletedRecipients(ctx, pool, attemptID)
	if err != nil {
		return event, err
	}
	event.Recipients = recipients
	return event, nil
}

func loadCompletedDocument(ctx context.Context, pool *pgxpool.Pool, documentID, attemptID string) (port.DocumentCompletedEvent, error) {
	var event port.DocumentCompletedEvent
	var isSandbox bool
	var rawMetadata json.RawMessage
	err := pool.QueryRow(ctx, `
		SELECT d.id,
			d.status,
			d.client_external_reference_id,
			d.title,
			d.created_at,
			d.updated_at,
			d.expires_at,
			d.metadata,
			w.code AS workspace_code,
			w.is_sandbox,
			t.code AS tenant_code,
			dt.code AS document_type_code
		FROM execution.documents d
			JOIN tenancy.workspaces w ON w.id = d.workspace_id
			JOIN tenancy.tenants t ON t.id = w.tenant_id
			JOIN content.template_versions tv ON tv.id = d.template_version_id
			JOIN content.templates tp ON tp.id = tv.template_id
			LEFT JOIN content.document_types dt ON dt.id = tp.document_type_id
		WHERE d.id = $1 AND d.active_attempt_id = $2
	`, documentID, attemptID).Scan(
		&event.DocumentID,
		&event.Status,
		&event.ExternalID,
		&event.Title,
		&event.CreatedAt,
		&event.UpdatedAt,
		&event.ExpiresAt,
		&rawMetadata,
		&event.WorkspaceCode,
		&isSandbox,
		&event.TenantCode,
		&event.DocumentTypeCode,
	)
	if err != nil {
		return event, fmt.Errorf("querying active completed document %s attempt %s: %w", documentID, attemptID, err)
	}
	event.Environment = entity.EnvironmentFromSandbox(isSandbox)
	if len(rawMetadata) > 0 {
		if err := json.Unmarshal(rawMetadata, &event.Metadata); err != nil {
			return event, fmt.Errorf("unmarshalling metadata for document %s: %w", documentID, err)
		}
	}
	return event, nil
}

func loadCompletedRecipients(ctx context.Context, pool *pgxpool.Pool, attemptID string) ([]port.CompletedRecipient, error) {
	rows, err := pool.Query(ctx, `
		SELECT sr.role_name, sar.signer_order, sar.name, sar.email, sar.status, sar.signed_at
		FROM execution.signing_attempt_recipients sar
		LEFT JOIN content.template_version_signer_roles sr ON sr.id = sar.template_version_role_id
		WHERE sar.attempt_id = $1
		ORDER BY sar.signer_order ASC
	`, attemptID)
	if err != nil {
		return nil, fmt.Errorf("querying attempt recipients for %s: %w", attemptID, err)
	}
	defer rows.Close()

	var recipients []port.CompletedRecipient
	for rows.Next() {
		var r port.CompletedRecipient
		if err := rows.Scan(&r.RoleName, &r.SignerOrder, &r.Name, &r.Email, &r.Status, &r.SignedAt); err != nil {
			return nil, fmt.Errorf("scanning attempt recipient: %w", err)
		}
		recipients = append(recipients, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating attempt recipients: %w", err)
	}
	return recipients, nil
}
