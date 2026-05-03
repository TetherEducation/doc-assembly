# Documenso Multi-Signature Layout Alignment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking. This plan is designed to be driven by `/ralph-loop:ralph-loop` so each iteration advances one or more pending layouts and commits.

**Goal:** Garantizar que el signature box generado en Documenso quede visualmente alineado sobre la línea de firma del PDF en TODAS las 13 combinaciones de layout × cantidad de firmas (1 a 4 firmas), validado en UI con screenshots y con asserts numéricos sobre `positionY + height` vs. la coordenada del anchor de cada firma.

**Architecture:** Ya existe un fix previo (commit `5149ef6`) que calibró 1 firma (`single-center`) usando un anchor invisible Typst en `dy: 20pt` y conversión `posY -= height`. El plan extiende esa estrategia a las 13 layouts soportadas (`single-{left,center,right}`, `dual-{sides,center,left,right}`, `triple-{row,pyramid,inverted}`, `quad-{grid,top-heavy,bottom-heavy}`), unifica las dos copias divergentes de `convertFieldToProviderPosition` (`document_service.go` y `riverqueue/helpers.go`), agrega fixtures `.docml` y un harness E2E reproducible, y se ejecuta iterativamente con ralph-loop hasta que las 13 combinaciones pasen tanto por test numérico (`positionY + height ≈ anchor top%`) como visual (screenshot del field sobre la línea). Stack local: PostgreSQL app DB en 5432, Documenso DB en 5433, Documenso en :3000, Mailpit en :8025, backend Go en :8081 (puerto 8080 ocupado por `relevo-broker-1`), frontend Vite por defecto.

**Tech Stack:** Go 1.25 + Gin + pgx + Wire + River + Typst + pdftotext (`-bbox`) | React 19 + TanStack Router + Zustand + TipTap | PostgreSQL 16 + Liquibase | Documenso embedded signing | Playwright MCP (verificación UI) | Docker Compose.

---

## Pre-flight & Source-of-truth

- **Memoria previa relevante (Engram):** observación `#956` (fix de single-center), `#957` y `#941` (resúmenes de la sesión 2026-04-22). Lee estas memorias con `mem_get_observation` antes de tocar código.
- **Spec previo:** `docs/superpowers/specs/2026-04-22-signing-attempts-river-design.md` (River + attempts; NO modifica el contrato pero define cómo se enruta el upload a Documenso).
- **Commit previo:** `5149ef6 fix: align Documenso signature fields` — partir desde aquí; NO revertir.
- **Reproducción del bug es OBLIGATORIA antes de cualquier fix.** Sin baseline visual no se puede medir mejora.

## File Structure

### Files que se modifican
- `core/internal/core/service/rendering/pdfrenderer/typst_converter_impl.go` — anchor markup (`renderTypstSignatureItemContent`), posiblemente `renderSignatureGrid` y `renderSplitLayout` si se necesita ajuste por fila.
- `core/internal/core/service/document/document_service.go` — exportar `ConvertFieldToProviderPosition` (mayúscula) y eliminar la copia privada.
- `core/internal/infra/riverqueue/helpers.go` — sustituir copia local por la función exportada del paquete `document`.
- `core/internal/core/service/document/signature_position_test.go` — ampliar a tabla con un caso por layout.
- `core/internal/core/service/rendering/pdfrenderer/typst_converter_test.go` — verificar que cada layout emite N anchors únicos en el orden correcto.

### Files nuevos
- `scripts/signature-layouts/fixtures/single-left.docml`, `single-center.docml`, `single-right.docml`, `dual-sides.docml`, `dual-center.docml`, `dual-left.docml`, `dual-right.docml`, `triple-row.docml`, `triple-pyramid.docml`, `triple-inverted.docml`, `quad-grid.docml`, `quad-top-heavy.docml`, `quad-bottom-heavy.docml` (13 fixtures, una por layout).
- `scripts/signature-layouts/run_e2e.sh` — bash que: limpia DB, levanta stack, crea tenant/workspace/document type, sube los 13 templates, dispara `proceed-to-signing` para cada uno, espera River, consulta Documenso DB y reporta `positionY`, `positionY+height`, anchor `Y%` esperado.
- `scripts/signature-layouts/verify_ui.sh` — toma screenshot por Playwright/Chrome MCP de cada envelope abierto en Documenso para verificación humana/visual.
- `docs/superpowers/evidence/2026-05-03-signature-layouts/progress.md` — tabla de tracking con columnas `layout | numeric_pass | visual_pass | screenshot | notes`. Se actualiza en cada iteración de ralph-loop.
- `docs/superpowers/evidence/2026-05-03-signature-layouts/baseline/<layout>.png` — screenshots ANTES de cualquier fix (reproducción del bug).
- `docs/superpowers/evidence/2026-05-03-signature-layouts/final/<layout>.png` — screenshots DESPUÉS del fix (verificación).

### Files que NO se tocan
- `db/src/**` — agentes nunca modifican Liquibase (regla del repo).
- `core/internal/adapters/secondary/signing/documenso/adapter.go` — el contrato de campo (`POST /envelope/field/create-many`) es estable; solo cambian los valores de `positionY` que se le envían.

---

## Ralph-loop completion promise

> **DONE WHEN:** `docs/superpowers/evidence/2026-05-03-signature-layouts/progress.md` muestra `numeric_pass=PASS` Y `visual_pass=PASS` para las 13 layouts, AND `make build && make test && make lint` (in `core/`) pasaron sin errores en la última iteración, AND existe un commit firmado en la rama `fix/documenso-multi-signature-alignment` con todos los cambios.

