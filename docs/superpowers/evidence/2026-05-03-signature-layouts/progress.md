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
