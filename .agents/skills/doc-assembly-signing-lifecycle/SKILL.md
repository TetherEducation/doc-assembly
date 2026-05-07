---
name: doc-assembly-signing-lifecycle
description: Project-specific workflow for doc-assembly signing document lifecycle work. Use when changing, debugging, validating, or documenting internal document creation/reset, signed document deprecation/cancelation, Documenso cleanup, River signing attempts, signing UI actions, public signing, or E2E checks involving doc-assembly + Documenso + DB.
---

# doc-assembly signing lifecycle

Use this skill for changes around document signing lifecycle in `doc-assembly`: internal create/reset, provider submission, signed document deprecate/cancel, public signing, Documenso cleanup, River signing attempts, and signing UI actions.

## Source-of-truth files

Start with these files before changing behavior:

- `AGENTS.md` — repo architecture, verification, docs requirements.
- `docs/backend/authorization-matrix.md` — update when endpoints/permissions change.
- `docs/backend/public-signing-flow.md` — public signing behavior.
- `docs/backend/worker-queue-guide.md` — River attempt lifecycle.
- `core/docs/swagger.yaml` — API contract after `make swagger`.
- `core/internal/adapters/primary/http/controller/document_controller.go` — authenticated document endpoints.
- `core/internal/adapters/primary/http/controller/internal_document_controller.go` — service-to-service internal endpoints.
- `core/internal/core/service/document/document_service.go` — document lifecycle business rules.
- `core/internal/core/service/document/internal_document_service.go` — internal create/reset flow.
- `core/internal/infra/riverqueue/` — asynchronous signing attempt execution.
- `core/internal/adapters/secondary/signing/documenso/adapter.go` — Documenso provider adapter.
- `app/src/features/signing/` — signing list/detail UI, API, hooks, types.

For the reset/deprecate implementation added in PR #41, read `references/reset-deprecate.md` when the task mentions reset, deprecate, cancel, invalidate signed contracts, Documenso cleanup, or internal document lifecycle.

## Rules for lifecycle changes

1. Keep reset/rebuild of existing contracts **internal/API-only** unless the user explicitly asks for frontend exposure.
2. Do not expose reset in the signing UI. Current product decision: frontend exposes `Deprecate` for completed documents only.
3. Deprecate signed/completed documents only from `COMPLETED` state. Persist `INVALIDATED`, set inactive, and record `DOCUMENT_DEPRECATED`.
4. Treat provider cleanup as best-effort. Persist cleanup outcome on the active signing attempt when there is a provider document id.
5. Do not upload/sign/provider-submit inline from public signing handlers. Use attempt/River flow where applicable.
6. When adding/changing endpoints, update Swagger and `docs/backend/authorization-matrix.md`.
7. When changing Documenso API behavior, verify against local Documenso or a dedicated adapter test. Do not rely on assumed endpoint names.

## Implementation workflow

1. Inspect existing controller/usecase/service/port/repo seams before coding.
2. Add/adjust tests first or alongside the implementation:
   - controller tests for endpoint behavior and auth/middleware edge cases;
   - service tests for state transitions when available;
   - adapter tests for Documenso endpoint/body/status mapping;
   - frontend component/hook tests for UI actions.
3. Keep DB changes out of `db/src/` unless the user explicitly approves migration work. Read schema for context only.
4. Run `make wire` after changing DI/provider signatures; production bootstrap may also require manual wiring checks.
5. Run `make swagger` after API/controller DTO changes.
6. Validate with the narrowest meaningful checks, then broader checks when practical.
7. For local E2E, validate all relevant surfaces: API response, app DB, provider DB/API, and browser UI when frontend is in scope.

## Local E2E expectations

Prefer local Docker services already defined by the repo. A complete signing lifecycle validation should include:

- backend running with dummy auth, worker enabled, local Postgres, local Documenso, and Mailpit;
- local Documenso API token available to the backend;
- internal API key inserted in `automation.api_keys` and backend restarted so memory auth reloads it;
- API calls with required internal headers for `/api/v1/internal/*`;
- DB assertions in `execution.documents`, `execution.signing_attempts`, and `execution.document_events`;
- Documenso assertions in its DB/API for envelope state or `deletedAt` when cleanup applies;
- Browser Use validation for signing UI actions when frontend changes.

Do not print plaintext API tokens. Store transient local-only tokens in `/tmp` if needed and mention paths, not secret values.

## Verification checklist

Use the relevant subset, then report exact commands and outcomes:

```bash
go test -C core ./internal/adapters/secondary/signing/documenso -count=1
go test -C core -tags=integration -run 'TestInternalDocumentController_(Reset|Deprecate)|TestDocumentController_DeprecateDocument' ./internal/adapters/primary/http/controller -count=1
pnpm --dir app test:run src/features/signing/hooks/useSigningDocuments.test.ts src/features/signing/components/DeprecateDocumentDialog.test.tsx
make -C core build
make -C core test
go build -C core -tags=integration ./...
pnpm --dir app build
pnpm --dir app lint
```

Known repo state from PR #41: `make -C core lint` may fail on pre-existing unrelated `pdfrenderer` lint findings. Do not report the branch as fully lint-clean unless this has been resolved or rechecked.
