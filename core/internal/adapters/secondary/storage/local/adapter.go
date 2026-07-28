package local

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// Adapter implements port.StorageAdapter using the local filesystem.
type Adapter struct {
	baseDir string
}

// New creates a new local filesystem storage adapter.
func New(baseDir string) (port.StorageAdapter, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("local storage: base directory is required")
	}

	if err := os.MkdirAll(baseDir, 0o750); err != nil {
		return nil, fmt.Errorf("local storage: creating base directory: %w", err)
	}

	return &Adapter{baseDir: baseDir}, nil
}

// Upload stores data with the given key and content type.
func (a *Adapter) Upload(_ context.Context, req *port.StorageUploadRequest) error {
	fullPath := filepath.Join(a.baseDir, req.Key)

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("local storage: creating directories: %w", err)
	}

	if err := os.WriteFile(fullPath, req.Data, 0o600); err != nil {
		return fmt.Errorf("local storage: writing file: %w", err)
	}

	return nil
}

// Download retrieves data by key.
func (a *Adapter) Download(_ context.Context, req *port.StorageRequest) ([]byte, error) {
	fullPath := filepath.Join(a.baseDir, req.Key)

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("local storage: reading file: %w", err)
	}

	return data, nil
}

// GetURL returns a file:// URL for accessing the object.
func (a *Adapter) GetURL(_ context.Context, req *port.StorageRequest) (string, error) {
	fullPath := filepath.Join(a.baseDir, req.Key)

	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("local storage: resolving path: %w", err)
	}

	return "file://" + absPath, nil
}

// Delete removes an object by key.
func (a *Adapter) Delete(_ context.Context, req *port.StorageRequest) error {
	fullPath := filepath.Join(a.baseDir, req.Key)

	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("local storage: deleting file: %w", err)
	}

	return nil
}

// Exists checks if an object exists at the given key.
func (a *Adapter) Exists(_ context.Context, req *port.StorageRequest) (bool, error) {
	fullPath := filepath.Join(a.baseDir, req.Key)

	_, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("local storage: checking file: %w", err)
	}

	return true, nil
}

// Verify Adapter implements StorageAdapter.
var _ port.StorageAdapter = (*Adapter)(nil)
