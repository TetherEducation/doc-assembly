package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// GalleryRepository defines the output port for gallery asset persistence.
type GalleryRepository interface {
	// Save persists a new gallery asset.
	Save(ctx context.Context, asset *entity.GalleryAsset) error

	// FindBySHA256 looks up an asset by its content hash within a workspace.
	// Returns nil, nil if not found.
	FindBySHA256(ctx context.Context, workspaceID, sha256 string) (*entity.GalleryAsset, error)

	// FindByKey looks up an asset by its storage key within a workspace.
	FindByKey(ctx context.Context, workspaceID, key string) (*entity.GalleryAsset, error)

	// List returns a paginated list of assets for a workspace, ordered by created_at DESC.
	List(ctx context.Context, workspaceID string, page, perPage int) ([]*entity.GalleryAsset, int, error)

	// Search returns assets whose filename matches the query string.
	Search(ctx context.Context, workspaceID, query string, page, perPage int) ([]*entity.GalleryAsset, int, error)

	// Delete removes an asset record by key.
	Delete(ctx context.Context, workspaceID, key string) error
}
