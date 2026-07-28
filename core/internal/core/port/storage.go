package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// StorageUploadRequest contains the data needed to upload a file to storage.
type StorageUploadRequest struct {
	Key         string
	Data        []byte
	ContentType string
	Environment entity.Environment
}

// StorageRequest contains the data needed to access a file in storage.
type StorageRequest struct {
	Key         string
	Environment entity.Environment
}

// StorageAdapter defines the interface for object storage services.
// Implementations handle the specifics of each provider (S3, GCS, Azure Blob, etc.)
// while exposing a unified interface to the application.
type StorageAdapter interface {
	// Upload stores data with the given key and content type.
	Upload(ctx context.Context, req *StorageUploadRequest) error

	// Download retrieves data by key.
	Download(ctx context.Context, req *StorageRequest) ([]byte, error)

	// GetURL returns a URL for accessing the object (signed URL if applicable).
	GetURL(ctx context.Context, req *StorageRequest) (string, error)

	// Delete removes an object by key.
	Delete(ctx context.Context, req *StorageRequest) error

	// Exists checks if an object exists at the given key.
	Exists(ctx context.Context, req *StorageRequest) (bool, error)
}
