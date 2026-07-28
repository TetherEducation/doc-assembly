package pdfrenderer

import (
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// TypstConverter defines the interface for converting portable document nodes to Typst markup.
type TypstConverter interface {
	// ConvertNodes converts a slice of portable document nodes to Typst markup.
	// Returns the Typst source string and any signature fields found during conversion.
	ConvertNodes(nodes []portabledoc.Node) (string, []port.SignatureField)

	// GetCurrentPage returns the current page number (1-indexed).
	// This accounts for page breaks encountered during conversion.
	GetCurrentPage() int

	// RemoteImages returns a map of remote image URLs to their placeholder filenames
	// used in the generated Typst source. These images need to be resolved (downloaded
	// or served from cache) before the Typst source can be compiled.
	RemoteImages() map[string]string

	// SetContentWidthPx sets the page content area width in pixels.
	// Used for computing proportional table column widths.
	SetContentWidthPx(width float64)

	// SetPageWidthPx sets the full page width in pixels (including margins).
	// Used for computing signature field width as a percentage of full page.
	SetPageWidthPx(width float64)

	// RegisterRemoteImage registers a URL or data URI for deferred download and
	// returns the local filename to reference in the Typst source.
	RegisterRemoteImage(url string) string

	// ResolveImageSource resolves the final image source after injectable substitution.
	ResolveImageSource(attrs map[string]any) string
}
