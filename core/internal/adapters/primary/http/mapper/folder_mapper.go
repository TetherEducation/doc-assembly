package mapper

import (
	"github.com/TetherEducation/doc-assembly/core/internal/adapters/primary/http/dto"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	cataloguc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/catalog"
)

// FolderMapper handles mapping between folder entities and DTOs.
type FolderMapper struct{}

// NewFolderMapper creates a new folder mapper.
func NewFolderMapper() *FolderMapper {
	return &FolderMapper{}
}

// ToResponse converts a Folder entity to a response DTO.
func (m *FolderMapper) ToResponse(f *entity.Folder) *dto.FolderResponse {
	if f == nil {
		return nil
	}
	return &dto.FolderResponse{
		ID:               f.ID,
		WorkspaceID:      f.WorkspaceID,
		ParentID:         f.ParentID,
		Name:             f.Name,
		ChildFolderCount: 0,
		TemplateCount:    0,
		CreatedAt:        f.CreatedAt,
		UpdatedAt:        f.UpdatedAt,
	}
}

// --- Package-level functions for backward compatibility ---

// FolderToResponse converts a Folder entity to a response DTO.
func FolderToResponse(f *entity.Folder) dto.FolderResponse {
	return dto.FolderResponse{
		ID:               f.ID,
		WorkspaceID:      f.WorkspaceID,
		ParentID:         f.ParentID,
		Name:             f.Name,
		ChildFolderCount: 0,
		TemplateCount:    0,
		CreatedAt:        f.CreatedAt,
		UpdatedAt:        f.UpdatedAt,
	}
}

// FolderWithCountsToResponse converts a FolderWithCounts entity to a response DTO.
func FolderWithCountsToResponse(f *entity.FolderWithCounts) dto.FolderResponse {
	return dto.FolderResponse{
		ID:               f.ID,
		WorkspaceID:      f.WorkspaceID,
		ParentID:         f.ParentID,
		Name:             f.Name,
		ChildFolderCount: f.ChildFolderCount,
		TemplateCount:    f.TemplateCount,
		CreatedAt:        f.CreatedAt,
		UpdatedAt:        f.UpdatedAt,
	}
}

// FoldersToResponses converts a slice of Folder entities to response DTOs.
func FoldersToResponses(folders []*entity.Folder) []dto.FolderResponse {
	result := make([]dto.FolderResponse, len(folders))
	for i, f := range folders {
		result[i] = FolderToResponse(f)
	}
	return result
}

// FoldersWithCountsToResponses converts a slice of FolderWithCounts entities to response DTOs.
func FoldersWithCountsToResponses(folders []*entity.FolderWithCounts) []dto.FolderResponse {
	result := make([]dto.FolderResponse, len(folders))
	for i, f := range folders {
		result[i] = FolderWithCountsToResponse(f)
	}
	return result
}

// FolderTreeToResponse converts a FolderTree entity to a response DTO.
func FolderTreeToResponse(ft *entity.FolderTree) *dto.FolderTreeResponse {
	if ft == nil {
		return nil
	}

	children := make([]*dto.FolderTreeResponse, len(ft.Children))
	for i, child := range ft.Children {
		children[i] = FolderTreeToResponse(child)
	}

	return &dto.FolderTreeResponse{
		ID:          ft.ID,
		WorkspaceID: ft.WorkspaceID,
		ParentID:    ft.ParentID,
		Name:        ft.Name,
		CreatedAt:   ft.CreatedAt,
		UpdatedAt:   ft.UpdatedAt,
		Children:    children,
	}
}

// FolderTreesToResponses converts a slice of FolderTree entities to response DTOs.
func FolderTreesToResponses(trees []*entity.FolderTree) []*dto.FolderTreeResponse {
	result := make([]*dto.FolderTreeResponse, len(trees))
	for i, t := range trees {
		result[i] = FolderTreeToResponse(t)
	}
	return result
}

// FolderPathToResponse converts a folder path (slice of folders) to a response DTO.
func FolderPathToResponse(folders []*entity.Folder) dto.FolderPathResponse {
	result := make([]dto.FolderResponse, len(folders))
	for i, f := range folders {
		result[i] = FolderToResponse(f)
	}
	return dto.FolderPathResponse{Folders: result}
}

// CreateFolderRequestToCommand converts a create request to a usecase command.
func CreateFolderRequestToCommand(workspaceID string, req dto.CreateFolderRequest, createdBy string) cataloguc.CreateFolderCommand {
	return cataloguc.CreateFolderCommand{
		WorkspaceID: workspaceID,
		ParentID:    req.ParentID,
		Name:        req.Name,
		CreatedBy:   createdBy,
	}
}

// UpdateFolderRequestToCommand converts an update request to a usecase command.
func UpdateFolderRequestToCommand(id string, req dto.UpdateFolderRequest) cataloguc.UpdateFolderCommand {
	return cataloguc.UpdateFolderCommand{
		ID:   id,
		Name: req.Name,
	}
}

// MoveFolderRequestToCommand converts a move request to a usecase command.
func MoveFolderRequestToCommand(id string, req dto.MoveFolderRequest) cataloguc.MoveFolderCommand {
	return cataloguc.MoveFolderCommand{
		ID:          id,
		NewParentID: req.NewParentID,
	}
}
