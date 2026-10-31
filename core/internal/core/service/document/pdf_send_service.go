package document

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	catalogcuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/catalog"
	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
	templateuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/template"
)

// UploadedPDFDocumentTypeCode is the document type every "Send a PDF" document
// gets, created in the tenant on first use.
const UploadedPDFDocumentTypeCode = "UPLOADED_PDF"

const maxPDFSendTitleLength = 200

// The steps PDFSendService runs, each through the service the panel or the
// automation API would use for it.
type (
	pdfSendTemplates interface {
		CreateTemplate(ctx context.Context, cmd templateuc.CreateTemplateCommand) (*entity.Template, *entity.TemplateVersion, error)
		AssignDocumentType(ctx context.Context, cmd templateuc.AssignDocumentTypeCommand) (*templateuc.AssignDocumentTypeResult, error)
		DeleteTemplate(ctx context.Context, id string) error
	}
	pdfSendVersions interface {
		PublishVersion(ctx context.Context, id string, userID string) error
	}
	pdfSendSources interface {
		StorePDF(ctx context.Context, cmd templateuc.StoreUploadedPDFCommand) (*templateuc.StoreUploadedPDFResult, error)
		ReplacePlacements(ctx context.Context, cmd templateuc.ReplacePlacementsCommand) (*templateuc.ReplacePlacementsResult, error)
	}
	pdfSendDocumentTypes interface {
		FindByCode(ctx context.Context, tenantID, code string) (*entity.DocumentType, error)
	}
	pdfSendDocumentTypeCreator interface {
		CreateDocumentType(ctx context.Context, cmd catalogcuc.CreateDocumentTypeCommand) (*entity.DocumentType, error)
	}
	pdfSendWorkspaces interface {
		FindByID(ctx context.Context, id string) (*entity.Workspace, error)
	}
	pdfSendDocuments interface {
		CreateAndSendDocument(ctx context.Context, cmd documentuc.CreateDocumentCommand) (*entity.DocumentWithRecipients, error)
	}
)

// PDFSendService is the panel's "Send a PDF" (setting signing.pdf_send): one
// call takes a PDF, its signers in order and their boxes, and sends it.
//
// It runs, as the caller, the steps the automation API offers one by one:
//  1. a one-off template in the caller's workspace, with a process of its own,
//     so it never conflicts with another template over a document type;
//  2. the PDF and the signers' boxes on its draft version;
//  3. the tenant's UPLOADED_PDF document type, created on first use;
//  4. publish, then the document with one recipient per signer.
//
// With signing.direct_send on, the document goes to the provider at once (an
// uploaded PDF has no signer-fillable fields). If a step fails before the
// document exists, the template is deleted again, best effort.
type PDFSendService struct {
	templates     pdfSendTemplates
	versions      pdfSendVersions
	sources       pdfSendSources
	documentTypes pdfSendDocumentTypes
	typeCreator   pdfSendDocumentTypeCreator
	workspaces    pdfSendWorkspaces
	documents     pdfSendDocuments
}

// NewPDFSendService builds the "Send a PDF" service.
func NewPDFSendService(
	templates pdfSendTemplates,
	versions pdfSendVersions,
	sources pdfSendSources,
	documentTypes pdfSendDocumentTypes,
	typeCreator pdfSendDocumentTypeCreator,
	workspaces pdfSendWorkspaces,
	documents pdfSendDocuments,
) *PDFSendService {
	return &PDFSendService{
		templates:     templates,
		versions:      versions,
		sources:       sources,
		documentTypes: documentTypes,
		typeCreator:   typeCreator,
		workspaces:    workspaces,
		documents:     documents,
	}
}

var _ documentuc.PDFSendUseCase = (*PDFSendService)(nil)

// SendPDF creates and sends a document from an uploaded PDF.
func (s *PDFSendService) SendPDF(ctx context.Context, cmd documentuc.SendPDFCommand) (*entity.DocumentWithRecipients, error) {
	title, err := validateSendPDF(cmd)
	if err != nil {
		return nil, err
	}

	workspace, err := s.workspaces.FindByID(ctx, cmd.WorkspaceID)
	if err != nil {
		return nil, fmt.Errorf("finding workspace: %w", err)
	}
	if workspace.TenantID == nil || *workspace.TenantID == "" {
		return nil, entity.ErrInvalidWorkspaceType
	}
	documentTypeID, err := s.uploadedPDFDocumentType(ctx, *workspace.TenantID)
	if err != nil {
		return nil, err
	}

	oneOff := uuid.NewString()
	template, version, err := s.templates.CreateTemplate(ctx, templateuc.CreateTemplateCommand{
		WorkspaceID: cmd.WorkspaceID,
		Title:       fmt.Sprintf("%s · PDF %s", title, oneOff[:8]),
		Process:     "pdf-send-" + oneOff,
		ProcessType: string(entity.ProcessTypeID),
		CreatedBy:   cmd.UserID,
	})
	if err != nil {
		return nil, fmt.Errorf("creating the template: %w", err)
	}

	doc, err := s.sendOnTemplate(ctx, cmd, title, template.ID, version.ID, documentTypeID)
	if err != nil {
		if delErr := s.templates.DeleteTemplate(ctx, template.ID); delErr != nil {
			slog.WarnContext(ctx, "send a PDF: could not delete the one-off template after a failure",
				slog.String("template_id", template.ID),
				slog.String("error", delErr.Error()),
			)
		}
		return nil, err
	}
	return doc, nil
}

