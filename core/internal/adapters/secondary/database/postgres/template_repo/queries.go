package templaterepo

// SQL queries for template operations.
const (
	queryCreate = `
		INSERT INTO content.templates (
			workspace_id, folder_id, document_type_id, title, is_public_library, process, process_type, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id`

	queryFindByID = `
		SELECT id, workspace_id, folder_id, document_type_id, title, is_public_library, process, process_type, created_at, updated_at
		FROM content.templates
		WHERE id = $1`

	queryPublishedVersion = `
		SELECT id, template_id, version_number, name, description, content_structure,
			status, scheduled_publish_at, scheduled_archive_at, published_at, archived_at,
			published_by, archived_by, created_by, created_at, updated_at
		FROM content.template_versions
		WHERE template_id = $1 AND status = 'PUBLISHED'`

	queryVersionInjectables = `
		SELECT
			tvi.id, tvi.template_version_id, tvi.injectable_definition_id, tvi.is_required, tvi.default_value, tvi.created_at,
			id.id, id.workspace_id, id.key, id.label, id.description, id.data_type, id.created_at, id.updated_at
		FROM content.template_version_injectables tvi
		JOIN content.injectable_definitions id ON tvi.injectable_definition_id = id.id
		WHERE tvi.template_version_id = $1
		ORDER BY id.key`

	queryVersionSignerRoles = `
		SELECT id, template_version_id, role_name, anchor_string, signer_order, created_at, updated_at
		FROM content.template_version_signer_roles
		WHERE template_version_id = $1
		ORDER BY signer_order`

	queryTemplateTags = `
		SELECT t.id, t.workspace_id, t.name, t.color, t.created_at, t.updated_at
		FROM organizer.tags t
		JOIN content.template_tags tt ON t.id = tt.tag_id
		WHERE tt.template_id = $1
		ORDER BY t.name`

	queryFolder = `
		SELECT id, workspace_id, parent_id, name, created_at, updated_at
		FROM organizer.folders
		WHERE id = $1`

	queryDocumentType = `
		SELECT id, tenant_id, code, name, description, created_at, updated_at
		FROM content.document_types
		WHERE id = $1`

	queryAllVersions = `
		SELECT id, template_id, version_number, name, description, content_structure,
			status, scheduled_publish_at, scheduled_archive_at, published_at, archived_at,
			published_by, archived_by, created_by, created_at, updated_at
		FROM content.template_versions
		WHERE template_id = $1
		ORDER BY version_number DESC`

	queryFindByWorkspaceBase = `
		SELECT
			t.id, t.workspace_id, t.folder_id, t.document_type_id,
			(SELECT dt.code FROM content.document_types dt WHERE dt.id = t.document_type_id) as document_type_code,
			t.title, t.is_public_library, t.process, t.process_type,
			t.created_at, t.updated_at,
			EXISTS(SELECT 1 FROM content.template_versions WHERE template_id = t.id AND status = 'PUBLISHED') as has_published,
			(SELECT COUNT(*) FROM content.template_versions WHERE template_id = t.id AND status != 'ARCHIVED') as version_count,
			(SELECT COUNT(*) FROM content.template_versions WHERE template_id = t.id AND status = 'SCHEDULED') as scheduled_version_count,
			(SELECT version_number FROM content.template_versions WHERE template_id = t.id AND status = 'PUBLISHED' LIMIT 1) as published_version_number
		FROM content.templates t
		LEFT JOIN organizer.folders f ON t.folder_id = f.id
		WHERE t.workspace_id = $1`

	queryFindByFolder = `
		SELECT
			t.id, t.workspace_id, t.folder_id, t.document_type_id,
			(SELECT dt.code FROM content.document_types dt WHERE dt.id = t.document_type_id) as document_type_code,
			t.title, t.is_public_library, t.process, t.process_type,
			t.created_at, t.updated_at,
			EXISTS(SELECT 1 FROM content.template_versions WHERE template_id = t.id AND status = 'PUBLISHED') as has_published,
			(SELECT COUNT(*) FROM content.template_versions WHERE template_id = t.id AND status != 'ARCHIVED') as version_count,
			(SELECT COUNT(*) FROM content.template_versions WHERE template_id = t.id AND status = 'SCHEDULED') as scheduled_version_count,
			(SELECT version_number FROM content.template_versions WHERE template_id = t.id AND status = 'PUBLISHED' LIMIT 1) as published_version_number
		FROM content.templates t
		WHERE t.folder_id = $1
		ORDER BY t.title`

	queryFindPublicLibrary = `
		SELECT
			t.id, t.workspace_id, t.folder_id, t.document_type_id,
			(SELECT dt.code FROM content.document_types dt WHERE dt.id = t.document_type_id) as document_type_code,
			t.title, t.is_public_library, t.process, t.process_type,
			t.created_at, t.updated_at,
			true as has_published,
			(SELECT COUNT(*) FROM content.template_versions WHERE template_id = t.id AND status != 'ARCHIVED') as version_count,
			(SELECT COUNT(*) FROM content.template_versions WHERE template_id = t.id AND status = 'SCHEDULED') as scheduled_version_count,
			(SELECT version_number FROM content.template_versions WHERE template_id = t.id AND status = 'PUBLISHED' LIMIT 1) as published_version_number
		FROM content.templates t
		WHERE t.is_public_library = true
			AND EXISTS(SELECT 1 FROM content.template_versions WHERE template_id = t.id AND status = 'PUBLISHED')
		ORDER BY t.title`

	queryUpdate = `
		UPDATE content.templates
		SET title = $2, folder_id = $3, document_type_id = $4, is_public_library = $5, process = $6, process_type = $7, updated_at = $8
		WHERE id = $1`

	queryDelete = `DELETE FROM content.templates WHERE id = $1`

	queryExistsByTitle = `SELECT EXISTS(SELECT 1 FROM content.templates WHERE workspace_id = $1 AND title = $2)`

	queryExistsByTitleExcluding = `SELECT EXISTS(SELECT 1 FROM content.templates WHERE workspace_id = $1 AND title = $2 AND id != $3)`

	queryCountByFolder = `SELECT COUNT(*) FROM content.templates WHERE folder_id = $1`

	queryTemplateTagsBatch = `
		SELECT tt.template_id, t.id, t.name, t.color
		FROM content.template_tags tt
		JOIN organizer.tags t ON t.id = tt.tag_id
		WHERE tt.template_id = ANY($1)
		ORDER BY t.name`

	queryFindInternalTemplateBaseContext = `
		WITH tenant AS (
			SELECT id, code, name, description, is_system, status, COALESCE(settings, '{}') AS settings, created_at, updated_at
			FROM tenancy.tenants
			WHERE code = UPPER(TRIM($1))
			LIMIT 1
		), workspace AS (
			SELECT w.id, w.tenant_id, w.name, w.code, w.type, w.status,
			       w.is_sandbox, w.sandbox_of_id, w.created_at, w.updated_at
			FROM tenancy.workspaces w
			JOIN tenant t ON w.tenant_id = t.id
			WHERE w.code = UPPER(TRIM($2))
			  AND w.is_sandbox = FALSE
			LIMIT 1
		), sys_tenant AS (
			SELECT id
			FROM tenancy.tenants
			WHERE is_system = true
			LIMIT 1
		), doc_type AS (
			SELECT dt.id, dt.tenant_id, dt.code, dt.name, COALESCE(dt.description, '{}') AS description,
			       CASE WHEN dt.tenant_id != (SELECT id FROM tenant) THEN true ELSE false END AS is_global,
			       dt.created_at, dt.updated_at
			FROM content.document_types dt
			JOIN tenant t ON TRUE
			LEFT JOIN sys_tenant st ON TRUE
			WHERE dt.code = UPPER(TRIM($3))
			  AND (dt.tenant_id = t.id OR dt.tenant_id = st.id)
			ORDER BY CASE WHEN dt.tenant_id = t.id THEN 0 ELSE 1 END
			LIMIT 1
		)
		SELECT
			t.id, t.code, t.name, t.description, t.is_system, t.status, t.settings, t.created_at, t.updated_at,
			w.id, w.tenant_id, w.name, w.code, w.type, w.status, w.is_sandbox, w.sandbox_of_id, w.created_at, w.updated_at,
			dt.id, dt.tenant_id, dt.code, dt.name, dt.description, dt.is_global, dt.created_at, dt.updated_at
		FROM tenant t
		LEFT JOIN workspace w ON TRUE
		LEFT JOIN doc_type dt ON TRUE
	`

	queryFindTemplateWorkspaceByTemplateID = `
		SELECT
			t.id, t.workspace_id, t.folder_id, t.document_type_id, t.title,
			t.is_public_library, t.process, t.process_type, t.created_at, t.updated_at,
			w.id, w.tenant_id, w.name, w.code, w.type, w.status,
			w.is_sandbox, w.sandbox_of_id, w.created_at, w.updated_at
		FROM content.templates t
		JOIN tenancy.workspaces w ON w.id = t.workspace_id
		WHERE t.id = $1
	`

	queryFindResolutionCandidates = `
		WITH input_workspaces AS (
			SELECT UPPER(TRIM(code)) AS code, ordinality::int AS priority
			FROM unnest($2::text[]) WITH ORDINALITY AS w(code, ordinality)
			WHERE TRIM(code) <> ''
		), requested_tags AS (
			SELECT UPPER(TRIM(tag)) AS name
			FROM unnest($5::text[]) AS tags(tag)
			WHERE TRIM(tag) <> ''
		), tenant AS (
			SELECT id, code
			FROM tenancy.tenants
			WHERE code = UPPER(TRIM($1))
			LIMIT 1
		), sys_tenant AS (
			SELECT id
			FROM tenancy.tenants
			WHERE is_system = true
			LIMIT 1
		), doc_type AS (
			SELECT dt.id, dt.code
			FROM content.document_types dt
			JOIN tenant t ON TRUE
			LEFT JOIN sys_tenant st ON TRUE
			WHERE dt.code = UPPER(TRIM($3))
			  AND (dt.tenant_id = t.id OR dt.tenant_id = st.id)
			ORDER BY CASE WHEN dt.tenant_id = t.id THEN 0 ELSE 1 END
			LIMIT 1
		), workspace_candidates AS (
			SELECT w.id, w.code, iw.priority
			FROM input_workspaces iw
			JOIN tenant t ON TRUE
			JOIN tenancy.workspaces w ON w.tenant_id = t.id AND w.code = iw.code
		), template_candidates AS (
			SELECT
				wc.id AS workspace_id,
				wc.code AS workspace_code,
				wc.priority,
				dt.id AS document_type_id,
				dt.code AS document_type_code,
				tpl.id AS template_id,
				tpl.process,
				CASE WHEN tpl.process = normalized.process THEN 0 ELSE 1 END AS process_priority
			FROM workspace_candidates wc
			JOIN doc_type dt ON TRUE
			CROSS JOIN LATERAL (
				SELECT COALESCE(NULLIF(UPPER(TRIM($4)), ''), 'DEFAULT') AS process
			) normalized
			JOIN content.templates tpl ON tpl.workspace_id = wc.id
				AND tpl.document_type_id = dt.id
				AND tpl.process IN (normalized.process, 'DEFAULT')
		), ranked_templates AS (
			SELECT *
			FROM (
				SELECT tc.*, ROW_NUMBER() OVER (PARTITION BY tc.workspace_id ORDER BY tc.process_priority ASC) AS rn
				FROM template_candidates tc
			) ranked
			WHERE rn = 1
		), version_candidates AS (
			SELECT
				t.id AS tenant_id,
				t.code AS tenant_code,
				rt.workspace_id,
				rt.workspace_code,
				rt.document_type_id,
				rt.document_type_code,
				rt.template_id,
				tv.id AS version_id,
				rt.priority
			FROM ranked_templates rt
			JOIN tenant t ON TRUE
			JOIN content.template_versions tv ON tv.template_id = rt.template_id
			WHERE (
				$6::boolean IS NULL
				OR ($6::boolean = TRUE AND tv.status = 'PUBLISHED')
				OR ($6::boolean = FALSE AND tv.status <> 'PUBLISHED')
			)
		), candidates_with_tags AS (
			SELECT
				vc.tenant_id,
				vc.tenant_code,
				vc.workspace_id,
				vc.workspace_code,
				vc.document_type_id,
				vc.document_type_code,
				vc.template_id,
				vc.version_id,
				COALESCE(array_agg(UPPER(t.name) ORDER BY UPPER(t.name)) FILTER (WHERE t.name IS NOT NULL), ARRAY[]::text[]) AS tags,
				vc.priority
			FROM version_candidates vc
			LEFT JOIN content.template_tags tt ON tt.template_id = vc.template_id AND cardinality($5::text[]) > 0
			LEFT JOIN organizer.tags t ON t.id = tt.tag_id AND cardinality($5::text[]) > 0
			GROUP BY vc.tenant_id, vc.tenant_code, vc.workspace_id, vc.workspace_code,
				vc.document_type_id, vc.document_type_code, vc.template_id, vc.version_id, vc.priority
		)
		SELECT tenant_id, tenant_code, workspace_id, workspace_code,
		       document_type_id, document_type_code, template_id, version_id, tags, priority
		FROM candidates_with_tags c
		WHERE NOT EXISTS (SELECT 1 FROM requested_tags)
		   OR EXISTS (
		       SELECT 1 FROM requested_tags rt
		       WHERE rt.name = ANY(c.tags)
		   )
		ORDER BY priority ASC
	`

	queryFindInternalTemplateContext = `
		WITH input_workspaces AS (
			SELECT UPPER(TRIM(code)) AS code, ordinality::int AS priority
			FROM unnest($3::text[]) WITH ORDINALITY AS w(code, ordinality)
			WHERE TRIM(code) <> ''
		), requested_tags AS (
			SELECT UPPER(TRIM(tag)) AS name
			FROM unnest($6::text[]) AS tags(tag)
			WHERE TRIM(tag) <> ''
		), resolution_env AS (
			SELECT COALESCE(NULLIF(LOWER(TRIM($8)), ''), 'prod') AS value
		), tenant AS (
			SELECT id, code, name, description, is_system, status, COALESCE(settings, '{}') AS settings, created_at, updated_at
			FROM tenancy.tenants
			WHERE code = UPPER(TRIM($1))
			LIMIT 1
		), requested_workspace AS (
			SELECT w.id, w.tenant_id, w.name, w.code, w.type, w.status,
			       w.is_sandbox, w.sandbox_of_id, w.created_at, w.updated_at
			FROM tenancy.workspaces w
			JOIN tenant t ON w.tenant_id = t.id
			WHERE w.code = UPPER(TRIM($2))
			  AND w.is_sandbox = FALSE
			LIMIT 1
		), sys_tenant AS (
			SELECT id
			FROM tenancy.tenants
			WHERE is_system = true
			LIMIT 1
		), doc_type AS (
			SELECT dt.id, dt.tenant_id, dt.code, dt.name, COALESCE(dt.description, '{}') AS description,
			       CASE WHEN dt.tenant_id != (SELECT id FROM tenant) THEN true ELSE false END AS is_global,
			       dt.created_at, dt.updated_at
			FROM content.document_types dt
			JOIN tenant t ON TRUE
			LEFT JOIN sys_tenant st ON TRUE
			WHERE dt.code = UPPER(TRIM($4))
			  AND (dt.tenant_id = t.id OR dt.tenant_id = st.id)
			ORDER BY CASE WHEN dt.tenant_id = t.id THEN 0 ELSE 1 END
			LIMIT 1
		), workspace_candidates AS (
			SELECT w.id, w.code, iw.priority
			FROM input_workspaces iw
			JOIN tenant t ON TRUE
			JOIN resolution_env re ON TRUE
			JOIN tenancy.workspaces w ON w.tenant_id = t.id AND w.code = iw.code
			WHERE w.type <> 'SYSTEM'
			  AND w.code <> 'SYS_WRKSP'
			  AND (
				(re.value = 'dev' AND w.is_sandbox = TRUE)
				OR (re.value = 'prod' AND w.is_sandbox = FALSE)
			  )
		), template_candidates AS (
			SELECT
				wc.id AS workspace_id,
				wc.code AS workspace_code,
				wc.priority,
				dt.id AS document_type_id,
				dt.code AS document_type_code,
				tpl.id AS template_id,
				tpl.process,
				CASE WHEN tpl.process = normalized.process THEN 0 ELSE 1 END AS process_priority
			FROM workspace_candidates wc
			JOIN doc_type dt ON TRUE
			CROSS JOIN LATERAL (
				SELECT COALESCE(NULLIF(UPPER(TRIM($5)), ''), 'DEFAULT') AS process
			) normalized
			JOIN content.templates tpl ON tpl.workspace_id = wc.id
				AND tpl.document_type_id = dt.id
				AND tpl.process IN (normalized.process, 'DEFAULT')
		), ranked_templates AS (
			SELECT *
			FROM (
				SELECT tc.*, ROW_NUMBER() OVER (PARTITION BY tc.workspace_id ORDER BY tc.process_priority ASC) AS rn
				FROM template_candidates tc
			) ranked
			WHERE rn = 1
		), version_candidates AS (
			SELECT
				rt.workspace_id,
				rt.workspace_code,
				rt.template_id,
				tv.id AS version_id,
				rt.priority
			FROM ranked_templates rt
			JOIN content.template_versions tv ON tv.template_id = rt.template_id
			WHERE (
				$7::boolean IS NULL
				OR ($7::boolean = TRUE AND tv.status = 'PUBLISHED')
				OR ($7::boolean = FALSE AND tv.status <> 'PUBLISHED')
			)
		), candidates_with_tags AS (
			SELECT
				vc.workspace_id,
				vc.workspace_code,
				vc.template_id,
				vc.version_id,
				COALESCE(array_agg(UPPER(t.name) ORDER BY UPPER(t.name)) FILTER (WHERE t.name IS NOT NULL), ARRAY[]::text[]) AS tags,
				vc.priority
			FROM version_candidates vc
			LEFT JOIN content.template_tags tt ON tt.template_id = vc.template_id AND cardinality($6::text[]) > 0
			LEFT JOIN organizer.tags t ON t.id = tt.tag_id AND cardinality($6::text[]) > 0
			GROUP BY vc.workspace_id, vc.workspace_code, vc.template_id, vc.version_id, vc.priority
		), selected AS (
			SELECT workspace_id, workspace_code, template_id, version_id
			FROM candidates_with_tags c
			WHERE NOT EXISTS (SELECT 1 FROM requested_tags)
			   OR EXISTS (
			       SELECT 1 FROM requested_tags rt
			       WHERE rt.name = ANY(c.tags)
			   )
			ORDER BY priority ASC
			LIMIT 1
		)
		SELECT
			ten.id, ten.code, ten.name, ten.description, ten.is_system, ten.status, ten.settings, ten.created_at, ten.updated_at,
			rw.id, rw.tenant_id, rw.name, rw.code, rw.type, rw.status, rw.is_sandbox, rw.sandbox_of_id, rw.created_at, rw.updated_at,
			dt.id, dt.tenant_id, dt.code, dt.name, dt.description, dt.is_global, dt.created_at, dt.updated_at,
			tv.id, tv.template_id, tv.version_number, tv.name, tv.description, tv.content_structure,
			tv.status, tv.scheduled_publish_at, tv.scheduled_archive_at, tv.signing_workflow_config,
			tv.published_at, tv.archived_at, tv.published_by, tv.archived_by, tv.created_by, tv.created_at, tv.updated_at,
			COALESCE(injectables.items, '[]'::jsonb) AS injectables,
			COALESCE(signer_roles.items, '[]'::jsonb) AS signer_roles,
			tpl.id, tpl.workspace_id, tpl.folder_id, tpl.document_type_id, tpl.title,
			tpl.is_public_library, tpl.process, tpl.process_type, tpl.created_at, tpl.updated_at,
			w.id, w.tenant_id, w.name, w.code, w.type, w.status,
			w.is_sandbox, w.sandbox_of_id, w.created_at, w.updated_at
		FROM tenant ten
		LEFT JOIN requested_workspace rw ON TRUE
		JOIN doc_type dt ON TRUE
		JOIN selected s ON TRUE
		JOIN content.template_versions tv ON tv.id = s.version_id
		JOIN content.templates tpl ON tpl.id = s.template_id
		JOIN tenancy.workspaces w ON w.id = s.workspace_id
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
	`

	// Document Type queries
	queryFindByDocumentType = `
		SELECT id, workspace_id, folder_id, document_type_id, title, is_public_library, process, process_type, created_at, updated_at
		FROM content.templates
		WHERE workspace_id = $1 AND document_type_id = $2 AND process = $3`

	queryUpdateProcessFields = `
		UPDATE content.templates
		SET process = $2, process_type = $3, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`

	queryFindByDocumentTypeCode = `
		SELECT
			t.id, t.workspace_id, t.folder_id, t.document_type_id, dt.code as document_type_code,
			t.title, t.is_public_library, t.process, t.process_type,
			t.created_at, t.updated_at,
			EXISTS(SELECT 1 FROM content.template_versions WHERE template_id = t.id AND status = 'PUBLISHED') as has_published,
			(SELECT COUNT(*) FROM content.template_versions WHERE template_id = t.id AND status != 'ARCHIVED') as version_count,
			(SELECT COUNT(*) FROM content.template_versions WHERE template_id = t.id AND status = 'SCHEDULED') as scheduled_version_count,
			(SELECT version_number FROM content.template_versions WHERE template_id = t.id AND status = 'PUBLISHED' LIMIT 1) as published_version_number
		FROM content.templates t
		JOIN content.document_types dt ON t.document_type_id = dt.id
		JOIN tenancy.workspaces w ON t.workspace_id = w.id
		WHERE w.tenant_id = $1 AND dt.code = $2
		ORDER BY t.title`

	queryUpdateDocumentType = `
		UPDATE content.templates
		SET document_type_id = $2, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1`
)
