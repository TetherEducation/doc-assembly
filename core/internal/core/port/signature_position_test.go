package port_test

import (
	"math"
	"testing"

	"github.com/rendis/doc-assembly/core/internal/core/port"
)

// Sole source of truth for the Documenso coordinate conversion. If this test
// fails to compile because of "undefined", someone re-duplicated the function.
//
// The invariant: Documenso centers the artwork vertically in the field, so the
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
