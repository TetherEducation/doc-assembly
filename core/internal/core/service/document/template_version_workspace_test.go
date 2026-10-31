package document

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
)

// The fakes below hold one published version, ver-1, of a template in ws-1
// with two signer roles.

type templateVersionScopeVersionRepoFake struct {
	port.TemplateVersionRepository
}

func (templateVersionScopeVersionRepoFake) FindByID(_ context.Context, id string) (*entity.TemplateVersion, error) {
	if id != "ver-1" {
		return nil, entity.ErrVersionNotFound
	}
	return &entity.TemplateVersion{ID: "ver-1", TemplateID: "tpl-1", Status: entity.VersionStatusPublished}, nil
}

type templateVersionScopeTemplateRepoFake struct {
	port.TemplateRepository
}

func (templateVersionScopeTemplateRepoFake) FindTemplateWorkspaceByTemplateID(context.Context, string) (*port.TemplateWorkspace, error) {
	docType := "doctype-1"
	return &port.TemplateWorkspace{
		Template:  &entity.Template{ID: "tpl-1", WorkspaceID: "ws-1", DocumentTypeID: &docType},
		Workspace: &entity.Workspace{ID: "ws-1", Type: entity.WorkspaceTypeClient},
	}, nil
}

type templateVersionScopeSignerRoleRepoFake struct {
	port.TemplateVersionSignerRoleRepository
}

func (templateVersionScopeSignerRoleRepoFake) FindByVersionID(context.Context, string) ([]*entity.TemplateVersionSignerRole, error) {
	return []*entity.TemplateVersionSignerRole{{ID: "role-1", SignerOrder: 1}, {ID: "role-2", SignerOrder: 2}}, nil
}

type templateVersionScopeDocumentRepoFake struct {
	port.DocumentRepository
	created int
}

func (f *templateVersionScopeDocumentRepoFake) Create(context.Context, *entity.Document) (string, error) {
	f.created++
	return "doc-1", nil
}

// A template version whose template is in another workspace answers exactly as
// a missing version does (see findTemplateVersion), before the recipients are
// checked against its signer roles, and nothing is created.
func TestCreateAndSendDocument_TemplateVersionFromAnotherWorkspace(t *testing.T) {
	docs := &templateVersionScopeDocumentRepoFake{}
	svc := &DocumentService{
		documentRepo:   docs,
		versionRepo:    templateVersionScopeVersionRepoFake{},
		templateRepo:   templateVersionScopeTemplateRepoFake{},
		signerRoleRepo: templateVersionScopeSignerRoleRepoFake{},
	}

	// One recipient for the version's two signer roles: a request that gets as
	// far as the role check fails there, and its answer gives the count away.
	createCmd := func(workspaceID, versionID string) documentuc.CreateDocumentCommand {
		return documentuc.CreateDocumentCommand{
			WorkspaceID:       workspaceID,
			TemplateVersionID: versionID,
			Title:             "Mutual NDA",
			Recipients: []documentuc.DocumentRecipientCommand{
				{RoleID: "role-1", Name: "Signer", Email: "signer@example.com"},
			},
		}
	}

	_, missingErr := svc.CreateAndSendDocument(context.Background(), createCmd("ws-2", "ver-missing"))
	require.ErrorIs(t, missingErr, entity.ErrVersionNotFound)

	got, err := svc.CreateAndSendDocument(context.Background(), createCmd("ws-2", "ver-1"))
	require.ErrorIs(t, err, entity.ErrVersionNotFound)
	assert.EqualError(t, err, missingErr.Error(), "the answer must be the one a missing version gets")
	assert.Nil(t, got)

	// From the template's own workspace the same request reaches the role check.
	_, err = svc.CreateAndSendDocument(context.Background(), createCmd("ws-1", "ver-1"))
	require.Error(t, err)
	assert.NotErrorIs(t, err, entity.ErrVersionNotFound)
	assert.Contains(t, err.Error(), "does not match signer role count")

	assert.Zero(t, docs.created, "no document may be created")
}
