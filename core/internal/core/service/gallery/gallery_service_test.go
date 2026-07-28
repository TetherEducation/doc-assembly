package gallery

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	galleryuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/gallery"
)

type stubStorageAdapter struct {
	uploadCalls   int
	deleteCalls   int
	getURLCalls   int
	downloadCalls int
	uploaded      *port.StorageUploadRequest
	deleted       *port.StorageRequest
	requestedURL  *port.StorageRequest
	downloaded    *port.StorageRequest
	url           string
	data          []byte
	uploadErr     error
	deleteErr     error
	getURLErr     error
	downloadErr   error
}

func (s *stubStorageAdapter) Upload(_ context.Context, req *port.StorageUploadRequest) error {
	s.uploadCalls++
	s.uploaded = req
	return s.uploadErr
}

func (s *stubStorageAdapter) Download(_ context.Context, req *port.StorageRequest) ([]byte, error) {
	s.downloadCalls++
	s.downloaded = req
	if s.downloadErr != nil {
		return nil, s.downloadErr
	}
	return s.data, nil
}

func (s *stubStorageAdapter) GetURL(_ context.Context, req *port.StorageRequest) (string, error) {
	s.getURLCalls++
	s.requestedURL = req
	if s.getURLErr != nil {
		return "", s.getURLErr
	}
	return s.url, nil
}

func (s *stubStorageAdapter) Delete(_ context.Context, req *port.StorageRequest) error {
	s.deleteCalls++
	s.deleted = req
	return s.deleteErr
}

func (s *stubStorageAdapter) Exists(_ context.Context, _ *port.StorageRequest) (bool, error) {
	return false, nil
}

type stubGalleryRepo struct {
	listAssets     []*entity.GalleryAsset
	listTotal      int
	searchAssets   []*entity.GalleryAsset
	searchTotal    int
	findBySHAAsset *entity.GalleryAsset
	findByKeyAsset *entity.GalleryAsset
	listErr        error
	searchErr      error
	findBySHAErr   error
	findByKeyErr   error
	saveErr        error
	deleteErr      error
	saved          *entity.GalleryAsset
	deletedKey     string
	deletedWS      string
	findBySHAWS    string
	findBySHAHash  string
	findByKeyWS    string
	findByKeyKey   string
}

func (r *stubGalleryRepo) Save(_ context.Context, asset *entity.GalleryAsset) error {
	r.saved = asset
	return r.saveErr
}

func (r *stubGalleryRepo) FindBySHA256(_ context.Context, workspaceID, sha256 string) (*entity.GalleryAsset, error) {
	r.findBySHAWS = workspaceID
	r.findBySHAHash = sha256
	return r.findBySHAAsset, r.findBySHAErr
}

func (r *stubGalleryRepo) FindByKey(_ context.Context, workspaceID, key string) (*entity.GalleryAsset, error) {
	r.findByKeyWS = workspaceID
	r.findByKeyKey = key
	return r.findByKeyAsset, r.findByKeyErr
}

func (r *stubGalleryRepo) List(_ context.Context, _ string, page, perPage int) ([]*entity.GalleryAsset, int, error) {
	if r.listErr != nil {
		return nil, 0, r.listErr
	}
	if page == 0 || perPage == 0 {
		return nil, 0, errors.New("unexpected zero pagination")
	}
	return r.listAssets, r.listTotal, nil
}

func (r *stubGalleryRepo) Search(_ context.Context, _ string, _ string, page, perPage int) ([]*entity.GalleryAsset, int, error) {
	if r.searchErr != nil {
		return nil, 0, r.searchErr
	}
	if page == 0 || perPage == 0 {
		return nil, 0, errors.New("unexpected zero pagination")
	}
	return r.searchAssets, r.searchTotal, nil
}

func (r *stubGalleryRepo) Delete(_ context.Context, workspaceID, key string) error {
	r.deletedWS = workspaceID
	r.deletedKey = key
	return r.deleteErr
}

func TestServiceListAssetsNormalizesPagination(t *testing.T) {
	repo := &stubGalleryRepo{listAssets: []*entity.GalleryAsset{}, listTotal: 5}
	svc := &Service{repo: repo}

	page, err := svc.ListAssets(context.Background(), galleryuc.ListAssetsCmd{
		WorkspaceID: "ws-1",
		Page:        0,
		PerPage:     999,
	})
	if err != nil {
		t.Fatalf("ListAssets returned error: %v", err)
	}
	if page.Page != galleryuc.DefaultPage {
		t.Fatalf("expected default page %d, got %d", galleryuc.DefaultPage, page.Page)
	}
	if page.PerPage != galleryuc.DefaultPerPage {
		t.Fatalf("expected default perPage %d, got %d", galleryuc.DefaultPerPage, page.PerPage)
	}
}

func TestServiceSearchAssetsValidatesQuery(t *testing.T) {
	svc := &Service{repo: &stubGalleryRepo{}}

	_, err := svc.SearchAssets(context.Background(), galleryuc.SearchAssetsCmd{WorkspaceID: "ws-1", Query: "   "})
	if !errors.Is(err, galleryuc.ErrQueryRequired) {
		t.Fatalf("expected ErrQueryRequired, got %v", err)
	}
}

