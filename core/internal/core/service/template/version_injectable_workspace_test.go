package template

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

// draftVersionRepoFake holds one draft version.
type draftVersionRepoFake struct {
	port.TemplateVersionRepository
	version *entity.TemplateVersion
}

func (f *draftVersionRepoFake) FindByID(_ context.Context, id string) (*entity.TemplateVersion, error) {
	if f.version != nil && f.version.ID == id {
		copied := *f.version
		return &copied, nil
	}
	return nil, entity.ErrVersionNotFound
}

// versionInjectableRepoFake knows the definitions a version can take and
// records the ones linked to it.
type versionInjectableRepoFake struct {
	port.TemplateVersionInjectableRepository
	available map[string]bool
	linked    []string
}

func (f *versionInjectableRepoFake) DefinitionAvailable(_ context.Context, _, injectableDefID string) (bool, error) {
	return f.available[injectableDefID], nil
}

func (f *versionInjectableRepoFake) Exists(context.Context, string, string) (bool, error) {
	return false, nil
}

func (f *versionInjectableRepoFake) Create(_ context.Context, injectable *entity.TemplateVersionInjectable) (string, error) {
	f.linked = append(f.linked, *injectable.InjectableDefinitionID)
	return injectable.ID, nil
}

// A version takes an injectable definition its template's workspace can use.
// Any other, like an ID that is not a UUID, answers exactly as a missing
// definition does, and is not linked.
func TestAddInjectable_DefinitionOfTheTemplatesWorkspace(t *testing.T) {
	const (
		versionID = "version-1"
		usableID  = "6f1c2a9e-3b4d-4e8f-9a01-2b3c4d5e6f70"
		otherID   = "7a2d3b0f-4c5e-4f90-8b12-3c4d5e6f7a81"
		missingID = "8b3e4c1a-5d6f-4a01-9c23-4d5e6f7a8b92"
	)
	add := func(injectableDefID string) ([]string, error) {
		links := &versionInjectableRepoFake{available: map[string]bool{usableID: true}}
		svc := &TemplateVersionService{
			versionRepo:    &draftVersionRepoFake{version: &entity.TemplateVersion{ID: versionID, Status: entity.VersionStatusDraft}},
			injectableRepo: links,
		}
		_, err := svc.AddInjectable(context.Background(), templateuc.AddVersionInjectableCommand{
			VersionID: versionID, InjectableDefinitionID: injectableDefID,
		})
		return links.linked, err
	}

	linked, err := add(usableID)
	require.NoError(t, err)
	assert.Equal(t, []string{usableID}, linked)

	_, missingErr := add(missingID)
	require.ErrorIs(t, missingErr, entity.ErrInjectableNotFound)

	for name, id := range map[string]string{"definition of another workspace": otherID, "not a UUID": "definition-1"} {
		t.Run(name, func(t *testing.T) {
			linked, err := add(id)
			require.ErrorIs(t, err, entity.ErrInjectableNotFound)
			assert.EqualError(t, err, missingErr.Error(), "the answer must be the one a missing definition gets")
			assert.Empty(t, linked, "no definition may be linked")
		})
	}
}
