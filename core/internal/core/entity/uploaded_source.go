package entity

import (
	"fmt"
	"strings"
)

// PageSize is one PDF page in points.
type PageSize struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// PlacedField is a box on an uploaded PDF.
// X and Y are the bottom-left corner in PDF points.
type PlacedField struct {
	RoleID string  `json:"roleId"`
	Type   string  `json:"type"`
	Page   int     `json:"page"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// UploadedSource is the stored PDF and its field boxes for one template version.
type UploadedSource struct {
	VersionID string
	ObjectKey string
	PageCount int
	PageSizes []PageSize
	Fields    []PlacedField
}

// NormalizeFieldType accepts the field types Documenso can place.
func NormalizeFieldType(raw string) (string, error) {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "SIGNATURE", "DATE", "TEXT", "INITIALS":
		return strings.ToUpper(strings.TrimSpace(raw)), nil
	default:
		return "", fmt.Errorf("%w: %q", ErrInvalidPlacedField, raw)
	}
}
