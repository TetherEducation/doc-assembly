// Package pdfpages reads page count and page sizes from a PDF.
//
// It walks the page tree with dslipak/pdf, so pages stored in object streams
// (DocuSign, LaTeX and most PDF 1.5+ writers) are read, and only pages that are
// in the tree are counted. Each size follows the rule Documenso uses when it
// stamps fields: the CropBox, or the MediaBox when that is smaller, with width
// and height swapped for pages rotated 90 or 270 degrees. Placed-field
// percentages computed from these sizes then land where Documenso draws them.
// Like DocuSign, sizes describe the page as the viewer sees it; a page whose
// visible area is offset from (0, 0) is refused, because Documenso would stamp
// its fields shifted by the offset.
package pdfpages

import (
	"bytes"
	"errors"
	"fmt"
	"math"

	"github.com/dslipak/pdf"
)

// Size is one page's width and height in PDF points, as the page is shown.
type Size struct {
	Width  float64
	Height float64
}

// maxTreeDepth bounds the /Parent walk so a cyclic page tree cannot loop.
const maxTreeDepth = 64

// originTolerance is how far, in points, a visible area's corner may sit from
// (0, 0) before the page is refused.
const originTolerance = 0.5

// ErrOffsetPage marks a page whose visible area does not start at (0, 0).
var ErrOffsetPage = errors.New("the visible area of the page does not start at its corner")

// Parse returns one size per page, in page order.
func Parse(data []byte) ([]Size, error) {
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, fmt.Errorf("not a PDF")
	}
	r, err := open(data)
	if errors.Is(err, errPassword) {
		return nil, err
	}
	if err != nil {
		// Only a file the library cannot open falls back to the literal scan;
		// a page it opened and refused stays refused.
		if legacy, legacyErr := scanLiteral(data); legacyErr == nil {
			return legacy, nil
		}
		return nil, err
	}
	return readTree(r)
}

var errPassword = errors.New("PDF is password protected")

// dslipak/pdf panics on some malformed files instead of returning an error,
// so both steps recover.
func open(data []byte) (r *pdf.Reader, err error) {
	defer func() {
		if p := recover(); p != nil {
			r, err = nil, fmt.Errorf("PDF could not be read: %v", p)
		}
	}()
	r, err = pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if errors.Is(err, pdf.ErrInvalidPassword) {
		return nil, errPassword
	}
	if err != nil {
		return nil, fmt.Errorf("PDF could not be read: %w", err)
	}
	return r, nil
}

func readTree(r *pdf.Reader) (sizes []Size, err error) {
	defer func() {
		if p := recover(); p != nil {
			sizes, err = nil, fmt.Errorf("PDF could not be read: %v", p)
		}
	}()
	n := r.NumPage()
	if n == 0 {
		return nil, fmt.Errorf("PDF has no pages")
	}
	sizes = make([]Size, 0, n)
	for i := 1; i <= n; i++ {
		size, err := pageSize(r.Page(i).V)
		if err != nil {
			return nil, fmt.Errorf("page %d: %w", i, err)
		}
		sizes = append(sizes, size)
	}
	return sizes, nil
}

func pageSize(page pdf.Value) (Size, error) {
	media, ok := box(inherited(page, "MediaBox"))
	if !ok {
		return Size{}, fmt.Errorf("no readable MediaBox")
	}
	visible := media
	if crop, ok := box(inherited(page, "CropBox")); ok {
		if !(media.width() < crop.width() || media.height() < crop.height()) {
			visible = crop
		}
	}
	w, h := visible.width(), visible.height()
	if w <= 0 || h <= 0 {
		return Size{}, fmt.Errorf("empty page box")
	}
	// Documenso stamps fields from the user-space origin, not from the corner
	// of the visible area, so a page whose visible area is offset would get
	// every field shifted by that offset. Refuse it rather than misplace a
	// signature; re-saving or printing the PDF moves the corner to (0, 0).
	if math.Abs(visible.llx) > originTolerance || math.Abs(visible.lly) > originTolerance {
		return Size{}, fmt.Errorf("%w (it starts at %.0f, %.0f); print or re-save the PDF and upload it again",
			ErrOffsetPage, visible.llx, visible.lly)
	}
	rotate := ((inherited(page, "Rotate").Int64() % 360) + 360) % 360
	switch rotate {
	case 0, 180:
	case 90, 270:
		w, h = h, w
	default:
		return Size{}, fmt.Errorf("unsupported rotation %d", rotate)
	}
	return Size{Width: w, Height: h}, nil
}

// inherited looks a page attribute up on the page, then on its /Pages
// ancestors (MediaBox, CropBox and Rotate are inheritable).
func inherited(v pdf.Value, key string) pdf.Value {
	for depth := 0; depth < maxTreeDepth && !v.IsNull(); depth++ {
		if found := v.Key(key); !found.IsNull() {
			return found
		}
		v = v.Key("Parent")
	}
	return pdf.Value{}
}

type rect struct{ llx, lly, urx, ury float64 }

func (r rect) width() float64  { return r.urx - r.llx }
func (r rect) height() float64 { return r.ury - r.lly }

func box(v pdf.Value) (rect, bool) {
	if v.Kind() != pdf.Array || v.Len() != 4 {
		return rect{}, false
	}
	var n [4]float64
	for i := range n {
		item := v.Index(i)
		if item.Kind() != pdf.Integer && item.Kind() != pdf.Real {
			return rect{}, false
		}
		n[i] = item.Float64()
	}
	// The spec allows any two opposite corners.
	return rect{
		llx: math.Min(n[0], n[2]), lly: math.Min(n[1], n[3]),
		urx: math.Max(n[0], n[2]), ury: math.Max(n[1], n[3]),
	}, true
}
