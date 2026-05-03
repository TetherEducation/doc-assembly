package port

// ConvertFieldToProviderPosition translates an extracted PDF anchor into Documenso
// percentage coordinates. The returned (posX, posY) describe the field's TOP-LEFT
// corner in % of page width/height. The bottom of the field aligns with the rendered
// signature line (anchor Y).
func ConvertFieldToProviderPosition(f SignatureField) (posX, posY float64) {
	posX, posY = f.PositionX, f.PositionY
	if f.PDFPageW > 0 && f.PDFPageH > 0 {
		anchorCenterPct := ((f.PDFPointX + f.PDFAnchorW/2) / f.PDFPageW) * 100
		posX = anchorCenterPct - f.Width/2
		posY = 100 - ((f.PDFPointY / f.PDFPageH) * 100)
		posY -= f.Height
	}
	posX = max(0.0, min(posX, 100-f.Width))
	posY = max(0.0, min(posY, 100-f.Height))
	return posX, posY
}