**Ralph-loop input recomendado:**

```bash
/ralph-loop:ralph-loop "Lee docs/superpowers/plans/2026-05-03-documenso-signature-multi-layout-alignment.md y avanza la siguiente tarea pendiente. Si el progress.md indica que las 13 layouts pasan numérica y visualmente y el commit final está hecho, declara la completion promise: 'TODAS_LAS_13_LAYOUTS_DOCUMENSO_ALINEADAS'." --completion-promise "TODAS_LAS_13_LAYOUTS_DOCUMENSO_ALINEADAS"
```

Cada iteración debe: (1) leer `progress.md`, (2) tomar la primera fila pendiente, (3) ejecutar/ajustar, (4) actualizar `progress.md`, (5) commit y salir. Si todas las filas pasan, declarar la promise.

---

## Task 1: Crear rama de trabajo y estructura de evidencia

**Files:**
- Branch: `fix/documenso-multi-signature-alignment`
- Create: `docs/superpowers/evidence/2026-05-03-signature-layouts/progress.md`
- Create: `docs/superpowers/evidence/2026-05-03-signature-layouts/baseline/.gitkeep`
- Create: `docs/superpowers/evidence/2026-05-03-signature-layouts/final/.gitkeep`

- [ ] **Step 1: Crear rama desde `main`**

```bash
cd /Users/rendis/Documents/Projects/Libraries/doc-assembly
git fetch origin
git checkout -b fix/documenso-multi-signature-alignment origin/main
```

- [ ] **Step 2: Crear el archivo de tracking `progress.md`**

```markdown
# Multi-signature layout alignment — progress

| Layout              | Count | Numeric | Visual | Baseline screenshot                                  | Final screenshot                                  | Notes |
|---------------------|-------|---------|--------|------------------------------------------------------|---------------------------------------------------|-------|
| single-left         | 1     | PENDING | PENDING| baseline/single-left.png                             | final/single-left.png                             |       |
| single-center       | 1     | PENDING | PENDING| baseline/single-center.png                           | final/single-center.png                           |       |
| single-right        | 1     | PENDING | PENDING| baseline/single-right.png                            | final/single-right.png                            |       |
| dual-sides          | 2     | PENDING | PENDING| baseline/dual-sides.png                              | final/dual-sides.png                              |       |
| dual-center         | 2     | PENDING | PENDING| baseline/dual-center.png                             | final/dual-center.png                             |       |
| dual-left           | 2     | PENDING | PENDING| baseline/dual-left.png                               | final/dual-left.png                               |       |
| dual-right          | 2     | PENDING | PENDING| baseline/dual-right.png                              | final/dual-right.png                              |       |
| triple-row          | 3     | PENDING | PENDING| baseline/triple-row.png                              | final/triple-row.png                              |       |
| triple-pyramid      | 3     | PENDING | PENDING| baseline/triple-pyramid.png                          | final/triple-pyramid.png                          |       |
| triple-inverted     | 3     | PENDING | PENDING| baseline/triple-inverted.png                         | final/triple-inverted.png                         |       |
| quad-grid           | 4     | PENDING | PENDING| baseline/quad-grid.png                               | final/quad-grid.png                               |       |
| quad-top-heavy      | 4     | PENDING | PENDING| baseline/quad-top-heavy.png                          | final/quad-top-heavy.png                          |       |
| quad-bottom-heavy   | 4     | PENDING | PENDING| baseline/quad-bottom-heavy.png                       | final/quad-bottom-heavy.png                       |       |

Legend: PENDING / PASS / FAIL. Visual=PASS only when a human reviewer (or Playwright + numeric tolerance < 0.5%) confirms the signature box visibly sits on the line in the Documenso editor.
```

- [ ] **Step 3: Commit**

```bash
git add docs/superpowers/plans/2026-05-03-documenso-signature-multi-layout-alignment.md \
        docs/superpowers/evidence/2026-05-03-signature-layouts/
git commit -m "chore: scaffold multi-signature alignment plan and progress tracking"
```

---

## Task 2: Levantar stack local y verificar puertos

**Files:**
- Read: `docker-compose.dev.yml`, `docker-compose.documenso.yml`
- Modify: `core/.env.local` (si existe; si no, exportar variables inline)

- [ ] **Step 1: Verificar que Docker está corriendo y los puertos no están ocupados**

```bash
docker info >/dev/null 2>&1 && echo "docker OK" || { echo "Docker no corre" ; exit 1 ; }
for p in 5432 5433 3000 8025 8081; do
  if lsof -nP -iTCP:$p -sTCP:LISTEN >/dev/null 2>&1; then
    echo "port $p OCUPADO"; lsof -nP -iTCP:$p -sTCP:LISTEN
  else
    echo "port $p LIBRE"
  fi
done
```
Expected: 5432, 5433, 3000, 8025, 8081 libres. Si 8081 está ocupado, escoger otro puerto > 8082 y propagarlo a `DOC_ENGINE_SERVER_PORT`.

- [ ] **Step 2: Levantar stacks Docker**

```bash
docker compose -f docker-compose.dev.yml up -d
docker compose -f docker-compose.documenso.yml up -d
docker compose ps
```
Expected: contenedores `postgres-dev` (5432), `documenso-db` (5433), `documenso` (3000), `mailpit` (8025) en estado `running`. Esperar 15–30s a que `documenso` termine de migrar internamente.

