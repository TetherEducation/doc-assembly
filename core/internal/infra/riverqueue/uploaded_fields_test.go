package riverqueue

import (
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

func TestPositionsFromPlacedFieldsUsesBottomLeftOrigin(t *testing.T) {
	fields, err := positionsFromPlacedFields([]entity.PlacedField{{
		RoleID: "role-1",
		Type:   "TEXT",
		Page:   1,
		X:      0,
		Y:      0,
		Width:  100,
		Height: 50,
	}}, []entity.PageSize{{Width: 200, Height: 400}})
	if err != nil {
		t.Fatal(err)
	}
	got := fields[0]
	if got.PositionX != 0 || got.Width != 50 || got.Height != 12.5 || got.PositionY != 87.5 || got.Type != "TEXT" {
		t.Fatalf("position = %+v", got)
	}
}
