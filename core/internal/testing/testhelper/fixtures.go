//go:build integration

package testhelper

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// TestUser represents a test user with authentication credentials.
type TestUser struct {
	ID           string
	Email        string
	FullName     string
	SystemRole   *entity.SystemRole
	Token        string
	BearerHeader string
}

// CreateTestUser creates a user in the database and returns a TestUser struct
// with a valid JWT token for authentication.
func CreateTestUser(t *testing.T, pool *pgxpool.Pool, email, fullName string, systemRole *entity.SystemRole) *TestUser {
	t.Helper()
	ctx := context.Background()

	userID := uuid.NewString()
	now := time.Now().UTC()

	// Insert user into identity.users
	_, err := pool.Exec(ctx, `
		INSERT INTO identity.users (id, external_identity_id, email, full_name, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		userID, email, email, fullName, entity.UserStatusActive, now)
	require.NoError(t, err, "failed to create test user")

	// Assign system role if provided
	if systemRole != nil {
		roleID := uuid.NewString()
		_, err = pool.Exec(ctx, `
			INSERT INTO identity.system_roles (id, user_id, role, granted_by, created_at)
			VALUES ($1, $2, $3, $4, $5)`,
			roleID, userID, *systemRole, nil, now)
		require.NoError(t, err, "failed to assign system role")
	}

	token := GenerateTestToken(email, fullName)

	return &TestUser{
		ID:           userID,
		Email:        email,
		FullName:     fullName,
		SystemRole:   systemRole,
		Token:        token,
		BearerHeader: "Bearer " + token,
	}
}

// CleanupUser removes a test user and their associated roles from the database.
func CleanupUser(t *testing.T, pool *pgxpool.Pool, userID string) {
	t.Helper()
	ctx := context.Background()

	// Delete in order of foreign key dependencies
	_, err := pool.Exec(ctx, "DELETE FROM identity.workspace_members WHERE user_id = $1", userID)
	require.NoError(t, err, "failed to cleanup workspace members for user")

	_, err = pool.Exec(ctx, "DELETE FROM identity.tenant_members WHERE user_id = $1", userID)
	require.NoError(t, err, "failed to cleanup tenant members for user")

	_, err = pool.Exec(ctx, "DELETE FROM identity.system_roles WHERE user_id = $1", userID)
	require.NoError(t, err, "failed to cleanup system roles for user")

	_, err = pool.Exec(ctx, "DELETE FROM identity.users WHERE id = $1", userID)
	require.NoError(t, err, "failed to cleanup user")
}

// CreateTestTenant creates a tenant in the database and returns its ID.
func CreateTestTenant(t *testing.T, pool *pgxpool.Pool, name, code string) string {
	t.Helper()
	ctx := context.Background()

	tenantID := uuid.NewString()
	now := time.Now().UTC()

	_, err := pool.Exec(ctx, `
		INSERT INTO tenancy.tenants (id, name, code, description, settings, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		tenantID, name, code, "Test tenant", "{}", now)
	require.NoError(t, err, "failed to create test tenant")

	return tenantID
}

// CleanupTenant removes a test tenant and all associated data.
func CleanupTenant(t *testing.T, pool *pgxpool.Pool, tenantID string) {
	t.Helper()
	ctx := context.Background()

	// Delete in order of foreign key dependencies
	_, err := pool.Exec(ctx, "DELETE FROM identity.workspace_members WHERE workspace_id IN (SELECT id FROM tenancy.workspaces WHERE tenant_id = $1)", tenantID)
	require.NoError(t, err, "failed to cleanup workspace members for tenant")

	_, err = pool.Exec(ctx, "DELETE FROM tenancy.workspaces WHERE tenant_id = $1", tenantID)
	require.NoError(t, err, "failed to cleanup workspaces for tenant")

	_, err = pool.Exec(ctx, "DELETE FROM identity.tenant_members WHERE tenant_id = $1", tenantID)
	require.NoError(t, err, "failed to cleanup tenant members")

	_, err = pool.Exec(ctx, "DELETE FROM tenancy.tenants WHERE id = $1", tenantID)
	require.NoError(t, err, "failed to cleanup tenant")
}

// CreateTestWorkspace creates a workspace in the database and returns its ID.
func CreateTestWorkspace(t *testing.T, pool *pgxpool.Pool, tenantID *string, name string, wsType entity.WorkspaceType) string {
	t.Helper()
	ctx := context.Background()

	workspaceID := uuid.NewString()
	now := time.Now().UTC()

	// Generate a unique workspace code from the first 8 chars of the UUID.
	code := "WS_" + strings.ToUpper(workspaceID[:8])

	_, err := pool.Exec(ctx, `
		INSERT INTO tenancy.workspaces (id, tenant_id, name, code, type, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		workspaceID, tenantID, name, code, wsType, entity.WorkspaceStatusActive, now)
	require.NoError(t, err, "failed to create test workspace")

	return workspaceID
}

// CreateTestSandboxWorkspace creates a sandbox workspace linked to the given parent workspace.
// Returns the sandbox workspace ID.
func CreateTestSandboxWorkspace(t *testing.T, pool *pgxpool.Pool, parentWorkspaceID string) string {
	t.Helper()
	ctx := context.Background()

	var tenantID, parentCode string
	var parentType entity.WorkspaceType
	var parentStatus entity.WorkspaceStatus
	err := pool.QueryRow(ctx,
		`SELECT tenant_id, code, type, status FROM tenancy.workspaces WHERE id = $1`,
		parentWorkspaceID,
	).Scan(&tenantID, &parentCode, &parentType, &parentStatus)
	require.NoError(t, err, "failed to load parent workspace for sandbox")

	workspaceID := uuid.NewString()
	now := time.Now().UTC()
	code := "SBX_" + parentCode

	var existingID string
	err = pool.QueryRow(ctx, `
		SELECT id
		FROM tenancy.workspaces
		WHERE sandbox_of_id = $1
		LIMIT 1`,
		parentWorkspaceID,
	).Scan(&existingID)
	if err == nil {
		return existingID
	}
	require.True(t, errors.Is(err, pgx.ErrNoRows), "failed to check existing sandbox workspace: %v", err)

	_, err = pool.Exec(ctx, `
		INSERT INTO tenancy.workspaces (id, tenant_id, name, code, type, status, is_sandbox, sandbox_of_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		workspaceID, tenantID, "Sandbox of "+parentCode, code, parentType, parentStatus, true, parentWorkspaceID, now)
	require.NoError(t, err, "failed to create sandbox workspace")

	return workspaceID
}

// CleanupWorkspace removes a test workspace and all associated data.
func CleanupWorkspace(t *testing.T, pool *pgxpool.Pool, workspaceID string) {
	t.Helper()
	ctx := context.Background()

	// Delete in order of foreign key dependencies
	_, err := pool.Exec(ctx, "DELETE FROM identity.workspace_members WHERE workspace_id = $1", workspaceID)
	require.NoError(t, err, "failed to cleanup workspace members")

	_, err = pool.Exec(ctx, `
		WITH related_documents AS (
			SELECT d.id
			FROM execution.documents d
			WHERE d.workspace_id = $1
			   OR d.template_version_id IN (
					SELECT tv.id
					FROM content.template_versions tv
					JOIN content.templates t ON t.id = tv.template_id
					WHERE t.workspace_id = $1
				)
		)
		DELETE FROM execution.document_access_tokens
		WHERE document_id IN (SELECT id FROM related_documents)
		   OR recipient_id IN (
				SELECT r.id
				FROM execution.document_recipients r
				WHERE r.document_id IN (SELECT id FROM related_documents)
			)
	`, workspaceID)
	require.NoError(t, err, "failed to cleanup document access tokens")

	_, err = pool.Exec(ctx, `
		WITH related_documents AS (
			SELECT d.id
			FROM execution.documents d
			WHERE d.workspace_id = $1
			   OR d.template_version_id IN (
					SELECT tv.id
					FROM content.template_versions tv
					JOIN content.templates t ON t.id = tv.template_id
					WHERE t.workspace_id = $1
				)
		)
		DELETE FROM execution.signing_attempt_events
		WHERE attempt_id IN (
			SELECT id FROM execution.signing_attempts WHERE document_id IN (SELECT id FROM related_documents)
		)
	`, workspaceID)
	require.NoError(t, err, "failed to cleanup signing attempt events")

	_, err = pool.Exec(ctx, `
		WITH related_documents AS (
			SELECT d.id
			FROM execution.documents d
			WHERE d.workspace_id = $1
			   OR d.template_version_id IN (
					SELECT tv.id
					FROM content.template_versions tv
					JOIN content.templates t ON t.id = tv.template_id
					WHERE t.workspace_id = $1
				)
		)
		DELETE FROM execution.signing_attempt_recipients
		WHERE attempt_id IN (
			SELECT id FROM execution.signing_attempts WHERE document_id IN (SELECT id FROM related_documents)
		)
	`, workspaceID)
	require.NoError(t, err, "failed to cleanup signing attempt recipients")

	_, err = pool.Exec(ctx, `
		WITH related_documents AS (
			SELECT d.id
			FROM execution.documents d
			WHERE d.workspace_id = $1
			   OR d.template_version_id IN (
					SELECT tv.id
					FROM content.template_versions tv
					JOIN content.templates t ON t.id = tv.template_id
					WHERE t.workspace_id = $1
				)
		)
		DELETE FROM execution.signing_attempts
		WHERE document_id IN (SELECT id FROM related_documents)
	`, workspaceID)
	require.NoError(t, err, "failed to cleanup signing attempts")

	_, err = pool.Exec(ctx, `
		WITH related_documents AS (
			SELECT d.id
			FROM execution.documents d
			WHERE d.workspace_id = $1
			   OR d.template_version_id IN (
					SELECT tv.id
					FROM content.template_versions tv
					JOIN content.templates t ON t.id = tv.template_id
					WHERE t.workspace_id = $1
				)
		)
		DELETE FROM execution.document_field_responses
		WHERE document_id IN (SELECT id FROM related_documents)
	`, workspaceID)
	require.NoError(t, err, "failed to cleanup document field responses")

	_, err = pool.Exec(ctx, `
		WITH related_documents AS (
			SELECT d.id
			FROM execution.documents d
			WHERE d.workspace_id = $1
			   OR d.template_version_id IN (
					SELECT tv.id
					FROM content.template_versions tv
					JOIN content.templates t ON t.id = tv.template_id
					WHERE t.workspace_id = $1
				)
		)
		DELETE FROM execution.document_events
		WHERE document_id IN (SELECT id FROM related_documents)
	`, workspaceID)
	require.NoError(t, err, "failed to cleanup document events")

	_, err = pool.Exec(ctx, `
		WITH related_documents AS (
			SELECT d.id
			FROM execution.documents d
			WHERE d.workspace_id = $1
			   OR d.template_version_id IN (
					SELECT tv.id
					FROM content.template_versions tv
					JOIN content.templates t ON t.id = tv.template_id
					WHERE t.workspace_id = $1
				)
		)
		DELETE FROM execution.document_recipients
		WHERE document_id IN (SELECT id FROM related_documents)
	`, workspaceID)
	require.NoError(t, err, "failed to cleanup document recipients")

	_, err = pool.Exec(ctx, `
		DELETE FROM execution.documents d
		WHERE d.workspace_id = $1
		   OR d.template_version_id IN (
				SELECT tv.id
				FROM content.template_versions tv
				JOIN content.templates t ON t.id = tv.template_id
				WHERE t.workspace_id = $1
			)
	`, workspaceID)
	require.NoError(t, err, "failed to cleanup documents")

	_, err = pool.Exec(ctx, "DELETE FROM tenancy.workspaces WHERE id = $1", workspaceID)
	require.NoError(t, err, "failed to cleanup workspace")
}

// AssignSystemRole assigns a system role to an existing user.
func AssignSystemRole(t *testing.T, pool *pgxpool.Pool, userID string, role entity.SystemRole, grantedBy *string) string {
	t.Helper()
	ctx := context.Background()

	roleID := uuid.NewString()
	now := time.Now().UTC()

	_, err := pool.Exec(ctx, `
		INSERT INTO identity.system_roles (id, user_id, role, granted_by, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		roleID, userID, role, grantedBy, now)
	require.NoError(t, err, "failed to assign system role")

	return roleID
}

// CleanupSystemRole removes a system role assignment.
func CleanupSystemRole(t *testing.T, pool *pgxpool.Pool, userID string) {
	t.Helper()
	ctx := context.Background()

	_, err := pool.Exec(ctx, "DELETE FROM identity.system_roles WHERE user_id = $1", userID)
	require.NoError(t, err, "failed to cleanup system role")
}

// CreateTestTenantMember creates a tenant membership in the database and returns its ID.
func CreateTestTenantMember(t *testing.T, pool *pgxpool.Pool,
	tenantID, userID string, role entity.TenantRole, grantedBy *string) string {
	t.Helper()
	ctx := context.Background()

	memberID := uuid.NewString()
	now := time.Now().UTC()

	_, err := pool.Exec(ctx, `
		INSERT INTO identity.tenant_members
			(id, tenant_id, user_id, role, membership_status, granted_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		memberID, tenantID, userID, role, entity.MembershipStatusActive, grantedBy, now)
	require.NoError(t, err, "failed to create tenant member")

	return memberID
}

// CreateTestTenantMemberWithStatus creates a tenant membership with a specific status.
func CreateTestTenantMemberWithStatus(t *testing.T, pool *pgxpool.Pool,
	tenantID, userID string, role entity.TenantRole, status entity.MembershipStatus, grantedBy *string) string {
	t.Helper()
	ctx := context.Background()

	memberID := uuid.NewString()
	now := time.Now().UTC()

	_, err := pool.Exec(ctx, `
		INSERT INTO identity.tenant_members
			(id, tenant_id, user_id, role, membership_status, granted_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		memberID, tenantID, userID, role, status, grantedBy, now)
	require.NoError(t, err, "failed to create tenant member")

	return memberID
}

// CreateTestWorkspaceMember creates a workspace membership in the database and returns its ID.
func CreateTestWorkspaceMember(t *testing.T, pool *pgxpool.Pool,
	workspaceID, userID string, role entity.WorkspaceRole, invitedBy *string) string {
	t.Helper()
	ctx := context.Background()

	memberID := uuid.NewString()
	now := time.Now().UTC()

	_, err := pool.Exec(ctx, `
		INSERT INTO identity.workspace_members
			(id, workspace_id, user_id, role, membership_status, invited_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		memberID, workspaceID, userID, role, entity.MembershipStatusActive, invitedBy, now)
	require.NoError(t, err, "failed to create workspace member")

	return memberID
}

// Ptr is a helper function to create a pointer to a value.
func Ptr[T any](v T) *T {
	return &v
}

// CreateTestFolder creates a folder in the database and returns its ID.
func CreateTestFolder(t *testing.T, pool *pgxpool.Pool,
	workspaceID, name string, parentID *string) string {
	t.Helper()
	ctx := context.Background()

	folderID := uuid.NewString()
	now := time.Now().UTC()

	_, err := pool.Exec(ctx, `
		INSERT INTO organizer.folders (id, workspace_id, parent_id, name, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		folderID, workspaceID, parentID, name, now)
	require.NoError(t, err, "failed to create test folder")

	return folderID
}

// CleanupFolder removes a test folder.
func CleanupFolder(t *testing.T, pool *pgxpool.Pool, folderID string) {
	t.Helper()
	ctx := context.Background()

	_, err := pool.Exec(ctx, "DELETE FROM organizer.folders WHERE id = $1", folderID)
	require.NoError(t, err, "failed to cleanup folder")
}

// CreateTestTag creates a tag in the database and returns its ID.
func CreateTestTag(t *testing.T, pool *pgxpool.Pool,
	workspaceID, name, color string) string {
	t.Helper()
	ctx := context.Background()

	tagID := uuid.NewString()
	now := time.Now().UTC()

	_, err := pool.Exec(ctx, `
		INSERT INTO organizer.tags (id, workspace_id, name, color, created_at)
		VALUES ($1, $2, $3, $4, $5)`,
		tagID, workspaceID, name, color, now)
	require.NoError(t, err, "failed to create test tag")

	return tagID
}

// CleanupTag removes a test tag.
func CleanupTag(t *testing.T, pool *pgxpool.Pool, tagID string) {
	t.Helper()
	ctx := context.Background()

	_, err := pool.Exec(ctx, "DELETE FROM organizer.tags WHERE id = $1", tagID)
	require.NoError(t, err, "failed to cleanup tag")
}

// --- Content Fixtures ---

// CreateTestInjectable creates an injectable definition in the database and returns its ID.
// Schema: content.injectable_definitions
func CreateTestInjectable(t *testing.T, pool *pgxpool.Pool,
	workspaceID *string, key, label string, dataType entity.InjectableDataType) string {
	t.Helper()
	ctx := context.Background()

	injectableID := uuid.NewString()
	now := time.Now().UTC()

	_, err := pool.Exec(ctx, `
		INSERT INTO content.injectable_definitions (id, workspace_id, key, label, description, data_type, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		injectableID, workspaceID, key, label, "", dataType, now)
	require.NoError(t, err, "failed to create test injectable")

	return injectableID
}

// CleanupInjectable removes a test injectable definition.
func CleanupInjectable(t *testing.T, pool *pgxpool.Pool, injectableID string) {
	t.Helper()
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "DELETE FROM content.injectable_definitions WHERE id = $1", injectableID)
}

// CreateTestDocumentType creates a document type for a tenant and returns its ID.
func CreateTestDocumentType(t *testing.T, pool *pgxpool.Pool, tenantID, code, name string) string {
	t.Helper()
	ctx := context.Background()

	docTypeID := uuid.NewString()
	now := time.Now().UTC()
	nameJSON, err := json.Marshal(map[string]string{"en": name})
	require.NoError(t, err, "failed to marshal document type name")

	_, err = pool.Exec(ctx, `
		INSERT INTO content.document_types (id, tenant_id, code, name, description, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		docTypeID, tenantID, code, nameJSON, nil, now)
	require.NoError(t, err, "failed to create test document type")

	return docTypeID
}

// CleanupDocumentType removes a test document type.
func CleanupDocumentType(t *testing.T, pool *pgxpool.Pool, docTypeID string) {
	t.Helper()
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "DELETE FROM content.document_types WHERE id = $1", docTypeID)
}

// SetTemplateDocumentType sets the document type for a template.
func SetTemplateDocumentType(t *testing.T, pool *pgxpool.Pool, templateID, docTypeID string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `UPDATE content.templates SET document_type_id = $2 WHERE id = $1`, templateID, docTypeID)
	require.NoError(t, err, "failed to set template document type")
}

// CreateTestTemplate creates a template in the database and returns its ID.
// Schema: content.templates
func CreateTestTemplate(t *testing.T, pool *pgxpool.Pool,
	workspaceID string, title string, folderID *string) string {
	t.Helper()
	ctx := context.Background()

	templateID := uuid.NewString()
	now := time.Now().UTC()

	_, err := pool.Exec(ctx, `
		INSERT INTO content.templates (id, workspace_id, folder_id, title, is_public_library, process, process_type, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		templateID, workspaceID, folderID, title, false, entity.DefaultProcess, entity.DefaultProcessType, now)
	require.NoError(t, err, "failed to create test template")

	return templateID
}

// CleanupTemplate removes a test template and all its associated data.
func CleanupTemplate(t *testing.T, pool *pgxpool.Pool, templateID string) {
	t.Helper()
	ctx := context.Background()
	// Cascade: signer roles, injectables, versions, tags, template
	_, _ = pool.Exec(ctx, "DELETE FROM content.template_version_signer_roles WHERE template_version_id IN (SELECT id FROM content.template_versions WHERE template_id = $1)", templateID)
	_, _ = pool.Exec(ctx, "DELETE FROM content.template_version_injectables WHERE template_version_id IN (SELECT id FROM content.template_versions WHERE template_id = $1)", templateID)
	_, _ = pool.Exec(ctx, "DELETE FROM content.template_versions WHERE template_id = $1", templateID)
	_, _ = pool.Exec(ctx, "DELETE FROM content.template_tags WHERE template_id = $1", templateID)
	_, _ = pool.Exec(ctx, "DELETE FROM content.templates WHERE id = $1", templateID)
}

// CreateTestTemplateVersion creates a template version in the database and returns its ID.
// Schema: content.template_versions
func CreateTestTemplateVersion(t *testing.T, pool *pgxpool.Pool,
	templateID string, versionNumber int, name string, status entity.VersionStatus) string {
	t.Helper()
	ctx := context.Background()

	versionID := uuid.NewString()
	now := time.Now().UTC()

	// Declares no signer roles on purpose. Callers that need them add explicit roles via
	// CreateTestSignerRole, with their own names, anchors and ordering.
	//
	// This blob used to declare a "Signer" role that no fixture ever inserted into
	// content.template_version_signer_roles — the exact inconsistency that
	// trigger_template_versions_require_signer_roles (migration 000022) now rejects, and
	// the same shape that took contract creation down fleet-wide on 2026-08-14. Keeping
	// the declaration here would have meant either publishing an inconsistent version or
	// injecting a role that collides with the ones tests create (the signer_roles table
	// is unique on version+name, version+anchor and version+order).
	content := []byte(`{"version":"1.1.0","meta":{"title":"Test Document","language":"en"},"pageConfig":{"formatId":"A4","width":595,"height":842,"margins":{"top":20,"bottom":20,"left":20,"right":20}},"variableIds":[],"signerRoles":[],"content":{"type":"doc","content":[]},"exportInfo":{"exportedAt":"2026-01-01T00:00:00Z","sourceApp":"integration-test"}}`)

	_, err := pool.Exec(ctx, `
		INSERT INTO content.template_versions
			(id, template_id, version_number, name, description, content_structure, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		versionID, templateID, versionNumber, name, nil, content, status, now)
	require.NoError(t, err, "failed to create test template version")

	return versionID
}

// CreateTestPublishableTemplateVersion creates a DRAFT version whose content declares a
// signer role and carries the matching signature node, so it passes ValidateForPublish.
//
// Use this for tests that publish through the service (POST .../publish). That path runs
// replaceSignerRoles before flipping the status, so the derived rows exist by the time
// trigger_template_versions_require_signer_roles evaluates the row.
//
// CreateTestTemplateVersion deliberately declares no signer roles, because tests that
// publish by direct SQL (PublishTestVersion) would otherwise be asserting the exact
// inconsistent shape the trigger exists to reject.
func CreateTestPublishableTemplateVersion(t *testing.T, pool *pgxpool.Pool,
	templateID string, versionNumber int, name string) string {
	t.Helper()
	ctx := context.Background()

	versionID := uuid.NewString()
	now := time.Now().UTC()

	content := []byte(`{"version":"1.1.0","meta":{"title":"Test Document","language":"en"},"pageConfig":{"formatId":"A4","width":595,"height":842,"margins":{"top":20,"bottom":20,"left":20,"right":20}},"variableIds":[],"signerRoles":[{"id":"role-001","label":"Signer","order":1,"name":{"type":"text","value":"Test Signer"},"email":{"type":"text","value":"signer@test.com"}}],"content":{"type":"doc","content":[{"type":"signature","attrs":{"count":1,"layout":"single-center","lineWidth":"md","signatures":[{"id":"sig-001","roleId":"role-001","label":"Signer"}]}}]},"exportInfo":{"exportedAt":"2026-01-01T00:00:00Z","sourceApp":"integration-test"}}`)

	_, err := pool.Exec(ctx, `
		INSERT INTO content.template_versions
			(id, template_id, version_number, name, description, content_structure, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		versionID, templateID, versionNumber, name, nil, content,
		entity.VersionStatusDraft, now)
	require.NoError(t, err, "failed to create publishable test template version")

	return versionID
}

// CleanupTemplateVersion removes a test template version and its associated data.
func CleanupTemplateVersion(t *testing.T, pool *pgxpool.Pool, versionID string) {
	t.Helper()
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "DELETE FROM content.template_version_signer_roles WHERE template_version_id = $1", versionID)
	_, _ = pool.Exec(ctx, "DELETE FROM content.template_version_injectables WHERE template_version_id = $1", versionID)
	_, _ = pool.Exec(ctx, "DELETE FROM content.template_versions WHERE id = $1", versionID)
}

// CreateTestVersionInjectable links an injectable definition to a version.
// Schema: content.template_version_injectables
func CreateTestVersionInjectable(t *testing.T, pool *pgxpool.Pool,
	versionID, injectableID string, isRequired bool) string {
	t.Helper()
	ctx := context.Background()

	id := uuid.NewString()
	now := time.Now().UTC()

	_, err := pool.Exec(ctx, `
		INSERT INTO content.template_version_injectables
			(id, template_version_id, injectable_definition_id, is_required, default_value, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		id, versionID, injectableID, isRequired, nil, now)
	require.NoError(t, err, "failed to create version injectable")

	return id
}

// CleanupVersionInjectable removes a test version injectable.
func CleanupVersionInjectable(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "DELETE FROM content.template_version_injectables WHERE id = $1", id)
}

// CreateTestSignerRole creates a signer role for a version.
// Schema: content.template_version_signer_roles
func CreateTestSignerRole(t *testing.T, pool *pgxpool.Pool,
	versionID, roleName, anchorString string, signerOrder int) string {
	t.Helper()
	ctx := context.Background()

	roleID := uuid.NewString()
	now := time.Now().UTC()

	_, err := pool.Exec(ctx, `
		INSERT INTO content.template_version_signer_roles
			(id, template_version_id, role_name, anchor_string, signer_order, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		roleID, versionID, roleName, anchorString, signerOrder, now)
	require.NoError(t, err, "failed to create signer role")

	return roleID
}

// CleanupSignerRole removes a test signer role.
func CleanupSignerRole(t *testing.T, pool *pgxpool.Pool, roleID string) {
	t.Helper()
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "DELETE FROM content.template_version_signer_roles WHERE id = $1", roleID)
}

// UpdateWorkspaceStatus updates a workspace's status directly in the database.
func UpdateWorkspaceStatus(t *testing.T, pool *pgxpool.Pool, workspaceID string, status entity.WorkspaceStatus) {
	t.Helper()
	ctx := context.Background()

	_, err := pool.Exec(ctx, `UPDATE tenancy.workspaces SET status = $1 WHERE id = $2`, status, workspaceID)
	require.NoError(t, err, "failed to update workspace status")
}

// CreateTestTemplateTag creates a template-tag relationship in the database.
func CreateTestTemplateTag(t *testing.T, pool *pgxpool.Pool, templateID, tagID string) {
	t.Helper()
	ctx := context.Background()

	_, err := pool.Exec(ctx, `
		INSERT INTO content.template_tags (template_id, tag_id)
		VALUES ($1, $2)`, templateID, tagID)
	require.NoError(t, err, "failed to create template tag")
}

// --- Document Fixtures ---

// CleanupDocument removes a test document and all associated data.
func CleanupDocument(t *testing.T, pool *pgxpool.Pool, documentID string) {
	t.Helper()
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "DELETE FROM execution.document_events WHERE document_id = $1", documentID)
	_, _ = pool.Exec(ctx, "DELETE FROM execution.document_recipients WHERE document_id = $1", documentID)
	_, _ = pool.Exec(ctx, "DELETE FROM execution.documents WHERE id = $1", documentID)
}

// PublishTestVersion updates a template version status to PUBLISHED directly in the database.
func PublishTestVersion(t *testing.T, pool *pgxpool.Pool, versionID string) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `UPDATE content.template_versions SET status = 'PUBLISHED' WHERE id = $1`, versionID)
	require.NoError(t, err, "failed to publish version")
}

// --- Automation Fixtures ---

// CreateTestAutomationKey inserts an API key directly into automation.api_keys.
// Returns (keyID, rawKey). The raw key has format "doca_<64 hex chars>" and is never stored.
func CreateTestAutomationKey(t *testing.T, pool *pgxpool.Pool, name string, allowedTenants []string) (keyID, rawKey string) {
	t.Helper()
	ctx := context.Background()

	b := make([]byte, 32)
	_, err := rand.Read(b)
	require.NoError(t, err, "failed to generate key bytes")
	rawKey = "doca_" + hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(rawKey))
	keyHash := hex.EncodeToString(sum[:])
	keyPrefix := rawKey[:12]

	var tenants interface{}
	if len(allowedTenants) > 0 {
		tenants = allowedTenants
	}

	err = pool.QueryRow(ctx, `
		INSERT INTO automation.api_keys (name, key_hash, key_prefix, allowed_tenants, is_active, created_by)
		VALUES ($1, $2, $3, $4, $5, gen_random_uuid())
		RETURNING id`,
		name, keyHash, keyPrefix, tenants, true,
	).Scan(&keyID)
	require.NoError(t, err, "failed to create test automation key")
	return keyID, rawKey
}

// CleanupAutomationKey removes a test automation key and its audit log entries.
func CleanupAutomationKey(t *testing.T, pool *pgxpool.Pool, keyID string) {
	t.Helper()
	ctx := context.Background()
	_, _ = pool.Exec(ctx, "DELETE FROM automation.audit_log WHERE api_key_id = $1", keyID)
	_, _ = pool.Exec(ctx, "DELETE FROM automation.api_keys WHERE id = $1", keyID)
}
