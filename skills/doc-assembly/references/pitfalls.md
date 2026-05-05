# Pitfalls — wrapper-author edition

These are real ways doc-assembly wrappers go wrong. Cross-check before declaring "done".

## Importing private packages

`github.com/rendis/doc-assembly/core/internal/...` and `core/cmd/...` are **private**. Only `core/sdk` is stable. Importing anything else means your wrapper will break on lib upgrades. Quick audit:

```bash
grep -RE 'doc-assembly/core/(internal|cmd)' .
```

The expected count is **0** in any wrapper repo. If you find imports, refactor through `sdk` aliases or open an upstream PR to expose what you need.

## Calling configuration methods after `Run()`

Every `Set*`/`Register*`/`Use*`/`On*` must be called BEFORE `engine.Run()`. There is no hot-reconfiguration. Calling them from inside an `OnStart` hook, an injector, or a goroutine is a no-op (or worse, a data race) — the engine has already snapshotted them.

## Running migrations from inside the engine boot

Do not call `engine.RunMigrations()` from the same process that also calls `engine.Run()`. Use a dedicated CLI subcommand (`go run . migrate`) so your deploy can apply migrations in a controlled step before rolling out the new binary. The scaffolded `main.go` already does this.

## Sharing the database with another app

The lib **owns** its schema. Do not point a separate service at the same Postgres database. `RunMigrations` is destructive in the sense that it adds tables, indexes, and triggers it expects to control. If you need the same data, expose it through the engine's API or a read-replica ETL — not by sharing tables.

## Leaving `auth.dummy: true` or `bootstrap.enabled: true` in production

Both are dev-time conveniences:

- `dummy: true` bypasses JWT verification entirely. Anyone can log in.
- `bootstrap.enabled: true` auto-promotes the first user to log in to `SUPERADMIN`. Convenient locally; an attacker's gift card in prod.

Toggle both off in production:

```yaml
auth:
  dummy: false
  panel:
    issuer: ...
    jwks_url: ...
bootstrap:
  enabled: false
```

…or via env: `DOC_ENGINE_AUTH_DUMMY=false`, `DOC_ENGINE_BOOTSTRAP_ENABLED=false`.

## Enabling failpoints in production

`worker.failpoints_enabled` injects synthetic failures into the signing pipeline for chaos testing. The lib refuses to honor it when `environment` is prod, but the safer default is to keep `failpoints_enabled: false` everywhere except local dev.

## Setting tenant/auth headers manually

The engine's middleware chain extracts `Authorization`, `X-Tenant-ID`, `X-Workspace-ID` from incoming requests. Do NOT rewrite or strip these from your `UseMiddleware`/`UseAPIMiddleware` — downstream layers depend on them. If you need to inject tenant context for a service-to-service call, use `internal_api.enabled: true` and call the `/internal/*` routes instead.

## Forgetting `Code()` uniqueness across the registry

If you implement a `WorkspaceInjectableProvider` AND register matching `Injector` codes, the engine fails at boot with a code-collision error. Decide per code which mechanism owns it.

## Non-idempotent `SigningProvider.SubmitAttemptDocument`

The engine may call `SubmitAttemptDocument` twice for the same `(DocumentID, AttemptID)` after a partial failure. If you create a brand-new provider record on every call you will end up with duplicate envelopes that the user has to clean up.

Pattern: look up by `CorrelationKey` first; if a usable provider record exists, return its IDs instead of creating a new one.

## Inlining provider calls in the public flow

`ProceedToSigning` MUST stay in `render_attempt_pdf` → River. Do not call your provider from inside an HTTP handler or middleware — that defeats the attempt model and the transactional enqueue, and it makes failure recovery impossible.

## Non-idempotent completion handler

Your `OnDocumentCompleted` handler will eventually be called twice for the same document — that is by design. De-dupe on `(DocumentID, attempt_id)` or on a unique key in your downstream system.

## Returning generic errors from `WebhookHandler.ParseWebhook`

The engine retries webhook deliveries that returned an error. If you return an error for *expected* duplicate webhooks (provider re-delivering an event you already saw), you will hammer your provider with NACKs. Parse silently into an event the engine can apply idempotently; let the engine de-dupe by `(ProviderName, ProviderDocumentID, ProviderRecipientID, EventType, Timestamp)`.

## Using `slog.Info` instead of `slog.InfoContext`

The engine's `ContextHandler` reads `tenant_id`, `workspace_id`, and `operation_id` from `context.Context`. Calling `slog.Info(...)` without a context loses them — your prod logs end up with millions of indistinguishable `{"msg":"X"}` lines.

```go
slog.InfoContext(ctx, "thing happened", "key", val)  // ✅
slog.Info("thing happened", "key", val)              // ❌ missing context attrs
```

## Replacing the frontend incompletely

If you call `SetFrontendFS(myFS)` and `myFS` does not contain `index.html`, the SPA fallback returns 404 for every non-API route. Either ship a complete static bundle (`index.html` + assets) or call `SetFrontendFS(nil)` to disable serving.

## Mounting under a sub-path without `server.public_url`

If you set `server.base_path` (e.g. `/docs`) but leave `server.public_url` empty, outbound email links built by the notifications module won't include the sub-path and recipients will land on broken URLs. Always set both, together, in tandem.

## Skipping verification before "ready"

These take less than a minute and catch the bulk of integration mistakes:

```bash
go build ./...
go run . migrate
go run .                # boots and prints banner
curl localhost:8080/health
# Trigger one render that exercises a custom injector and assert the value lands in the PDF.
# Sign one document end-to-end if your wrapper uses signing.
```

If any of these fail and you cannot fix them in three iterations, stop and report — do not paper over with retries or env toggles.
