package gallery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	galleryuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/gallery"
)

const galleryKeyPrefix = "gallery"

// Service implements the GalleryUseCase input port.
type Service struct {
	adapter   port.StorageAdapter
	repo      port.GalleryRepository
	publicURL string
}

// New creates a new gallery service.
func New(adapter port.StorageAdapter, repo port.GalleryRepository, publicURL string) galleryuc.GalleryUseCase {
	return &Service{
		adapter:   adapter,
		repo:      repo,
		publicURL: strings.TrimRight(strings.TrimSpace(publicURL), "/"),
	}
}

// ListAssets returns a paginated list of workspace gallery assets.
func (s *Service) ListAssets(ctx context.Context, cmd galleryuc.ListAssetsCmd) (*galleryuc.AssetsPage, error) {
	page := normalizePage(cmd.Page)
	perPage := normalizePerPage(cmd.PerPage)

	assets, total, err := s.repo.List(ctx, cmd.WorkspaceID, page, perPage)
	if err != nil {
		return nil, fmt.Errorf("listing gallery assets: %w", err)
	}

	return &galleryuc.AssetsPage{
		Assets:  assets,
		Total:   total,
		Page:    page,
		PerPage: perPage,
	}, nil
}

// SearchAssets returns assets matching the search query.
func (s *Service) SearchAssets(ctx context.Context, cmd galleryuc.SearchAssetsCmd) (*galleryuc.AssetsPage, error) {
	query := strings.TrimSpace(cmd.Query)
	if query == "" {
		return nil, galleryuc.ErrQueryRequired
	}

	page := normalizePage(cmd.Page)
	perPage := normalizePerPage(cmd.PerPage)

	assets, total, err := s.repo.Search(ctx, cmd.WorkspaceID, query, page, perPage)
	if err != nil {
		return nil, fmt.Errorf("searching gallery assets: %w", err)
	}

	return &galleryuc.AssetsPage{
		Assets:  assets,
		Total:   total,
		Page:    page,
		PerPage: perPage,
	}, nil
}

// UploadAsset uploads a new image file and registers it in the gallery.
// Returns the existing asset if an identical file (same SHA-256) already exists.
func (s *Service) UploadAsset(ctx context.Context, cmd galleryuc.UploadAssetCmd) (*entity.GalleryAsset, error) {
	if err := validateUpload(cmd.ContentType, int64(len(cmd.Data))); err != nil {
		return nil, err
	}

	sum := sha256.Sum256(cmd.Data)
	digest := hex.EncodeToString(sum[:])

	existing, err := s.repo.FindBySHA256(ctx, cmd.WorkspaceID, digest)
	if err != nil {
		return nil, fmt.Errorf("checking duplicate gallery asset: %w", err)
	}
	if existing != nil {
		slog.InfoContext(ctx, "gallery asset deduplicated",
			slog.String("workspace_id", cmd.WorkspaceID),
			slog.String("key", existing.Key),
			slog.String("sha256", digest),
		)
		return existing, nil
	}

	key := buildStorageKey(cmd.WorkspaceID, digest, cmd.Filename)
	if err := s.adapter.Upload(ctx, &port.StorageUploadRequest{
		Key:         key,
		Data:        cmd.Data,
		ContentType: cmd.ContentType,
		Environment: entity.EnvironmentProd,
	}); err != nil {
		return nil, fmt.Errorf("uploading gallery asset: %w", err)
	}

	asset := &entity.GalleryAsset{
		TenantID:    cmd.TenantID,
		WorkspaceID: cmd.WorkspaceID,
		Key:         key,
		Filename:    cmd.Filename,
		ContentType: cmd.ContentType,
		Size:        int64(len(cmd.Data)),
		SHA256:      digest,
		CreatedBy:   cmd.UserID,
		CreatedAt:   time.Now().UTC(),
	}
	if err := s.repo.Save(ctx, asset); err != nil {
		return nil, fmt.Errorf("saving gallery asset metadata: %w", err)
	}

	slog.InfoContext(ctx, "gallery asset uploaded",
		slog.String("workspace_id", cmd.WorkspaceID),
		slog.String("key", asset.Key),
		slog.Int64("size", asset.Size),
	)

	return asset, nil
}

