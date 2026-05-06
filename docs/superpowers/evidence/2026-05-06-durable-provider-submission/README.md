# Durable provider submission — visual/local evidence

This folder contains sanitized evidence for the local iframe signing-flow validation on 2026-05-06.

## Scope validated

- Local Docker-backed doc-assembly environment.
- Local wrapper page embedding the public signing URL as an iframe, matching how the library is consumed by the host app.
- Visual flow: generate contract → preview → request signature → open provider document → sign → return/complete.
- Recovery UX for long-running provider preparation states.

## Evidence files

- `01-processing-failpoint.png` — public iframe stuck in processing while the provider submit failpoint is active.
- `02-recovering-support-code.png` — bounded processing UX with support code/recovery copy.
- `03-recovered-documenso-signing-ui.png` — recovered flow reaching the Documenso signing UI.
- `proxy-server.mjs` — local-only iframe proxy/test wrapper used for visual validation.
- `setup-proxy.example.json` — placeholder config shape for the proxy.

## Secret hygiene

`setup-proxy.json` is intentionally ignored and must remain local-only because it may contain live public signing tokens or document IDs. Commit only `setup-proxy.example.json`.

## Verification trail

Representative checks run during this branch validation included:

- `make -C core build`
- `make -C core test`
- `make -C core lint`
- `go test -C core ./internal/adapters/secondary/signing/documenso -count=1`
- `go test -C core ./internal/infra/riverqueue -count=1`
- `go test -C core -tags=integration -run 'TestSigningAttemptExecutor_(ProviderPhaseTransitionNoopsWhenPhaseAdvancedAfterProviderCall|ProviderTransitionNoopsWhenAttemptSupersededAfterProviderCall|RefreshProviderStatusPropagatesSignedRecipients)' -count=1 ./internal/infra/riverqueue`
- `go test -C core -tags=integration -run 'TestSigningExecutionUnitOfWork_TransitionAndEnqueueNoopsForTerminalAttempt' -count=1 ./internal/infra/riverqueue`
- `pnpm -C app build`
- `pnpm -C app lint`

The final broad verification commands should be rerun before merge after any further code changes.
