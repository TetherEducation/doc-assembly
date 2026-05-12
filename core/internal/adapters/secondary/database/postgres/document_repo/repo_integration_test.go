//go:build integration

package documentrepo_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	documentrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/document_repo"
	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
	"github.com/rendis/doc-assembly/core/internal/testing/testhelper"
)

func TestRepository_FindByWorkspace_EnrichedFields(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	repo := documentrepo.NewConcrete(pool)
	ctx := context.Background()

	tenantID := testhelper.CreateTestTenant(t, pool, "Repo Tenant", "RPTEN1")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, tenantID) })

	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "Repo Workspace", entity.WorkspaceTypeClient)
	t.Cleanup(func() { testhelper.CleanupWorkspace(t, pool, workspaceID) })

	docTypeID := testhelper.CreateTestDocumentType(t, pool, tenantID, "REPO_DOC", "Repository Document")
	t.Cleanup(func() { testhelper.CleanupDocumentType(t, pool, docTypeID) })

	templateID := testhelper.CreateTestTemplate(t, pool, workspaceID, "Repo Template", nil)
	t.Cleanup(func() { testhelper.CleanupTemplate(t, pool, templateID) })
	testhelper.SetTemplateDocumentType(t, pool, templateID, docTypeID)

	versionID := testhelper.CreateTestTemplateVersion(t, pool, templateID, 1, "v1.0", entity.VersionStatusPublished)
	t.Cleanup(func() { testhelper.CleanupTemplateVersion(t, pool, versionID) })

	roleA := testhelper.CreateTestSignerRole(t, pool, versionID, "Signer A", "__sig_a__", 1)
	t.Cleanup(func() { testhelper.CleanupSignerRole(t, pool, roleA) })
	roleB := testhelper.CreateTestSignerRole(t, pool, versionID, "Signer B", "__sig_b__", 2)
	t.Cleanup(func() { testhelper.CleanupSignerRole(t, pool, roleB) })

	doc := entity.NewDocument(workspaceID, versionID)
	doc.DocumentTypeID = docTypeID
	doc.SetTitle("Repo List Doc")
	doc.SetExternalReference("EXT-001")
	doc.SetTransactionalID(uuid.NewString())
	if err := doc.MarkAsAwaitingInput(); err != nil {
		t.Fatalf("marking doc awaiting input: %v", err)
	}

	docID, err := repo.Create(ctx, doc)
	require.NoError(t, err)
	t.Cleanup(func() { testhelper.CleanupDocument(t, pool, docID) })

	_, err = pool.Exec(ctx, `
		INSERT INTO execution.document_recipients
			(id, document_id, template_version_role_id, name, email, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7),
		       ($8, $9, $10, $11, $12, $13, $14)
	`,
		uuid.NewString(), docID, roleA, "Alice", "alice.repo@test.com", entity.RecipientStatusPending, time.Now().UTC(),
		uuid.NewString(), docID, roleB, "Bob", "bob.repo@test.com", entity.RecipientStatusPending, time.Now().UTC(),
	)
	require.NoError(t, err)

	items, err := repo.FindByWorkspace(ctx, workspaceID, port.DocumentFilters{Limit: 10, Offset: 0})
	require.NoError(t, err)
	require.NotEmpty(t, items)

	byEmail, err := repo.FindByWorkspace(ctx, workspaceID, port.DocumentFilters{
		Search: "alice.repo",
		Limit:  10,
		Offset: 0,
	})
	require.NoError(t, err)
	require.NotEmpty(t, byEmail)
	var foundByEmail *entity.DocumentListItem
	for _, it := range byEmail {
		if it.ID == docID {
			foundByEmail = it
			break
		}
	}
	require.NotNil(t, foundByEmail, "search should match signer email substring, not title")

	var got *entity.DocumentListItem
	for _, it := range items {
		if it.ID == docID {
			got = it
			break
		}
	}
	require.NotNil(t, got)
	assert.Equal(t, workspaceID, got.WorkspaceID)
	assert.Equal(t, versionID, got.TemplateVersionID)
	assert.Equal(t, docTypeID, got.DocumentTypeID)
	require.NotNil(t, got.DocumentTypeName)
	assert.Equal(t, "Repository Document", *got.DocumentTypeName)
	assert.Equal(t, "Repo Template", got.TemplateName)
	require.NotNil(t, got.Title)
	assert.Equal(t, "Repo List Doc", *got.Title)
	assert.Len(t, got.Recipients, 2)
	assert.Equal(t, "alice.repo@test.com", got.Recipients[0].Email)
	assert.Equal(t, "Signer A", *got.Recipients[0].RoleName)
	assert.Equal(t, "bob.repo@test.com", got.Recipients[1].Email)
	assert.Equal(t, "Signer B", *got.Recipients[1].RoleName)

	docTypeAlt := testhelper.CreateTestDocumentType(t, pool, tenantID, "REPO_ALT", "Alternate Type")
	t.Cleanup(func() { testhelper.CleanupDocumentType(t, pool, docTypeAlt) })

	_, err = pool.Exec(ctx, `UPDATE execution.documents SET document_type_id = $2 WHERE id = $1`, docID, docTypeAlt)
	require.NoError(t, err)

	byType, err := repo.FindByWorkspace(ctx, workspaceID, port.DocumentFilters{
		DocumentTypeIDs: []string{docTypeAlt},
		Limit:           10,
		Offset:          0,
	})
	require.NoError(t, err)
	var match *entity.DocumentListItem
	for _, it := range byType {
		if it.ID == docID {
			match = it
			break
		}
	}
	require.NotNil(t, match)
	assert.Equal(t, docTypeAlt, match.DocumentTypeID)

	emptyType, err := repo.FindByWorkspace(ctx, workspaceID, port.DocumentFilters{
		DocumentTypeIDs: []string{docTypeID},
		Limit:           10,
		Offset:          0,
	})
	require.NoError(t, err)
	for _, it := range emptyType {
		assert.NotEqual(t, docID, it.ID, "document no longer has original type id")
	}

	opts, err := repo.ListDistinctDocumentTypesForWorkspace(ctx, workspaceID)
	require.NoError(t, err)
	require.NotEmpty(t, opts)
	foundAlt := false
	for _, o := range opts {
		if o.ID == docTypeAlt {
			foundAlt = true
			assert.Equal(t, "Alternate Type", o.Name)
		}
	}
	assert.True(t, foundAlt)
}