- [ ] **Step 3: Migrar la app DB con Liquibase**

```bash
cd db
liquibase --defaults-file=liquibase-local.properties update
cd ..
```
Expected: `Liquibase command 'update' was executed successfully.`. Si pide password, exportar `DOC_ENGINE_DATABASE_PASSWORD=postgres` (la contraseña por defecto del compose dev difiere del `.env`).

- [ ] **Step 4: Build core y arrancar backend en :8081**

```bash
cd core
make wire && make build
DOC_ENGINE_SERVER_PORT=8081 \
DOC_ENGINE_WORKER_ENABLED=true \
DOC_ENGINE_WORKER_RUNTIME_ENVIRONMENT=local \
DOC_ENGINE_DATABASE_PASSWORD=postgres \
./bin/api &
echo $! > /tmp/doc-assembly-backend.pid
cd ..
sleep 5
curl -s http://localhost:8081/health
```
Expected: `200 OK` o JSON con `{"status":"ok"}`.

- [ ] **Step 5: Smoke test al frontend**

```bash
cd app
pnpm install
VITE_API_URL=http://localhost:8081/api/v1 VITE_USE_MOCK_AUTH=true pnpm dev &
echo $! > /tmp/doc-assembly-frontend.pid
cd ..
sleep 8
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:5173
```
Expected: `200`.

- [ ] **Step 6: Commit (si hubo cambios de configuración)**

```bash
git add -p
git commit -m "chore: configure local stack for multi-signature alignment runs" || echo "no changes"
```

---

## Task 3: Crear las 13 fixtures `.docml`

**Files:**
- Create: `scripts/signature-layouts/fixtures/<layout>.docml` × 13

- [ ] **Step 1: Escribir un fixture base reutilizable y derivar 13 variantes**

Usa exactamente este patrón. Sustituye `__LAYOUT__` por el nombre del layout y `__BLOCK__` por el bloque `@signature(...)` correspondiente. Solo varía: el título, el bloque `@signature`, los roles `---roles---` y la lista `|` debajo de la firma. El cuerpo del documento se mantiene idéntico para que el ancla de cada firma quede en la misma página y la única variable sea el layout.

```text
---meta---
title: Test alineación firma — __LAYOUT__
description: Fixture E2E para validar que el signature box queda sobre la línea en Documenso.
language: es
page: LETTER
margins: 72

---roles---
__ROLES__

---workflow---
mode: sequential

---content---
@center **TEST DE ALINEACIÓN — __LAYOUT__**

Este documento prueba que el signature box generado por Documenso queda visualmente sobre la línea de firma renderizada por Typst.

Texto de relleno para empujar la firma hacia el medio/abajo de la página y reproducir la alineación real:

[student_first_name|Nombre Estudiante] [student_first_last_name|Apellido].

Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.

__BLOCK__

**FIRMA ELECTRÓNICA**
Este documento es suscrito mediante firma electrónica simple, conforme a la Ley N° 19.799.

**Fecha:** [today|Fecha]
```

Para cada layout, los valores de `__ROLES__` y `__BLOCK__` son:

```text
single-left:
  ROLES = "apoderado: Apoderado/a [order:1]"
  BLOCK = "@signature(single-left, md)\n  | apoderado: Firma Apoderado/a | Apoderado/a"

single-center:
  ROLES = "apoderado: Apoderado/a [order:1]"
  BLOCK = "@signature(single-center, md)\n  | apoderado: Firma Apoderado/a | Apoderado/a"

single-right:
  ROLES = "apoderado: Apoderado/a [order:1]"
  BLOCK = "@signature(single-right, md)\n  | apoderado: Firma Apoderado/a | Apoderado/a"

dual-sides:
  ROLES = "apoderado: Apoderado/a [order:1]\nestudiante: Estudiante [order:2]"
  BLOCK = "@signature(dual-sides, md)\n  | apoderado: Firma Apoderado/a | Apoderado/a\n  | estudiante: Firma Estudiante | Estudiante"

dual-center:
  ROLES = igual a dual-sides
  BLOCK = "@signature(dual-center, md)\n  | apoderado: ... | ...\n  | estudiante: ... | ..."

dual-left:    misma forma con layout dual-left
dual-right:   misma forma con layout dual-right

triple-row:
  ROLES = "apoderado: Apoderado/a [order:1]\nestudiante: Estudiante [order:2]\ntestigo: Testigo [order:3]"
  BLOCK = "@signature(triple-row, md)\n  | apoderado: ...\n  | estudiante: ...\n  | testigo: ..."

triple-pyramid:    misma forma con layout triple-pyramid (3 roles)
triple-inverted:   misma forma con layout triple-inverted (3 roles)

quad-grid:
  ROLES = "apoderado: Apoderado/a [order:1]\nestudiante: Estudiante [order:2]\ntestigo: Testigo [order:3]\ndirector: Director [order:4]"
  BLOCK = "@signature(quad-grid, md)\n  | apoderado: ...\n  | estudiante: ...\n  | testigo: ...\n  | director: ..."

quad-top-heavy:    misma forma con layout quad-top-heavy (4 roles)
quad-bottom-heavy: misma forma con layout quad-bottom-heavy (4 roles)
```

Crea los 13 archivos. No es necesario un test unitario aquí; los fixtures se validarán por el harness E2E en Task 5.

- [ ] **Step 2: Validar que cada fixture compila a JSON con docml2json**

