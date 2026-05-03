package port_test

import (
	"math"
	"testing"

	"github.com/rendis/doc-assembly/core/internal/core/port"
)

// Cubre la única fuente de verdad: ConvertFieldToProviderPosition vive en port.
// Si esta prueba se rompe por "undefined", alguien re-duplicó la función en otro paquete.
func TestConvertFieldToProviderPosition_BottomAlignsToAnchor(t *testing.T) {
	f := port.SignatureField{
		PositionX: 0, PositionY: 0,
		Width: 30, Height: 8,
		PDFPointX: 100, PDFPointY: 600,
		PDFAnchorW: 10, PDFPageW: 612, PDFPageH: 792,
	}
	posX, posY := port.ConvertFieldToProviderPosition(f)

	wantBottom := 100 - (600.0/792.0)*100
	gotBottom := posY + f.Height
	if math.Abs(gotBottom-wantBottom) > 0.01 {
		t.Fatalf("bottom mismatch: got %v want %v", gotBottom, wantBottom)
	}
	if posX < 0 || posX > 100-f.Width {
		t.Fatalf("posX out of range: %v", posX)
	}
	if posY < 0 || posY > 100-f.Height {
		t.Fatalf("posY out of range: %v", posY)
	}
}

func TestConvertFieldToProviderPosition_FallbackWhenNoPDFData(t *testing.T) {
	f := port.SignatureField{
		PositionX: 35, PositionY: 55,
		Width: 30, Height: 8,
	}
	x, y := port.ConvertFieldToProviderPosition(f)
	if x != 35 || y != 55 {
		t.Fatalf("fallback should preserve PositionX/Y: got (%v,%v)", x, y)
	}
}
