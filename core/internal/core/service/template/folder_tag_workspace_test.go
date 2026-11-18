package template

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

const (
	scopeWorkspace      = "ws-caller"
	scopeSandbox        = "ws-caller-sandbox"
	scopeOtherWorkspace = "ws-other"

	scopeOwnFolder     = "6f1c2a9e-3b4d-4e8f-9a01-2b3c4d5e6f70"
	scopeOtherFolder   = "7a2d3b0f-4c5e-4f90-8b12-3c4d5e6f7a81"
	scopeMissingFolder = "8b3e4c1a-5d6f-4a01-9c23-4d5e6f7a8b92"

	scopeOwnTag     = "9c4f5d2b-6e7a-4b12-8d34-5e6f7a8b9ca3"
	scopeSandboxTag = "ad5a6e3c-7f8b-4c23-9e45-6f7a8b9cadb4"
	scopeOtherTag   = "be6b7f4d-8a9c-4d34-8f56-7a8b9cadbec5"
	scopeMissingTag = "cf7c8a5e-9bad-4e45-9a67-8b9cadbecfd6"
)

type scopeFolderRepoFake struct {
	port.FolderRepository
	folders map[string]*entity.Folder
}

func (f *scopeFolderRepoFake) FindByID(_ context.Context, id string) (*entity.Folder, error) {
	if folder, ok := f.folders[id]; ok {
		return folder, nil
	}
	return nil, entity.ErrFolderNotFound
}

type scopeTagRepoFake struct {
	port.TagRepository
	tags map[string]*entity.Tag
}

func (f *scopeTagRepoFake) FindByID(_ context.Context, id string) (*entity.Tag, error) {
	if tag, ok := f.tags[id]; ok {
		return tag, nil
	}
	return nil, entity.ErrTagNotFound
}

type scopeTemplateRepoFake struct {
	port.TemplateRepository
	templates map[string]*entity.Template
}

func (f *scopeTemplateRepoFake) FindByID(_ context.Context, id string) (*entity.Template, error) {
	if template, ok := f.templates[id]; ok {
		return template, nil
	}
	return nil, entity.ErrTemplateNotFound
}

type scopeWorkspaceRepoFake struct {
	port.WorkspaceRepository
	workspaces map[string]*entity.Workspace
}

func (f *scopeWorkspaceRepoFake) FindByID(_ context.Context, id string) (*entity.Workspace, error) {
	if workspace, ok := f.workspaces[id]; ok {
		return workspace, nil
	}
	return nil, entity.ErrWorkspaceNotFound
}

// scopeTemplateTagRepoFake records the tags linked to templates.
type scopeTemplateTagRepoFake struct {
	port.TemplateTagRepository
	links []string
}

func (f *scopeTemplateTagRepoFake) Exists(context.Context, string, string) (bool, error) {
	return false, nil
}

func (f *scopeTemplateTagRepoFake) AddTag(_ context.Context, templateID, tagID string) error {
	f.links = append(f.links, templateID+" "+tagID)
	return nil
}

// A folder of another workspace, like an ID that is not a UUID, answers exactly
// as a missing folder does.
func TestFolderInWorkspace(t *testing.T) {
	ctx := context.Background()
	folders := &scopeFolderRepoFake{folders: map[string]*entity.Folder{
		scopeOwnFolder:   {ID: scopeOwnFolder, WorkspaceID: scopeWorkspace},
		scopeOtherFolder: {ID: scopeOtherFolder, WorkspaceID: scopeOtherWorkspace},
	}}
	folderID := func(id string) *string { return &id }

	require.NoError(t, folderInWorkspace(ctx, folders, nil, scopeWorkspace), "no folder named")
	require.NoError(t, folderInWorkspace(ctx, folders, folderID(scopeOwnFolder), scopeWorkspace))

	missingErr := folderInWorkspace(ctx, folders, folderID(scopeMissingFolder), scopeWorkspace)
	require.ErrorIs(t, missingErr, entity.ErrFolderNotFound)

	for name, id := range map[string]string{
		"folder of another workspace": scopeOtherFolder,
		"not a UUID":                  "root",
	} {
		t.Run(name, func(t *testing.T) {
			err := folderInWorkspace(ctx, folders, folderID(id), scopeWorkspace)
			require.ErrorIs(t, err, entity.ErrFolderNotFound)
			assert.EqualError(t, err, missingErr.Error(), "the answer must be the one a missing folder gets")
		})
	}
}

// A template takes the tags of its own workspace, and a sandbox template those
// of the workspace the sandbox belongs to. Any other tag answers exactly as a
// missing tag does, and is not linked.
func TestAddTag_TagOfTheTemplatesWorkspace(t *testing.T) {
	const (
		ownTemplate     = "template-own"
		sandboxTemplate = "template-sandbox"
	)
	parent := scopeWorkspace
	newService := func() (*TemplateService, *scopeTemplateTagRepoFake) {
		links := &scopeTemplateTagRepoFake{}
		return &TemplateService{
			templateRepo: &scopeTemplateRepoFake{templates: map[string]*entity.Template{
				ownTemplate:     {ID: ownTemplate, WorkspaceID: scopeWorkspace},
				sandboxTemplate: {ID: sandboxTemplate, WorkspaceID: scopeSandbox},
			}},
			tagRepo: links,
			workspaceTagRepo: &scopeTagRepoFake{tags: map[string]*entity.Tag{
				scopeOwnTag:     {ID: scopeOwnTag, WorkspaceID: scopeWorkspace},
				scopeSandboxTag: {ID: scopeSandboxTag, WorkspaceID: scopeSandbox},
				scopeOtherTag:   {ID: scopeOtherTag, WorkspaceID: scopeOtherWorkspace},
			}},
			workspaceRepo: &scopeWorkspaceRepoFake{workspaces: map[string]*entity.Workspace{
				scopeWorkspace:      {ID: scopeWorkspace},
				scopeSandbox:        {ID: scopeSandbox, IsSandbox: true, SandboxOfID: &parent},
				scopeOtherWorkspace: {ID: scopeOtherWorkspace},
			}},
		}, links
	}

	usable := []struct{ name, templateID, tagID string }{
		{"template and tag of one workspace", ownTemplate, scopeOwnTag},
		{"sandbox template, tag of its workspace", sandboxTemplate, scopeOwnTag},
		{"sandbox template, tag of the sandbox", sandboxTemplate, scopeSandboxTag},
	}
	for _, tc := range usable {
		t.Run(tc.name, func(t *testing.T) {
			svc, links := newService()
			require.NoError(t, svc.AddTag(context.Background(), tc.templateID, tc.tagID))
			assert.Equal(t, []string{tc.templateID + " " + tc.tagID}, links.links)
		})
	}

	refused := []struct{ name, templateID, tagID string }{
		{"tag of another workspace", ownTemplate, scopeOtherTag},
		{"sandbox template, tag of another workspace", sandboxTemplate, scopeOtherTag},
		{"tag of the workspace's sandbox", ownTemplate, scopeSandboxTag},
		{"not a UUID", ownTemplate, "tag-1"},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			svc, links := newService()
			missingErr := svc.AddTag(context.Background(), tc.templateID, scopeMissingTag)
			require.ErrorIs(t, missingErr, entity.ErrTagNotFound)

			err := svc.AddTag(context.Background(), tc.templateID, tc.tagID)
			require.ErrorIs(t, err, entity.ErrTagNotFound)
			assert.EqualError(t, err, missingErr.Error(), "the answer must be the one a missing tag gets")
			assert.Empty(t, links.links, "no tag may be linked")
		})
	}
}
