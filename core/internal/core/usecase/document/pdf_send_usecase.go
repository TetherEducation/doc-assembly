package document

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// SendPDFCommand sends an uploaded PDF for signature in one call.
type SendPDFCommand struct {
	WorkspaceID string
	// UserID is the caller's internal user ID, recorded as the creator and
	// publisher of the one-off template.
	UserID  string
	Title   string
	PDF     []byte
	Signers []PDFSigner
}

// PDFSigner is one signer of an uploaded PDF and the boxes they fill.
type PDFSigner struct {
	Name  string
	Email string
	// Order is the signing order, starting at 1 and unique per document.
	Order int
	// Fields are in PDF points with the origin at the bottom-left of the page;
	// RoleID is ignored and set from the signer.
	Fields []entity.PlacedField
}

// PDFSendUseCase is the panel's "Send a PDF" (setting signing.pdf_send).
type PDFSendUseCase interface {
	SendPDF(ctx context.Context, cmd SendPDFCommand) (*entity.DocumentWithRecipients, error)
}
