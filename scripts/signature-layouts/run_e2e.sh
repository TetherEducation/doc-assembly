#!/usr/bin/env bash
# run_e2e.sh — E2E harness for signature layout numeric verification.
#
# Prerequisites:
#   eval "$(bash scripts/signature-layouts/bootstrap.sh)"
#   (sets TENANT_ID, WORKSPACE_ID, DOC_TYPE_ID, AUTH_TOKEN, API_BASE)
#
# Optional env overrides:
#   LAYOUTS_FILTER=<comma-separated layout names>  — run only these layouts
#   WAIT_TIMEOUT=<seconds>                         — per-layout River wait (default: 60)
#   CSV_OUT=<path>                                 — output CSV path
#   API_BASE=<url>                                 — backend base URL
#
# Output: CSV at docs/superpowers/evidence/2026-05-03-signature-layouts/numeric_results.csv

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

source "${SCRIPT_DIR}/lib/api.sh"
source "${SCRIPT_DIR}/lib/db.sh"

# ---- Config ----------------------------------------------------------------
API_BASE="${API_BASE:-http://localhost:8081/api/v1}"
AUTH_TOKEN="${AUTH_TOKEN:-dummy}"
WAIT_TIMEOUT="${WAIT_TIMEOUT:-60}"
DELTA_THRESHOLD="0.5"
FIXTURES_DIR="${FIXTURES_DIR:-${SCRIPT_DIR}/fixtures}"
DOCML2JSON="${REPO_ROOT}/scripts/docml2json/docml2json.py"
PATCH_CONTENT="${SCRIPT_DIR}/patch_content.py"
CSV_DIR="${REPO_ROOT}/docs/superpowers/evidence/2026-05-03-signature-layouts"
CSV_OUT="${CSV_OUT:-${CSV_DIR}/numeric_results.csv}"
URLS_TSV="${URLS_TSV:-${CSV_DIR}/signing_urls.tsv}"

# All 13 layouts
ALL_LAYOUTS=(
  single-center
  single-left
  single-right
  dual-center
  dual-left
  dual-right
  dual-sides
  triple-row
  triple-pyramid
  triple-inverted
  quad-grid
  quad-top-heavy
  quad-bottom-heavy
)

# Terminal statuses that mean we can proceed to read data.
# SIGNING_READY = Documenso has the envelope and is waiting for signatures (success).
TERMINAL_OK_STATUSES=("SIGNING_READY" "COMPLETED")
# Terminal statuses that mean we should fail this layout.
TERMINAL_ERR_STATUSES=("FAILED_PERMANENT" "INVALIDATED" "REQUIRES_REVIEW" "CANCELLED" "SUPERSEDED" "DECLINED")

# Required env vars
: "${TENANT_ID:?TENANT_ID not set — run: eval \"\$(bootstrap.sh)\"}"
: "${WORKSPACE_ID:?WORKSPACE_ID not set — run: eval \"\$(bootstrap.sh)\"}"
: "${DOC_TYPE_ID:?DOC_TYPE_ID not set — run: eval \"\$(bootstrap.sh)\"}"

mkdir -p "$CSV_DIR"

# ---- CSV header ------------------------------------------------------------
if [[ ! -f "$CSV_OUT" ]]; then
  echo "layout,role,positionY,height,bottom_pct,line_top_pct,delta_pct,verdict" > "$CSV_OUT"
fi

# ---- Signing URLs TSV header -----------------------------------------------
if [[ ! -f "$URLS_TSV" ]]; then
  printf 'layout\trecipient_email\tsigning_url\n' > "$URLS_TSV"
fi

# ---- Layout filter ---------------------------------------------------------
if [[ -n "${LAYOUTS_FILTER:-}" ]]; then
  IFS=',' read -ra LAYOUTS <<< "$LAYOUTS_FILTER"
else
  LAYOUTS=("${ALL_LAYOUTS[@]}")
fi

# ---- Helpers ---------------------------------------------------------------

