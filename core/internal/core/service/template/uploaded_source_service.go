package template

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/pdfpages"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

const maxUploadedPDFBytes = 12 << 20

type versionLookup interface {
	FindByID(ctx context.Context, id string) (*entity.TemplateVersion, error)
}

type roleWriter interface {
	DeleteByVersionID(ctx context.Context, versionID string) error
	Create(ctx context.Context, role *entity.TemplateVersionSignerRole) (string, error)
}

// NewUploadedSourceService creates the uploaded-PDF use case.
func NewUploadedSourceService(
	versions versionLookup,
	roles roleWriter,
	sources port.UploadedSourceRepository,
	storage port.StorageAdapter,
) templateuc.UploadedSourceUseCase {
	return &UploadedSourceService{
		versions: versions,
		roles:    roles,
		sources:  sources,
		storage:  storage,
	}
}

// UploadedSourceService stores a PDF and the boxes an agent places on it.
type UploadedSourceService struct {
	versions versionLookup
	roles    roleWriter
	sources  port.UploadedSourceRepository
	storage  port.StorageAdapter
}

// StorePDF validates a PDF, stores the bytes, and records page sizes.
func (s *UploadedSourceService) StorePDF(ctx context.Context, cmd templateuc.StoreUploadedPDFCommand) (*templateuc.StoreUploadedPDFResult, error) {
	version, err := s.editableVersion(ctx, cmd.TemplateID, cmd.VersionID)
	if err != nil {
		return nil, err
	}
	if s.storage == nil {
		return nil, fmt.Errorf("storage is not configured")
	}
	if len(cmd.PDF) == 0 || len(cmd.PDF) > maxUploadedPDFBytes {
		return nil, entity.ErrInvalidPDF
	}
	parsed, err := pdfpages.Parse(cmd.PDF)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", entity.ErrInvalidPDF, err.Error())
	}
	sizes := make([]entity.PageSize, len(parsed))
	for i, page := range parsed {
		sizes[i] = entity.PageSize{Width: page.Width, Height: page.Height}
	}
	existing, err := s.sources.FindByVersionID(ctx, version.ID)
	if err != nil && !errors.Is(err, entity.ErrUploadedSourceNotFound) {
		return nil, err
	}
	if existing != nil {
		for _, field := range existing.Fields {
			if field.Page < 1 || field.Page > len(sizes) {
				return nil, fmt.Errorf("%w: existing field is on page %d", entity.ErrInvalidPlacedField, field.Page)
			}
		}
	}
	key := fmt.Sprintf("templates/%s/source.pdf", version.ID)
	if err := s.storage.Upload(ctx, &port.StorageUploadRequest{
		Key:         key,
		Data:        cmd.PDF,
		ContentType: "application/pdf",
		Environment: entity.EnvironmentProd,
	}); err != nil {
		return nil, fmt.Errorf("storing uploaded pdf: %w", err)
	}
	if err := s.sources.UpsertPDF(ctx, &entity.UploadedSource{
		VersionID: version.ID,
		ObjectKey: key,
		PageCount: len(sizes),
		PageSizes: sizes,
	}); err != nil {
		return nil, err
	}
	return &templateuc.StoreUploadedPDFResult{PageCount: len(sizes), PageSizes: sizes}, nil
}

// ReplacePlacements recreates signer roles and stores their field boxes.
func (s *UploadedSourceService) ReplacePlacements(ctx context.Context, cmd templateuc.ReplacePlacementsCommand) (*templateuc.ReplacePlacementsResult, error) {
	version, err := s.editableVersion(ctx, cmd.TemplateID, cmd.VersionID)
	if err != nil {
		return nil, err
	}
	source, err := s.sources.FindByVersionID(ctx, version.ID)
	if err != nil {
		return nil, err
	}
	if err := validatePlacements(cmd.Signers, source.PageSizes); err != nil {
		return nil, err
	}
	if err := s.roles.DeleteByVersionID(ctx, version.ID); err != nil {
		return nil, fmt.Errorf("clearing signer roles: %w", err)
	}
	result := &templateuc.ReplacePlacementsResult{Signers: make([]templateuc.PlacedSigner, 0, len(cmd.Signers))}
	fields := make([]entity.PlacedField, 0)
	usedAnchors := map[string]struct{}{}
	for _, signer := range cmd.Signers {
		anchor := uniqueAnchor(signer.Name, signer.Order, usedAnchors)
		role := entity.NewTemplateVersionSignerRole(version.ID, signer.Name, anchor, signer.Order)
		if err := role.Validate(); err != nil {
			return nil, err
		}
		roleID, err := s.roles.Create(ctx, role)
		if err != nil {
			return nil, fmt.Errorf("creating signer role: %w", err)
		}
		result.Signers = append(result.Signers, templateuc.PlacedSigner{
			RoleID: roleID,
			Name:   signer.Name,
			Order:  signer.Order,
			Anchor: anchor,
		})
		for _, field := range signer.Fields {
			fieldType, err := entity.NormalizeFieldType(field.Type)
			if err != nil {
				return nil, err
			}
			field.RoleID = roleID
			field.Type = fieldType
			fields = append(fields, field)
		}
	}
	if err := s.sources.ReplaceFields(ctx, version.ID, fields); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *UploadedSourceService) editableVersion(ctx context.Context, templateID, versionID string) (*entity.TemplateVersion, error) {
	version, err := s.versions.FindByID(ctx, versionID)
	if err != nil {
		return nil, err
	}
	if version.TemplateID != templateID {
		return nil, entity.ErrVersionDoesNotBelongToTemplate
	}
	if err := version.CanEdit(); err != nil {
		return nil, err
	}
	return version, nil
}

func validatePlacements(signers []templateuc.SignerPlacement, pages []entity.PageSize) error {
	if len(signers) == 0 {
		return fmt.Errorf("%w: at least one signer is required", entity.ErrInvalidPlacedField)
	}
	orders := map[int]struct{}{}
	for _, signer := range signers {
		name := strings.TrimSpace(signer.Name)
		if name == "" || signer.Order < 1 {
			return entity.ErrInvalidSignerRole
		}
		if _, ok := orders[signer.Order]; ok {
			return entity.ErrDuplicateSignerOrder
		}
		orders[signer.Order] = struct{}{}
		if len(signer.Fields) == 0 {
			return fmt.Errorf("%w: signer %s has no fields", entity.ErrInvalidPlacedField, name)
		}
		for _, field := range signer.Fields {
			if _, err := entity.NormalizeFieldType(field.Type); err != nil {
				return err
			}
			if field.Page < 1 || field.Page > len(pages) {
				return fmt.Errorf("%w: page %d is outside the PDF", entity.ErrInvalidPlacedField, field.Page)
			}
			page := pages[field.Page-1]
			if field.Width <= 0 || field.Height <= 0 || field.X < 0 || field.Y < 0 ||
				field.X+field.Width > page.Width+0.5 || field.Y+field.Height > page.Height+0.5 {
				return fmt.Errorf("%w: box is outside page %d", entity.ErrInvalidPlacedField, field.Page)
			}
		}
	}
	return nil
}

func uniqueAnchor(name string, order int, used map[string]struct{}) string {
	anchor := portabledoc.GenerateAnchorString(name)
	if _, ok := used[anchor]; ok {
		anchor = portabledoc.GenerateAnchorString(fmt.Sprintf("%s %d", name, order))
	}
	used[anchor] = struct{}{}
	return anchor
}
