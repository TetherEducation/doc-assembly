# Internal reset and signed deprecate flow

This reference documents the reset/deprecate lifecycle implemented for doc-assembly in PR #41 / issue #40.

## Product decision

- Reset/regeneration of existing contracts is internal/API-only.
- Frontend signing UI must not expose reset.
- Frontend signing UI may expose `Deprecate` for `COMPLETED` signing documents.

## Internal reset

Endpoint:

```http
POST /api/v1/internal/documents/reset
```

Required headers are the internal logical-key headers:

- `X-API-Key`
- `X-Tenant-Code`
- `X-Workspace-Code`
- `X-Document-Type`
- `X-External-ID`
- `X-Transactional-ID`
- `X-Environment` when applicable

Body shape mirrors internal create, but reset forces `forceCreate=true` server-side:

```json
{
  "supersedeReason": "internal reset",
  "payload": {}
}
```

Behavior:

1. Resolve tenant/workspace/document type from internal headers.
2. Find the active document by logical key: workspace + document type + external id.
3. If active doc is `COMPLETED`, return conflict (`409`).
4. Call the existing internal create flow with force semantics.
5. Re-run mapper/init/injectors and assemble the new document from the incoming payload.
6. Supersede the previous active doc and mark the new doc active.

Key backend files:

- `core/internal/core/usecase/document/internal_document_usecase.go`
- `core/internal/core/service/document/internal_document_service.go`
- `core/internal/adapters/primary/http/controller/internal_document_controller.go`
- `core/internal/adapters/secondary/database/postgres/document_repo/internal_create.go`
- `core/internal/core/port/document_repository.go`

Expected DB evidence:

- previous doc: `is_active=false`, `superseded_by_document_id=<new doc>`, `supersede_reason=<reason>`;
- new doc: `is_active=true`, rebuilt from new payload.

## Signed/completed deprecate

Endpoints:

```http
POST /api/v1/internal/documents/{documentId}/deprecate
POST /api/v1/documents/{documentId}/deprecate
```

Internal endpoint uses `X-API-Key`. Authenticated endpoint uses normal auth/context and requires OPERATOR+ via document controller middleware.

Request body:

```json
{
  "reason": "replacement signed"
}
```

Behavior:

1. Load document.
2. Require `status == COMPLETED`; otherwise reject state.
3. If active signing attempt has a provider document id and provider supports cleanup, call provider cleanup best-effort.
4. Persist cleanup action/status/error on the signing attempt.
5. Mark document `INVALIDATED` and `is_active=false`.
6. Store `supersede_reason` / deprecation reason.
7. Emit `DOCUMENT_DEPRECATED`.

Key backend files:

- `core/internal/core/usecase/document/document_usecase.go`
- `core/internal/core/service/document/document_service.go`
- `core/internal/adapters/primary/http/controller/document_controller.go`
- `core/internal/adapters/primary/http/controller/internal_document_controller.go`
- `core/internal/core/entity/document_event.go`
- `core/internal/adapters/primary/http/dto/internal_document_dto.go`

Expected DB evidence:

```text
execution.documents.status = INVALIDATED
execution.documents.is_active = false
execution.documents.supersede_reason = <reason>
execution.document_events.event_type = DOCUMENT_DEPRECATED
```

If provider cleanup ran:

```text
execution.signing_attempts.cleanup_action = CANCEL
execution.signing_attempts.cleanup_status = SUCCEEDED | FAILED_RETRYABLE | FAILED_PERMANENT
execution.signing_attempts.cleanup_error = <provider error if failed>
```

## Documenso cleanup

Local Documenso used for E2E does **not** expose `POST /api/v2/envelope/cancel`.

Use:

```http
POST /api/v2/envelope/delete
Authorization: <documenso api token>
Content-Type: application/json

{"envelopeId":"<providerDocumentId>"}
```

Adapter semantics remain cleanup action `CANCEL`, because doc-assembly is cancelling/deprecating the provider-side active artifact.

Observed local behavior:

- For `COMPLETED` envelopes, Documenso sets `deletedAt`.
- For pending/draft envelopes, Documenso may remove the envelope row.

Key files:

- `core/internal/adapters/secondary/signing/documenso/adapter.go`
- `core/internal/adapters/secondary/signing/documenso/adapter_test.go`

Test to keep this endpoint fixed:

```bash
go test -C core ./internal/adapters/secondary/signing/documenso -count=1
```

## Frontend deprecate action

Frontend scope:

- show `Deprecate` only for signing documents with `status === COMPLETED` and `DOCUMENT_DEPRECATE` permission;
- do not show reset anywhere in signing list/detail;
- submit reason through authenticated `POST /documents/{documentId}/deprecate`;
- refresh/invalidate list/detail query after success.

Key frontend files:

- `app/src/features/auth/rbac/rules.ts`
- `app/src/features/signing/api/signing-api.ts`
- `app/src/features/signing/hooks/useSigningDocuments.ts`
- `app/src/features/signing/types/index.ts`
- `app/src/features/signing/components/DeprecateDocumentDialog.tsx`
- `app/src/features/signing/components/SigningDocumentRow.tsx`
- `app/src/features/signing/components/SigningListPage.tsx`
- `app/src/features/signing/components/SigningDetailPage.tsx`
- `app/public/locales/en/translation.json`
- `app/public/locales/es/translation.json`

Focused frontend test:

```bash
pnpm --dir app test:run src/features/signing/hooks/useSigningDocuments.test.ts src/features/signing/components/DeprecateDocumentDialog.test.tsx
```

## Local E2E notes from PR #41

Validated surfaces:

- internal reset creates a new active doc and supersedes the previous active doc;
- internal reset against active completed doc returns `409`;
- internal deprecate marks doc invalidated/inactive and records provider cleanup;
- Documenso completed envelope receives `deletedAt` after `/envelope/delete`;
- Browser Use UI: completed document menu shows `Deprecate`, does not show `Reset`, dialog submit changes row to `Invalidated`;
- DB after UI deprecate had `DOCUMENT_DEPRECATED` event.

Transient local token convention used during validation:

- Documenso API token plaintext: `/tmp/docassembly_documenso_token`;
- internal API key plaintext: `/tmp/docassembly_internal_key`.

Do not print their contents.
