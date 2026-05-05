package injectablerepo

// SQL queries for injectable definitions (read-only operations).
const (
	queryFindByID = `
		SELECT id, workspace_id, key, label, description, data_type, metadata, format_config, is_active, is_deleted, created_at, updated_at
		FROM content.injectable_definitions
		WHERE id = $1 AND is_active = true AND is_deleted = false`

	queryFindByWorkspace = `
		SELECT id, workspace_id, key, label, description, data_type, metadata, format_config, is_active, is_deleted, created_at, updated_at
		FROM content.injectable_definitions
		WHERE (workspace_id = $1 OR workspace_id IS NULL) AND is_active = true AND is_deleted = false
		ORDER BY key`

	queryFindByWorkspaceAndKeys = `
		SELECT id, workspace_id, key, label, description, data_type, metadata, format_config, is_active, is_deleted, created_at, updated_at
		FROM content.injectable_definitions
		WHERE (workspace_id = $1 OR workspace_id IS NULL)
		  AND key = ANY($2)
		  AND is_active = true AND is_deleted = false
		ORDER BY key`

	queryFindGenerationAvailableByWorkspaceAndKeys = `
		WITH requested_keys AS (
			SELECT DISTINCT key
			FROM unnest($2::text[]) AS keys(key)
			WHERE key <> ''
		), db_injectables AS (
			SELECT
				'db'::text AS source,
				id, workspace_id, key, label, description, data_type, metadata, format_config,
				is_active, is_deleted, created_at, updated_at
			FROM content.injectable_definitions
			WHERE (workspace_id = $1 OR workspace_id IS NULL)
			  AND key IN (SELECT key FROM requested_keys)
			  AND is_active = true
			  AND is_deleted = false
		), workspace_tenant AS (
			SELECT tenant_id
			FROM tenancy.workspaces
			WHERE id = $1
		), best_assignment AS (
			SELECT DISTINCT ON (injectable_key)
				injectable_key,
				is_active
			FROM content.system_injectable_assignments
			WHERE injectable_key IN (SELECT key FROM requested_keys)
			  AND (
				scope_type = 'PUBLIC'
				OR (scope_type = 'TENANT' AND tenant_id = (SELECT tenant_id FROM workspace_tenant))
				OR (scope_type = 'WORKSPACE' AND workspace_id = $1)
			  )
			ORDER BY injectable_key,
				CASE scope_type WHEN 'WORKSPACE' THEN 1 WHEN 'TENANT' THEN 2 ELSE 3 END
		), system_keys AS (
			SELECT
				'system'::text AS source,
				NULL::uuid AS id,
				NULL::uuid AS workspace_id,
				sid.key,
				NULL::text AS label,
				NULL::text AS description,
				NULL::injectable_data_type AS data_type,
				NULL::jsonb AS metadata,
				NULL::jsonb AS format_config,
				sid.is_active,
				false AS is_deleted,
				NULL::timestamptz AS created_at,
				NULL::timestamptz AS updated_at
			FROM content.system_injectable_definitions sid
			JOIN best_assignment ba ON sid.key = ba.injectable_key
			WHERE sid.is_active
			  AND ba.is_active
		)
		SELECT source, id, workspace_id, key, label, description, data_type, metadata, format_config,
		       is_active, is_deleted, created_at, updated_at
		FROM db_injectables
		UNION ALL
		SELECT source, id, workspace_id, key, label, description, data_type, metadata, format_config,
		       is_active, is_deleted, created_at, updated_at
		FROM system_keys
		ORDER BY key`

	queryFindGlobal = `
		SELECT id, workspace_id, key, label, description, data_type, metadata, format_config, is_active, is_deleted, created_at, updated_at
		FROM content.injectable_definitions
		WHERE workspace_id IS NULL AND is_active = true AND is_deleted = false
		ORDER BY key`

	queryFindByKeyGlobal = `
		SELECT id, workspace_id, key, label, description, data_type, metadata, format_config, is_active, is_deleted, created_at, updated_at
		FROM content.injectable_definitions
		WHERE workspace_id IS NULL AND key = $1 AND is_active = true AND is_deleted = false`

	queryFindByKeyWorkspace = `
		SELECT id, workspace_id, key, label, description, data_type, metadata, format_config, is_active, is_deleted, created_at, updated_at
		FROM content.injectable_definitions
		WHERE (workspace_id = $1 OR workspace_id IS NULL) AND key = $2 AND is_active = true AND is_deleted = false
		ORDER BY workspace_id NULLS LAST
		LIMIT 1`

	queryExistsByKeyGlobal = `
		SELECT EXISTS(SELECT 1 FROM content.injectable_definitions WHERE workspace_id IS NULL AND key = $1 AND is_deleted = false)`

	queryExistsByKeyWorkspace = `
		SELECT EXISTS(SELECT 1 FROM content.injectable_definitions WHERE workspace_id = $1 AND key = $2 AND is_deleted = false)`

	queryExistsByKeyGlobalExcluding = `
		SELECT EXISTS(SELECT 1 FROM content.injectable_definitions WHERE workspace_id IS NULL AND key = $1 AND id != $2 AND is_deleted = false)`

	queryExistsByKeyWorkspaceExcluding = `
		SELECT EXISTS(SELECT 1 FROM content.injectable_definitions WHERE workspace_id = $1 AND key = $2 AND id != $3 AND is_deleted = false)`

	queryIsInUse = `
		SELECT EXISTS(SELECT 1 FROM content.template_version_injectables WHERE injectable_definition_id = $1)`

	queryGetVersionCount = `
		SELECT COUNT(*) FROM content.template_version_injectables WHERE injectable_definition_id = $1`

	// queryFindKeysByWorkspace returns all keys accessible to a workspace (workspace-specific + global)
	// Used to validate that document variables reference accessible injectables
	queryFindKeysByWorkspace = `
		SELECT key
		FROM content.injectable_definitions
		WHERE (workspace_id = $1 OR workspace_id IS NULL)
		  AND key = ANY($2)
		  AND is_active = true AND is_deleted = false`
)
