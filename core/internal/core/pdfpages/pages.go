// Package pdfpages reads page count and page sizes from a PDF.
//
// It reads the file with pdfcpu, so pages stored in object streams (DocuSign,
// LaTeX and most PDF 1.5+ writers) and encrypted files that open without a
// password are read, then walks the page tree itself, so only pages in the
// tree are counted, in page order. Each size follows the rule Documenso uses
// when it stamps fields: the CropBox, or the MediaBox when that is smaller,
// with width and height swapped for pages rotated 90 or 270 degrees.
// Placed-field percentages computed from these sizes then land where Documenso
// draws them. Like DocuSign, sizes describe the page as the viewer sees it; a
// page whose visible area is offset from (0, 0) is refused, because Documenso
// would stamp its fields shifted by the offset.
//
// Uploads are untrusted: reading is bounded in time and decoded bytes, and the
// walk ignores /Count, refuses cycles and caps depth, nodes and pages.
package pdfpages

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Size is one page's width and height in PDF points, as the page is shown.
type Size struct {
	Width  float64
	Height float64
}

const (
	// MaxPages is the largest page count accepted.
	MaxPages = 2000
	// maxTreeDepth and maxTreeNodes bound the page-tree walk.
	maxTreeDepth = 64
	maxTreeNodes = 10000
	// readTimeout bounds reading one file.
	readTimeout = 10 * time.Second
	// originTolerance is how far, in points, a visible area's corner may sit
	// from (0, 0) before the page is refused.
	originTolerance = 0.5
)

var (
	// ErrOffsetPage marks a page whose visible area does not start at (0, 0).
	ErrOffsetPage = errors.New("the visible area of the page does not start at its corner")
	errPassword   = errors.New("PDF is password protected")
)

func init() {
	// pdfcpu otherwise creates and reads a user config directory.
	api.DisableConfigDir()
}

// limits are far below pdfcpu's defaults: uploads are at most 12 MiB, and a
// contract needs nothing near these sizes.
func limits() model.ResourceLimits {
	l := model.DefaultResourceLimits()
	l.MaxStreamBytes = 16 << 20
	l.MaxDecodeBytes = 64 << 20
	l.MaxObjectCount = 500_000
	l.MaxObjectStreamCount = 100_000
	l.MaxXRefEntries = 500_000
	return l
}

// Parse returns one size per page, in page order.
func Parse(data []byte) ([]Size, error) {
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, fmt.Errorf("not a PDF")
	}
	c, cancel := context.WithTimeout(context.Background(), readTimeout)
	defer cancel()
	ctx, err := read(c, data)
	if err != nil {
		return nil, err
	}
	return walk(ctx)
}

func read(c context.Context, data []byte) (ctx *model.Context, err error) {
	defer func() {
		if p := recover(); p != nil {
			ctx, err = nil, fmt.Errorf("PDF could not be read: %v", p)
		}
	}()
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	conf.Limits = limits()
	ctx, err = pdfcpu.ReadWithContext(c, bytes.NewReader(data), conf)
	if errors.Is(err, pdfcpu.ErrWrongPassword) {
		return nil, errPassword
	}
	if err != nil {
		return nil, fmt.Errorf("PDF could not be read: %w", err)
	}
	return ctx, nil
}

// pageAttrs are the inheritable attributes that matter for page size.
type pageAttrs struct {
	mediaBox, cropBox, rotate types.Object
}

func (a pageAttrs) with(node types.Dict) pageAttrs {
	if v, ok := node.Find("MediaBox"); ok && v != nil {
		a.mediaBox = v
	}
	if v, ok := node.Find("CropBox"); ok && v != nil {
		a.cropBox = v
	}
	if v, ok := node.Find("Rotate"); ok && v != nil {
		a.rotate = v
	}
	return a
}

type treeNode struct {
	ref   types.Object
	attrs pageAttrs
	depth int
}

