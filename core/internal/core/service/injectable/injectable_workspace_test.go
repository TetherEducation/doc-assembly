package injectable

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

type definitionRepoFake struct {
	port.InjectableRepository
	definitions map[string]*entity.InjectableDefinition
}

func (f *definitionRepoFake) FindByID(_ context.Context, id string) (*entity.InjectableDefinition, error) {
	if definition, ok := f.definitions[id]; ok {
		return definition, nil
	}
	return nil, entity.ErrInjectableNotFound
}

// A workspace reads its own injectable definitions and the global ones. A
// definition of another workspace, like an ID that is not a UUID, answers as a
// missing definition does.
func TestGetInjectable_DefinitionOfAnotherWorkspace(t *testing.T) {
	const (
		workspaceID = "ws-caller"
		ownID       = "6f1c2a9e-3b4d-4e8f-9a01-2b3c4d5e6f70"
		globalID    = "7a2d3b0f-4c5e-4f90-8b12-3c4d5e6f7a81"
		otherID     = "8b3e4c1a-5d6f-4a01-9c23-4d5e6f7a8b92"
		missingID   = "9c4f5d2b-6e7a-4b12-8d34-5e6f7a8b9ca3"
	)
	own, other := workspaceID, "ws-other"
	svc := &InjectableService{injectableRepo: &definitionRepoFake{definitions: map[string]*entity.InjectableDefinition{
		ownID:    {ID: ownID, WorkspaceID: &own, Key: "own"},
		globalID: {ID: globalID, Key: "global"},
		otherID:  {ID: otherID, WorkspaceID: &other, Key: "other"},
	}}}
	ctx := context.Background()

	for _, id := range []string{ownID, globalID} {
		definition, err := svc.GetInjectable(ctx, id, workspaceID)
		require.NoError(t, err)
		assert.Equal(t, id, definition.ID)
	}

	_, missingErr := svc.GetInjectable(ctx, missingID, workspaceID)
	require.ErrorIs(t, missingErr, entity.ErrInjectableNotFound)

	for name, id := range map[string]string{"definition of another workspace": otherID, "not a UUID": "definition-1"} {
		t.Run(name, func(t *testing.T) {
			definition, err := svc.GetInjectable(ctx, id, workspaceID)
			require.ErrorIs(t, err, entity.ErrInjectableNotFound)
			// The message repeats the ID that was asked for, and nothing else differs.
			assert.Equal(t, strings.ReplaceAll(missingErr.Error(), missingID, "{id}"), strings.ReplaceAll(err.Error(), id, "{id}"),
				"the answer must be the one a missing definition gets")
			assert.Nil(t, definition)
		})
	}
}
