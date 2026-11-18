package templateversioninjectablerepo

// SQL queries for template version injectable operations.
const (
	queryCreate = `
		INSERT INTO content.template_version_injectables (
			template_version_id, injectable_definition_id, system_injectable_key, is_required, default_value, created_at
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`

	queryFindByID = `
		SELECT id, template_version_id, injectable_definition_id, system_injectable_key, is_required, default_value, created_at
		FROM content.template_version_injectables
		WHERE id = $1`

	queryFindByVersionID = `
		SELECT
			tvi.id, tvi.template_version_id, tvi.injectable_definition_id, tvi.system_injectable_key, tvi.is_required, tvi.default_value, tvi.created_at,
			id.id, id.workspace_id, id.key, id.label, id.description, id.data_type, id.metadata, id.format_config, id.created_at, id.updated_at
		FROM content.template_version_injectables tvi
		LEFT JOIN content.injectable_definitions id ON tvi.injectable_definition_id = id.id
		WHERE tvi.template_version_id = $1
		ORDER BY COALESCE(id.key, tvi.system_injectable_key)`

	queryUpdate = `
		UPDATE content.template_version_injectables
		SET is_required = $2, default_value = $3
		WHERE id = $1`

	queryDelete = `DELETE FROM content.template_version_injectables WHERE id = $1`

	queryDeleteByVersionID = `DELETE FROM content.template_version_injectables WHERE template_version_id = $1`

	// A version takes the global definitions and those of its template's
	// workspace. A sandbox template also takes those of the workspace the
	// sandbox belongs to, where a workspace's definitions are managed.
	queryDefinitionAvailable = `
		SELECT EXISTS(
			SELECT 1
			FROM content.injectable_definitions d
			JOIN content.template_versions v ON v.id = $1
			JOIN content.templates t ON t.id = v.template_id
			JOIN tenancy.workspaces w ON w.id = t.workspace_id
			WHERE d.id = $2
			  AND (d.workspace_id IS NULL OR d.workspace_id = w.id OR (w.is_sandbox AND d.workspace_id = w.sandbox_of_id))
		)`

	queryExists = `SELECT EXISTS(SELECT 1 FROM content.template_version_injectables WHERE template_version_id = $1 AND injectable_definition_id = $2)`

	queryExistsSystemKey = `SELECT EXISTS(SELECT 1 FROM content.template_version_injectables WHERE template_version_id = $1 AND system_injectable_key = $2)`

	queryCopyFromVersion = `
		INSERT INTO content.template_version_injectables (
			template_version_id, injectable_definition_id, system_injectable_key, is_required, default_value, created_at
		)
		SELECT $2, injectable_definition_id, system_injectable_key, is_required, default_value, NOW()
		FROM content.template_version_injectables
		WHERE template_version_id = $1`
)
