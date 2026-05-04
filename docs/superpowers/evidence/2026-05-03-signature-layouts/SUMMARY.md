# Signature alignment fix — visual summary

**Date:** 2026-05-04 (final iteration)
**Commits:** `da53f26` (anchor + center formula), `e30f70b` (height 8%→6% + 25pt gap), `d268255` (rect-above-line invariant)
**Fix:** Documenso signature field sits entirely above the rendered Typst signature line for all 13 layouts. The line acts as the baseline: rect above, line, label below.

## Root cause

Two bugs combined to place signature fields well above their lines:

1. **Typst anchor at `dy: 20.0pt`** — the invisible anchor text was rendered 20pt below the block top (= 20pt below the `#line`), so pdftotext reported an inflated Y coordinate.
2. **Bottom-alignment formula** — `posY = anchorTopPct - height` placed the field *bottom* at the (already-wrong) anchor, putting the whole rectangle above the true line position.

## Fix history

**Iteration 1 (`da53f26`):**
- `typst_converter_impl.go`: anchor moved to `dy: 0.0pt` — reports the exact line Y.
- `signature_position.go`: center-align formula — `posY = lineTopPct - height/2` — field center = line Y.

**Iteration 2 (`e30f70b`) — label visibility fix:**
- Problem: with `Height=8%`, the centered rect extended ~32pt below the line, covering the label.
- `typst_converter_impl.go`: `defaultHeight` 8% → 6% (rect now extends ~24pt below line).
- `typst_converter_impl.go`: `#v(25.0pt)` gap added after `#line` so label renders ≥25pt below line.

**Iteration 3 (`d268255`) — rect-above-line invariant:**
- Problem: user screenshot confirmed the rect crossed the line (cursive "Firma" bisected the line).
  Center-alignment placed half the rect below the line by design.
- `signature_position.go`: `posY = lineTopPct - height` (rect bottom = line, rect entirely above).
- `typst_converter_impl.go`: removed `#v(25.0pt)` gap (no longer needed) and restored `defaultHeight = 8%`.
- Harness: metric changed from `|center - line_top|` to `|bottom - line_top|`; CSV column `center_pct` → `bottom_pct`.

## dual-sides — baseline vs. final (iteration 3)

### Baseline (before fix)
![dual-sides baseline](baseline/dual-sides.png)

### Final (iteration 3 — rect entirely above line)
![dual-sides final](final/dual-sides.png)

**Observation:** In the baseline, both signature rectangles float above their respective lines with incorrect positioning. In the final, the active (green) rect sits entirely above its line, with the line acting as the bottom boundary. Both "Firma Apoderado/a" / "Firma Estudiante" labels and subtitles are fully visible below the line.

---

## quad-grid — baseline vs. final (iteration 3)

### Baseline (before fix)
![quad-grid baseline](baseline/quad-grid.png)

### Final (iteration 3 — rect entirely above line)
![quad-grid final](final/quad-grid.png)

**Observation:** In the baseline, all four signature rectangles (2x2 grid) sit above their lines with incorrect positioning. In the final, all four active (green) rects sit entirely above their respective lines — including the second-row signatures which use a distinct anchor Y bucket. All labels and subtitles are visible below the lines.

---

## All 13 layouts — numeric results (iteration 3)

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

Metric: `|bottom_pct - line_top_pct| < 0.5%` where `bottom_pct = positionY + height` and `line_top_pct = snapshot_pos_y + snapshot_height`.

All results verified with fresh documents created by binary at `21:10:07` (after commit `d268255`).
Height confirmed as `8.0%` in Documenso field table for all 38 fields.
Rect entirely above the signature line. Label and subtitle fully visible below. All 13 layouts PASS.
