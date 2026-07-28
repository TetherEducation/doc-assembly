package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// FolderRepository defines the interface for folder data access.
type FolderRepository interface {
	// Create creates a new folder.
	Create(ctx context.Context, folder *entity.Folder) (string, error)

	// FindByID finds a folder by ID.
	FindByID(ctx context.Context, id string) (*entity.Folder, error)

	// FindByIDWithCounts finds a folder by ID including item counts.
	FindByIDWithCounts(ctx context.Context, id string) (*entity.FolderWithCounts, error)

	// FindByWorkspace lists all folders in a workspace.
	FindByWorkspace(ctx context.Context, workspaceID string) ([]*entity.Folder, error)

	// FindByWorkspaceWithCounts lists all folders in a workspace including item counts.
	FindByWorkspaceWithCounts(ctx context.Context, workspaceID string) ([]*entity.FolderWithCounts, error)

	// FindByParent lists all child folders of a parent folder.
	FindByParent(ctx context.Context, workspaceID string, parentID *string) ([]*entity.Folder, error)

	// FindRootFolders lists all root folders in a workspace.
	FindRootFolders(ctx context.Context, workspaceID string) ([]*entity.Folder, error)

	// Update updates a folder.
	Update(ctx context.Context, folder *entity.Folder) error

	// Delete deletes a folder.
	Delete(ctx context.Context, id string) error

	// HasChildren checks if a folder has child folders.
	HasChildren(ctx context.Context, id string) (bool, error)

	// HasTemplates checks if a folder contains templates.
	HasTemplates(ctx context.Context, id string) (bool, error)

	// ExistsByNameAndParent checks if a folder with the same name exists under the same parent.
	ExistsByNameAndParent(ctx context.Context, workspaceID string, parentID *string, name string) (bool, error)

	// ExistsByNameAndParentExcluding checks excluding a specific folder ID.
	ExistsByNameAndParentExcluding(ctx context.Context, workspaceID string, parentID *string, name, excludeID string) (bool, error)
}
