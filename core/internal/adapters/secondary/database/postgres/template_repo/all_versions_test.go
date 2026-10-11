package templaterepo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

func versionWithID(id string) *entity.TemplateVersionWithDetails {
	return &entity.TemplateVersionWithDetails{TemplateVersion: entity.TemplateVersion{ID: id}}
}

func injectableFor(id, versionID, key string) *entity.VersionInjectableWithDefinition {
	return &entity.VersionInjectableWithDefinition{
		TemplateVersionInjectable: entity.TemplateVersionInjectable{ID: id, TemplateVersionID: versionID},
		Definition:                &entity.InjectableDefinition{Key: key},
	}
}

func signerRoleFor(id, versionID string, order int) *entity.TemplateVersionSignerRole {
	return &entity.TemplateVersionSignerRole{ID: id, TemplateVersionID: versionID, SignerOrder: order}
}

// The all-versions endpoint feeds the Create Document wizard, which reads each version's
// signerRoles and injectables. They are loaded in one batch per kind for all versions and
// must land on the version they belong to, keeping the query's order.
func TestAttachVersionDetails_GroupsByVersionPreservingOrder(t *testing.T) {
	v1, v2, v3 := versionWithID("v1"), versionWithID("v2"), versionWithID("v3")
	versions := []*entity.TemplateVersionWithDetails{v3, v2, v1}

	injectables := []*entity.VersionInjectableWithDefinition{
		injectableFor("i1", "v1", "alpha"),
		injectableFor("i2", "v2", "alpha"),
		injectableFor("i3", "v1", "beta"),
	}
	roles := []*entity.TemplateVersionSignerRole{
		signerRoleFor("r1", "v1", 1),
		signerRoleFor("r2", "v1", 2),
		signerRoleFor("r3", "v2", 1),
	}

	attachVersionDetails(versions, injectables, roles)

	require.Len(t, v1.Injectables, 2)
	assert.Equal(t, "i1", v1.Injectables[0].ID)
	assert.Equal(t, "i3", v1.Injectables[1].ID)
	require.Len(t, v1.SignerRoles, 2)
	assert.Equal(t, "r1", v1.SignerRoles[0].ID)
	assert.Equal(t, "r2", v1.SignerRoles[1].ID)

	require.Len(t, v2.Injectables, 1)
	assert.Equal(t, "i2", v2.Injectables[0].ID)
	require.Len(t, v2.SignerRoles, 1)
	assert.Equal(t, "r3", v2.SignerRoles[0].ID)

	assert.Empty(t, v3.Injectables)
	assert.Empty(t, v3.SignerRoles)
}

func TestAttachVersionDetails_IgnoresRowsForUnknownVersions(t *testing.T) {
	v1 := versionWithID("v1")

	attachVersionDetails(
		[]*entity.TemplateVersionWithDetails{v1},
		[]*entity.VersionInjectableWithDefinition{injectableFor("i9", "other", "k")},
		[]*entity.TemplateVersionSignerRole{signerRoleFor("r9", "other", 1)},
	)

	assert.Empty(t, v1.Injectables)
	assert.Empty(t, v1.SignerRoles)
}

func TestVersionIDs(t *testing.T) {
	assert.Equal(t, []string{"a", "b"},
		versionIDs([]*entity.TemplateVersionWithDetails{versionWithID("a"), versionWithID("b")}))
	assert.Empty(t, versionIDs(nil))
}
