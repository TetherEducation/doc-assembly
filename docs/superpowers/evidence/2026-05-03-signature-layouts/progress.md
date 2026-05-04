# Multi-signature layout alignment — progress

| Layout              | Count | Numeric | Visual | Baseline screenshot                                  | Final screenshot                                  | Notes              |
|---------------------|-------|---------|--------|------------------------------------------------------|---------------------------------------------------|--------------------|
| single-left         | 1     | PASS    | PASS   | baseline/single-left.png                             | final/single-left.png                             | delta=0.000        |
| single-center       | 1     | PASS    | PASS   | baseline/single-center.png                           | final/single-center.png                           | delta=0.000        |
| single-right        | 1     | PASS    | PASS   | baseline/single-right.png                            | final/single-right.png                            | delta=0.000        |
| dual-sides          | 2     | PASS    | PASS   | baseline/dual-sides.png                              | final/dual-sides.png                              | delta=0.000        |
| dual-center         | 2     | PASS    | PASS   | baseline/dual-center.png                             | final/dual-center.png                             | delta=0.000        |
| dual-left           | 2     | PASS    | PASS   | baseline/dual-left.png                               | final/dual-left.png                               | delta=0.000        |
| dual-right          | 2     | PASS    | PASS   | baseline/dual-right.png                              | final/dual-right.png                              | delta=0.000        |
| triple-row          | 3     | PASS    | PASS   | baseline/triple-row.png                              | final/triple-row.png                              | delta=0.000        |
| triple-pyramid      | 3     | PASS    | PASS   | baseline/triple-pyramid.png                          | final/triple-pyramid.png                          | delta=0.000        |
| triple-inverted     | 3     | PASS    | PASS   | baseline/triple-inverted.png                         | final/triple-inverted.png                         | delta=0.000        |
| quad-grid           | 4     | PASS    | PASS   | baseline/quad-grid.png                               | final/quad-grid.png                               | delta=0.000        |
| quad-top-heavy      | 4     | PASS    | PASS   | baseline/quad-top-heavy.png                          | final/quad-top-heavy.png                          | delta=0.000        |
| quad-bottom-heavy   | 4     | PASS    | PASS   | baseline/quad-bottom-heavy.png                       | final/quad-bottom-heavy.png                       | delta=0.000        |

Legend: PENDING / PASS / FAIL. Visual=PASS only when a human reviewer (or Playwright + numeric tolerance < 0.5%) confirms the signature box visibly sits on the line in the Documenso editor.

## Fix summary (2026-05-03)

**Hypothesis confirmed:** H1 — Documenso centers artwork vertically inside the field (`verticalAlign="middle"`).

**Root cause (two-part):**
1. The Typst invisible anchor was placed at `dy: 20.0pt` inside the block, which is 20pt *below* the block top — i.e., 20pt below the rendered `#line`. pdftotext reported this inflated Y, making every field appear 20pt lower than its true line position.
2. The coordinate conversion formula used bottom-alignment (`posY = anchorTopPct - height`), placing the field *bottom* at the anchor, which visually put the entire rectangle above the line.

**Fix applied (commit `da53f26`):**
- `typst_converter_impl.go`: `dy: 20.0pt` → `dy: 0.0pt` — anchor now sits at the exact block top = line Y.
- `signature_position.go`: formula changed to center-align — `posY = lineTopPct - height/2` — field center aligns with the line.
- `signature_position_test.go`: invariant updated from "field bottom == anchor" to "field center == anchor"; table-driven tests added for 3 observed anchor-Y buckets.
- `run_e2e.sh`: metric updated from `bottom_pct` to `center_pct`; pagination bug fixed (search by `?q=<code>` to avoid 10-item page truncation for SINGLE_RIGHT and TRIPLE_* layouts).

**Numeric result:** delta=0.000% for all 13 layouts (center_pct == line_top_pct within 0.001%).
**Visual result:** Playwright screenshots confirm field rectangle sits on the signature line for all 13 layouts.

## Second iteration fix — rect-overlaps-label (commit `e30f70b`)

**Problem discovered:** With `Height=8%`, the centered rect extended ~32pt below the signature line,
covering the role label and subtitle text in the rendered PDF background.

**Fix applied (E3 combined):**
- `typst_converter_impl.go`: `defaultHeight` 8.0 → 6.0 (rect extends ~24pt below line on Letter)
- `typst_converter_impl.go`: added `#v(25.0pt)` between `#line` and label so label renders below rect
- `typst_converter_test.go`: updated test expectation from `dy: 20.0pt` to `dy: 0.0pt`
- `config/config.go`: added `applyDatabaseEnvOverrides` so `DOC_ENGINE_DATABASE_*` env vars work
- `bootstrap/preflight.go`: skip signing session auth check in dummy auth mode

**Rerun verification:** All 13 layouts rerun with fresh documents using new binary (mtime 14:18:45, after both fix commits). All numeric: delta=0.0000, height=6.0000. All visual: Playwright screenshots show rect on line, label and subtitle fully visible.

**All 13 layouts pass visual bar:** rect-on-line=YES, label-visible=YES, subtitle-visible=YES.

## Third iteration fix — rect-above-line invariant (commit `d268255`)

**Problem discovered:** User screenshot confirmed the green rect was crossing the signature line —
the cursive "Firma" artwork inside the rect visually bisected the line.
The `posY = lineTopPct - height/2` formula placed the field *center* on the line,
so the bottom half of the rect extended below the line into the label area.

**Fix applied:**
- `signature_position.go`: `posY = lineTopPct - height` (was `- height/2`). Rect bottom now sits exactly on the line; entire rect is above the line.
- `typst_converter_impl.go`: removed `labelGapBelowLinePt` and the `#v(25.0pt)` write (no longer needed — the rect no longer crosses the line). Restored `defaultHeight = 8%` (was 6%).
- Tests updated to bottom-aligns invariant: `posY + height == lineTopPct`.
- Harness `run_e2e.sh`: metric updated from center_pct (posY+h/2) to bottom_pct (posY+h); SQL query updated; CSV header updated; Python delta changed to `|bottom_doc - line_top_pct|`.

**Rerun verification:** All 13 layouts rerun with fresh documents using new binary (mtime 21:10:07, after commit d268255). All numeric: delta=0.0000, height=8.0000. All visual: Playwright screenshots show rect entirely above the signature line, label and subtitle fully visible below.

**All 13 layouts pass visual bar:** rect-above-line=YES, label-visible=YES, subtitle-visible=YES.
