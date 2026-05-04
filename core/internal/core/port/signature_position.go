package port

// ConvertFieldToDocumensoPosition translates an extracted PDF anchor into Documenso
// percentage coordinates. The returned (posX, posY) describe the field's TOP-LEFT
// corner in % of page width/height.
//
// PDF coordinates are bottom-left origin; Documenso uses top-left percentage.
// The formula `100 - (PDFPointY / PDFPageH) * 100` performs that inversion.
//
// The Typst anchor is placed at dy:0 inside the #block, so its PDF Y coordinate
// equals the signature line's Y coordinate. Documenso centers the signature artwork
// vertically in the field (verticalAlign="middle"), so the field must be centered on
// the line: posY = line_top_pct - height/2.
//
// This conversion is specific to Documenso (top-left % coordinates, centered artwork).
// Other providers may need their own conversion.
func ConvertFieldToDocumensoPosition(f SignatureField) (posX, posY float64) {
	posX, posY = f.PositionX, f.PositionY
	if f.PDFPageW > 0 && f.PDFPageH > 0 {
		anchorCenterPct := ((f.PDFPointX + f.PDFAnchorW/2) / f.PDFPageW) * 100
		posX = anchorCenterPct - f.Width/2
		// The anchor sits at the signature line Y. Documenso centers artwork in the
		// field, so center the field on the line.
		lineTopPct := 100 - ((f.PDFPointY / f.PDFPageH) * 100)
		posY = lineTopPct - f.Height/2
	}
	posX = max(0.0, min(posX, 100-f.Width))
	posY = max(0.0, min(posY, 100-f.Height))
	return posX, posY
}
