package riverqueue

import (
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

func TestMapSignatureFieldPositionsRequiresExtractedPDFCoordinates(t *testing.T) {
	_, err := mapSignatureFieldPositions(
		[]port.SignatureField{{RoleID: "portable_role", AnchorString: "__sig_signer__", Page: 1, PositionX: 35, PositionY: 55, Width: 30, Height: 8}},
		[]*entity.TemplateVersionSignerRole{{ID: "db_role", AnchorString: "__sig_signer__"}},
		[]portabledoc.SignerRole{{ID: "portable_role", Label: "Signer"}},
	)
	if err == nil {
		t.Fatal("expected missing extracted PDF coordinates to fail")
	}
}

func TestMapSignatureFieldPositionsUsesExtractedPDFCoordinates(t *testing.T) {
	positions, err := mapSignatureFieldPositions(
		[]port.SignatureField{{
			RoleID:       "portable_role",
			AnchorString: "__sig_signer__",
			Page:         2,
			PositionX:    35,
			PositionY:    55,
			Width:        30,
			Height:       8,
			PDFPointX:    250,
			PDFPointY:    500,
			PDFPageW:     612,
			PDFPageH:     792,
		}},
		[]*entity.TemplateVersionSignerRole{{ID: "db_role", AnchorString: "__sig_signer__"}},
		[]portabledoc.SignerRole{{ID: "portable_role", Label: "Signer"}},
	)
	if err != nil {
		t.Fatalf("mapSignatureFieldPositions() error = %v", err)
	}
	if len(positions) != 1 {
		t.Fatalf("positions = %d, want 1", len(positions))
	}
	if positions[0].RoleID != "db_role" || positions[0].Page != 2 {
		t.Fatalf("unexpected mapped position: %+v", positions[0])
	}
}