```bash
mkdir -p /tmp/sig-layouts-json
for f in scripts/signature-layouts/fixtures/*.docml; do
  out="/tmp/sig-layouts-json/$(basename "${f%.docml}").json"
  python3 scripts/docml2json/docml2json.py "$f" -o "$out" || { echo "FAIL $f"; exit 1; }
done
ls -1 /tmp/sig-layouts-json
```
Expected: 13 JSON sin errores.

- [ ] **Step 3: Commit**

```bash
git add scripts/signature-layouts/fixtures
git commit -m "test: add multi-signature layout docml fixtures (1-4 signatures)"
```

---

## Task 4: Unificar `convertFieldToProviderPosition` (DRY)

Esta tarea elimina la copia duplicada y deja una sola fuente de verdad — exigida porque la del paquete `document` es la que tiene test unitario, pero la que se ejecuta en runtime es la del paquete `riverqueue`. Una divergencia silenciosa fue lo que motivó memoizar este riesgo.

**Files:**
- Modify: `core/internal/core/service/document/document_service.go:269-294`
- Modify: `core/internal/infra/riverqueue/helpers.go:77-88`
- Modify: `core/internal/core/service/document/signature_position_test.go`

- [ ] **Step 1: Escribir test que falle: importar la función exportada desde el paquete `document` en un test ubicado en `riverqueue` para verificar que la copia local desapareció**

Crea `core/internal/infra/riverqueue/signature_position_dry_test.go`:

```go
package riverqueue_test

import (
	"testing"

	docsvc "doc-engine/internal/core/service/document"
	"doc-engine/internal/core/port"
)

// Garantiza que riverqueue NO duplica la lógica de conversión.
// Si esta prueba se rompe, alguien volvió a copiarla.
func TestConvertFieldToProviderPositionUsesDocumentPackage(t *testing.T) {
	f := port.SignatureField{
		PositionX:  0, PositionY: 0,
		Width:      30, Height: 8,
		PDFPointX:  100, PDFPointY: 600,
		PDFAnchorW: 10, PDFPageW: 612, PDFPageH: 792,
	}
	x, y := docsvc.ConvertFieldToProviderPosition(f)
	if x < 0 || x > 100-f.Width {
		t.Fatalf("posX out of range: %v", x)
	}
	if y < 0 || y > 100-f.Height {
		t.Fatalf("posY out of range: %v", y)
	}
}
```

- [ ] **Step 2: Verificar que el test no compila aún**

```bash
cd core && go test ./internal/infra/riverqueue/... -run TestConvertFieldToProviderPositionUsesDocumentPackage 2>&1 | head -20
```
Expected: error de compilación tipo `undefined: docsvc.ConvertFieldToProviderPosition` (porque la función está privada).

- [ ] **Step 3: Exportar la función en `document_service.go`**

Renombra el método/función privado a uno exportado. El cambio mínimo:

```go
// ConvertFieldToProviderPosition translates an extracted PDF anchor into Documenso
// percentage coordinates. The returned (posX, posY) describe the field's TOP-LEFT
// corner in % of page width/height. The bottom of the field aligns with the rendered
// signature line (anchor Y).
func ConvertFieldToProviderPosition(f port.SignatureField) (posX, posY float64) {
	posX, posY = f.PositionX, f.PositionY
	if f.PDFPageW > 0 && f.PDFPageH > 0 {
		anchorCenterPct := ((f.PDFPointX + f.PDFAnchorW/2) / f.PDFPageW) * 100
		posX = anchorCenterPct - f.Width/2
		posY = 100 - ((f.PDFPointY / f.PDFPageH) * 100)
		posY -= f.Height
	}
	posX = max(0, min(posX, 100-f.Width))
	posY = max(0, min(posY, 100-f.Height))
	return posX, posY
}
```

Si había un wrapper privado `convertFieldToProviderPosition` con la misma firma dentro del paquete `document`, sustitúyelo por `return ConvertFieldToProviderPosition(f)` o reemplaza directamente todas las llamadas.

- [ ] **Step 4: Reemplazar la copia en `riverqueue/helpers.go` por una llamada al paquete `document`**

```go
import (
	docsvc "doc-engine/internal/core/service/document"
)

func convertFieldToProviderPosition(f port.SignatureField) (posX, posY float64) {
	return docsvc.ConvertFieldToProviderPosition(f)
}
```

Si esto introduce un ciclo de imports (riverqueue ↔ document), mover la función a `core/internal/core/port/signature_position.go` con la misma firma y que ambos paquetes la importen. Ese movimiento es preferible si el ciclo aparece.

- [ ] **Step 5: Ajustar el test existente en `document` para que use el nombre exportado**

Editar `signature_position_test.go` para llamar `ConvertFieldToProviderPosition` (mayúscula).

- [ ] **Step 6: Correr los tests**

```bash
cd core && go test ./internal/core/service/document/... ./internal/infra/riverqueue/... -run 'Convert|Signature' -v
```
Expected: PASS.

- [ ] **Step 7: Verificar build, wire, lint**

```bash
cd core && make wire && make build && make lint
```
Expected: sin errores.

- [ ] **Step 8: Commit**

```bash
git add core/
git commit -m "refactor: unify ConvertFieldToProviderPosition into document package"
```

---

## Task 5: Crear el harness E2E `run_e2e.sh`

Este script es la columna vertebral de la verificación numérica. Para cada layout: limpia DB de prueba, sube el JSON, crea documento, dispara `proceed-to-signing`, espera River, y consulta Documenso DB para extraer `positionY` y `height` del campo de cada role; luego compara contra el anchor `Y%` extraído del PDF (que está en la tabla de signature_fields).

