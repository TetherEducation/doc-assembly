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
// would stamp its fields shifted by the offset. Anything Documenso's pdf-lib
// would read differently (a node without the right /Type, a /Parent that does
// not point back, a /Rotate other than 0, 90, 180 or 270) is refused too.
//
// Uploads are untrusted: reading and walking share a timeout, decoded object
// streams are bounded in size and number, and the walk ignores /Count,
// refuses cycles and caps depth, nodes and pages.
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
	// pdfcpu decodes an object stream the first time an object in it is
	// dereferenced and keeps the result, so memory is bounded by
	// maxDecodeBytes per stream times maxObjectStreams streams (64 MiB kept,
	// about 200 MiB allocated at worst). Real page-tree object streams decode
	// to tens of KiB.
	maxDecodeBytes   = 1 << 20
	maxObjectStreams = 64
	// timeout bounds reading and walking one file together.
	timeout = 10 * time.Second
	// originTolerance is how far, in points, a visible area's corner may sit
	// from (0, 0) before the page is refused.
	originTolerance = 0.5
)

var (
	// ErrOffsetPage marks a page whose visible area does not start at (0, 0).
	ErrOffsetPage = errors.New("the visible area of the page does not start at its corner")
	errPassword   = errors.New("PDF is password protected")
	errDamaged    = errors.New("PDF page tree is damaged")
	// errLimit stops the walk: the timeout passed or the budget ran out.
	errLimit = errors.New("PDF is too large or slow to read safely")
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
	l.MaxDecodeBytes = maxDecodeBytes
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
	c, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	ctx, err := read(c, data)
	if err != nil {
		return nil, err
	}
	w := &walker{c: c, ctx: ctx, streams: map[int]bool{}}
	return w.walk()
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

// walker dereferences objects for the walk, checking the timeout and the
// object-stream budget before each one.
type walker struct {
	c       context.Context
	ctx     *model.Context
	streams map[int]bool
}

func (w *walker) touch(o types.Object) error {
	if err := w.c.Err(); err != nil {
		return fmt.Errorf("%w: %v", errLimit, err)
	}
	ref, ok := o.(types.IndirectRef)
	if !ok {
		return nil
	}
	entry, found := w.ctx.FindTableEntryForIndRef(&ref)
	// pdfcpu clears Compressed after reading but keeps ObjectStream.
	if !found || entry == nil || entry.ObjectStream == nil {
		return nil
	}
	if !w.streams[*entry.ObjectStream] {
		if len(w.streams) == maxObjectStreams {
			return fmt.Errorf("%w: its pages span more than %d object streams", errLimit, maxObjectStreams)
		}
		w.streams[*entry.ObjectStream] = true
	}
	return nil
}

func (w *walker) dict(o types.Object) (types.Dict, error) {
	if err := w.touch(o); err != nil {
		return nil, err
	}
	return w.ctx.DereferenceDict(o)
}

func (w *walker) array(o types.Object) (types.Array, error) {
	if err := w.touch(o); err != nil {
		return nil, err
	}
	return w.ctx.DereferenceArray(o)
}

func (w *walker) number(o types.Object) (float64, error) {
	if err := w.touch(o); err != nil {
		return 0, err
	}
	v, err := w.ctx.Dereference(o)
	if err != nil {
		return 0, err
	}
	switch n := v.(type) {
	case types.Integer:
		return float64(n.Value()), nil
	case types.Float:
		return n.Value(), nil
	}
	return 0, fmt.Errorf("not a number")
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
	ref    types.IndirectRef
	parent *types.IndirectRef // nil for the root
	attrs  pageAttrs
	depth  int
}

// walk reads the page tree depth first, in page order, passing inherited
// attributes down. pdf-lib (Documenso) resolves them through /Parent, so each
// node's /Parent must be the node that listed it.
func (w *walker) walk() (sizes []Size, err error) {
	defer func() {
		if p := recover(); p != nil {
			sizes, err = nil, fmt.Errorf("PDF could not be read: %v", p)
		}
	}()
	catalog, err := w.ctx.Catalog()
	if err != nil {
		return nil, fmt.Errorf("PDF has no catalog: %w", err)
	}
	pages, ok := catalog.Find("Pages")
	root, isRef := pages.(types.IndirectRef)
	if !ok || !isRef {
		return nil, fmt.Errorf("PDF has no page tree")
	}
	seen := map[int]bool{}
	stack := []treeNode{{ref: root}}
	for nodes := 0; len(stack) > 0; nodes++ {
		if nodes >= maxTreeNodes {
			return nil, fmt.Errorf("PDF page tree is too large")
		}
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		nr := node.ref.ObjectNumber.Value()
		if seen[nr] {
			return nil, fmt.Errorf("%w: it lists the same node twice", errDamaged)
		}
		seen[nr] = true
		dict, err := w.dict(node.ref)
		if err != nil {
			return nil, err
		}
		if dict == nil {
			return nil, errDamaged
		}
		if node.parent != nil {
			parent, ok := dict.Find("Parent")
			ref, isRef := parent.(types.IndirectRef)
			if !ok || !isRef || ref.ObjectNumber != node.parent.ObjectNumber {
				return nil, fmt.Errorf("%w: a node's /Parent does not point back", errDamaged)
			}
		}
		attrs := node.attrs.with(dict)
		typ := ""
		if t := dict.Type(); t != nil {
			typ = *t
		}
		switch typ {
		case "Page":
			if len(sizes) == MaxPages {
				return nil, fmt.Errorf("PDF has more than %d pages", MaxPages)
			}
			size, err := w.pageSize(attrs)
			if err != nil {
				return nil, fmt.Errorf("page %d: %w", len(sizes)+1, err)
			}
			sizes = append(sizes, size)
		case "Pages":
			kidsObj, _ := dict.Find("Kids")
			kids, err := w.array(kidsObj)
			if err != nil {
				return nil, err
			}
			if kids == nil {
				return nil, fmt.Errorf("%w: a /Pages node has no /Kids", errDamaged)
			}
			if node.depth+1 > maxTreeDepth {
				return nil, fmt.Errorf("PDF page tree is too deep")
			}
			if len(kids) > maxTreeNodes {
				return nil, fmt.Errorf("PDF page tree is too large")
			}
			// Push in reverse so the first kid is read first.
			for i := len(kids) - 1; i >= 0; i-- {
				kid, ok := kids[i].(types.IndirectRef)
				if !ok {
					return nil, fmt.Errorf("%w: /Kids holds something other than a reference", errDamaged)
				}
				stack = append(stack, treeNode{ref: kid, parent: &node.ref, attrs: attrs, depth: node.depth + 1})
			}
		default:
			return nil, fmt.Errorf("%w: a node is neither /Page nor /Pages", errDamaged)
		}
	}
	if len(sizes) == 0 {
		return nil, fmt.Errorf("PDF has no pages")
	}
	return sizes, nil
}

func (w *walker) pageSize(attrs pageAttrs) (Size, error) {
	media, ok, err := w.box(attrs.mediaBox)
	if err != nil {
		return Size{}, err
	}
	if !ok {
		return Size{}, fmt.Errorf("no readable MediaBox")
	}
	visible := media
	crop, ok, err := w.box(attrs.cropBox)
	if err != nil {
		return Size{}, err
	}
	if ok && !(media.width() < crop.width() || media.height() < crop.height()) {
		visible = crop
	}
	width, height := visible.width(), visible.height()
	if width <= 0 || height <= 0 {
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
	rotate, err := w.rotation(attrs.rotate)
	if err != nil {
		return Size{}, err
	}
	if rotate == 90 || rotate == 270 {
		width, height = height, width
	}
	return Size{Width: width, Height: height}, nil
}

// rotation accepts exactly 0, 90, 180 or 270. Documenso swaps width and
// height only for a literal 90 or 270 and does not normalise other values, so
// a -90 or 450 would be stamped differently from how it is sized here. Reals
// are read as numbers, as pdf-lib does, so 90.0 is 90.
func (w *walker) rotation(o types.Object) (int, error) {
	if o == nil {
		return 0, nil
	}
	deg, err := w.number(o)
	if errors.Is(err, errLimit) {
		return 0, err
	}
	if err != nil {
		return 0, fmt.Errorf("unreadable rotation")
	}
	switch deg {
	case 0, 90, 180, 270:
		return int(deg), nil
	}
	return 0, fmt.Errorf("unsupported rotation %v", deg)
}

type rect struct{ llx, lly, urx, ury float64 }

func (r rect) width() float64  { return r.urx - r.llx }
func (r rect) height() float64 { return r.ury - r.lly }

// box reads a rectangle. ok is false when the entry is absent or not a
// four-number array; err is set only when the walk itself must stop.
func (w *walker) box(o types.Object) (r rect, ok bool, err error) {
	if o == nil {
		return rect{}, false, nil
	}
	arr, err := w.array(o)
	if err != nil {
		if errors.Is(err, errLimit) {
			return rect{}, false, err
		}
		return rect{}, false, nil
	}
	if len(arr) != 4 {
		return rect{}, false, nil
	}
	var n [4]float64
	for i := range n {
		v, err := w.number(arr[i])
		if err != nil {
			if errors.Is(err, errLimit) {
				return rect{}, false, err
			}
			return rect{}, false, nil
		}
		n[i] = v
	}
	// The spec allows any two opposite corners.
	return rect{
		llx: math.Min(n[0], n[2]), lly: math.Min(n[1], n[3]),
		urx: math.Max(n[0], n[2]), ury: math.Max(n[1], n[3]),
	}, true, nil
}
