package catalog

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	cataloguc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/catalog"
)

// NewFolderService creates a new folder service.
func NewFolderService(folderRepo port.FolderRepository) cataloguc.FolderUseCase {
	return &FolderService{
		folderRepo: folderRepo,
	}
}

// FolderService implements folder business logic.
type FolderService struct {
	folderRepo port.FolderRepository
}

// CreateFolder creates a new folder.
func (s *FolderService) CreateFolder(ctx context.Context, cmd cataloguc.CreateFolderCommand) (*entity.Folder, error) {
	// Check for duplicate name under same parent
	exists, err := s.folderRepo.ExistsByNameAndParent(ctx, cmd.WorkspaceID, cmd.ParentID, cmd.Name)
	if err != nil {
		return nil, fmt.Errorf("checking folder existence: %w", err)
	}
	if exists {
		return nil, entity.ErrFolderAlreadyExists
	}

	// Validate parent folder if specified
	if cmd.ParentID != nil {
		parent, err := s.folderRepo.FindByID(ctx, *cmd.ParentID)
		if err != nil {
			return nil, fmt.Errorf("finding parent folder: %w", err)
		}
		if parent.WorkspaceID != cmd.WorkspaceID {
			return nil, entity.ErrInvalidParentFolder
		}
	}

	folder := &entity.Folder{
		ID:          uuid.NewString(),
		WorkspaceID: cmd.WorkspaceID,
		ParentID:    cmd.ParentID,
		Name:        cmd.Name,
		CreatedAt:   time.Now().UTC(),
	}

	if err := folder.Validate(); err != nil {
		return nil, fmt.Errorf("validating folder: %w", err)
	}

	id, err := s.folderRepo.Create(ctx, folder)
	if err != nil {
		return nil, fmt.Errorf("creating folder: %w", err)
	}
	folder.ID = id

	slog.InfoContext(ctx, "folder created",
		slog.String("folder_id", folder.ID),
		slog.String("name", folder.Name),
		slog.String("workspace_id", folder.WorkspaceID),
	)

	return folder, nil
}

// GetFolder retrieves a folder by ID.
func (s *FolderService) GetFolder(ctx context.Context, id string) (*entity.Folder, error) {
	folder, err := s.folderRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding folder %s: %w", id, err)
	}
	return folder, nil
}

// GetFolderWithCounts retrieves a folder by ID including item counts.
func (s *FolderService) GetFolderWithCounts(ctx context.Context, id string) (*entity.FolderWithCounts, error) {
	folder, err := s.folderRepo.FindByIDWithCounts(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding folder %s with counts: %w", id, err)
	}
	return folder, nil
}

// ListFolders lists all folders in a workspace.
func (s *FolderService) ListFolders(ctx context.Context, workspaceID string) ([]*entity.Folder, error) {
	folders, err := s.folderRepo.FindByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("listing folders: %w", err)
	}
	return folders, nil
}

// ListFoldersWithCounts lists all folders in a workspace including item counts.
func (s *FolderService) ListFoldersWithCounts(ctx context.Context, workspaceID string) ([]*entity.FolderWithCounts, error) {
	folders, err := s.folderRepo.FindByWorkspaceWithCounts(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("listing folders with counts: %w", err)
	}
	return folders, nil
}

// ListChildFolders lists all child folders of a parent.
func (s *FolderService) ListChildFolders(ctx context.Context, workspaceID string, parentID *string) ([]*entity.Folder, error) {
	folders, err := s.folderRepo.FindByParent(ctx, workspaceID, parentID)
	if err != nil {
		return nil, fmt.Errorf("listing child folders: %w", err)
	}
	return folders, nil
}

// ListRootFolders lists all root folders in a workspace.
func (s *FolderService) ListRootFolders(ctx context.Context, workspaceID string) ([]*entity.Folder, error) {
	folders, err := s.folderRepo.FindRootFolders(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("listing root folders: %w", err)
	}
	return folders, nil
}

// GetFolderTree retrieves the complete folder tree for a workspace.
func (s *FolderService) GetFolderTree(ctx context.Context, workspaceID string) ([]*entity.FolderTree, error) {
	folders, err := s.folderRepo.FindByWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("listing folders for tree: %w", err)
	}
	return entity.BuildFolderTree(folders), nil
}