**Files:**
- Create: `scripts/signature-layouts/run_e2e.sh`
- Create: `scripts/signature-layouts/lib/api.sh` (helpers HTTP)
- Create: `scripts/signature-layouts/lib/db.sh` (helpers SQL)

- [ ] **Step 1: Crear `lib/api.sh` con helpers**

```bash
#!/usr/bin/env bash
# Reusable HTTP helpers. Requires: curl, jq, env vars BASE_URL, TENANT_ID, WORKSPACE_ID, JWT.
api() {
  local method="$1" path="$2" body="${3:-}"
  if [[ -n "$body" ]]; then
    curl -sS -X "$method" "$BASE_URL$path" \
      -H "Authorization: Bearer $JWT" \
      -H "X-Tenant-ID: $TENANT_ID" \
      -H "X-Workspace-ID: $WORKSPACE_ID" \
      -H "Content-Type: application/json" \
      -d "$body"
  else
    curl -sS -X "$method" "$BASE_URL$path" \
      -H "Authorization: Bearer $JWT" \
      -H "X-Tenant-ID: $TENANT_ID" \
      -H "X-Workspace-ID: $WORKSPACE_ID"
  fi
}
```

- [ ] **Step 2: Crear `lib/db.sh`**

```bash
#!/usr/bin/env bash
# Documenso DB query helpers. PGURL_DOCUMENSO defaults to local docker.
: "${PGURL_DOCUMENSO:=postgres://documenso:postgres@localhost:5433/documenso}"
: "${PGURL_APP:=postgres://postgres:postgres@localhost:5432/doc_engine}"

query_documenso() { psql "$PGURL_DOCUMENSO" -At -c "$1"; }
query_app()       { psql "$PGURL_APP"       -At -c "$1"; }

# Returns CSV: roleId,positionY,height,positionY+height for a given envelope.
fetch_field_positions() {
  local envelope_id="$1"
  query_documenso "SELECT recipient_id::text, \"positionY\", height, (\"positionY\" + height) FROM \"Field\" WHERE \"envelopeId\" = '$envelope_id' ORDER BY recipient_id;"
}
```

- [ ] **Step 3: Crear `run_e2e.sh`**

```bash
#!/usr/bin/env bash
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../.." && pwd)"
source "$HERE/lib/api.sh"
source "$HERE/lib/db.sh"

: "${BASE_URL:=http://localhost:8081/api/v1}"
: "${JWT:?export JWT (use VITE_USE_MOCK_AUTH=true mock token, see app/src/lib/auth.ts)}"
: "${TENANT_ID:?export TENANT_ID}"
: "${WORKSPACE_ID:?export WORKSPACE_ID}"

LAYOUTS=(single-left single-center single-right \
         dual-sides dual-center dual-left dual-right \
         triple-row triple-pyramid triple-inverted \
         quad-grid quad-top-heavy quad-bottom-heavy)

OUT="$ROOT/docs/superpowers/evidence/2026-05-03-signature-layouts/numeric_results.csv"
echo "layout,role,positionY,height,bottom_pct,anchor_top_pct,delta_pct,verdict" > "$OUT"

for layout in "${LAYOUTS[@]}"; do
  json="/tmp/sig-layouts-json/$layout.json"
  python3 "$ROOT/scripts/docml2json/docml2json.py" "$ROOT/scripts/signature-layouts/fixtures/$layout.docml" -o "$json"

  # 1. Crear template
  template_id=$(api POST /document-types/<TYPE_ID>/templates "$(jq -n --arg n "$layout" --slurpfile c "$json" \
      '{name:$n, version:"1.0.0", content:$c[0], pageConfig:{formatId:"A4", orientation:"PORTRAIT"}}')" \
      | jq -r '.id')

  # 2. Crear documento
  doc_id=$(api POST /documents "$(jq -n --arg t "$template_id" \
      '{templateId:$t, signers:[{roleId:"apoderado", email:"a@example.com", name:"A"}]}')" \
      | jq -r '.id')

  # 3. Public access + proceed-to-signing
  token=$(api POST /public/doc/$doc_id/request-access '{"email":"a@example.com"}' | jq -r '.devToken')
  api POST /public/sign/$token/proceed | jq -r '.signingUrl' > /dev/null

  # 4. Wait for River
  sleep 8

  # 5. Recoger positionY/height por role desde Documenso DB
  envelope_id=$(query_app "SELECT provider_document_id FROM execution.signing_attempts WHERE document_id='$doc_id' ORDER BY created_at DESC LIMIT 1;")
  fields_csv=$(fetch_field_positions "$envelope_id")

  # 6. Recoger anchor Y% por role desde la app DB
  while IFS='|' read -r role_id posY height bottom_pct; do
    anchor_pct=$(query_app "SELECT 100 - ((pdf_point_y / pdf_page_h) * 100) FROM execution.signature_fields WHERE attempt_id IN (SELECT id FROM execution.signing_attempts WHERE document_id='$doc_id' ORDER BY created_at DESC LIMIT 1) AND role_id='$role_id';")
    delta=$(python3 -c "print(abs($bottom_pct - $anchor_pct))")
    verdict=$([[ $(python3 -c "print(int($delta < 0.5))") == 1 ]] && echo PASS || echo FAIL)
    echo "$layout,$role_id,$posY,$height,$bottom_pct,$anchor_pct,$delta,$verdict" >> "$OUT"
  done <<< "$fields_csv"
done

cat "$OUT"
```

