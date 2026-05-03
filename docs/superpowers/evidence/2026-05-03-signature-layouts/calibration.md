# Signature alignment calibration — 2026-05-03

## Environment

- Page size: 612 × 792 pt (US Letter)
- Documenso coordinates: top-left percentage (0–100% of page width/height)
- Konva.js stage scale: 1.307 (Documenso signing editor)
- Artwork alignment: `verticalAlign="middle"` (confirmed via Konva inspection)
- Field dimensions used: Width=20%, Height=8%

## pdftotext anchor extraction (post-fix, dy=0)

All measurements taken from actual PDFs rendered after the `dy: 0.0pt` fix.
`yMin` is from page top (pdftotext -bbox output). `y_from_bottom = 792 - yMin`.

| Layout group      | anchor yMin (top, pt) | y_from_bottom (pt) | line_top_pct (%) |
|-------------------|-----------------------|--------------------|------------------|
| First-row lines   | 335.98                | 456.02             | 42.425           |
| Second-row lines  | 355.98 (stacked)      | 436.02             | 44.947           |
| Quad second-row   | 372.18                | 419.82             | 46.996           |

Note: `line_top_pct = 100 - (y_from_bottom / 792) * 100`

## Konva empirical measurement (pre-fix, dy=20)

Playwright inspection of `Konva.stages[0].findOne('.field-group')` on single-center layout:

```
field y (Konva logical px): 616.38
field height (Konva logical px): 63.36
stage height: 1034.97 px (logical)
→ field center from top: 616.38 + 63.36/2 = 647.55 px = 62.58%
```

Anchor from pdftotext with dy=20: yMin=475.98pt from top → y_from_bottom=316.02pt → 39.90% from top.
Old formula placed field bottom at 52.10% from top → field was ~12% above the true line position.

## Post-fix numeric results (all 13 layouts)

All deltas from `numeric_results.csv` (center_pct − line_top_pct):

```
single-left:        delta = 0.000%
single-center:      delta = 0.000%
single-right:       delta = 0.000%
dual-sides:         delta = 0.000% (both fields)
dual-center:        delta = 0.000% (both fields)
dual-left:          delta = 0.000% (both fields)
dual-right:         delta = 0.000% (both fields)
triple-row:         delta = 0.000% (all 3 fields)
triple-pyramid:     delta = 0.000% (all 3 fields)
triple-inverted:    delta = 0.000% (all 3 fields)
quad-grid:          delta = 0.000% (all 4 fields)
quad-top-heavy:     delta = 0.000% (all 4 fields)
quad-bottom-heavy:  delta = 0.000% (all 4 fields)
```

## Coordinate conversion formula (post-fix)

```
anchorCenterPct = ((PDFPointX + PDFAnchorW/2) / PDFPageW) * 100
posX = anchorCenterPct - Width/2

lineTopPct = 100 - (PDFPointY / PDFPageH) * 100
posY = lineTopPct - Height/2
```

Where `PDFPointY` = anchor Y measured from PDF bottom = `792 - yMin(pdftotext)`.

This ensures `posY + Height/2 == lineTopPct`, i.e., the field center is on the line.
