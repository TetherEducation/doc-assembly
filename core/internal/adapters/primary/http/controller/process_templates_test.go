package controller

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/mapper"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	cataloguc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/catalog"
)

// processTemplatesUseCaseFake holds one process and the templates assigned to
// it, and records the processes it is asked to delete. Methods the tests don't
// call fall through to the nil embedded interface and panic.
type processTemplatesUseCaseFake struct {
	cataloguc.ProcessUseCase
	process   *entity.Process
	templates []*entity.ProcessTemplateInfo
	deleted   []string
}

func (f *processTemplatesUseCaseFake) find(code string) (*entity.Process, error) {
	if f.process != nil && f.process.Code == code {
		return f.process, nil
	}
	return nil, fmt.Errorf("finding process by code %s: %w", code, entity.ErrProcessNotFound)
}

func (f *processTemplatesUseCaseFake) GetProcessByCode(_ context.Context, _, code string) (*entity.Process, error) {
	return f.find(code)
}

func (f *processTemplatesUseCaseFake) ListProcessTemplates(_ context.Context, _, code string) ([]*entity.ProcessTemplateInfo, error) {
	if _, err := f.find(code); err != nil {
		return nil, err
	}
	return f.templates, nil
}

func (f *processTemplatesUseCaseFake) DeleteProcess(_ context.Context, cmd cataloguc.DeleteProcessCommand) (*cataloguc.DeleteProcessResult, error) {
	f.deleted = append(f.deleted, cmd.ID)
	return &cataloguc.DeleteProcessResult{Deleted: true}, nil
}

func listProcessTemplates(t *testing.T, processes *processTemplatesUseCaseFake, code string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctrl := NewProcessController(processes, mapper.NewProcessMapper())

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/tenant/processes/code/"+code+"/templates", nil)
	ctx.Params = gin.Params{{Key: "code", Value: code}}
	ctx.Set("tenant_id", "tenant-1")

	ctrl.ListTemplatesByProcessCode(ctx)
	return rec
}

// Listing the templates of a process is a read: it must not delete the
// process, whether templates use it or not.
func TestListTemplatesByProcessCode_OnlyReads(t *testing.T) {
	process := &entity.Process{ID: "process-1", TenantID: "tenant-1", Code: "CONTRACTS"}

	t.Run("a process no template uses", func(t *testing.T) {
		processes := &processTemplatesUseCaseFake{process: process}

		rec := listProcessTemplates(t, processes, "CONTRACTS")

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.JSONEq(t, `[]`, rec.Body.String())
		assert.Empty(t, processes.deleted, "the process must not be deleted")
	})

	t.Run("a process templates use", func(t *testing.T) {
		processes := &processTemplatesUseCaseFake{process: process, templates: []*entity.ProcessTemplateInfo{
			{ID: "template-1", Title: "Lease", WorkspaceID: "ws-1", WorkspaceName: "Leasing"},
		}}

		rec := listProcessTemplates(t, processes, "CONTRACTS")

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.JSONEq(t, `[{"id":"template-1","title":"Lease","workspaceId":"ws-1","workspaceName":"Leasing"}]`, rec.Body.String())
		assert.Empty(t, processes.deleted, "the process must not be deleted")
	})

	t.Run("a process that does not exist", func(t *testing.T) {
		processes := &processTemplatesUseCaseFake{process: process}

		rec := listProcessTemplates(t, processes, "MISSING")

		assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
		assert.Empty(t, processes.deleted)
	})
}