> **Nota:** los placeholders `<TYPE_ID>` se resuelven en Step 4 corriendo un `bootstrap.sh` que crea tenant/workspace/document_type una sola vez y exporta los IDs. Si el repo ya tiene un seeder, úsalo.

- [ ] **Step 4: Crear `scripts/signature-layouts/bootstrap.sh`**

```bash
#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/lib/api.sh"
TENANT_ID=$(api POST /tenants '{"name":"sig-test","slug":"sig-test"}' | jq -r '.id')
WORKSPACE_ID=$(api POST /workspaces "$(jq -n --arg t "$TENANT_ID" '{tenantId:$t,name:"sig-ws"}')" | jq -r '.id')
TYPE_ID=$(api POST /document-types "$(jq -n --arg w "$WORKSPACE_ID" '{workspaceId:$w,name:"sig-type",code:"SIG"}')" | jq -r '.id')
echo "export TENANT_ID=$TENANT_ID"
echo "export WORKSPACE_ID=$WORKSPACE_ID"
echo "export TYPE_ID=$TYPE_ID"
```

- [ ] **Step 5: Hacer ejecutables y commit**

```bash
chmod +x scripts/signature-layouts/*.sh scripts/signature-layouts/lib/*.sh
git add scripts/signature-layouts/
git commit -m "test: add E2E harness for multi-signature layout numeric verification"
```

---

## Task 6: Reproducir bug — capturar baseline numérico y screenshots

Esta tarea es OBLIGATORIA antes de cualquier fix. Si todos los layouts pasan en el baseline, el bug ya estaba resuelto y solo queda confirmar visualmente.

**Files:**
- Update: `docs/superpowers/evidence/2026-05-03-signature-layouts/progress.md`
- Create: `docs/superpowers/evidence/2026-05-03-signature-layouts/baseline/<layout>.png` × 13

- [ ] **Step 1: Bootstrap y correr el harness numérico**

```bash
eval "$(scripts/signature-layouts/bootstrap.sh)"
scripts/signature-layouts/run_e2e.sh
```
Expected: `numeric_results.csv` con una fila por (layout, role). Identificar qué (layout, role) tienen `verdict=FAIL`.

- [ ] **Step 2: Tomar screenshots de baseline en Documenso por cada envelope**

Por cada layout, abre Documenso (`http://localhost:3000/sign/<token>`) en Playwright MCP (`mcp__plugin_playwright_playwright__browser_navigate`), espera el iframe del PDF, y toma screenshot con `mcp__plugin_playwright_playwright__browser_take_screenshot` guardándolo en `docs/superpowers/evidence/2026-05-03-signature-layouts/baseline/<layout>.png`. Si el harness ya generó URLs en `numeric_results.csv` añade una columna `signing_url`.

```bash
# Pseudo-loop — invocar Playwright MCP por cada layout
for layout in single-left single-center ... ; do
  url=$(grep "$layout" numeric_results.csv | head -1 | awk -F, '{print $LAST}')
  # mcp__plugin_playwright_playwright__browser_navigate url=$url
  # mcp__plugin_playwright_playwright__browser_take_screenshot filename=baseline/$layout.png
done
```

- [ ] **Step 3: Llenar `progress.md` con verdict numérico y dejar `Visual=PENDING` para revisión**

Actualizar la tabla con `Numeric=PASS|FAIL` por layout (FAIL si CUALQUIER role del layout falló). Visual queda PENDING hasta verificación humana o tolerance check.

- [ ] **Step 4: Commit del baseline**

```bash
git add docs/superpowers/evidence/2026-05-03-signature-layouts/
git commit -m "test: capture baseline screenshots and numeric results for 13 layouts"
```

---

## Task 7: Iteración ralph-loop — ajustar layouts FAIL uno por uno

Esta es la tarea iterativa. Cada vuelta del ralph-loop ejecuta los siguientes pasos sobre EL PRIMER layout con `Numeric=FAIL` o `Visual=FAIL` en `progress.md`. Si todos pasan, declara la completion promise.

**Algoritmo por iteración (bite-sized):**

- [ ] **Step 1: Leer `progress.md` y elegir el primer layout pendiente o fallido**

```bash
LAYOUT=$(awk -F'|' 'NR>2 && ($3 ~ /FAIL|PENDING/ || $4 ~ /FAIL|PENDING/) {gsub(/ /,"",$2); print $2; exit}' \
         docs/superpowers/evidence/2026-05-03-signature-layouts/progress.md)
echo "Trabajando en: $LAYOUT"
```
Si vacío → ir a Task 9 (verificación final).

- [ ] **Step 2: Inspeccionar el delta numérico de ese layout**

```bash
grep "^$LAYOUT," docs/superpowers/evidence/2026-05-03-signature-layouts/numeric_results.csv
```
Lectura: `delta_pct` indica cuántos puntos % se desvía el `bottom_pct` (campo Documenso) respecto del `anchor_top_pct` (línea PDF). Si `delta < 0.5%` se considera PASS numérico; visualmente se verifica con screenshot.

- [ ] **Step 3: Decidir tipo de ajuste**

