---
name: doc-assembly
description: Use when building or modifying a Go microservice that imports github.com/rendis/doc-assembly/core/sdk as a library — registering injectors, mappers, custom signing providers, storage adapters, notification providers, public-access authenticators, completion handlers, design tokens, or frontend overrides on the doc-assembly Engine. Covers the consumer-facing API surface only; not for working on the doc-assembly internals.
---

# doc-assembly — Consumer Skill

You are integrating **doc-assembly** as a library into a wrapper Go microservice. The wrapper provides a `main.go`, registers extensions, and runs the embedded engine. Everything inside `core/internal/...` is private to the lib; only `github.com/rendis/doc-assembly/core/sdk` is your public API.

```go
import "github.com/rendis/doc-assembly/core/sdk"

engine := sdk.New()
extensions.Register(engine)        // your injectors, mappers, providers
engine.OnDocumentCompleted(handler) // completion callbacks
_ = engine.Run()                   // blocks until SIGINT/SIGTERM
```

## When to use this skill

Use whenever you are working in a wrapper repo whose `go.mod` requires `github.com/rendis/doc-assembly` and whose `main.go` calls `sdk.New()`. Triggers include: writing or reviewing custom injectors, plugging a custom signing provider, replacing storage/notification adapters, configuring `settings/app.yaml`, wiring auth, customizing PDF look, or handling completion events.

**Do not use** for changes inside `github.com/rendis/doc-assembly` itself — that is a different audience.

## How to read this skill

`SKILL.md` (this file) is the index. Each reference is loaded on demand via `Read`:

| If you are doing… | Read |
|---|---|
| First-time wrapper scaffold, project layout, mod path, migrations, minimum to bring it up | [references/quickstart.md](references/quickstart.md) |
| Looking up a specific `engine.Set*/Register*/Use*/On*` method (full surface in one place) | [references/engine-api.md](references/engine-api.md) |
| Writing a custom **Injector**, **RequestMapper**, **InitFunc**, **TemplateResolver**, **ProcessResolver**, or **WorkspaceInjectableProvider** | [references/extensibility.md](references/extensibility.md) |
| Implementing a **SigningProvider** + **WebhookHandler**, understanding the attempt model and correlation keys | [references/signing.md](references/signing.md) |
| Implementing **StorageAdapter** or **NotificationProvider**, or plugging **PublicDocumentAccessAuthenticator** / **SigningSessionAuthenticator** | [references/adapters.md](references/adapters.md) |
| Full `settings/app.yaml` schema and every `DOC_ENGINE_*` env override | [references/settings.md](references/settings.md) |
| Customizing **TypstDesignTokens**, replacing the embedded SPA, adding **middleware** or **OnStart/OnShutdown** hooks | [references/customization.md](references/customization.md) |
| Reacting to a document reaching `COMPLETED` (`OnDocumentCompleted`) | [references/completion-events.md](references/completion-events.md) |
| Avoiding the gotchas wrapper authors hit | [references/pitfalls.md](references/pitfalls.md) |

## Stable surface — what you may import

Only the `core/sdk` package is stable. Treat anything else as private:

- `sdk.New()` / `sdk.NewWithConfig(path)` → `*sdk.Engine`
- Engine fluent methods (every method returns `*Engine` to allow chaining): see [references/engine-api.md](references/engine-api.md).
- Type aliases for ports, DTOs, value constructors, enums: see [core/sdk/types.go](../../core/sdk/types.go) and [core/sdk/interfaces.go](../../core/sdk/interfaces.go).

If you find yourself reaching for `core/internal/...`, stop — that is a private path and may break on any release. Open an issue or PR upstream to expose what you need.

## Engine lifecycle (what `Run()` actually does)

1. `loadConfig()` — `settings/app.yaml` (or path passed to `NewWithConfig`); env overrides via `DOC_ENGINE_*`.
2. `configureDefaultLogger()` — JSON `slog` with the configured level + `ContextHandler`.
3. `entity.InitEnvironmentAliases()` — applies `environment_aliases` from config.
4. `preflightChecks()` — verifies DB connectivity, Typst binary, signing config, etc.
5. `initialize()` — manual DI: builds repos, services, controllers, River workers, scheduler, HTTP server.
6. `OnStart` hooks (in registration order).
7. Scheduler + River workers start, then HTTP server.
8. Block on `SIGINT`/`SIGTERM`.
9. `OnShutdown` hooks (reverse / LIFO).
10. Cleanup pool/clients and exit.

`engine.RunMigrations()` runs steps 1–2 and then applies pending SQL migrations embedded in the lib.

## Working assumptions

- Go 1.25+ in your wrapper.
- A reachable PostgreSQL 16 (the lib applies its own migrations — do not try to share its schema with another app).
- Typst binary on `PATH` (the lib renders PDFs with Typst, not Chromium). Configurable in `settings/app.yaml → typst.bin_path`.
- For embedded signing UI you need a frontend; the lib ships an embedded SPA but you can replace it with `engine.SetFrontendFS(...)`.

## Verification before declaring "done"

```bash
go build ./...        # wrapper compiles
go vet ./...
golangci-lint run     # if you have it
go run . migrate      # migrations apply against your local DB
go run .              # engine boots, banner shows API + (optional) Swagger + Frontend
```

Smoke-test the live engine: hit `GET /health`, then exercise at least one custom injector via the rendering API or the embedded UI. Do not ship a wrapper whose registered extension was never invoked end-to-end.
