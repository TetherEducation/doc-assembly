package port

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// UploadedSourceRepository stores one uploaded PDF per template version.
type UploadedSourceRepository interface {
	// UpsertPDF stores the object location and page geometry.
	// Existing field boxes are kept.
	UpsertPDF(ctx context.Context, source *entity.UploadedSource) error

	// ReplaceFields replaces the field boxes for a version that already has a PDF.
	ReplaceFields(ctx context.Context, versionID string, fields []entity.PlacedField) error

	// FindByVersionID returns the uploaded source.
	// ErrUploadedSourceNotFound when the version is authored.
	FindByVersionID(ctx context.Context, versionID string) (*entity.UploadedSource, error)
}