| Síntoma                                                          | Causa probable                                         | Ajuste                                                                                              |
|------------------------------------------------------------------|--------------------------------------------------------|----------------------------------------------------------------------------------------------------|
| Todos los roles del layout fallan con MISMO delta y MISMO signo | Offset constante mal calibrado para el layout          | Subir/bajar `anchorLineOffsetPt` SOLO para ese layout vía un map `layoutAnchorOffsetPt`            |
| Solo roles de la fila inferior de un split layout fallan         | `renderSignatureGroup` no propaga el slot superior     | Añadir `#v(slotHeightPt)` al inicio del segundo grupo en `renderSplitLayout`                       |
| Roles en columnas externas (left/right) desviados en X           | `pdftotext -bbox` reporta xMin del primer carácter    | Usar `(xMin+xMax)/2` para anchorCenter (ya implementado; revisar `anchor_extractor.go:178-194`)   |
| Algún role falta en `numeric_results.csv`                       | Anchor no extraído (texto no encontrado)              | El anchor `__sig_<label>__` chocó con caracteres espaciales del label; revisar `getAnchorString` |

- [ ] **Step 4: Aplicar el ajuste mínimo**

Ejemplo si la causa es "offset por layout":

```go
// En typst_converter_impl.go, encima de renderTypstSignatureItemContent
var layoutAnchorOffsetPt = map[string]float64{
    "quad-grid":         22.0,
    "quad-top-heavy":    22.0,
    "quad-bottom-heavy": 22.0,
    // resto usa el default 20.0
}

func (c *typstConverter) anchorOffsetPt(layout string) float64 {
    if v, ok := layoutAnchorOffsetPt[layout]; ok { return v }
    return 20.0
}
```

Y propagar el `layout` a `renderTypstSignatureItemContent` (cambiar firma para aceptar `layout string`). Mantén el cambio quirúrgico: solo lo que el síntoma exige.

- [ ] **Step 5: Añadir test unitario que cubra el caso del layout**

En `core/internal/core/service/rendering/pdfrenderer/typst_converter_test.go` añade:

```go
func TestRenderSignatureBlock_<Layout>_AnchorOffset(t *testing.T) {
    c := newConverter()
    out := c.signature(portabledoc.Node{Attrs: map[string]any{
        "layout": "<layout-string>",
        "count":  <N>,
        "signatures": []any{ /* N items con roleId */ },
    }})
    // Assert: el output contiene N anchors, cada uno con dy: <expected>pt
    if !strings.Contains(out, "dy: 22.0pt") { t.Fatal("expected new offset") }
}
```

Y en `signature_position_test.go` agrega un caso table-driven:

```go
{name: "<layout> aligns bottom to anchor", field: port.SignatureField{
    PDFPointX: 100, PDFPointY: 600, PDFAnchorW: 10,
    PDFPageW: 612, PDFPageH: 792, Width: 30, Height: 8,
}, expectBottomCloseTo: 100 - (600.0/792.0)*100},
```

- [ ] **Step 6: Build + lint + tests focalizados**

```bash
cd core && go test ./internal/core/service/rendering/pdfrenderer/... \
  ./internal/core/service/document/... -run "Signature" -v
make build && make lint
```
Expected: PASS.

- [ ] **Step 7: Re-correr el harness solo para este layout**

```bash
LAYOUTS_FILTER="$LAYOUT" scripts/signature-layouts/run_e2e.sh
```
(modifica el harness para aceptar `LAYOUTS_FILTER` y procesar solo ese layout cuando esté seteado).

Expected: la(s) fila(s) del CSV con `verdict=PASS` para todos los roles.

- [ ] **Step 8: Tomar screenshot final con Playwright MCP**

Guardar en `docs/superpowers/evidence/2026-05-03-signature-layouts/final/<layout>.png`. Comparar visualmente con el baseline.

- [ ] **Step 9: Actualizar `progress.md` para ese layout (Numeric=PASS, Visual=PASS)**

- [ ] **Step 10: Commit y salir de la iteración**

```bash
git add core/ scripts/signature-layouts/ docs/superpowers/evidence/
git commit -m "fix: align Documenso signature box for $LAYOUT layout"
```

Ralph-loop reinyectará el prompt y la siguiente iteración tomará el próximo pendiente.

---

## Task 8: Salvaguardas de regresión — test que cubra los 13 layouts en una sola tabla

Después de que las 13 iteraciones de Task 7 hayan pasado individualmente, consolidar en un test table-driven para evitar regresiones futuras.

**Files:**
- Modify: `core/internal/core/service/document/signature_position_test.go`

- [ ] **Step 1: Escribir el test consolidado**

```go
func TestConvertFieldToProviderPosition_AllLayouts(t *testing.T) {
    cases := []struct {
        name       string
        f          port.SignatureField
        wantBottom float64 // top% + height; debe coincidir con 100 - (PDFPointY/PDFPageH)*100
    }{
        {"single-center", port.SignatureField{PDFPointX:300,PDFPointY:600,PDFAnchorW:10,PDFPageW:612,PDFPageH:792,Width:30,Height:8}, 100 - (600.0/792.0)*100},
        // ... 12 más, una por layout, con coordenadas representativas observadas en run_e2e
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            _, posY := docsvc.ConvertFieldToProviderPosition(tc.f)
            got := posY + tc.f.Height
            if math.Abs(got-tc.wantBottom) > 0.01 {
                t.Fatalf("bottom mismatch: got %v want %v", got, tc.wantBottom)
            }
        })
    }
}
```

- [ ] **Step 2: Run**

```bash
cd core && go test ./internal/core/service/document/ -run TestConvertFieldToProviderPosition_AllLayouts -v
```
Expected: PASS para 13 sub-tests.

- [ ] **Step 3: Commit**

```bash
git add core/internal/core/service/document/signature_position_test.go
git commit -m "test: consolidate 13-layout regression coverage for signature alignment"
```

---

