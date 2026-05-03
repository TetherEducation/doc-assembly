# Signature alignment fix — visual summary

**Date:** 2026-05-03
**Commits:** `da53f26` (anchor + center formula), `e30f70b` (E3: height 8%→6% + 25pt gap)
**Fix:** Documenso signature field center now aligns with the rendered Typst signature line for all 13 layouts, and the unsigned green rect no longer covers the role label or subtitle.

## Root cause

Two bugs combined to place signature fields well above their lines:

1. **Typst anchor at `dy: 20.0pt`** — the invisible anchor text was rendered 20pt below the block top (= 20pt below the `#line`), so pdftotext reported an inflated Y coordinate.
2. **Bottom-alignment formula** — `posY = anchorTopPct - height` placed the field *bottom* at the (already-wrong) anchor, putting the whole rectangle above the true line position.

## Fix

**Iteration 1 (`da53f26`):**
- `typst_converter_impl.go`: anchor moved to `dy: 0.0pt` — reports the exact line Y.
- `signature_position.go`: center-align formula — `posY = lineTopPct - height/2` — field center = line Y.

**Iteration 2 (`e30f70b`) — label visibility fix:**
- Problem: with `Height=8%`, the centered rect extended ~32pt below the line, covering the label.
- `typst_converter_impl.go`: `defaultHeight` 8% → 6% (rect now extends ~24pt below line).
- `typst_converter_impl.go`: `#v(25.0pt)` gap added after `#line` so label renders ≥25pt below line.
- Result: label and subtitle are fully visible below the unsigned green rect on all 13 layouts.

## dual-sides — baseline vs. final

### Baseline (before fix)
![dual-sides baseline](baseline/dual-sides.png)

### Final (after fix)
![dual-sides final](final/dual-sides.png)

**Observation:** In the baseline, both signature rectangles float clearly above their respective lines. In the final, both rectangles are centered on their lines, and both "Firma Apoderado/a" / "Firma Estudiante" labels and subtitles are fully visible below the rect.

---

## quad-grid — baseline vs. final

### Baseline (before fix)
![quad-grid baseline](baseline/quad-grid.png)

### Final (after fix)
![quad-grid final](final/quad-grid.png)

**Observation:** In the baseline, all four signature rectangles (2x2 grid) sit above their lines. In the final, all four are centered on their lines — including the second-row signatures which use a distinct anchor Y bucket (~299.82pt from bottom). All labels and subtitles are visible below the rects.

---

## All 13 layouts — numeric results

| Layout              | Numeric delta | Visual |
|---------------------|---------------|--------|
| single-left         | 0.000%        | PASS   |
| single-center       | 0.000%        | PASS   |
| single-right        | 0.000%        | PASS   |
| dual-sides          | 0.000%        | PASS   |
| dual-center         | 0.000%        | PASS   |
| dual-left           | 0.000%        | PASS   |
| dual-right          | 0.000%        | PASS   |
| triple-row          | 0.000%        | PASS   |
| triple-pyramid      | 0.000%        | PASS   |
| triple-inverted     | 0.000%        | PASS   |
| quad-grid           | 0.000%        | PASS   |
| quad-top-heavy      | 0.000%        | PASS   |
| quad-bottom-heavy   | 0.000%        | PASS   |

Metric: |center_pct - line_top_pct| < 0.5% where center_pct = positionY + height/2 and line_top_pct = 100 - (PDFPointY / PDFPageH) * 100.

All results verified with fresh documents created by binary at `14:18:45` (after both fix commits).
Height confirmed as 6.0% in Documenso field table for all 32 fields.
Unsigned green rect does not cover label or subtitle in any layout.