# wait_for_attempt_status <document_id> <timeout_secs>
# Polls every 2s until attempt reaches a terminal status or timeout.
# Prints ONLY the final status to stdout; logs progress to stderr.
wait_for_attempt_status() {
  local doc_id="$1"
  local timeout_secs="$2"

  local deadline=$((SECONDS + timeout_secs))
  local last_status=""
  while [[ $SECONDS -lt $deadline ]]; do
    local status
    status=$(get_attempt_status "$doc_id" | tr -d '[:space:]')
    if [[ "$status" != "$last_status" ]]; then
      echo "  [attempt] status: ${status}" >&2
      last_status="$status"
    fi
    for s in "${TERMINAL_OK_STATUSES[@]}" "${TERMINAL_ERR_STATUSES[@]}"; do
      if [[ "$status" == "$s" ]]; then
        echo "$status"
        return 0
      fi
    done
    sleep 2
  done
  echo "TIMEOUT"
  return 1
}

# is_ok_status <status>
is_ok_status() {
  local s="$1"
  for ok in "${TERMINAL_OK_STATUSES[@]}"; do
    [[ "$s" == "$ok" ]] && return 0
  done
  return 1
}

# ---- Summary tracking ------------------------------------------------------
PASS_COUNT=0
FAIL_COUNT=0
ERROR_COUNT=0
LAYOUT_RESULTS_KEYS=""
LAYOUT_RESULTS_VALS=""

# set_layout_result <layout> <result>
set_layout_result() {
  LAYOUT_RESULTS_KEYS="${LAYOUT_RESULTS_KEYS}${1}|"
  LAYOUT_RESULTS_VALS="${LAYOUT_RESULTS_VALS}${2}|"
}

# get_layout_result <layout> — prints result or SKIPPED.
# Bash 3.2 (macOS default) lacks `declare -A`, so we use parallel `|`-delimited
# strings. Layout names are controlled constants (no `|`), so the workaround is safe.
get_layout_result() {
  local key="$1"
  local i=0
  local saved_IFS="$IFS"
  IFS='|'
  for k in $LAYOUT_RESULTS_KEYS; do
    i=$((i+1))
    if [[ "$k" == "$key" ]]; then
      IFS="$saved_IFS"
      echo "$LAYOUT_RESULTS_VALS" | cut -d'|' -f"$i"
      return
    fi
  done
  IFS="$saved_IFS"
  echo "SKIPPED"
}