// DeleteAsset removes an asset from both storage and the gallery registry.
func (s *Service) DeleteAsset(ctx context.Context, cmd galleryuc.DeleteAssetCmd) error {
	asset, err := s.resolveOwnedAsset(ctx, cmd.WorkspaceID, cmd.Key)
	if err != nil {
		return err
	}

	if err := s.adapter.Delete(ctx, &port.StorageRequest{
		Key:         asset.Key,
		Environment: entity.EnvironmentProd,
	}); err != nil {
		return fmt.Errorf("deleting gallery asset from storage: %w", err)
	}

	if err := s.repo.Delete(ctx, asset.WorkspaceID, asset.Key); err != nil {
		return fmt.Errorf("deleting gallery asset record: %w", err)
	}

	slog.InfoContext(ctx, "gallery asset deleted",
		slog.String("workspace_id", asset.WorkspaceID),
		slog.String("key", asset.Key),
	)

	return nil
}

// GetAssetURL returns an HTTP-resolvable URL for the given asset key.
// For local storage (file:// URLs), it returns an API serve URL instead.
func (s *Service) GetAssetURL(ctx context.Context, cmd galleryuc.GetAssetURLCmd) (string, error) {
	asset, err := s.resolveOwnedAsset(ctx, cmd.WorkspaceID, cmd.Key)
	if err != nil {
		return "", err
	}

	rawURL, err := s.adapter.GetURL(ctx, &port.StorageRequest{
		Key:         asset.Key,
		Environment: entity.EnvironmentProd,
	})
	if err != nil {
		return "", fmt.Errorf("resolving gallery asset URL: %w", err)
	}

	if strings.HasPrefix(rawURL, "file://") {
		return s.buildServeURL(asset.Key), nil
	}

	return rawURL, nil
}

// ServeAsset streams an owned asset directly from storage.
func (s *Service) ServeAsset(ctx context.Context, cmd galleryuc.ServeAssetCmd) (*galleryuc.AssetPayload, error) {
	asset, err := s.resolveOwnedAsset(ctx, cmd.WorkspaceID, cmd.Key)
	if err != nil {
		return nil, err
	}

	data, err := s.adapter.Download(ctx, &port.StorageRequest{
		Key:         asset.Key,
		Environment: entity.EnvironmentProd,
	})
	if err != nil {
		return nil, fmt.Errorf("downloading gallery asset: %w", err)
	}

	contentType := strings.TrimSpace(asset.ContentType)
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	return &galleryuc.AssetPayload{
		Data:        data,
		ContentType: contentType,
	}, nil
}

func (s *Service) resolveOwnedAsset(ctx context.Context, workspaceID, key string) (*entity.GalleryAsset, error) {
	trimmedKey := strings.TrimSpace(key)
	if trimmedKey == "" {
		return nil, galleryuc.ErrAssetKeyRequired
	}

	asset, err := s.repo.FindByKey(ctx, workspaceID, trimmedKey)
	if err != nil {
		if errors.Is(err, entity.ErrGalleryAssetNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("finding gallery asset by workspace ownership: %w", err)
	}

	return asset, nil
}

func (s *Service) buildServeURL(key string) string {
	path := fmt.Sprintf("/api/v1/workspace/gallery/serve?key=%s", url.QueryEscape(key))
	if s.publicURL == "" {
		return path
	}
	return s.publicURL + path
}

func normalizePage(page int) int {
	if page < 1 {
		return galleryuc.DefaultPage
	}
	return page
}

func normalizePerPage(perPage int) int {
	if perPage < 1 || perPage > 100 {
		return galleryuc.DefaultPerPage
	}
	return perPage
}

func validateUpload(contentType string, size int64) error {
	if size <= 0 {
		return galleryuc.ErrUploadEmpty
	}
	if !strings.HasPrefix(contentType, "image/") {
		return entity.ErrGalleryInvalidContentType
	}
	if size > galleryuc.MaxUploadSize {
		return entity.ErrGalleryFileTooLarge
	}
	return nil
}

func buildStorageKey(workspaceID, digest, filename string) string {
	safeFilename := sanitizeFilename(filename)
	return fmt.Sprintf("%s/%s/%s-%s", galleryKeyPrefix, workspaceID, digest[:12], safeFilename)
}

func sanitizeFilename(name string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", "\x00", "")
	sanitized := strings.TrimSpace(replacer.Replace(name))
	if sanitized == "" {
		return "asset"
	}
	return sanitized
}
