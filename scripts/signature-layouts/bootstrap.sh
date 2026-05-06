#!/usr/bin/env bash
# bootstrap.sh — Creates (or reuses) the tenant/workspace/document-type needed
# for the E2E signature layout harness.
#
# Usage:
#   eval "$(bash scripts/signature-layouts/bootstrap.sh)"
#
# Output: exports TENANT_ID, WORKSPACE_ID, DOC_TYPE_ID, AUTH_TOKEN, API_BASE
#
# Idempotent: if a tenant/workspace/doctype with the same code already exists
# (detected from the response), their IDs are reused.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "${SCRIPT_DIR}/lib/api.sh"

# Override log to write to stderr so eval "$(bootstrap.sh)" works correctly.
log() {
  echo "[$(date -u '+%H:%M:%S')] $*" >&2
}

API_BASE="${API_BASE:-http://localhost:8081/api/v1}"
# In dummy auth mode, any non-empty token works. Hardcode so the exported
# value is predictable regardless of what's in the environment.
AUTH_TOKEN="dummy"

TENANT_CODE="SIGLAYOUTT"
WORKSPACE_CODE="SIGLAYOUTW"
DOC_TYPE_CODE="SIGLAYOUT"

# ---- Tenant ----------------------------------------------------------------

log "Checking/creating tenant (code=${TENANT_CODE})..."
TENANT_RESP=$(api GET "/api/v1/system/tenants?q=${TENANT_CODE}")
EXISTING_TENANT_ID=$(echo "$TENANT_RESP" | jq -r '.data[]? | select(.code == "'"${TENANT_CODE}"'") | .id' 2>/dev/null | head -1)

if [[ -n "$EXISTING_TENANT_ID" ]]; then
  TENANT_ID="$EXISTING_TENANT_ID"
  log "Reusing tenant: ${TENANT_ID}"
else
  CREATE_RESP=$(api_post "/api/v1/system/tenants" \
    "{\"code\":\"${TENANT_CODE}\",\"name\":\"Sig Layout Tests\",\"description\":\"E2E signature alignment harness\"}")
  TENANT_ID=$(echo "$CREATE_RESP" | jq -r '.id // empty')
  if [[ -z "$TENANT_ID" ]]; then
    echo "ERROR: Failed to create tenant. Response: $CREATE_RESP" >&2
    exit 1
  fi
  log "Created tenant: ${TENANT_ID}"
fi

export TENANT_ID

# ---- Workspace -------------------------------------------------------------

log "Checking/creating workspace (code=${WORKSPACE_CODE})..."
WS_LIST=$(api GET "/api/v1/system/tenants/${TENANT_ID}/workspaces")
EXISTING_WS_ID=$(echo "$WS_LIST" | jq -r '.data[]? | select(.code == "'"${WORKSPACE_CODE}"'") | .id' 2>/dev/null | head -1)

if [[ -n "$EXISTING_WS_ID" ]]; then
  WORKSPACE_ID="$EXISTING_WS_ID"
  log "Reusing workspace: ${WORKSPACE_ID}"
else
  CREATE_WS=$(api_post "/api/v1/tenant/workspaces" \
    "{\"code\":\"${WORKSPACE_CODE}\",\"name\":\"Sig Layout Workspace\",\"type\":\"CLIENT\"}")
  WORKSPACE_ID=$(echo "$CREATE_WS" | jq -r '.id // empty')
  if [[ -z "$WORKSPACE_ID" ]]; then
    echo "ERROR: Failed to create workspace. Response: $CREATE_WS" >&2
    exit 1
  fi
  log "Created workspace: ${WORKSPACE_ID}"
fi

export WORKSPACE_ID

# ---- Document Type ---------------------------------------------------------

log "Checking/creating document type (code=${DOC_TYPE_CODE})..."
DT_LIST=$(api GET "/api/v1/tenant/document-types?q=${DOC_TYPE_CODE}")
EXISTING_DT_ID=$(echo "$DT_LIST" | jq -r '.data[]? | select(.code == "'"${DOC_TYPE_CODE}"'") | .id' 2>/dev/null | head -1)

if [[ -n "$EXISTING_DT_ID" ]]; then
  DOC_TYPE_ID="$EXISTING_DT_ID"
  log "Reusing document type: ${DOC_TYPE_ID}"
else
  CREATE_DT=$(api_post "/api/v1/tenant/document-types" \
    "{\"code\":\"${DOC_TYPE_CODE}\",\"name\":{\"en\":\"Signature Layout Test\",\"es\":\"Prueba de Alineación de Firma\"}}")
  DOC_TYPE_ID=$(echo "$CREATE_DT" | jq -r '.id // empty')
  if [[ -z "$DOC_TYPE_ID" ]]; then
    echo "ERROR: Failed to create document type. Response: $CREATE_DT" >&2
    exit 1
  fi
  log "Created document type: ${DOC_TYPE_ID}"
fi

export DOC_TYPE_ID

# ---- Emit exports ----------------------------------------------------------

cat <<EOF
export TENANT_ID="${TENANT_ID}"
export WORKSPACE_ID="${WORKSPACE_ID}"
export DOC_TYPE_ID="${DOC_TYPE_ID}"
export AUTH_TOKEN="${AUTH_TOKEN}"
export API_BASE="${API_BASE}"
EOF