# ---- Main loop -------------------------------------------------------------
for LAYOUT in "${LAYOUTS[@]}"; do
  FIXTURE="${FIXTURES_DIR}/${LAYOUT}.docml"
  if [[ ! -f "$FIXTURE" ]]; then
    log "SKIP: fixture not found: $FIXTURE"
    set_layout_result "$LAYOUT" "SKIPPED"
    continue
  fi

  log "============================================================"
  log "Layout: ${LAYOUT}"
  log "============================================================"

  LAYOUT_STATUS="PASS"

  # Isolate each layout in a subshell so errors are catchable
  (
    set -euo pipefail

    # 1. Convert docml → JSON, patch signer roles
    TMP_DIR=$(mktemp -d)
    trap "rm -rf '$TMP_DIR'" EXIT

    RAW_JSON="${TMP_DIR}/${LAYOUT}.json"
    PATCHED_JSON="${TMP_DIR}/${LAYOUT}-patched.json"

    python3 "$DOCML2JSON" "$FIXTURE" -o "$RAW_JSON" 2>/dev/null
    python3 "$PATCH_CONTENT" "$RAW_JSON" "$PATCHED_JSON" 2>/dev/null

    log "[${LAYOUT}] Content converted and patched"

    # 1b. Get or create a per-layout document type (avoids force-unlink conflicts)
    TS=$(date -u +%s)
    # Use a short deterministic code from the layout name (max 20 chars for code)
    LAYOUT_DT_CODE=$(echo "${LAYOUT}" | tr '[:lower:]-' '[:upper:]_' | cut -c1-18)
    # Search by code directly to avoid pagination issues with the list endpoint
    DT_LIST_RESP=$(api_get "/api/v1/tenant/document-types?q=${LAYOUT_DT_CODE}")
    LAYOUT_DT_ID=$(echo "$DT_LIST_RESP" | jq -r ".data[]? | select(.code == \"${LAYOUT_DT_CODE}\") | .id" 2>/dev/null | head -1)
    if [[ -z "$LAYOUT_DT_ID" ]]; then
      CREATE_DT_RESP=$(api_post "/api/v1/tenant/document-types" \
        "{\"code\":\"${LAYOUT_DT_CODE}\",\"name\":{\"en\":\"${LAYOUT}\",\"es\":\"${LAYOUT}\"}}")
      LAYOUT_DT_ID=$(echo "$CREATE_DT_RESP" | jq -r '.id // empty')
      if [[ -z "$LAYOUT_DT_ID" ]]; then
        echo "ERROR[${LAYOUT}]: failed to create document type: $CREATE_DT_RESP" >&2
        exit 1
      fi
      log "[${LAYOUT}] Created doc type: ${LAYOUT_DT_ID}"
    else
      log "[${LAYOUT}] Reusing doc type: ${LAYOUT_DT_ID}"
    fi

    # 2. Create template
    TMPL_RESP=$(api_post "/api/v1/content/templates" \
      "{\"title\":\"E2E ${LAYOUT} ${TS}\"}")
    TEMPLATE_ID=$(echo "$TMPL_RESP" | jq -r '.template.id // empty')
    VERSION_ID=$(echo "$TMPL_RESP" | jq -r '.initialVersion.id // empty')
    if [[ -z "$TEMPLATE_ID" || -z "$VERSION_ID" ]]; then
      echo "ERROR[${LAYOUT}]: failed to create template: $TMPL_RESP" >&2
      exit 1
    fi
    log "[${LAYOUT}] Template: ${TEMPLATE_ID}, Version: ${VERSION_ID}"

    # 3. Set content structure on version
    UPDATE_PAYLOAD=$(python3 -c "
import json
with open('${PATCHED_JSON}') as f:
    content = json.load(f)
print(json.dumps({'contentStructure': content}))
")
    UPDATE_RESP=$(api_put "/api/v1/content/templates/${TEMPLATE_ID}/versions/${VERSION_ID}" \
      "$UPDATE_PAYLOAD")
    if ! echo "$UPDATE_RESP" | jq -e '.id' >/dev/null 2>&1; then
      echo "ERROR[${LAYOUT}]: failed to set content: $UPDATE_RESP" >&2
      exit 1
    fi
    log "[${LAYOUT}] Content set"

    # 4. Assign document type (force=true to override any existing assignment from prior runs)
    DT_RESP=$(api PUT "/api/v1/content/templates/${TEMPLATE_ID}/document-type" \
      -d "{\"documentTypeId\":\"${LAYOUT_DT_ID}\",\"force\":true}")
    if ! echo "$DT_RESP" | jq -e '.template' >/dev/null 2>&1; then
      echo "ERROR[${LAYOUT}]: failed to assign document type: $DT_RESP" >&2
      exit 1
    fi

    log "[${LAYOUT}] Document type assigned"

    # 5. Publish version
    PUB_RESP=$(api_post "/api/v1/content/templates/${TEMPLATE_ID}/versions/${VERSION_ID}/publish" '{}')
    if echo "$PUB_RESP" | jq -e '.error' >/dev/null 2>&1; then
      echo "ERROR[${LAYOUT}]: publish failed: $PUB_RESP" >&2
      exit 1
    fi
    log "[${LAYOUT}] Version published"

    # 6. Fetch DB signer role IDs from the published version
    VER_DETAIL=$(api_get "/api/v1/content/templates/${TEMPLATE_ID}/versions/${VERSION_ID}")
    DB_ROLES_JSON=$(echo "$VER_DETAIL" | jq -c '[.signerRoles[] | {id, signerOrder}]')
    log "[${LAYOUT}] DB signer roles: $DB_ROLES_JSON"

    # 7. Build recipients JSON using DB role IDs, matched by signerOrder
    # Content roles have order field; DB roles have signerOrder field.
    # Write DB roles to a temp file for Python consumption
    echo "$DB_ROLES_JSON" > "${TMP_DIR}/db_roles.json"

    RECIPIENTS_FILE="${TMP_DIR}/recipients.json"
    python3 - "$PATCHED_JSON" "${TMP_DIR}/db_roles.json" "$RECIPIENTS_FILE" <<'PYEOF'
import json
import sys

with open(sys.argv[1]) as f:
    content = json.load(f)

with open(sys.argv[2]) as f:
    db_roles = json.load(f)

recipients_file = sys.argv[3]

# Build map: order -> DB role id
db_role_by_order = {r['signerOrder']: r['id'] for r in db_roles}

recipients = []
for role in sorted(content.get('signerRoles', []), key=lambda r: r.get('order', 1)):
    order = role.get('order', 1)
    label = role.get('label', f'Signer {order}')
    email_field = role.get('email', {})
    email = email_field.get('value', f'signer-{order}@example.test')

    db_role_id = db_role_by_order.get(order)
    if not db_role_id:
        print(f"ERROR: no DB role for order {order}", file=sys.stderr)
        sys.exit(1)

    recipients.append({
        'roleId': db_role_id,
        'name': label,
        'email': email,
    })

with open(recipients_file, 'w') as f:
    json.dump(recipients, f)
PYEOF

    RECIPIENT_COUNT=$(jq length "$RECIPIENTS_FILE")
    log "[${LAYOUT}] Building document with ${RECIPIENT_COUNT} recipient(s)"

    # 8. Create document — build payload using temp files
    DOC_PAYLOAD_FILE="${TMP_DIR}/doc_payload.json"
    python3 -c "
import json
with open('${RECIPIENTS_FILE}') as f:
    recipients = json.load(f)
payload = {
    'title': 'E2E ${LAYOUT} ${TS}',
    'templateVersionId': '${VERSION_ID}',
    'recipients': recipients,
}
with open('${DOC_PAYLOAD_FILE}', 'w') as f:
    json.dump(payload, f)
"

    DOC_RESP=$(api_post "/api/v1/documents" "$(cat "$DOC_PAYLOAD_FILE")")
    DOC_ID=$(echo "$DOC_RESP" | jq -r '.id // empty')
    if [[ -z "$DOC_ID" ]]; then
      echo "ERROR[${LAYOUT}]: failed to create document: $DOC_RESP" >&2
      exit 1
    fi
    log "[${LAYOUT}] Document: ${DOC_ID}"

    # 9. Request access for the first signer (order=1)
    FIRST_EMAIL=$(jq -r '.[0].email' "$RECIPIENTS_FILE")

    log "[${LAYOUT}] Requesting access for: ${FIRST_EMAIL}"
    api_post "/public/doc/${DOC_ID}/request-access" "{\"email\":\"${FIRST_EMAIL}\"}" >/dev/null

    # 10. Get token from DB
    TOKEN=""
    for i in $(seq 1 5); do
      TOKEN=$(get_signing_token "$DOC_ID" "$FIRST_EMAIL" | tr -d '[:space:]')
      [[ -n "$TOKEN" ]] && break
      sleep 2
    done

    if [[ -z "$TOKEN" ]]; then
      echo "ERROR[${LAYOUT}]: no signing token in DB for ${FIRST_EMAIL}" >&2
      exit 1
    fi
    log "[${LAYOUT}] Got signing token"

    # 11. Proceed to signing
    PROCEED_RESP=$(api_post "/public/sign/${TOKEN}/proceed" '{}')
    log "[${LAYOUT}] Proceed: $(echo "$PROCEED_RESP" | jq -r '.step // .error // "unknown"')"

    # 12. Wait for River to submit to provider
    log "[${LAYOUT}] Waiting up to ${WAIT_TIMEOUT}s for terminal status..."
    FINAL_STATUS=$(wait_for_attempt_status "$DOC_ID" "$WAIT_TIMEOUT" || echo "TIMEOUT")

    log "[${LAYOUT}] Final attempt status: ${FINAL_STATUS}"

    if ! is_ok_status "$FINAL_STATUS"; then
      echo "ERROR[${LAYOUT}]: attempt did not succeed (status: ${FINAL_STATUS})" >&2
      exit 1
    fi

    # 13. Get attempt data
    ATTEMPT_ID=$(get_attempt_id "$DOC_ID" | tr -d '[:space:]')
    PROVIDER_DOC_ID=$(get_provider_document_id "$ATTEMPT_ID" | tr -d '[:space:]')
    # JSON: strip only newlines so embedded spaces in string values survive.
    SNAPSHOT=$(get_signature_field_snapshot "$ATTEMPT_ID" | tr -d '\n\r')

    if [[ -z "$ATTEMPT_ID" || -z "$PROVIDER_DOC_ID" || -z "$SNAPSHOT" || "$SNAPSHOT" == "null" ]]; then
      echo "ERROR[${LAYOUT}]: missing attempt data (attempt=${ATTEMPT_ID} provider=${PROVIDER_DOC_ID} snapshot_len=${#SNAPSHOT})" >&2
      exit 1
    fi
    log "[${LAYOUT}] Attempt ${ATTEMPT_ID}, envelope ${PROVIDER_DOC_ID}"

    # 13b. Capture signing URL for first recipient (signer_order=1)
    FIRST_SIGNING_URL=$(app_query "
      SELECT signing_url
      FROM execution.signing_attempt_recipients
      WHERE attempt_id = '${ATTEMPT_ID}'
        AND signer_order = 1
      LIMIT 1
    " | tr -d '[:space:]')
    if [[ -n "$FIRST_SIGNING_URL" ]]; then
      printf '%s\t%s\t%s\n' "${LAYOUT}" "${FIRST_EMAIL}" "${FIRST_SIGNING_URL}" >> "${URLS_TSV}"
      log "[${LAYOUT}] Signing URL captured for ${FIRST_EMAIL}"
    else
      log "[${LAYOUT}] WARN: no signing_url found for signer_order=1"
      printf '%s\t%s\t%s\n' "${LAYOUT}" "${FIRST_EMAIL}" "NONE" >> "${URLS_TSV}"
    fi

    # 14. Query Documenso fields
    # bottom_pct = positionY + height: the rect bottom, which must align to line_top_pct (rect-above-line invariant).
    DOCUMENSO_FIELDS=$(documenso_query "
      SELECT f.\"recipientId\", f.\"positionY\", f.height, (f.\"positionY\" + f.height) AS bottom_pct
      FROM \"Field\" f
      WHERE f.\"envelopeId\" = '${PROVIDER_DOC_ID}'
      ORDER BY f.\"recipientId\"
    ")

    if [[ -z "$DOCUMENSO_FIELDS" ]]; then
      echo "ERROR[${LAYOUT}]: no Documenso fields for envelope ${PROVIDER_DOC_ID}" >&2
      exit 1
    fi

    log "[${LAYOUT}] Documenso fields retrieved"

    # 15. Pre-fetch role → recipientId mapping in one query, then compute deltas.
    # Metric (rect-above-line invariant): the field bottom (posY + height) must equal line_top_pct.
    # posY = lineTopPct - height  →  posY + height = lineTopPct.
    # snapshot_pos_y + snapshot_height is the lineTopPct stored during conversion.
    # Both are in Documenso's top-down % coordinate system.
    RECIPIENT_MAP=$(get_attempt_recipient_map "$ATTEMPT_ID")
    HAS_FAIL=0
    python3 - \
      "${LAYOUT}" \
      "${DELTA_THRESHOLD}" \
      "${CSV_OUT}" \
      "${SNAPSHOT}" \
      "${DOCUMENSO_FIELDS}" \
      "${RECIPIENT_MAP}" <<'PYEOF' || HAS_FAIL=$?
import json
import sys

layout = sys.argv[1]
threshold = float(sys.argv[2])
csv_out = sys.argv[3]
snapshot_raw = sys.argv[4]
documenso_raw = sys.argv[5]
recipient_map_raw = sys.argv[6]

snapshot = json.loads(snapshot_raw)

# Parse Documenso fields: recipientId (int), positionY, height, bottom_pct
# bottom_pct = positionY + height (rect-above-line invariant: bottom of rect = line_top_pct).
documenso_map = {}
for line in documenso_raw.strip().split('\n'):
    if not line.strip():
        continue
    parts = line.split('\t')
    if len(parts) >= 4:
        rec_id = int(parts[0].strip())
        pos_y = float(parts[1].strip())
        h = float(parts[2].strip())
        bottom = float(parts[3].strip())
        documenso_map[rec_id] = {'positionY': pos_y, 'height': h, 'bottom_pct': bottom}

# Parse role_id → provider_recipient_id mapping (TSV from get_attempt_recipient_map)
role_to_recipient = {}
for line in recipient_map_raw.strip().split('\n'):
    if not line.strip():
        continue
    parts = line.split('\t')
    if len(parts) == 2:
        role_to_recipient[parts[0].strip()] = parts[1].strip()

any_fail = False
rows = []
for field in snapshot:
    role_id = (field.get('RoleID') or field.get('roleId') or field.get('role_id', '')).strip()
    # PositionY in the snapshot is the field top % we computed via ConvertFieldToDocumensoPosition.
    # Rect-above-line invariant: posY = lineTopPct - height  →  posY + height = lineTopPct.
    snapshot_pos_y = float(field.get('PositionY') or field.get('positionY') or field.get('position_y') or 0)
    snapshot_height = float(field.get('Height') or field.get('height') or 8.0)
    # snapshot_line_top = posY + height is the line position stored during conversion.
    snapshot_line_top = snapshot_pos_y + snapshot_height

    prov_rec_id_str = role_to_recipient.get(role_id, '')

    if not prov_rec_id_str:
        print(f"  WARN: no provider_recipient_id for role {role_id[:8]}", file=sys.stderr)
        rows.append(f"{layout},{role_id},N/A,N/A,N/A,{snapshot_line_top:.4f},N/A,ERROR")
        continue

    try:
        prov_rec_id = int(prov_rec_id_str)
    except ValueError:
        print(f"  WARN: non-integer provider_recipient_id '{prov_rec_id_str}'", file=sys.stderr)
        rows.append(f"{layout},{role_id},N/A,N/A,N/A,{snapshot_line_top:.4f},N/A,ERROR")
        continue

    if prov_rec_id not in documenso_map:
        print(f"  WARN: recipientId {prov_rec_id} not in Documenso (envelope {provider_doc_id})", file=sys.stderr)
        rows.append(f"{layout},{role_id},N/A,N/A,N/A,{snapshot_line_top:.4f},N/A,ERROR")
        continue

    df = documenso_map[prov_rec_id]
    pos_y_doc = df['positionY']
    h_doc = df['height']
    bottom_doc = df['bottom_pct']  # positionY + height from Documenso DB

    # Metric (rect-above-line invariant): the bottom of the Documenso field must equal lineTopPct.
    # posY = lineTopPct - height → posY + height = lineTopPct.
    # Both derive from the same conversion, so delta near zero confirms the API roundtrip.
    # Visual correctness (rect entirely above line) is validated via Playwright screenshots.
    delta = abs(bottom_doc - snapshot_line_top)
    verdict = 'PASS' if delta < threshold else 'FAIL'

    row = f"{layout},{role_id},{pos_y_doc:.4f},{h_doc:.4f},{bottom_doc:.4f},{snapshot_line_top:.4f},{delta:.4f},{verdict}"
    rows.append(row)
    print(f"  role={role_id[:8]} posY={pos_y_doc:.4f} h={h_doc:.4f} bottom={bottom_doc:.4f} line_top={snapshot_line_top:.4f} delta={delta:.4f} -> {verdict}")

with open(csv_out, 'a') as f:
    for r in rows:
        f.write(r + '\n')

# Distinguish verdict mismatch (FAIL=1) from infrastructure error (ERROR=2)
# so the bash summary can route them to separate counters.
has_fail = any(',FAIL' in r for r in rows)
has_err = any(',ERROR' in r for r in rows)
sys.exit(2 if has_err else (1 if has_fail else 0))
PYEOF

    log "[${LAYOUT}] Done"
    exit "${HAS_FAIL:-0}"
  )
  LAYOUT_RC=$?

  case "$LAYOUT_RC" in
    0) LAYOUT_STATUS="PASS";  PASS_COUNT=$((PASS_COUNT + 1)) ;;
    1) LAYOUT_STATUS="FAIL";  FAIL_COUNT=$((FAIL_COUNT + 1)) ;;
    *) LAYOUT_STATUS="ERROR"; ERROR_COUNT=$((ERROR_COUNT + 1)) ;;
  esac

  log "[${LAYOUT}] Result: ${LAYOUT_STATUS}"
  set_layout_result "$LAYOUT" "$LAYOUT_STATUS"
done

# ---- Summary ---------------------------------------------------------------
log ""
log "============================================================"
log "SUMMARY"
log "============================================================"
for LAYOUT in "${LAYOUTS[@]}"; do
  result=$(get_layout_result "$LAYOUT")
  log "  ${LAYOUT}: ${result}"
done
log ""
log "Layouts processed: $((PASS_COUNT + FAIL_COUNT + ERROR_COUNT))"
log "PASS:   ${PASS_COUNT}"
log "FAIL:   ${FAIL_COUNT}"
log "ERROR:  ${ERROR_COUNT}"
log ""
log "CSV output: ${CSV_OUT}"

if [[ -f "$CSV_OUT" ]]; then
  log ""
  log "CSV results:"
  cat "$CSV_OUT"
fi
