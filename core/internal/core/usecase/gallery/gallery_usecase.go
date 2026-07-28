package gallery

import (
	"context"
	"errors"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

const (
	// DefaultPage is the default gallery page number.
	DefaultPage = 1
	// DefaultPerPage is the default gallery page size.
	DefaultPerPage = 20
	// MaxUploadSize is the maximum accepted upload size in bytes (10 MB).
	MaxUploadSize = 10 << 20
)

var (
	// ErrQueryRequired indicates that the search query is required.
	ErrQueryRequired = errors.New("query parameter 'q' is required")
	// ErrAssetKeyRequired indicates that the asset key is required.
	ErrAssetKeyRequired = errors.New("query parameter 'key' is required")
	// ErrUploadEmpty indicates that an upload body was empty.
	ErrUploadEmpty = errors.New("gallery upload is empty")
)

// AssetsPage holds a paginated list of gallery assets.
type AssetsPage struct {
	Assets  []*entity.GalleryAsset
	Total   int
	Page    int
	PerPage int
}

// AssetPayload is a binary gallery asset resolved for HTTP serving.
type AssetPayload struct {
	Data        []byte
	ContentType string
}

// ListAssetsCmd is the command for listing gallery assets.
type ListAssetsCmd struct {
	WorkspaceID string
	Page        int
	PerPage     int
}

// SearchAssetsCmd is the command for searching gallery assets.
type SearchAssetsCmd struct {
	WorkspaceID string
	Query       string
	Page        int
	PerPage     int
}

// UploadAssetCmd is the command for uploading a new gallery asset.
type UploadAssetCmd struct {
	TenantID    string
	WorkspaceID string
	UserID      string
	Filename    string
	ContentType string
	Data        []byte
}

// DeleteAssetCmd is the command for deleting a gallery asset.
type DeleteAssetCmd struct {
	WorkspaceID string
	Key         string
}

// GetAssetURLCmd is the command for resolving a gallery asset URL.
type GetAssetURLCmd struct {
	WorkspaceID string
	Key         string
}

// ServeAssetCmd is the command for resolving gallery asset bytes for HTTP streaming.
type ServeAssetCmd struct {
	WorkspaceID string
	Key         string
}

// GalleryUseCase defines the input port for gallery operations.
type GalleryUseCase interface {
	// ListAssets returns a paginated list of assets for the workspace.
	ListAssets(ctx context.Context, cmd ListAssetsCmd) (*AssetsPage, error)

	// SearchAssets returns assets matching the search query.
	SearchAssets(ctx context.Context, cmd SearchAssetsCmd) (*AssetsPage, error)

	// UploadAsset uploads a new image and registers it in the gallery.
	// Deduplicates by SHA-256: returns the existing asset if the content is identical.
	UploadAsset(ctx context.Context, cmd UploadAssetCmd) (*entity.GalleryAsset, error)

	// DeleteAsset removes an asset from storage and from the gallery registry.
	DeleteAsset(ctx context.Context, cmd DeleteAssetCmd) error

	// GetAssetURL returns a resolvable HTTP URL for the given asset key.
	GetAssetURL(ctx context.Context, cmd GetAssetURLCmd) (string, error)

	// ServeAsset resolves an owned asset and returns its payload for direct HTTP serving.
	ServeAsset(ctx context.Context, cmd ServeAssetCmd) (*AssetPayload, error)
}
