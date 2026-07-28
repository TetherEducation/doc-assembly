package workspacerepo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

func TestRepository_CreateSandbox_Validation(t *testing.T) {
	repo := &Repository{}

	parentID := "parent-workspace-id"

	tests := []struct {
		name    string
		sandbox *entity.Workspace
		wantErr error
	}{
		{
			name:    "nil sandbox returns required field error",
			sandbox: nil,
			wantErr: entity.ErrRequiredField,
		},
		{
			name: "missing sandbox parent returns required field error",
			sandbox: &entity.Workspace{
				ID:        "sandbox-id",
				Type:      entity.WorkspaceTypeClient,
				IsSandbox: true,
			},
			wantErr: entity.ErrRequiredField,
		},
		{
			name: "non-sandbox workspace is rejected",
			sandbox: &entity.Workspace{
				ID:          "workspace-id",
				Type:        entity.WorkspaceTypeClient,
				IsSandbox:   false,
				SandboxOfID: &parentID,
			},
			wantErr: entity.ErrSandboxNotSupported,
		},
		{
			name: "system workspace sandbox is rejected",
			sandbox: &entity.Workspace{
				ID:          "sandbox-system-id",
				Type:        entity.WorkspaceTypeSystem,
				IsSandbox:   true,
				SandboxOfID: &parentID,
			},
			wantErr: entity.ErrSandboxNotSupported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := repo.CreateSandbox(context.Background(), tt.sandbox)
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}
