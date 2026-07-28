package port_test

import (
	"math"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// Invariant: the field rect sits ENTIRELY ABOVE the signature line so the line
// acts as a baseline, not a strikethrough. This means the rect's BOTTOM aligns
// with the anchor:
//
//	posY + height ≈ 100 - (PDFPointY / PDFPageH) * 100  (= line_top_pct)
func TestConvertFieldToDocumensoPosition_BottomAlignsToLine(t *testing.T) {
	f := port.SignatureField{
		PositionX: 0, PositionY: 0,
		Width: 30, Height: 8,
		PDFPointX: 100, PDFPointY: 336,
		PDFAnchorW: 10, PDFPageW: 612, PDFPageH: 792,
	}
	posX, posY := port.ConvertFieldToDocumensoPosition(f)

	wantLine := 100 - (336.0/792.0)*100 // ≈ 57.575%
	gotBottom := posY + f.Height
	if math.Abs(gotBottom-wantLine) > 0.01 {
		t.Fatalf("bottom mismatch: got %v want %v", gotBottom, wantLine)
	}
	if posX < 0 || posX > 100-f.Width {
		t.Fatalf("posX out of range: %v", posX)
	}
	if posY < 0 || posY > 100-f.Height {
		t.Fatalf("posY out of range: %v", posY)
	}
}

// Table-driven test covering Y values observed from the 13-layout harness run.
// All anchors are placed at dy:0 (= the signature line itself) by Typst.
func TestConvertFieldToDocumensoPosition_TableDriven(t *testing.T) {
	const pageW, pageH = 612.0, 792.0
	const fieldW, fieldH = 20.0, 8.0

	cases := []struct {
		name      string
		pdfPointY float64 // anchor Y from PDF bottom (= line Y)
		wantLine  float64 // expected line_top_pct (= field bottom %)
	}{
		{"single-row-line", 336.02, 100 - (336.02/pageH)*100},
		{"second-row-line", 316.02, 100 - (316.02/pageH)*100},
		{"quad-second-row", 299.82, 100 - (299.82/pageH)*100},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := port.SignatureField{
				Width: fieldW, Height: fieldH,
				PDFPointX: 100, PDFPointY: tc.pdfPointY,
				PDFAnchorW: 10, PDFPageW: pageW, PDFPageH: pageH,
			}
			_, posY := port.ConvertFieldToDocumensoPosition(f)
			gotBottom := posY + fieldH
			if math.Abs(gotBottom-tc.wantLine) > 0.01 {
				t.Fatalf("bottom mismatch for %s: got %.4f want %.4f", tc.name, gotBottom, tc.wantLine)
			}
		})
	}
}

func TestConvertFieldToDocumensoPosition_FallbackWhenNoPDFData(t *testing.T) {
	f := port.SignatureField{
		PositionX: 35, PositionY: 55,
		Width: 30, Height: 8,
	}
	x, y := port.ConvertFieldToDocumensoPosition(f)
	if x != 35 || y != 55 {
		t.Fatalf("fallback should preserve PositionX/Y: got (%v,%v)", x, y)
	}
}

// Boundary cases: anchor near the page edges drives the unclamped formula
// outside [0, 100-dim]. The clamp must cap it within range so we never send
// out-of-page coordinates to Documenso.
func TestConvertFieldToDocumensoPosition_ClampsToPageBounds(t *testing.T) {
	const pageW, pageH = 612.0, 792.0

	cases := []struct {
		name     string
		f        port.SignatureField
		wantPosY float64 // -1 means "don't assert exact value, only that it's in range"
		wantPosX float64
	}{
		{
			name: "anchor at page top → posY clamps to 0",
			f: port.SignatureField{
				Width: 20, Height: 8,
				PDFPointX: 300, PDFPointY: 790, // unclamped posY ≈ -7.75
				PDFAnchorW: 10, PDFPageW: pageW, PDFPageH: pageH,
			},
			wantPosY: 0, wantPosX: -1,
		},
		{
			name: "anchor at left edge → posX clamps to 0",
			f: port.SignatureField{
				Width: 30, Height: 8,
				PDFPointX: 0, PDFPointY: 400,
				PDFAnchorW: 0, PDFPageW: pageW, PDFPageH: pageH,
			},
			wantPosX: 0, wantPosY: -1,
		},
		{
			name: "anchor at right edge → posX clamps to 100-Width",
			f: port.SignatureField{
				Width: 30, Height: 8,
				PDFPointX: pageW, PDFPointY: 400,
				PDFAnchorW: 0, PDFPageW: pageW, PDFPageH: pageH,
			},
			wantPosX: 100 - 30, wantPosY: -1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			x, y := port.ConvertFieldToDocumensoPosition(tc.f)
			if tc.wantPosX >= 0 && math.Abs(x-tc.wantPosX) > 0.01 {
				t.Errorf("posX: got %.4f want %.4f", x, tc.wantPosX)
			}
			if tc.wantPosY >= 0 && math.Abs(y-tc.wantPosY) > 0.01 {
				t.Errorf("posY: got %.4f want %.4f", y, tc.wantPosY)
			}
			if x < 0 || x > 100-tc.f.Width {
				t.Errorf("posX out of clamp range [0, %v]: %v", 100-tc.f.Width, x)
			}
			if y < 0 || y > 100-tc.f.Height {
				t.Errorf("posY out of clamp range [0, %v]: %v", 100-tc.f.Height, y)
			}
		})
	}
}
