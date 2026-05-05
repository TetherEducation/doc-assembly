package document

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

type stubInternalTemplateContextSearchAdapter struct {
	responses map[string]*port.InternalTemplateContext
	errors    map[string]error
}

func (s *stubInternalTemplateContextSearchAdapter) ResolveInternalTemplateContext(
	_ context.Context,
	params port.InternalTemplateContextSearchParams,
) (*port.InternalTemplateContext, error) {
	workspaceCode := ""
	if len(params.WorkspaceCodes) > 0 {
		workspaceCode = params.WorkspaceCodes[0]
	}
	key := fmt.Sprintf("%s|%s|%s", params.TenantCode, workspaceCode, params.DocumentType)
	if err, ok := s.errors[key]; ok {
		return nil, err
	}
	return s.responses[key], nil
}

func TestDefaultTemplateResolver_Resolve(t *testing.T) {
	resolver := NewDefaultTemplateResolver()
	req := &port.TemplateResolverRequest{
		TenantCode:    "TENANT_A",
		WorkspaceCode: "CLIENT_WS",
		DocumentType:  "CONTRACT",
	}

	t.Run("level 1 hit", func(t *testing.T) {
		adapter := &stubInternalTemplateContextSearchAdapter{
			responses: map[string]*port.InternalTemplateContext{
				"TENANT_A|CLIENT_WS|CONTRACT": templateContext("v-level-1"),
			},
		}

		resolved, err := resolver.Resolve(context.Background(), req, adapter)
		require.NoError(t, err)
		require.NotNil(t, resolved)
		assert.Equal(t, "v-level-1", resolved.Version.ID)
	})

	t.Run("level 2 fallback hit", func(t *testing.T) {
		adapter := &stubInternalTemplateContextSearchAdapter{
			responses: map[string]*port.InternalTemplateContext{
				"TENANT_A|SYS_WRKSP|CONTRACT": templateContext("v-level-2"),
			},
		}

		resolved, err := resolver.Resolve(context.Background(), req, adapter)
		require.NoError(t, err)
		require.NotNil(t, resolved)
		assert.Equal(t, "v-level-2", resolved.Version.ID)
	})

	t.Run("level 3 fallback hit", func(t *testing.T) {
		adapter := &stubInternalTemplateContextSearchAdapter{
			responses: map[string]*port.InternalTemplateContext{
				"SYS|SYS_WRKSP|CONTRACT": templateContext("v-level-3"),
			},
		}

		resolved, err := resolver.Resolve(context.Background(), req, adapter)
		require.NoError(t, err)
		require.NotNil(t, resolved)
		assert.Equal(t, "v-level-3", resolved.Version.ID)
	})

	t.Run("not found", func(t *testing.T) {
		adapter := &stubInternalTemplateContextSearchAdapter{responses: map[string]*port.InternalTemplateContext{}}

		resolved, err := resolver.Resolve(context.Background(), req, adapter)
		require.ErrorIs(t, err, entity.ErrInternalTemplateResolutionNotFound)
		assert.Nil(t, resolved)
	})

	t.Run("adapter error", func(t *testing.T) {
		expectedErr := errors.New("db failed")
		adapter := &stubInternalTemplateContextSearchAdapter{
			errors: map[string]error{
				"TENANT_A|CLIENT_WS|CONTRACT": expectedErr,
			},
		}

		resolved, err := resolver.Resolve(context.Background(), req, adapter)
		require.Error(t, err)
		assert.ErrorContains(t, err, "default template resolution failed at stage tenant_workspace")
		assert.ErrorContains(t, err, "db failed")
		assert.Nil(t, resolved)
	})
}

func templateContext(versionID string) *port.InternalTemplateContext {
	docTypeID := "dt-1"
	return &port.InternalTemplateContext{
		Tenant:       &entity.Tenant{ID: "t-1", Code: "TENANT_A"},
		DocumentType: &entity.DocumentType{ID: docTypeID, Code: "CONTRACT"},
		Template:     &entity.Template{ID: "tpl-1", DocumentTypeID: &docTypeID},
		Workspace:    &entity.Workspace{ID: "w-1", Code: "CLIENT_WS"},
		Version: &entity.TemplateVersionWithDetails{
			TemplateVersion: entity.TemplateVersion{ID: versionID, Status: entity.VersionStatusPublished},
		},
	}
}
