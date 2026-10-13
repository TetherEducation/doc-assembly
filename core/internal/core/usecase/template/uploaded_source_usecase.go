package template

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// StoreUploadedPDFCommand is an uploaded PDF for one draft template version.
type StoreUploadedPDFCommand struct {
	TemplateID string
	VersionID  string
	PDF        []byte
}

// StoreUploadedPDFResult reports the pages the PDF contained.
type StoreUploadedPDFResult struct {
	PageCount int
	PageSizes []entity.PageSize
}

// SignerPlacement is one signer and the boxes that belong to them.
type SignerPlacement struct {
	Name   string
	Order  int
	Fields []entity.PlacedField
}

// ReplacePlacementsCommand replaces signer roles and field boxes on an uploaded version.
type ReplacePlacementsCommand struct {
	TemplateID string
	VersionID  string
	Signers    []SignerPlacement
}

// PlacedSigner is a signer role created for an uploaded PDF.
type PlacedSigner struct {
	RoleID string `json:"roleId"`
	Name   string `json:"name"`
	Order  int    `json:"order"`
	Anchor string `json:"anchor"`
}

// ReplacePlacementsResult is the roles an agent uses when creating the document.
type ReplacePlacementsResult struct {
	Signers []PlacedSigner `json:"signers"`
}

// UploadedSourceUseCase is the agent API for a one-off PDF template.
type UploadedSourceUseCase interface {
	StorePDF(ctx context.Context, cmd StoreUploadedPDFCommand) (*StoreUploadedPDFResult, error)
	ReplacePlacements(ctx context.Context, cmd ReplacePlacementsCommand) (*ReplacePlacementsResult, error)
}
