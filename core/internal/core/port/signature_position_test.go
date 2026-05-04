package port_test

import (
	"math"
	"testing"

	"github.com/rendis/doc-assembly/core/internal/core/port"
)

// Invariant: Documenso centers the artwork vertically in the field, so the
// center of the field must align with the anchor (= the signature line).
//
//	center_pct = posY + height/2  ≈  100 - (PDFPointY / PDFPageH) * 100
func TestConvertFieldToDocumensoPosition_CenterAlignsToLine(t *testing.T) {
	f := port.SignatureField{
		PositionX: 0, PositionY: 0,
		Width: 30, Height: 8,
		PDFPointX: 100, PDFPointY: 336,
		PDFAnchorW: 10, PDFPageW: 612, PDFPageH: 792,
	}
	posX, posY := port.ConvertFieldToDocumensoPosition(f)

	// The anchor Y in Documenso top-down percentage coordinates
	wantCenter := 100 - (336.0/792.0)*100 // = 57.57...%
	gotCenter := posY + f.Height/2
	if math.Abs(gotCenter-wantCenter) > 0.01 {
		t.Fatalf("center mismatch: got %v want %v", gotCenter, wantCenter)
	}
	if posX < 0 || posX > 100-f.Width {
		t.Fatalf("posX out of range: %v", posX)
	}
	if posY < 0 || posY > 100-f.Height {
		t.Fatalf("posY out of range: %v", posY)
	}
}

// Table-driven test covering the 3 Y values observed from the 13-layout harness run:
//
//	line anchor (y_from_bottom_pt):   336.02  (single row / first row)
//	                                  316.02  (second row, stacked layouts)
//	quad-grid second row anchor:      ~316.XX (same second-row bucket)
//
// All coordinates derived from actual pdftotext extraction on single-center layout.
func TestConvertFieldToDocumensoPosition_TableDriven(t *testing.T) {
	const pageW, pageH = 612.0, 792.0
	const fieldW, fieldH = 20.0, 8.0

	cases := []struct {
		name        string
		pdfPointY   float64 // anchor Y from PDF bottom
		wantCenterY float64 // expected center % from top
	}{
		// single-center: anchor at dy:0 on #line → y_from_bottom = 336.02
		{"single-row-line", 336.02, 100 - (336.02/pageH)*100},
		// dual-center second signature (stacked): second-row anchor
		{"second-row-line", 316.02, 100 - (316.02/pageH)*100},
		// quad-grid second row (slightly different due to grid spacing)
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
			gotCenter := posY + fieldH/2
			if math.Abs(gotCenter-tc.wantCenterY) > 0.01 {
				t.Fatalf("center mismatch for %s: got %.4f want %.4f", tc.name, gotCenter, tc.wantCenterY)
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
				PDFPointX: 300, PDFPointY: 790, // unclamped posY ≈ -3.75
				PDFAnchorW: 10, PDFPageW: pageW, PDFPageH: pageH,
			},
			wantPosY: 0, wantPosX: -1,
		},
		{
			name: "anchor at page bottom → posY clamps to 100-Height",
			f: port.SignatureField{
				Width: 20, Height: 8,
				PDFPointX: 300, PDFPointY: 5, // unclamped posY ≈ 95.37
				PDFAnchorW: 10, PDFPageW: pageW, PDFPageH: pageH,
			},
			wantPosY: 100 - 8, wantPosX: -1,
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
