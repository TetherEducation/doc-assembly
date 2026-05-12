package documentrepo

const fullDocumentColumns = `
	id, workspace_id, template_version_id, document_type_id, title, client_external_reference_id,
	transactional_id, operation_type, related_document_id, active_attempt_id,
	status, injected_values_snapshot, completed_pdf_url, is_active, superseded_at,
	superseded_by_document_id, supersede_reason, expires_at, metadata,
	created_at, updated_at
`

const (
	queryCreate = `
		INSERT INTO execution.documents (
			workspace_id, template_version_id, document_type_id, title, client_external_reference_id,
			transactional_id, operation_type, related_document_id, active_attempt_id,
			status, injected_values_snapshot, completed_pdf_url, is_active, superseded_at,
			superseded_by_document_id, supersede_reason, expires_at, metadata, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19)
		RETURNING id
	`

	queryFindByID = `SELECT ` + fullDocumentColumns + ` FROM execution.documents WHERE id = $1`

	queryFindByClientExternalRef = `
		SELECT ` + fullDocumentColumns + `
		FROM execution.documents
		WHERE workspace_id = $1 AND client_external_reference_id = $2
		ORDER BY created_at DESC
	`

	queryFindByTemplateVersion = `
		SELECT id, workspace_id, template_version_id, title, client_external_reference_id,
		       NULL::text AS signer_provider, status, created_at, updated_at
		FROM execution.documents
		WHERE template_version_id = $1
		ORDER BY created_at DESC
	`

	queryFindExpired = `
		SELECT ` + fullDocumentColumns + `
		FROM execution.documents
		WHERE status IN ('READY_TO_SIGN', 'SIGNING')
		  AND expires_at IS NOT NULL
		  AND expires_at <= NOW()
		ORDER BY expires_at ASC
		LIMIT $1
	`

	queryFindByWorkspaceBase = `
		WITH base_documents AS (
			SELECT
				d.id,
				d.workspace_id,
				d.template_version_id,
				d.document_type_id,
				d.title,
				d.client_external_reference_id,
				d.status,
				d.created_at,
				d.updated_at
			FROM execution.documents d
			WHERE d.workspace_id = $1
	`

	queryFindByWorkspaceProjection = `
		)
		SELECT
			b.id,
			b.workspace_id,
			b.template_version_id,
			b.document_type_id,
			COALESCE(dt.name->>'en', dt.name->>'es') AS document_type_name,
			COALESCE(t.title, tv.name, '') AS template_name,
			b.title,
			b.client_external_reference_id,
			NULL::text AS signer_provider,
			COALESCE(recipients.items, '[]'::jsonb) AS recipients,
			b.status,
			b.created_at,
			b.updated_at
		FROM base_documents b
		LEFT JOIN content.template_versions tv ON tv.id = b.template_version_id
		LEFT JOIN content.templates t ON t.id = tv.template_id
		LEFT JOIN content.document_types dt ON dt.id = b.document_type_id
		LEFT JOIN LATERAL (
			SELECT jsonb_agg(
				jsonb_build_object(
					'id', dr.id,
					'documentId', dr.document_id,
					'templateVersionRoleId', dr.template_version_role_id,
					'name', dr.name,
					'email', dr.email,
					'status', dr.status,
					'roleName', tvsr.role_name,
					'signerOrder', tvsr.signer_order
				)
				ORDER BY tvsr.signer_order ASC NULLS LAST, dr.created_at ASC
			) AS items
			FROM execution.document_recipients dr
			LEFT JOIN content.template_version_signer_roles tvsr ON tvsr.id = dr.template_version_role_id
			WHERE dr.document_id = b.id
		) recipients ON TRUE
		ORDER BY b.created_at DESC
	`

	queryUpdate = `
		UPDATE execution.documents
		SET document_type_id = $2, title = $3, client_external_reference_id = $4,
			transactional_id = $5, operation_type = $6, related_document_id = $7,
			active_attempt_id = $8, status = $9, injected_values_snapshot = $10,
			completed_pdf_url = $11, is_active = $12, superseded_at = $13,
			superseded_by_document_id = $14, supersede_reason = $15, expires_at = $16,
			metadata = $17, updated_at = $18
		WHERE id = $1
	`

	queryUpdateStatus = `
		UPDATE execution.documents
		SET status = $2, updated_at = NOW()
		WHERE id = $1
	`

	queryDelete = `DELETE FROM execution.documents WHERE id = $1`

	queryCountByWorkspace = `SELECT COUNT(*) FROM execution.documents WHERE workspace_id = $1`

	queryCountByStatus = `SELECT COUNT(*) FROM execution.documents WHERE workspace_id = $1 AND status = $2`

	queryListDistinctDocumentTypesForWorkspace = `
		SELECT DISTINCT
			d.document_type_id,
			COALESCE(dt.name->>'en', dt.name->>'es', '') AS name
		FROM execution.documents d
		LEFT JOIN content.document_types dt ON dt.id = d.document_type_id
		WHERE d.workspace_id = $1
		  AND d.document_type_id IS NOT NULL
		ORDER BY name ASC, d.document_type_id ASC
	`
)
