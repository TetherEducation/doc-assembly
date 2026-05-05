package templateversionrepo

// SQL queries for template version operations.
const (
	queryCreate = `
		INSERT INTO content.template_versions (
			template_id, version_number, name, description, content_structure,
			status, scheduled_publish_at, scheduled_archive_at, signing_workflow_config,
			created_by, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id`

	queryFindByID = `
		SELECT id, template_id, version_number, name, description, content_structure,
			status, scheduled_publish_at, scheduled_archive_at, signing_workflow_config,
			published_at, archived_at, published_by, archived_by, created_by, created_at, updated_at
		FROM content.template_versions
		WHERE id = $1`

	queryFindByIDWithDetails = `
		SELECT
			tv.id, tv.template_id, tv.version_number, tv.name, tv.description, tv.content_structure,
			tv.status, tv.scheduled_publish_at, tv.scheduled_archive_at, tv.signing_workflow_config,
			tv.published_at, tv.archived_at, tv.published_by, tv.archived_by, tv.created_by, tv.created_at, tv.updated_at,
			COALESCE(injectables.items, '[]'::jsonb) AS injectables,
			COALESCE(signer_roles.items, '[]'::jsonb) AS signer_roles
		FROM content.template_versions tv
		LEFT JOIN LATERAL (
			SELECT jsonb_agg(
				jsonb_build_object(
					'id', tvi.id,
					'templateVersionId', tvi.template_version_id,
					'injectableDefinitionId', tvi.injectable_definition_id,
					'systemInjectableKey', tvi.system_injectable_key,
					'isRequired', tvi.is_required,
					'defaultValue', tvi.default_value,
					'createdAt', tvi.created_at,
					'definition', CASE WHEN id.id IS NULL THEN NULL ELSE jsonb_build_object(
						'id', id.id,
						'workspaceId', id.workspace_id,
						'key', id.key,
						'label', id.label,
						'description', id.description,
						'dataType', id.data_type,
						'createdAt', id.created_at,
						'updatedAt', id.updated_at
					) END
				)
				ORDER BY COALESCE(id.key, tvi.system_injectable_key)
			) AS items
			FROM content.template_version_injectables tvi
			LEFT JOIN content.injectable_definitions id ON tvi.injectable_definition_id = id.id
			WHERE tvi.template_version_id = tv.id
		) injectables ON TRUE
		LEFT JOIN LATERAL (
			SELECT jsonb_agg(
				jsonb_build_object(
					'id', id,
					'templateVersionId', template_version_id,
					'roleName', role_name,
					'anchorString', anchor_string,
					'signerOrder', signer_order,
					'createdAt', created_at,
					'updatedAt', updated_at
				)
				ORDER BY signer_order
			) AS items
			FROM content.template_version_signer_roles
			WHERE template_version_id = tv.id
		) signer_roles ON TRUE
		WHERE tv.id = $1`

	queryFindByIDWithDetailsAndTemplateWorkspace = `
		SELECT
			tv.id, tv.template_id, tv.version_number, tv.name, tv.description, tv.content_structure,
			tv.status, tv.scheduled_publish_at, tv.scheduled_archive_at, tv.signing_workflow_config,
			tv.published_at, tv.archived_at, tv.published_by, tv.archived_by, tv.created_by, tv.created_at, tv.updated_at,
			COALESCE(injectables.items, '[]'::jsonb) AS injectables,
			COALESCE(signer_roles.items, '[]'::jsonb) AS signer_roles,
			t.id, t.workspace_id, t.folder_id, t.document_type_id, t.title,
			t.is_public_library, t.process, t.process_type, t.created_at, t.updated_at,
			w.id, w.tenant_id, w.name, w.code, w.type, w.status,
			w.is_sandbox, w.sandbox_of_id, w.created_at, w.updated_at
		FROM content.template_versions tv
		JOIN content.templates t ON t.id = tv.template_id
		JOIN tenancy.workspaces w ON w.id = t.workspace_id
		LEFT JOIN LATERAL (
			SELECT jsonb_agg(
				jsonb_build_object(
					'id', tvi.id,
					'templateVersionId', tvi.template_version_id,
					'injectableDefinitionId', tvi.injectable_definition_id,
					'systemInjectableKey', tvi.system_injectable_key,
					'isRequired', tvi.is_required,
					'defaultValue', tvi.default_value,
					'createdAt', tvi.created_at,
					'definition', CASE WHEN id.id IS NULL THEN NULL ELSE jsonb_build_object(
						'id', id.id,
						'workspaceId', id.workspace_id,
						'key', id.key,
						'label', id.label,
						'description', id.description,
						'dataType', id.data_type,
						'createdAt', id.created_at,
						'updatedAt', id.updated_at
					) END
				)
				ORDER BY COALESCE(id.key, tvi.system_injectable_key)
			) AS items
			FROM content.template_version_injectables tvi
			LEFT JOIN content.injectable_definitions id ON tvi.injectable_definition_id = id.id
			WHERE tvi.template_version_id = tv.id
		) injectables ON TRUE
		LEFT JOIN LATERAL (
			SELECT jsonb_agg(
				jsonb_build_object(
					'id', id,
					'templateVersionId', template_version_id,
					'roleName', role_name,
					'anchorString', anchor_string,
					'signerOrder', signer_order,
					'createdAt', created_at,
					'updatedAt', updated_at
				)
				ORDER BY signer_order
			) AS items
			FROM content.template_version_signer_roles
			WHERE template_version_id = tv.id
		) signer_roles ON TRUE
		WHERE tv.id = $1`

	queryFindByTemplateID = `
		SELECT id, template_id, version_number, name, description, content_structure,
			status, scheduled_publish_at, scheduled_archive_at, signing_workflow_config,
			published_at, archived_at, published_by, archived_by, created_by, created_at, updated_at
		FROM content.template_versions
		WHERE template_id = $1
		ORDER BY version_number DESC`

	queryFindPublishedByTemplateID = `
		SELECT id, template_id, version_number, name, description, content_structure,
			status, scheduled_publish_at, scheduled_archive_at, signing_workflow_config,
			published_at, archived_at, published_by, archived_by, created_by, created_at, updated_at
		FROM content.template_versions
		WHERE template_id = $1 AND status = 'PUBLISHED'`

	queryFindScheduledToPublish = `
		SELECT id, template_id, version_number, name, description, content_structure,
			status, scheduled_publish_at, scheduled_archive_at, signing_workflow_config,
			published_at, archived_at, published_by, archived_by, created_by, created_at, updated_at
		FROM content.template_versions
		WHERE status = 'SCHEDULED' AND scheduled_publish_at <= $1
		ORDER BY scheduled_publish_at`

	queryFindScheduledToArchive = `
		SELECT id, template_id, version_number, name, description, content_structure,
			status, scheduled_publish_at, scheduled_archive_at, signing_workflow_config,
			published_at, archived_at, published_by, archived_by, created_by, created_at, updated_at
		FROM content.template_versions
		WHERE status = 'PUBLISHED' AND scheduled_archive_at IS NOT NULL AND scheduled_archive_at <= $1
		ORDER BY scheduled_archive_at`

	queryUpdate = `
		UPDATE content.template_versions
		SET name = $2, description = $3, content_structure = $4, status = $5,
			scheduled_publish_at = $6, scheduled_archive_at = $7, signing_workflow_config = $8,
			published_at = $9, archived_at = $10, published_by = $11, archived_by = $12,
			updated_at = $13
		WHERE id = $1`

	queryUpdateStatusPublished = `
		UPDATE content.template_versions
		SET status = $2, published_at = NOW(), published_by = $3, updated_at = NOW()
		WHERE id = $1`

	queryUpdateStatusArchived = `
		UPDATE content.template_versions
		SET status = $2, archived_at = NOW(), archived_by = $3, updated_at = NOW()
		WHERE id = $1`

	queryUpdateStatusDefault = `UPDATE content.template_versions SET status = $2, updated_at = NOW() WHERE id = $1`

	queryDelete = `DELETE FROM content.template_versions WHERE id = $1`

	queryExistsByVersionNumber = `SELECT EXISTS(SELECT 1 FROM content.template_versions WHERE template_id = $1 AND version_number = $2)`

	queryExistsByName = `SELECT EXISTS(SELECT 1 FROM content.template_versions WHERE template_id = $1 AND name = $2)`

	queryExistsByNameExcluding = `SELECT EXISTS(SELECT 1 FROM content.template_versions WHERE template_id = $1 AND name = $2 AND id != $3)`

	queryGetNextVersionNumber = `SELECT COALESCE(MAX(version_number), 0) + 1 FROM content.template_versions WHERE template_id = $1`

	queryHasScheduledVersion = `SELECT EXISTS(SELECT 1 FROM content.template_versions WHERE template_id = $1 AND status = 'SCHEDULED')`

	queryExistsScheduledAtTime = `SELECT EXISTS(SELECT 1 FROM content.template_versions WHERE template_id = $1 AND status = 'SCHEDULED' AND scheduled_publish_at = $2 AND ($3::uuid IS NULL OR id != $3::uuid))`

	queryCountByTemplateID = `SELECT COUNT(*) FROM content.template_versions WHERE template_id = $1`
)
