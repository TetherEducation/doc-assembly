package document

import (
	"context"
	"errors"
	"fmt"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// findUploadedSource returns the uploaded PDF behind a template version, or nil
// for an authored version (and when uploads are not wired at all).
func findUploadedSource(ctx context.Context, sources port.UploadedSourceRepository, versionID string) (*entity.UploadedSource, error) {
	if sources == nil {
		return nil, nil
	}
	source, err := sources.FindByVersionID(ctx, versionID)
	if errors.Is(err, entity.ErrUploadedSourceNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("finding uploaded PDF: %w", err)
	}
	return source, nil
}

// loadUploadedPDF selects the immutable uploaded bytes after the caller has
// established document/token access. Only authored versions fall back to Typst:
// once a version is known to be uploaded, a storage failure is an error rather
// than a reason to render content the version does not have.
func loadUploadedPDF(ctx context.Context, sources port.UploadedSourceRepository, storage port.StorageAdapter, versionID string) ([]byte, bool, error) {
	source, err := findUploadedSource(ctx, sources, versionID)
	if err != nil || source == nil {
		return nil, false, err
	}
	if storage == nil {
		return nil, true, fmt.Errorf("uploaded PDF storage is not configured")
	}
	pdf, err := storage.Download(ctx, &port.StorageRequest{Key: source.ObjectKey, Environment: entity.EnvironmentProd})
	if err != nil {
		return nil, true, fmt.Errorf("loading uploaded PDF: %w", err)
	}
	if len(pdf) == 0 {
		return nil, true, fmt.Errorf("uploaded PDF is empty")
	}
	return pdf, true, nil
}