## Task 9: Verificación final completa y checklist del repo

**Files:** sin cambios; solo verifica.

- [ ] **Step 1: Checklist obligatorio del repo (de `CLAUDE.md`)**

```bash
cd core && make wire && make build && make test && make lint
go build -tags=integration ./...
make test-integration  # requiere Docker; verifica River, attempts, signing path
```
Expected: todo PASS.

- [ ] **Step 2: Re-correr el harness E2E completo**

```bash
scripts/signature-layouts/run_e2e.sh
awk -F, 'NR>1 && $NF=="FAIL"' docs/superpowers/evidence/2026-05-03-signature-layouts/numeric_results.csv | wc -l
```
Expected: `0` filas FAIL.

- [ ] **Step 3: Confirmar que `progress.md` tiene 13 PASS/PASS**

```bash
grep -c 'PASS *| *PASS' docs/superpowers/evidence/2026-05-03-signature-layouts/progress.md
```
Expected: `13`.

- [ ] **Step 4: Commit de cierre y push**

```bash
git add -A
git commit -m "docs: close multi-signature alignment evidence (13/13 layouts PASS)" || echo "no changes"
git push -u origin fix/documenso-multi-signature-alignment
```

- [ ] **Step 5: Crear PR (no auto-merge; requiere revisión humana)**

```bash
gh pr create --title "fix: align Documenso signature box for all 13 layouts (1-4 signatures)" \
  --body "$(cat <<'EOF'
## Summary
- Unifica la conversión PDF → Documenso (`ConvertFieldToProviderPosition`) en el paquete `document` (elimina copia divergente en `riverqueue`).
- Calibra el anchor Typst y/o el offset de altura por layout cuando hace falta, garantizando que el bottom del field Documenso quede sobre la línea renderizada en las 13 layouts (single/dual/triple/quad).
- Añade 13 fixtures `.docml`, harness E2E (`scripts/signature-layouts/run_e2e.sh`), test de regresión table-driven, y screenshots before/after.

## Evidence
- `docs/superpowers/evidence/2026-05-03-signature-layouts/numeric_results.csv` (todas las filas `verdict=PASS`)
- `docs/superpowers/evidence/2026-05-03-signature-layouts/progress.md` (13/13 PASS/PASS)
- `docs/superpowers/evidence/2026-05-03-signature-layouts/baseline/*.png` y `final/*.png`

## Test plan
- [x] `make build && make test && make lint` en `core/`
- [x] `go build -tags=integration ./...` en `core/`
- [x] `make test-integration` en `core/`
- [x] `scripts/signature-layouts/run_e2e.sh` con stack local Docker arriba
- [x] Verificación visual UI por layout en Documenso (`http://localhost:3000`)
EOF
)"
```

- [ ] **Step 6: Declarar la completion promise (ralph-loop)**

Output literal: `TODAS_LAS_13_LAYOUTS_DOCUMENSO_ALINEADAS`.

---

## Task 10: Limpieza local

- [ ] **Step 1: Detener procesos y contenedores**

```bash
kill $(cat /tmp/doc-assembly-backend.pid 2>/dev/null) 2>/dev/null || true
kill $(cat /tmp/doc-assembly-frontend.pid 2>/dev/null) 2>/dev/null || true
docker compose -f docker-compose.documenso.yml down
docker compose -f docker-compose.dev.yml down
```

- [ ] **Step 2: Borrar artefactos locales sensibles**

```bash
rm -rf /tmp/sig-layouts-json /tmp/doc-assembly-backend.pid /tmp/doc-assembly-frontend.pid
```

- [ ] **Step 3: Guardar memoria (Engram)**

Llama `mem_save` con `type=bugfix`, `topic_key=bugfix/documenso-multi-signature-alignment`, contenido `{What, Why, Where, Learned}` describiendo qué offset/ajuste resolvió cada layout problemático. Luego `mem_session_summary` con goal/discoveries/files.

---

## Notes for the executing agent

1. **Ralph-loop discipline:** Cada iteración hace UN layout, commit, sale. NO intentes acelerar saltando a Task 8/9 antes de que Task 7 haya cubierto los 13 layouts.
2. **Baseline obligatorio:** No empieces a ajustar código sin haber capturado los screenshots de baseline en Task 6. Sin baseline no hay forma de demostrar mejora.
3. **Fix mínimo:** El offset `dy:20pt` y `posY -= height` ya resolvieron 1 firma. Probablemente la mayoría de layouts ya pasan. NO inventes ajustes para layouts que pasan en baseline.
4. **Riesgo de import cycle:** Si exportar `ConvertFieldToProviderPosition` en el paquete `document` crea un ciclo con `riverqueue`, mueve la función a `core/internal/core/port/signature_position.go` (Task 4 Step 4 contempla este caso).
5. **Documenso DB schema:** la tabla relevante es `Field` con columnas `envelopeId`, `recipientId`, `positionX`, `positionY`, `width`, `height`. No usar el cliente HTTP de Documenso para verificar; ir a la DB directamente para evitar latencia y autenticación.
6. **Mock auth:** Para `JWT` exporta el token mock generado por `VITE_USE_MOCK_AUTH=true`. Si no existe un endpoint `/auth/dev-token`, ver `app/src/lib/auth.ts` para el shape esperado y firmarlo localmente con HS256 contra `DOC_ENGINE_AUTH_JWKS_URL` mock.
7. **No tocar `db/src/`:** Si el harness necesita una columna nueva, escríbela como observación y termina la iteración. El usuario decide si crear el changeset.
