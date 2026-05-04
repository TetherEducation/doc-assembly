#!/usr/bin/env bash
# api.sh — curl helpers that inject multi-tenant auth headers.
# Requires: TENANT_ID, WORKSPACE_ID, AUTH_TOKEN, API_BASE to be set.
# Compatible with bash 3.2+ (macOS default).

API_BASE="${API_BASE:-http://localhost:8081/api/v1}"
AUTH_TOKEN="${AUTH_TOKEN:-dummy}"

# api <method> <path> [extra curl args...]
# Path must start with /api/v1 or /public.
# Automatically injects Authorization, X-Tenant-ID, X-Workspace-ID for /api/v1 routes.
# Public routes get no auth headers.
api() {
  local method="$1"
  local path="$2"
  shift 2

  local url
  if [[ "$path" == http* ]]; then
    url="$path"
  else
    url="${API_BASE%/api/v1}${path}"
  fi

  if [[ "$path" == /public/* ]]; then
    # Public routes: no auth headers
    curl -s -X "$method" "$url" \
      -H "Content-Type: application/json" \
      "$@"
  else
    # Panel routes: inject auth + tenant headers
    local -a extra_headers
    extra_headers=(-H "Authorization: Bearer ${AUTH_TOKEN}")
    if [[ -n "${TENANT_ID:-}" ]]; then
      extra_headers=("${extra_headers[@]}" -H "X-Tenant-ID: ${TENANT_ID}")
    fi
    if [[ -n "${WORKSPACE_ID:-}" ]]; then
      extra_headers=("${extra_headers[@]}" -H "X-Workspace-ID: ${WORKSPACE_ID}")
    fi

    curl -s -X "$method" "$url" \
      "${extra_headers[@]}" \
      -H "Content-Type: application/json" \
      "$@"
  fi
}

# api_post <path> <json_body>
api_post() {
  api POST "$1" -d "$2"
}

# api_put <path> <json_body>
api_put() {
  api PUT "$1" -d "$2"
}

# api_get <path>
api_get() {
  api GET "$1"
}

# jq_get <json_string> <jq_filter>
jq_get() {
  echo "$1" | jq -r "$2"
}

# die <message> — print to stderr and exit 1
die() {
  echo "ERROR: $*" >&2
  exit 1
}

# log <message> — timestamped info log to stderr
# Writing to stderr prevents interference with stdout capture in subshells.
log() {
  echo "[$(date -u '+%H:%M:%S')] $*" >&2
}
