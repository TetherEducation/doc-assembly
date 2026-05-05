package systeminjectablerepo

// SQL queries for system injectable operations.
const (
	// queryFindActiveKeysForWorkspace returns active system injectable keys for a workspace.
	// Uses priority resolution: WORKSPACE > TENANT > PUBLIC.
	// Allows "exceptions" where WORKSPACE is_active=FALSE can override PUBLIC is_active=TRUE.
	queryFindActiveKeysForWorkspace = `
WITH workspace_tenant AS (
    SELECT tenant_id FROM tenancy.workspaces WHERE id = $1
),
best_assignment AS (
    SELECT DISTINCT ON (injectable_key)
        injectable_key,
        is_active
    FROM content.system_injectable_assignments
    WHERE scope_type = 'PUBLIC'
       OR (scope_type = 'TENANT' AND tenant_id = (SELECT tenant_id FROM workspace_tenant))
       OR (scope_type = 'WORKSPACE' AND workspace_id = $1)
    ORDER BY injectable_key,
             CASE scope_type WHEN 'WORKSPACE' THEN 1 WHEN 'TENANT' THEN 2 ELSE 3 END
)
SELECT sid.key
FROM content.system_injectable_definitions sid
JOIN best_assignment ba ON sid.key = ba.injectable_key
WHERE sid.is_active AND ba.is_active`

	queryFindActiveKeysForWorkspaceAndKeys = queryFindActiveKeysForWorkspace + `
  AND sid.key = ANY($2)`

	// queryFindAllDefinitions returns all definition keys with their is_active status.
	queryFindAllDefinitions = `
SELECT key, is_active FROM content.system_injectable_definitions`

	// queryUpsertDefinition creates or updates a definition's is_active status.
	queryUpsertDefinition = `
INSERT INTO content.system_injectable_definitions (key, is_active, created_at, updated_at)
VALUES ($1, $2, NOW(), NOW())
ON CONFLICT (key) DO UPDATE SET is_active = $2, updated_at = NOW()`

	// queryFindAssignmentsByKey returns all assignments for a given injectable key with tenant/workspace names.
	// For WORKSPACE scope: tenant info comes from workspace's tenant (wt)
	// For TENANT scope: tenant info comes from direct tenant join (t)
	queryFindAssignmentsByKey = `
SELECT
    a.id, a.injectable_key, a.scope_type,
    a.tenant_id, t.name AS tenant_name,
    a.workspace_id, w.name AS workspace_name,
    w.tenant_id AS workspace_tenant_id, wt.name AS workspace_tenant_name,
    a.is_active, a.created_at
FROM content.system_injectable_assignments a
LEFT JOIN tenancy.tenants t ON a.tenant_id = t.id
LEFT JOIN tenancy.workspaces w ON a.workspace_id = w.id
LEFT JOIN tenancy.tenants wt ON w.tenant_id = wt.id
WHERE a.injectable_key = $1
ORDER BY a.scope_type, a.created_at`

	// queryCreateAssignment inserts a new assignment.
	queryCreateAssignment = `
INSERT INTO content.system_injectable_assignments
(id, injectable_key, scope_type, tenant_id, workspace_id, is_active, created_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW())`

	// queryDeleteAssignment removes an assignment by ID.
	queryDeleteAssignment = `DELETE FROM content.system_injectable_assignments WHERE id = $1`

	// querySetAssignmentActive updates the is_active flag for an assignment.
	querySetAssignmentActive = `UPDATE content.system_injectable_assignments SET is_active = $1 WHERE id = $2`

	// queryFindPublicActiveKeys returns injectable keys that have an active PUBLIC assignment.
	queryFindPublicActiveKeys = `
SELECT injectable_key
FROM content.system_injectable_assignments
WHERE scope_type = 'PUBLIC' AND is_active = true`

	// queryCreatePublicAssignment inserts a PUBLIC assignment for a single key.
	// Used in batch operations. Returns id only if inserted.
	queryCreatePublicAssignment = `
INSERT INTO content.system_injectable_assignments (id, injectable_key, scope_type, is_active, created_at)
VALUES (gen_random_uuid(), $1, 'PUBLIC', true, NOW())
ON CONFLICT DO NOTHING`

	// queryDeletePublicAssignments deletes PUBLIC assignments for multiple keys.
	queryDeletePublicAssignments = `
DELETE FROM content.system_injectable_assignments
WHERE injectable_key = ANY($1) AND scope_type = 'PUBLIC'`

	// queryFindPublicAssignmentsByKeys returns PUBLIC assignments for given keys.
	queryFindPublicAssignmentsByKeys = `
SELECT injectable_key, id
FROM content.system_injectable_assignments
WHERE injectable_key = ANY($1) AND scope_type = 'PUBLIC'`
)
