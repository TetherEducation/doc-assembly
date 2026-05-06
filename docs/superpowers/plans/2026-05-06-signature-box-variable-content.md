# Signature Box Variable Content Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reproduce and fix Documenso signature field placement so every provider box sits directly above its rendered signature line across variable contract content, page breaks, and 1–4 signer compositions.

**Architecture:** Keep the provider contract unchanged. Build a deterministic reproduction loop at the renderer/signature-field seam first, then run the existing live Documenso harness for end-to-end confirmation. The fix must make the extracted anchor/line position content-flow invariant, not tune a static percentage.

**Tech Stack:** Go 1.25, Typst PDF renderer, `pdftotext -bbox`/Go PDF extraction, River signing attempts, local PostgreSQL + local Documenso + MailPit Docker stack.

---

## File map

- Modify: `core/internal/core/service/rendering/pdfrenderer/service_test.go` — add renderer-level regression tests with variable content before signature blocks and assertions on extracted `SignatureFields`.
- Modify if root cause requires it: `core/internal/core/service/rendering/pdfrenderer/typst_converter_impl.go` — adjust anchor placement/rendering only if the extracted anchor does not track the visible signature line.
- Modify if root cause requires it: `core/internal/core/service/rendering/pdfrenderer/anchor_extractor.go` — require complete anchor extraction or improve matching only if reproduction shows missing/partial anchors.
- Modify if root cause requires it: `core/internal/core/port/signature_position.go` — adjust PDF→Documenso conversion only if extracted coordinates are correct but provider positions are wrong.
- Modify: `scripts/signature-layouts/fixtures/*.json` or add new fixture directory if needed — live E2E contract variants with different content lengths/compositions.
- Modify: `scripts/signature-layouts/run_e2e.sh` only if the current harness cannot express content variants.
- Create/update: `docs/superpowers/evidence/2026-05-06-signature-variable-content/` — reproduction CSV, screenshots, logs, and final evidence.

---

## Task 1: Ambientar y confirmar stack local

**Files:** no production changes.

- [ ] **Step 1: Verify Docker services are up**

Run:

```bash
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}\t{{.Status}}'
```

Expected: these containers are running:

```text
doc-assembly-postgres-1       postgres:16-alpine      0.0.0.0:5432->5432/tcp
doc-assembly-documenso-db-1   postgres:16-alpine      0.0.0.0:5433->5432/tcp
doc-assembly-documenso-1      documenso/documenso     0.0.0.0:3000->3000/tcp
doc-assembly-mailpit-1        axllent/mailpit         0.0.0.0:8025->8025/tcp, 1025
```

- [ ] **Step 2: Start missing services only if needed**

Run only if Step 1 shows missing containers:

```bash
docker compose -f docker-compose.dev.yml up -d postgres
docker compose -f docker-compose.documenso.yml up -d
```

Expected: `docker compose` exits 0 and containers become healthy/running.

- [ ] **Step 3: Run migrations**

Run:

```bash
DOC_ENGINE_DATABASE_PASSWORD=postgres \
DOC_ENGINE_AUTH_DUMMY=true \
make -C core migrate
```

Expected: migrations complete without error.

- [ ] **Step 4: Start backend worker-enabled dummy auth against local Documenso**

Run in a long-lived terminal:

```bash
DOC_ENGINE_DATABASE_HOST=localhost \
DOC_ENGINE_DATABASE_PORT=5432 \
DOC_ENGINE_DATABASE_USER=postgres \
DOC_ENGINE_DATABASE_PASSWORD=postgres \
DOC_ENGINE_DATABASE_NAME=doc_assembly \
DOC_ENGINE_AUTH_DUMMY=true \
DOC_ENGINE_WORKER_ENABLED=true \
DOC_ENGINE_SIGNING_PROVIDER=documenso \
DOC_ENGINE_SIGNING_BASE_URL=http://localhost:3000/api/v2 \
DOC_ENGINE_SIGNING_SIGNING_BASE_URL=http://localhost:3000 \
DOC_ENGINE_SIGNING_API_KEY="$DOCUMENSO_API_KEY" \
DOC_ENGINE_NOTIFICATION_PROVIDER=smtp \
DOC_ENGINE_NOTIFICATION_HOST=localhost \
DOC_ENGINE_NOTIFICATION_PORT=1025 \
make -C core run
```

Expected: backend listens on `http://localhost:8080` and River workers start. If no `DOCUMENSO_API_KEY` is present, create/reuse a local Documenso token before live E2E.

---

## Task 2: Build renderer-level reproduction matrix

**Files:** `core/internal/core/service/rendering/pdfrenderer/service_test.go`

- [ ] **Step 1: Add failing regression test for content-flow invariance**

Add a table-driven test that renders at least these cases:

