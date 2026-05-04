package port

// ConvertFieldToDocumensoPosition translates an extracted PDF anchor into Documenso
// percentage coordinates. The returned (posX, posY) describe the field's TOP-LEFT
// corner in % of page width/height.
//
// PDF coordinates are bottom-left origin; Documenso uses top-left percentage.
// The formula `100 - (PDFPointY / PDFPageH) * 100` performs that inversion.
//
// The Typst anchor is placed at dy:0 inside the #block, so its PDF Y coordinate
// equals the signature line's Y coordinate. The field rect must sit ENTIRELY ABOVE
// the line so the line acts as a baseline beneath the signature (paper-signature
// convention), not as a strikethrough through it: posY = line_top_pct - height.
// Documenso then centers the signature artwork inside the rect (verticalAlign=
// "middle"), placing it height/2 above the line — visually "on" the line, with
// the line itself remaining visible underneath.
//
// This conversion is specific to Documenso (top-left % coordinates, centered artwork).
// Other providers may need their own conversion.
func ConvertFieldToDocumensoPosition(f SignatureField) (posX, posY float64) {
	posX, posY = f.PositionX, f.PositionY
	if f.PDFPageW > 0 && f.PDFPageH > 0 {
		anchorCenterPct := ((f.PDFPointX + f.PDFAnchorW/2) / f.PDFPageW) * 100
		posX = anchorCenterPct - f.Width/2
		// Place the rect bottom on the signature line; the rect extends upward.
		lineTopPct := 100 - ((f.PDFPointY / f.PDFPageH) * 100)
		posY = lineTopPct - f.Height
	}
	posX = max(0.0, min(posX, 100-f.Width))
	posY = max(0.0, min(posY, 100-f.Height))
	return posX, posY
}
