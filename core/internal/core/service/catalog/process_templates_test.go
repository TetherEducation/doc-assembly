package catalog

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// processTemplatesRepoFake holds the processes of a tenant and of the system
// tenant, by code, and the templates assigned to each code in the tenant. It
// records the lookups for templates and any delete.
type processTemplatesRepoFake struct {
	port.ProcessRepository
	own       map[string]*entity.Process
	global    map[string]*entity.Process
	templates map[string][]*entity.ProcessTemplateInfo
	listed    []string
	deleted   []string
}

func (f *processTemplatesRepoFake) IsSysTenant(context.Context, string) (bool, error) {
	return false, nil
}

func (f *processTemplatesRepoFake) FindByCodeWithGlobalFallback(_ context.Context, _, code string) (*entity.Process, error) {
	if process, ok := f.own[code]; ok {
		return process, nil
	}
	if process, ok := f.global[code]; ok {
		return process, nil
	}
	return nil, entity.ErrProcessNotFound
}

func (f *processTemplatesRepoFake) FindTemplatesByProcess(_ context.Context, tenantID, code string) ([]*entity.ProcessTemplateInfo, error) {
	f.listed = append(f.listed, tenantID+" "+code)
	return f.templates[code], nil
}

func (f *processTemplatesRepoFake) Delete(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	return nil
}

// Listing the templates of a process reads the tenant's templates that use the
// process's code, for a process of the tenant and for a global one, and
// deletes nothing.
func TestProcessService_ListProcessTemplates(t *testing.T) {
	const tenantID = "tenant-caller"
	lease := &entity.ProcessTemplateInfo{ID: "template-1", Title: "Lease", WorkspaceID: "ws-1", WorkspaceName: "Leasing"}
	newService := func() (*ProcessService, *processTemplatesRepoFake) {
		repo := &processTemplatesRepoFake{
			own: map[string]*entity.Process{
				"CONTRACTS": {ID: "process-own", TenantID: tenantID, Code: "CONTRACTS"},
				"UNUSED":    {ID: "process-unused", TenantID: tenantID, Code: "UNUSED"},
			},
			global: map[string]*entity.Process{
				"DEFAULT": {ID: "process-global", TenantID: "tenant-system", Code: "DEFAULT"},
			},
			templates: map[string][]*entity.ProcessTemplateInfo{"CONTRACTS": {lease}, "DEFAULT": {lease}},
		}
		return &ProcessService{processRepo: repo}, repo
	}

	for code, want := range map[string][]*entity.ProcessTemplateInfo{"CONTRACTS": {lease}, "DEFAULT": {lease}, "UNUSED": nil} {
		t.Run(code, func(t *testing.T) {
			svc, repo := newService()

			templates, err := svc.ListProcessTemplates(context.Background(), tenantID, code)

			require.NoError(t, err)
			assert.Equal(t, want, templates)
			assert.Equal(t, []string{tenantID + " " + code}, repo.listed, "the templates are the tenant's own")
			assert.Empty(t, repo.deleted, "nothing may be deleted")
		})
	}

	t.Run("a process that does not exist", func(t *testing.T) {
		svc, repo := newService()

		templates, err := svc.ListProcessTemplates(context.Background(), tenantID, "MISSING")

		require.ErrorIs(t, err, entity.ErrProcessNotFound)
		assert.Nil(t, templates)
		assert.Empty(t, repo.listed)
		assert.Empty(t, repo.deleted)
	})
}
