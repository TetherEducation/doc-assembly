package port

import (
	"context"
	"encoding/json"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
)

// RenderPreviewRequest contains the data needed to render a preview PDF.
type RenderPreviewRequest struct {
	// Document is the parsed portable document to render.
	Document *portabledoc.Document

	// Injectables contains the values to inject into the document.
	// Keys are variable IDs, values are the actual values.
	Injectables map[string]any

	// InjectableDefaults contains default values for injectables.
	// Keys are variable IDs, values are the default string values.
	// Used as fallback when Injectables doesn't contain a value.
	InjectableDefaults map[string]string

	// SignerRoleValues contains resolved values for signer roles.
	// Keys are role IDs, values contain name and email.
	SignerRoleValues map[string]SignerRoleValue

	// FieldResponses contains responses to interactive fields.
	// Keys are field IDs, values are the response JSON
	// (e.g., {"selectedOptionIds":["id1"]} or {"text":"value"}).
	FieldResponses map[string]json.RawMessage
}

// SignerRoleValue contains the resolved name and email for a signer role.
type SignerRoleValue struct {
	Name  string
	Email string
}

// RenderPreviewResult contains the result of rendering a preview PDF.
type RenderPreviewResult struct {
	// PDF contains the raw PDF bytes.
	PDF []byte

	// Filename is the suggested filename for the PDF.
	Filename string

	// PageCount is the number of pages in the generated PDF.
	PageCount int

	// SignatureFields contains position information for each signature field.
	SignatureFields []SignatureField
}

// SignatureField contains position information for a signature field in the PDF.
type SignatureField struct {
	// RoleID is the portable doc role ID associated with this signature.
	RoleID string

	// AnchorString is the anchor identifier (e.g., "__sig_rol_1__").
	AnchorString string

	// Page is the 1-indexed page number where the signature appears.
	Page int

	// PositionX is the default X position as a percentage (0-100) from left edge.
	PositionX float64

	// PositionY is the default Y position as a percentage (0-100) from top edge.
	PositionY float64

	// Width is the field width as a percentage (0-100) of page width.
	Width float64

	// Height is the field height as a percentage (0-100) of page height.
	Height float64

	// Raw PDF coordinates populated by anchor extraction (zero if not extracted).
	// These are in PDF standard units (points, bottom-left origin).
	PDFPointX  float64
	PDFPointY  float64
	PDFPageW   float64
	PDFPageH   float64
	PDFAnchorW float64 // anchor text width in points (for horizontal centering)
}

// PDFRenderer defines the interface for PDF rendering operations.
type PDFRenderer interface {
	// RenderPreview generates a preview PDF with injected values.
	// The document is rendered with all variables replaced by their provided values.
	// Conditional blocks are evaluated based on the injectable values.
	// Signature blocks include the anchorString for external signing platforms.
	RenderPreview(ctx context.Context, req *RenderPreviewRequest) (*RenderPreviewResult, error)

	// Close releases any resources held by the renderer.
	// This should be called when the renderer is no longer needed.
	Close() error
}
