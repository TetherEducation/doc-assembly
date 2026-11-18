package catalog

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	cataloguc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/catalog"
)

// parentFolderRepoFake finds folders by ID and records what is written.
type parentFolderRepoFake struct {
	port.FolderRepository
	folders map[string]*entity.Folder
	writes  []string
}

func (f *parentFolderRepoFake) FindByID(_ context.Context, id string) (*entity.Folder, error) {
	if folder, ok := f.folders[id]; ok {
		copied := *folder
		return &copied, nil
	}
	return nil, entity.ErrFolderNotFound
}

func (f *parentFolderRepoFake) ExistsByNameAndParent(context.Context, string, *string, string) (bool, error) {
	return false, nil
}

func (f *parentFolderRepoFake) ExistsByNameAndParentExcluding(context.Context, string, *string, string, string) (bool, error) {
	return false, nil
}

func (f *parentFolderRepoFake) Create(_ context.Context, folder *entity.Folder) (string, error) {
	f.writes = append(f.writes, "create "+folder.Name)
	return folder.ID, nil
}

func (f *parentFolderRepoFake) Update(_ context.Context, folder *entity.Folder) error {
	f.writes = append(f.writes, "update "+folder.ID)
	return nil
}

// A parent folder of another workspace, like an ID that is not a UUID, answers
// exactly as a missing parent does, when creating a folder and when moving
// one, and nothing is written.
func TestFolderService_ParentOfTheFoldersWorkspace(t *testing.T) {
	const (
		workspaceID   = "ws-caller"
		ownFolder     = "6f1c2a9e-3b4d-4e8f-9a01-2b3c4d5e6f70"
		ownParent     = "7a2d3b0f-4c5e-4f90-8b12-3c4d5e6f7a81"
		otherParent   = "8b3e4c1a-5d6f-4a01-9c23-4d5e6f7a8b92"
		missingParent = "9c4f5d2b-6e7a-4b12-8d34-5e6f7a8b9ca3"
	)
	newService := func() (*FolderService, *parentFolderRepoFake) {
		repo := &parentFolderRepoFake{folders: map[string]*entity.Folder{
			ownFolder:   {ID: ownFolder, WorkspaceID: workspaceID, Name: "Folder"},
			ownParent:   {ID: ownParent, WorkspaceID: workspaceID, Name: "Parent"},
			otherParent: {ID: otherParent, WorkspaceID: "ws-other", Name: "Other"},
		}}
		return &FolderService{folderRepo: repo}, repo
	}
	create := func(svc *FolderService, parentID string) error {
		_, err := svc.CreateFolder(context.Background(), cataloguc.CreateFolderCommand{
			WorkspaceID: workspaceID, ParentID: &parentID, Name: "Nested",
		})
		return err
	}
	move := func(svc *FolderService, parentID string) error {
		_, err := svc.MoveFolder(context.Background(), cataloguc.MoveFolderCommand{ID: ownFolder, NewParentID: &parentID})
		return err
	}

	for name, act := range map[string]func(*FolderService, string) error{"create": create, "move": move} {
		t.Run(name, func(t *testing.T) {
			svc, repo := newService()
			require.NoError(t, act(svc, ownParent))
			assert.Len(t, repo.writes, 1, "a parent of the folder's workspace is accepted")

			svc, repo = newService()
			missingErr := act(svc, missingParent)
			require.ErrorIs(t, missingErr, entity.ErrFolderNotFound)

			for _, parentID := range []string{otherParent, "not-a-uuid"} {
				err := act(svc, parentID)
				require.ErrorIs(t, err, entity.ErrFolderNotFound)
				assert.EqualError(t, err, missingErr.Error(), "the answer must be the one a missing parent gets")
			}
			assert.Empty(t, repo.writes, "nothing may be written")
		})
	}
}