// UpdateFolder updates a folder's details.
func (s *FolderService) UpdateFolder(ctx context.Context, cmd cataloguc.UpdateFolderCommand) (*entity.Folder, error) {
	folder, err := s.folderRepo.FindByID(ctx, cmd.ID)
	if err != nil {
		return nil, fmt.Errorf("finding folder: %w", err)
	}

	// Check for duplicate name if name changed
	if folder.Name != cmd.Name {
		exists, err := s.folderRepo.ExistsByNameAndParentExcluding(ctx, folder.WorkspaceID, folder.ParentID, cmd.Name, folder.ID)
		if err != nil {
			return nil, fmt.Errorf("checking folder name: %w", err)
		}
		if exists {
			return nil, entity.ErrFolderAlreadyExists
		}
	}

	folder.Name = cmd.Name
	now := time.Now().UTC()
	folder.UpdatedAt = &now

	if err := folder.Validate(); err != nil {
		return nil, fmt.Errorf("validating folder: %w", err)
	}

	if err := s.folderRepo.Update(ctx, folder); err != nil {
		return nil, fmt.Errorf("updating folder: %w", err)
	}

	slog.InfoContext(ctx, "folder updated",
		slog.String("folder_id", folder.ID),
		slog.String("name", folder.Name),
	)

	return folder, nil
}

// MoveFolder moves a folder to a new parent.
func (s *FolderService) MoveFolder(ctx context.Context, cmd cataloguc.MoveFolderCommand) (*entity.Folder, error) {
	folder, err := s.folderRepo.FindByID(ctx, cmd.ID)
	if err != nil {
		return nil, fmt.Errorf("finding folder: %w", err)
	}

	if cmd.NewParentID != nil {
		if err := s.validateNewParent(ctx, folder, *cmd.NewParentID); err != nil {
			return nil, err
		}
	}

	// Check for duplicate name under new parent
	exists, err := s.folderRepo.ExistsByNameAndParentExcluding(ctx, folder.WorkspaceID, cmd.NewParentID, folder.Name, folder.ID)
	if err != nil {
		return nil, fmt.Errorf("checking folder name: %w", err)
	}
	if exists {
		return nil, entity.ErrFolderAlreadyExists
	}

	folder.ParentID = cmd.NewParentID
	now := time.Now().UTC()
	folder.UpdatedAt = &now

	if err := s.folderRepo.Update(ctx, folder); err != nil {
		return nil, fmt.Errorf("moving folder: %w", err)
	}

	slog.InfoContext(ctx, "folder moved",
		slog.String("folder_id", folder.ID),
		slog.Any("new_parent_id", cmd.NewParentID),
	)

	return folder, nil
}

// DeleteFolder deletes a folder.
func (s *FolderService) DeleteFolder(ctx context.Context, id string) error {
	// Check if folder has children
	hasChildren, err := s.folderRepo.HasChildren(ctx, id)
	if err != nil {
		return fmt.Errorf("checking folder children: %w", err)
	}
	if hasChildren {
		return entity.ErrFolderHasChildren
	}

	// Check if folder has templates
	hasTemplates, err := s.folderRepo.HasTemplates(ctx, id)
	if err != nil {
		return fmt.Errorf("checking folder templates: %w", err)
	}
	if hasTemplates {
		return entity.ErrFolderHasTemplates
	}

	if err := s.folderRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("deleting folder: %w", err)
	}

	slog.InfoContext(ctx, "folder deleted", slog.String("folder_id", id))
	return nil
}

// GetFolderPath retrieves the full path of a folder from root.
func (s *FolderService) GetFolderPath(ctx context.Context, id string) ([]*entity.Folder, error) {
	var path []*entity.Folder
	currentID := id

	for currentID != "" {
		folder, err := s.folderRepo.FindByID(ctx, currentID)
		if err != nil {
			return nil, fmt.Errorf("building folder path: %w", err)
		}
		path = append([]*entity.Folder{folder}, path...)
		if folder.ParentID == nil {
			break
		}
		currentID = *folder.ParentID
	}

	return path, nil
}

// validateNewParent validates that a folder can be moved to a new parent.
func (s *FolderService) validateNewParent(ctx context.Context, folder *entity.Folder, newParentID string) error {
	if newParentID == folder.ID {
		return entity.ErrCircularReference
	}

	parent, err := s.folderRepo.FindByID(ctx, newParentID)
	if err != nil {
		return fmt.Errorf("finding new parent folder: %w", err)
	}

	if parent.WorkspaceID != folder.WorkspaceID {
		return entity.ErrInvalidParentFolder
	}

	return s.checkCircularReference(ctx, folder.ID, newParentID)
}

// checkCircularReference checks if moving a folder to a new parent would create a cycle.
func (s *FolderService) checkCircularReference(ctx context.Context, folderID, newParentID string) error {
	currentID := newParentID

	for currentID != "" {
		if currentID == folderID {
			return entity.ErrCircularReference
		}
		parent, err := s.folderRepo.FindByID(ctx, currentID)
		if err != nil {
			return fmt.Errorf("checking circular reference: %w", err)
		}
		if parent.ParentID == nil {
			break
		}
		currentID = *parent.ParentID
	}

	return nil
}