func TestServiceUploadAssetValidatesMetadata(t *testing.T) {
	svc := &Service{}

	_, err := svc.UploadAsset(context.Background(), galleryuc.UploadAssetCmd{
		WorkspaceID: "ws-1",
		ContentType: "application/pdf",
		Data:        []byte("abc"),
	})
	if !errors.Is(err, entity.ErrGalleryInvalidContentType) {
		t.Fatalf("expected invalid content type error, got %v", err)
	}

	_, err = svc.UploadAsset(context.Background(), galleryuc.UploadAssetCmd{
		WorkspaceID: "ws-1",
		ContentType: "image/png",
		Data:        make([]byte, galleryuc.MaxUploadSize+1),
	})
	if !errors.Is(err, entity.ErrGalleryFileTooLarge) {
		t.Fatalf("expected file too large error, got %v", err)
	}
}

func TestServiceUploadAssetDeduplicatesByHash(t *testing.T) {
	existing := &entity.GalleryAsset{ID: "asset-1", Key: "gallery/ws-1/existing.png"}
	repo := &stubGalleryRepo{findBySHAAsset: existing}
	storage := &stubStorageAdapter{}
	svc := &Service{repo: repo, adapter: storage}

	asset, err := svc.UploadAsset(context.Background(), galleryuc.UploadAssetCmd{
		WorkspaceID: "ws-1",
		ContentType: "image/png",
		Filename:    "logo.png",
		Data:        []byte("same-image"),
	})
	if err != nil {
		t.Fatalf("UploadAsset returned error: %v", err)
	}
	if asset != existing {
		t.Fatalf("expected existing asset to be returned")
	}
	if storage.uploadCalls != 0 {
		t.Fatalf("expected no storage upload on duplicate, got %d", storage.uploadCalls)
	}
}

func TestServiceAssetOwnershipCheckedBeforeStorageOperations(t *testing.T) {
	repo := &stubGalleryRepo{findByKeyErr: entity.ErrGalleryAssetNotFound}
	storage := &stubStorageAdapter{}
	svc := &Service{repo: repo, adapter: storage}
	ctx := context.Background()

	_, err := svc.GetAssetURL(ctx, galleryuc.GetAssetURLCmd{WorkspaceID: "ws-2", Key: "gallery/ws-1/file.png"})
	if !errors.Is(err, entity.ErrGalleryAssetNotFound) {
		t.Fatalf("expected not found from GetAssetURL, got %v", err)
	}
	if storage.getURLCalls != 0 {
		t.Fatalf("expected GetURL not to touch storage without ownership, got %d calls", storage.getURLCalls)
	}

	_, err = svc.ServeAsset(ctx, galleryuc.ServeAssetCmd{WorkspaceID: "ws-2", Key: "gallery/ws-1/file.png"})
	if !errors.Is(err, entity.ErrGalleryAssetNotFound) {
		t.Fatalf("expected not found from ServeAsset, got %v", err)
	}
	if storage.downloadCalls != 0 {
		t.Fatalf("expected ServeAsset not to touch storage without ownership, got %d calls", storage.downloadCalls)
	}

	err = svc.DeleteAsset(ctx, galleryuc.DeleteAssetCmd{WorkspaceID: "ws-2", Key: "gallery/ws-1/file.png"})
	if !errors.Is(err, entity.ErrGalleryAssetNotFound) {
		t.Fatalf("expected not found from DeleteAsset, got %v", err)
	}
	if storage.deleteCalls != 0 {
		t.Fatalf("expected DeleteAsset not to touch storage without ownership, got %d calls", storage.deleteCalls)
	}
}

func TestServiceKeyValidationForOwnedOperations(t *testing.T) {
	svc := &Service{repo: &stubGalleryRepo{}, adapter: &stubStorageAdapter{}}
	ctx := context.Background()

	_, err := svc.GetAssetURL(ctx, galleryuc.GetAssetURLCmd{WorkspaceID: "ws-1"})
	if !errors.Is(err, galleryuc.ErrAssetKeyRequired) {
		t.Fatalf("expected ErrAssetKeyRequired from GetAssetURL, got %v", err)
	}

	_, err = svc.ServeAsset(ctx, galleryuc.ServeAssetCmd{WorkspaceID: "ws-1"})
	if !errors.Is(err, galleryuc.ErrAssetKeyRequired) {
		t.Fatalf("expected ErrAssetKeyRequired from ServeAsset, got %v", err)
	}

	err = svc.DeleteAsset(ctx, galleryuc.DeleteAssetCmd{WorkspaceID: "ws-1"})
	if !errors.Is(err, galleryuc.ErrAssetKeyRequired) {
		t.Fatalf("expected ErrAssetKeyRequired from DeleteAsset, got %v", err)
	}
}

func TestServiceGetAssetURLReturnsServeURLForLocalStorage(t *testing.T) {
	asset := &entity.GalleryAsset{
		ID:          "asset-1",
		WorkspaceID: "ws-1",
		Key:         "gallery/ws-1/file.png",
		ContentType: "image/png",
		CreatedAt:   time.Now().UTC(),
	}
	repo := &stubGalleryRepo{findByKeyAsset: asset}
	storage := &stubStorageAdapter{url: "file:///tmp/gallery/file.png", data: []byte("pngdata")}
	svc := &Service{repo: repo, adapter: storage, publicURL: "https://app.example.com"}

	resolved, err := svc.GetAssetURL(context.Background(), galleryuc.GetAssetURLCmd{
		WorkspaceID: "ws-1",
		Key:         asset.Key,
	})
	if err != nil {
		t.Fatalf("GetAssetURL returned error: %v", err)
	}
	want := "https://app.example.com/api/v1/workspace/gallery/serve?key=gallery%2Fws-1%2Ffile.png"
	if resolved != want {
		t.Fatalf("expected %q, got %q", want, resolved)
	}
}