func (s *PDFSendService) sendOnTemplate(
	ctx context.Context,
	cmd documentuc.SendPDFCommand,
	title, templateID, versionID, documentTypeID string,
) (*entity.DocumentWithRecipients, error) {
	if _, err := s.sources.StorePDF(ctx, templateuc.StoreUploadedPDFCommand{
		TemplateID: templateID,
		VersionID:  versionID,
		PDF:        cmd.PDF,
	}); err != nil {
		return nil, err
	}

	placements := make([]templateuc.SignerPlacement, len(cmd.Signers))
	for i, signer := range cmd.Signers {
		placements[i] = templateuc.SignerPlacement{
			Name:   strings.TrimSpace(signer.Name),
			Order:  signer.Order,
			Fields: signer.Fields,
		}
	}
	placed, err := s.sources.ReplacePlacements(ctx, templateuc.ReplacePlacementsCommand{
		TemplateID: templateID,
		VersionID:  versionID,
		Signers:    placements,
	})
	if err != nil {
		return nil, err
	}
	if len(placed.Signers) != len(cmd.Signers) {
		return nil, fmt.Errorf("placing signers: got %d roles for %d signers", len(placed.Signers), len(cmd.Signers))
	}

	assigned, err := s.templates.AssignDocumentType(ctx, templateuc.AssignDocumentTypeCommand{
		TemplateID:     templateID,
		WorkspaceID:    cmd.WorkspaceID,
		DocumentTypeID: &documentTypeID,
	})
	if err != nil {
		return nil, fmt.Errorf("assigning the document type: %w", err)
	}
	if assigned.Conflict != nil {
		// Unreachable while each one-off template has its own process.
		return nil, fmt.Errorf("assigning the document type: %w", entity.ErrDocumentTypeAlreadyAssigned)
	}

	if err := s.versions.PublishVersion(ctx, versionID, cmd.UserID); err != nil {
		return nil, fmt.Errorf("publishing the template: %w", err)
	}

	recipients := make([]documentuc.DocumentRecipientCommand, len(cmd.Signers))
	for i, signer := range cmd.Signers {
		recipients[i] = documentuc.DocumentRecipientCommand{
			RoleID: placed.Signers[i].RoleID,
			Name:   strings.TrimSpace(signer.Name),
			Email:  strings.TrimSpace(signer.Email),
		}
	}
	return s.documents.CreateAndSendDocument(ctx, documentuc.CreateDocumentCommand{
		WorkspaceID:       cmd.WorkspaceID,
		TemplateVersionID: versionID,
		Title:             title,
		Recipients:        recipients,
		SigningEmail:      cmd.SigningEmail,
	})
}

// uploadedPDFDocumentType returns the tenant's UPLOADED_PDF type, creating it
// the first time. A concurrent first send may create it in between (caught by
// the service's existence check or by the unique index on the code), so a
// failed create reads it back before giving up.
func (s *PDFSendService) uploadedPDFDocumentType(ctx context.Context, tenantID string) (string, error) {
	existing, err := s.documentTypes.FindByCode(ctx, tenantID, UploadedPDFDocumentTypeCode)
	if err == nil && existing != nil {
		return existing.ID, nil
	}
	if err != nil && !errors.Is(err, entity.ErrDocumentTypeNotFound) {
		return "", fmt.Errorf("finding the uploaded PDF document type: %w", err)
	}

	created, err := s.typeCreator.CreateDocumentType(ctx, catalogcuc.CreateDocumentTypeCommand{
		TenantID: tenantID,
		Code:     UploadedPDFDocumentTypeCode,
		Name:     entity.I18nText{"en": "Uploaded PDF", "es": "PDF subido"},
		Description: entity.I18nText{
			"en": "Documents sent from an uploaded PDF",
			"es": "Documentos enviados desde un PDF subido",
		},
	})
	if err != nil {
		if again, findErr := s.documentTypes.FindByCode(ctx, tenantID, UploadedPDFDocumentTypeCode); findErr == nil && again != nil {
			return again.ID, nil
		}
		return "", fmt.Errorf("creating the uploaded PDF document type: %w", err)
	}
	return created.ID, nil
}

// validateSendPDF checks what the steps would only reject after the template
// exists: the title and each signer's name and email, by the same rules as
// document recipients. Box positions, orders and types are checked when they
// are placed, before anything is published.
func validateSendPDF(cmd documentuc.SendPDFCommand) (string, error) {
	title := strings.TrimSpace(cmd.Title)
	if title == "" {
		return "", fmt.Errorf("%w: a title is required", entity.ErrInvalidPlacedField)
	}
	if len([]rune(title)) > maxPDFSendTitleLength {
		return "", fmt.Errorf("%w: the title is longer than %d characters", entity.ErrInvalidPlacedField, maxPDFSendTitleLength)
	}
	if len(cmd.PDF) == 0 {
		return "", fmt.Errorf("%w: no PDF", entity.ErrInvalidPDF)
	}
	// Checked before the one-off template is created, so a bad email leaves nothing behind.
	if cmd.SigningEmail != nil {
		if err := cmd.SigningEmail.Validate(); err != nil {
			return "", err
		}
	}
	if len(cmd.Signers) == 0 {
		return "", fmt.Errorf("%w: at least one signer is required", entity.ErrInvalidPlacedField)
	}
	for _, signer := range cmd.Signers {
		recipient := entity.NewDocumentRecipient("pending", "pending", strings.TrimSpace(signer.Name), strings.TrimSpace(signer.Email))
		if err := recipient.Validate(); err != nil {
			return "", fmt.Errorf("%w: signer %q: %w", entity.ErrInvalidPlacedField, signer.Name, err)
		}
	}
	return title, nil
}
