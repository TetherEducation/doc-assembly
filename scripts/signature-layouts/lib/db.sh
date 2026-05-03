#!/usr/bin/env bash
# db.sh — psql wrappers for both app and Documenso databases.

# App DB (PostgreSQL on 5432)
APP_DB_URL="${APP_DB_URL:-postgresql://postgres:postgres@localhost:5432/doc_assembly}"
# Documenso DB (PostgreSQL on 5433)
DOCUMENSO_DB_URL="${DOCUMENSO_DB_URL:-postgresql://documenso:documenso@localhost:5433/documenso}"

# app_query <sql> — run SQL against app DB, return tuples-only output
app_query() {
  PGPASSWORD=postgres psql \
    -h localhost -p 5432 -U postgres -d doc_assembly \
    -t -A -F $'\t' \
    -c "$1" 2>&1
}

# documenso_query <sql> — run SQL against Documenso DB
documenso_query() {
  PGPASSWORD=documenso psql \
    -h localhost -p 5433 -U documenso -d documenso \
    -t -A -F $'\t' \
    -c "$1" 2>&1
}

# get_signing_token <document_id> <recipient_email>
# Reads the latest valid (unused) signing token for a recipient from the app DB.
get_signing_token() {
  local doc_id="$1"
  local email="$2"
  app_query "
    SELECT dat.token
    FROM execution.document_access_tokens dat
    JOIN execution.document_recipients dr ON dr.id = dat.recipient_id
    WHERE dat.document_id = '${doc_id}'
      AND dr.email = '${email}'
      AND dat.used_at IS NULL
      AND dat.expires_at > NOW()
    ORDER BY dat.created_at DESC
    LIMIT 1
  "
}

# get_attempt_status <document_id>
# Returns the status of the most recent signing attempt for a document.
get_attempt_status() {
  local doc_id="$1"
  app_query "
    SELECT status
    FROM execution.signing_attempts
    WHERE document_id = '${doc_id}'
    ORDER BY created_at DESC
    LIMIT 1
  "
}

# get_attempt_id <document_id>
# Returns the ID of the most recent signing attempt for a document.
get_attempt_id() {
  local doc_id="$1"
  app_query "
    SELECT id
    FROM execution.signing_attempts
    WHERE document_id = '${doc_id}'
    ORDER BY created_at DESC
    LIMIT 1
  "
}

# get_signature_field_snapshot <attempt_id>
# Returns the JSON signature_field_snapshot for a signing attempt.
get_signature_field_snapshot() {
  local attempt_id="$1"
  app_query "
    SELECT signature_field_snapshot::text
    FROM execution.signing_attempts
    WHERE id = '${attempt_id}'
  "
}

# get_provider_document_id <attempt_id>
# Returns the Documenso envelope/document ID.
get_provider_document_id() {
  local attempt_id="$1"
  app_query "
    SELECT provider_document_id
    FROM execution.signing_attempts
    WHERE id = '${attempt_id}'
  "
}

# get_documenso_fields <envelope_id>
# Returns positionY, height, bottom_pct per recipient from Documenso.
get_documenso_fields() {
  local envelope_id="$1"
  documenso_query "
    SELECT \"recipientId\", \"positionY\", height, (\"positionY\" + height)
    FROM \"Field\"
    WHERE \"documentId\" = ${envelope_id}
    ORDER BY \"recipientId\"
  "
}

# get_provider_recipient_id <attempt_id> <role_id>
# Returns Documenso integer recipientId for a given role in an attempt.
get_provider_recipient_id() {
  local attempt_id="$1"
  local role_id="$2"
  app_query "
    SELECT provider_recipient_id::int
    FROM execution.signing_attempt_recipients
    WHERE attempt_id = '${attempt_id}'
      AND role_id = '${role_id}'
    LIMIT 1
  "
}
