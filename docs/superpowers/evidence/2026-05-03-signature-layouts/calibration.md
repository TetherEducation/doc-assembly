# Signature alignment calibration — 2026-05-03

## Environment

- Page size: 612 × 792 pt (US Letter)
- Documenso coordinates: top-left percentage (0–100% of page width/height)
- Konva.js stage scale: 1.307 (Documenso signing editor)
- Artwork alignment: `verticalAlign="middle"` (confirmed via Konva inspection)
- Field dimensions used: Width=20%, Height=6% (reduced from 8% — see E3 fix below)

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

## E3 fix — rect-overlaps-label (iteration 2)

With `Height=8%`, the unsigned Documenso green rect extended `8/100 * 792 / 2 ≈ 32pt` below the
signature line, covering the role label and subtitle rendered in the Typst PDF background.

**Option chosen: E3 (combined)**

1. Reduce `defaultHeight` from `8.0` to `6.0` (% of page height):
   - Rect now extends `6/100 * 792 / 2 ≈ 24pt` below the line.
2. Add `#v(25.0pt)` gap in Typst between `#line` and the label:
   - Label renders ≥25pt below the line (1pt margin above rect bottom).
   - Label and subtitle are fully visible as the green rect does not reach them.

**Why E3 over E1 (pure gap) or E2 (smaller rect only):**
- E2 alone would need `height < 2.5%` to avoid covering a label 10pt below the line — too small for legible artwork.
- E1 alone at 8% would need a 36pt gap, which creates a large visual void in the signed PDF.
- E3 with 6%+25pt achieves the minimum necessary gap with a natural-looking layout.

**Numerical verification (rerun with new binary, all 13 layouts):**
All `delta_pct = 0.0000` (center of 6% field exactly on the anchor line).
Height confirmed as `6.0000` in Documenso field table for all 32 fields.
