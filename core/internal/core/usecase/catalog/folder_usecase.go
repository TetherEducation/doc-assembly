package catalog

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// CreateFolderCommand represents the command to create a folder.
type CreateFolderCommand struct {
	WorkspaceID string
	ParentID    *string
	Name        string
	CreatedBy   string
}

// UpdateFolderCommand represents the command to update a folder.
type UpdateFolderCommand struct {
	ID   string
	Name string
}

// MoveFolderCommand represents the command to move a folder to a new parent.
type MoveFolderCommand struct {
	ID          string
	NewParentID *string
}

// FolderUseCase defines the input port for folder operations.
type FolderUseCase interface {
	// CreateFolder creates a new folder.
	CreateFolder(ctx context.Context, cmd CreateFolderCommand) (*entity.Folder, error)

	// GetFolder retrieves a folder by ID.
	GetFolder(ctx context.Context, id string) (*entity.Folder, error)

	// GetFolderWithCounts retrieves a folder by ID including item counts.
	GetFolderWithCounts(ctx context.Context, id string) (*entity.FolderWithCounts, error)

	// ListFolders lists all folders in a workspace.
	ListFolders(ctx context.Context, workspaceID string) ([]*entity.Folder, error)

	// ListFoldersWithCounts lists all folders in a workspace including item counts.
	ListFoldersWithCounts(ctx context.Context, workspaceID string) ([]*entity.FolderWithCounts, error)

	// ListChildFolders lists all child folders of a parent.
	ListChildFolders(ctx context.Context, workspaceID string, parentID *string) ([]*entity.Folder, error)

	// ListRootFolders lists all root folders in a workspace.
	ListRootFolders(ctx context.Context, workspaceID string) ([]*entity.Folder, error)

	// GetFolderTree retrieves the complete folder tree for a workspace.
	GetFolderTree(ctx context.Context, workspaceID string) ([]*entity.FolderTree, error)

	// UpdateFolder updates a folder's details.
	UpdateFolder(ctx context.Context, cmd UpdateFolderCommand) (*entity.Folder, error)

	// MoveFolder moves a folder to a new parent.
	MoveFolder(ctx context.Context, cmd MoveFolderCommand) (*entity.Folder, error)

	// DeleteFolder deletes a folder.
	// Returns error if folder has children or contains templates.
	DeleteFolder(ctx context.Context, id string) error

	// GetFolderPath retrieves the full path of a folder from root.
	GetFolderPath(ctx context.Context, id string) ([]*entity.Folder, error)
}