```go
func TestRenderSignatureFieldsTrackLineWhenContentBeforeSignatureVaries(t *testing.T) {
    cases := []struct {
        name          string
        paragraphRepeats int
        signatures    []map[string]any
    }{
        {name: "short_content_single_signer", paragraphRepeats: 1, signatures: singleSignerAttrs()},
        {name: "medium_content_single_signer", paragraphRepeats: 8, signatures: singleSignerAttrs()},
        {name: "long_content_near_page_bottom", paragraphRepeats: 18, signatures: singleSignerAttrs()},
        {name: "long_content_two_signers", paragraphRepeats: 18, signatures: twoSignerAttrs()},
    }

    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            result := renderPortableDocForTest(t, buildSignatureFlowDoc(tc.paragraphRepeats, tc.signatures))
            if len(result.SignatureFields) != len(tc.signatures) {
                t.Fatalf("signature fields = %d, want %d", len(result.SignatureFields), len(tc.signatures))
            }
            for _, field := range result.SignatureFields {
                if field.PDFPointY == 0 || field.PDFPageH == 0 {
                    t.Fatalf("field %s used fallback/no extracted PDF coordinates: %+v", field.RoleID, field)
                }
                _, posY := port.ConvertFieldToDocumensoPosition(field)
                lineTopPct := 100 - ((field.PDFPointY / field.PDFPageH) * 100)
                bottomPct := posY + field.Height
                if math.Abs(bottomPct-lineTopPct) > 0.01 {
                    t.Fatalf("field %s bottom %.4f does not align with line %.4f", field.RoleID, bottomPct, lineTopPct)
                }
            }
        })
    }
}
```

- [ ] **Step 2: Run RED**

Run:

```bash
go test -C core ./internal/core/service/rendering/pdfrenderer -run TestRenderSignatureFieldsTrackLineWhenContentBeforeSignatureVaries -count=1 -v
```

Expected: FAIL reproducing one of these concrete failure modes:

```text
field <role> used fallback/no extracted PDF coordinates
```

or

```text
field <role> bottom <value> does not align with line <value>
```

If the test passes, it is too shallow; add cases with page breaks, four signers, long labels/subtitles, and repeated content until it captures the reported failure.

---

## Task 3: Extend live Documenso E2E matrix

**Files:** `scripts/signature-layouts/fixtures/`, `scripts/signature-layouts/run_e2e.sh`, `docs/superpowers/evidence/2026-05-06-signature-variable-content/`

- [ ] **Step 1: Add or generate contract variants**

Use these scenario names:

```text
short-single
medium-single
near-page-bottom-single
page-two-single
dual-long-labels
quad-grid-long-content
```

Each fixture must differ in body content before the `signature` node. Do not only vary signature layout.

- [ ] **Step 2: Run live numeric harness before fix**

Run:

```bash
CSV_OUT=docs/superpowers/evidence/2026-05-06-signature-variable-content/before_numeric_results.csv \
LAYOUTS_FILTER=short-single,medium-single,near-page-bottom-single,page-two-single,dual-long-labels,quad-grid-long-content \
bash scripts/signature-layouts/run_e2e.sh
```

Expected: at least one variant FAILs or produces visual mismatch matching the screenshot. Save signing URLs and screenshots under the evidence directory.

---

## Task 4: Fix root cause with minimal code

**Files:** depends on Task 2/3 evidence.

- [ ] **Step 1: Choose the component from evidence**

Use this decision table:

```text
Extracted PDFPointY/PDFPageH missing or zero -> fix anchor extraction completeness/fallback path.
Extracted anchor exists but moves independently of visible line -> fix Typst anchor placement relative to line.
Extracted anchor equals line but Documenso DB position wrong -> fix ConvertFieldToDocumensoPosition or adapter payload mapping.
Documenso DB position correct but UI renders elsewhere -> inspect Documenso scaling/page mapping visually before changing code.
```

- [ ] **Step 2: Implement the smallest production change**

Keep the invariant:

```go
lineTopPct := 100 - ((field.PDFPointY / field.PDFPageH) * 100)
posY := lineTopPct - field.Height
bottomPct := posY + field.Height
// bottomPct == lineTopPct
```

Do not add static offsets, content-length heuristics, or layout-specific special cases.

- [ ] **Step 3: Run GREEN**

Run:

```bash
go test -C core ./internal/core/service/rendering/pdfrenderer ./internal/core/port -count=1
```

Expected: new regression and existing conversion tests pass.

---

## Task 5: Verify final behavior end-to-end

**Files:** evidence only unless docs need updating.

- [ ] **Step 1: Run targeted unit tests**

```bash
go test -C core ./internal/core/port ./internal/core/service/rendering/pdfrenderer ./internal/infra/riverqueue ./internal/adapters/secondary/signing/documenso -count=1
```

Expected: all pass.

- [ ] **Step 2: Run live Documenso matrix after fix**

```bash
CSV_OUT=docs/superpowers/evidence/2026-05-06-signature-variable-content/after_numeric_results.csv \
LAYOUTS_FILTER=short-single,medium-single,near-page-bottom-single,page-two-single,dual-long-labels,quad-grid-long-content \
bash scripts/signature-layouts/run_e2e.sh
```

Expected: all rows PASS with `bottom_pct == line_top_pct` within tolerance.

- [ ] **Step 3: Capture visual proof**

For each scenario, open the signing URL in local Documenso and save screenshots. Acceptance criterion: the green box sits above the line with its bottom edge on the signature line; it must not cross the line, cover signer name/subtitle, or float over document text.

- [ ] **Step 4: Run broader project checks**

```bash
make -C core test
go test -C core -tags=integration ./...
```

Expected: unit tests pass and integration-tag packages compile. Run full `make -C core test-integration` if Docker time budget allows.