// walk reads the page tree depth first, in page order, passing inherited
// attributes down instead of following /Parent.
func walk(ctx *model.Context) (sizes []Size, err error) {
	defer func() {
		if p := recover(); p != nil {
			sizes, err = nil, fmt.Errorf("PDF could not be read: %v", p)
		}
	}()
	catalog, err := ctx.Catalog()
	if err != nil {
		return nil, fmt.Errorf("PDF has no catalog: %w", err)
	}
	pages, ok := catalog.Find("Pages")
	if !ok || pages == nil {
		return nil, fmt.Errorf("PDF has no page tree")
	}
	seen := map[int]bool{}
	stack := []treeNode{{ref: pages}}
	for nodes := 0; len(stack) > 0; nodes++ {
		if nodes >= maxTreeNodes {
			return nil, fmt.Errorf("PDF page tree is too large")
		}
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if ref, ok := node.ref.(types.IndirectRef); ok {
			if seen[ref.ObjectNumber.Value()] {
				return nil, fmt.Errorf("PDF page tree is damaged: it refers to the same node twice")
			}
			seen[ref.ObjectNumber.Value()] = true
		}
		dict, err := ctx.DereferenceDict(node.ref)
		if err != nil || dict == nil {
			return nil, fmt.Errorf("PDF page tree is damaged")
		}
		attrs := node.attrs.with(dict)
		kidsObj, hasKids := dict.Find("Kids")
		typ := ""
		if t := dict.Type(); t != nil {
			typ = *t
		}
		if typ == "Page" || (typ != "Pages" && !hasKids) {
			if len(sizes) == MaxPages {
				return nil, fmt.Errorf("PDF has more than %d pages", MaxPages)
			}
			size, err := pageSize(ctx, attrs)
			if err != nil {
				return nil, fmt.Errorf("page %d: %w", len(sizes)+1, err)
			}
			sizes = append(sizes, size)
			continue
		}
		kids, err := ctx.DereferenceArray(kidsObj)
		if err != nil || kids == nil {
			return nil, fmt.Errorf("PDF page tree node has no /Kids")
		}
		if node.depth+1 > maxTreeDepth {
			return nil, fmt.Errorf("PDF page tree is too deep")
		}
		if len(kids) > maxTreeNodes {
			return nil, fmt.Errorf("PDF page tree is too large")
		}
		// Push in reverse so the first kid is read first.
		for i := len(kids) - 1; i >= 0; i-- {
			stack = append(stack, treeNode{ref: kids[i], attrs: attrs, depth: node.depth + 1})
		}
	}
	if len(sizes) == 0 {
		return nil, fmt.Errorf("PDF has no pages")
	}
	return sizes, nil
}

func pageSize(ctx *model.Context, attrs pageAttrs) (Size, error) {
	media, ok := box(ctx, attrs.mediaBox)
	if !ok {
		return Size{}, fmt.Errorf("no readable MediaBox")
	}
	visible := media
	if crop, ok := box(ctx, attrs.cropBox); ok {
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
	rotate, err := rotation(ctx, attrs.rotate)
	if err != nil {
		return Size{}, err
	}
	if rotate == 90 || rotate == 270 {
		w, h = h, w
	}
	return Size{Width: w, Height: h}, nil
}

// rotation reads /Rotate as a whole multiple of 90, normalised to 0..270.
// It reads reals too: pdf-lib does, so "90.0" must not be taken as 0.
func rotation(ctx *model.Context, o types.Object) (int, error) {
	if o == nil {
		return 0, nil
	}
	deg, ok := number(ctx, o)
	if !ok {
		return 0, fmt.Errorf("unreadable rotation")
	}
	if math.Abs(deg) > 3600 || deg != math.Trunc(deg) || int(deg)%90 != 0 {
		return 0, fmt.Errorf("unsupported rotation %v", deg)
	}
	return ((int(deg) % 360) + 360) % 360, nil
}

func number(ctx *model.Context, o types.Object) (float64, bool) {
	v, err := ctx.Dereference(o)
	if err != nil {
		return 0, false
	}
	switch n := v.(type) {
	case types.Integer:
		return float64(n.Value()), true
	case types.Float:
		return n.Value(), true
	}
	return 0, false
}

type rect struct{ llx, lly, urx, ury float64 }

func (r rect) width() float64  { return r.urx - r.llx }
func (r rect) height() float64 { return r.ury - r.lly }

func box(ctx *model.Context, o types.Object) (rect, bool) {
	if o == nil {
		return rect{}, false
	}
	arr, err := ctx.DereferenceArray(o)
	if err != nil || len(arr) != 4 {
		return rect{}, false
	}
	var n [4]float64
	for i := range n {
		v, ok := number(ctx, arr[i])
		if !ok {
			return rect{}, false
		}
		n[i] = v
	}
	// The spec allows any two opposite corners.
	return rect{
		llx: math.Min(n[0], n[2]), lly: math.Min(n[1], n[3]),
		urx: math.Max(n[0], n[2]), ury: math.Max(n[1], n[3]),
	}, true
}
