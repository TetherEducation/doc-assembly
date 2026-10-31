//go:build integration

package templaterepo_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	templaterepo "github.com/TetherEducation/doc-assembly/core/internal/adapters/secondary/database/postgres/template_repo"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	"github.com/TetherEducation/doc-assembly/core/internal/testing/testhelper"
)

// "Send a PDF" builds a one-off template per document. Template lists leave
// those out unless asked, so the Templates page and Create Document's picker
// show only templates people made.
func TestFindByWorkspace_LeavesOutPDFSendTemplates(t *testing.T) {
	pool := testhelper.GetTestPool(t)
	tenantID := testhelper.CreateTestTenant(t, pool, "PDF Send List Tenant", "PDFSL1")
	t.Cleanup(func() { testhelper.CleanupTenant(t, pool, tenantID) })
	workspaceID := testhelper.CreateTestWorkspace(t, pool, &tenantID, "PDF Send List", entity.WorkspaceTypeClient)
	t.Cleanup(func() { testhelper.CleanupWorkspace(t, pool, workspaceID) })

	regular := testhelper.CreateTestTemplate(t, pool, workspaceID, "Mutual NDA", nil)
	t.Cleanup(func() { testhelper.CleanupTemplate(t, pool, regular) })
	oneOff := testhelper.CreateTestTemplate(t, pool, workspaceID, "SLEP contract · PDF 1a2b3c4d", nil)
	t.Cleanup(func() { testhelper.CleanupTemplate(t, pool, oneOff) })
	setTemplateProcess(t, pool, oneOff, entity.PDFSendProcessPrefix+"1a2b3c4d-0000-0000-0000-000000000000")

	repo := templaterepo.New(pool)
	ids := func(filters port.TemplateFilters) []string {
		t.Helper()
		items, err := repo.FindByWorkspace(context.Background(), workspaceID, filters)
		require.NoError(t, err)
		out := make([]string, 0, len(items))
		for _, item := range items {
			out = append(out, item.ID)
		}
		return out
	}

	assert.Equal(t, []string{regular}, ids(port.TemplateFilters{}), "the one-off template is left out by default")
	assert.Equal(t, []string{regular}, ids(port.TemplateFilters{Search: "n", RootOnly: true}), "and with other filters")
	assert.ElementsMatch(t, []string{regular, oneOff}, ids(port.TemplateFilters{IncludePDFSends: true}), "listed when asked for")
}
