# Customization — Design Tokens, Frontend, Middleware, Lifecycle Hooks

These do not change *what* the engine does, only *how it looks* and *how it boots*.

## TypstDesignTokens — PDF look & feel

The engine renders PDFs with Typst. `TypstDesignTokens` controls fonts, colors, spacing, and heading styles applied to every rendered template:

```go
import "github.com/TetherEducation/doc-assembly/core/sdk"

tokens := sdk.DefaultDesignTokens()
tokens.BodyFont = "Inter"
tokens.HeadingFont = "IBM Plex Sans"
tokens.PrimaryColor = "#0B5FFF"

engine.SetDesignTokens(tokens)
```

If you do not call `SetDesignTokens`, the lib uses `sdk.DefaultDesignTokens()`. To inspect every available field, read [core/internal/core/service/rendering/pdfrenderer/](../../../core/internal/core/service/rendering/pdfrenderer/) — the `TypstDesignTokens` struct lives there and is re-exported via [core/sdk/design.go](../../../core/sdk/design.go).

For custom fonts: install them in the host or container, then point `typst.font_dirs` (in YAML) at the directory. Tokens reference fonts by name, not by file path.

## Frontend FS — replace or disable the embedded SPA

The lib ships a React SPA embedded at build time. You have three options:

```go
// 1. Default: serve the embedded SPA — do nothing.

// 2. Replace with your own pre-built static bundle:
//    layout: index.html + assets/* at the FS root.
engine.SetFrontendFS(os.DirFS("./web/dist"))

// 3. Disable frontend serving entirely (API-only deployment):
engine.SetFrontendFS(nil)
```

When disabled, the engine still mounts `/api/v1/*`, `/public/*`, `/health`, optionally `/swagger/*` and `/internal/*`, and `/webhooks/signing/*`. Anything else returns 404.

For embedded signing inside an iframe served by another origin, set `server.public_signing_frame_ancestors` in YAML (or `DOC_ENGINE_SERVER_PUBLIC_SIGNING_FRAME_ANCESTORS`) to allow that origin in the CSP `frame-ancestors`.

## Middleware

You can insert middleware at two layers:

```go
engine.UseMiddleware(myGlobalMW)    // Global
engine.UseAPIMiddleware(myAPIMW)    // /api/v1/* only
```

| Layer | Order |
|---|---|
| Global | Recovery → Logger → CORS → **your global middleware** → routes |
| API (`/api/v1/*`) | Operation → Auth → Identity → Roles → **your API middleware** → controller |

Use cases:

| Need | Layer |
|---|---|
| Inject a request ID header on everything | Global |
| Audit log every authenticated API call | API |
| Forward upstream tracing context | Global |
| Enforce rate limits on `/api/v1/*` only | API |

**Do not** use middleware for authentication of `/public/*` or `/api/v1/signing-sessions/*` — there are dedicated authenticator hooks (see [adapters.md](adapters.md)). Do not strip / rewrite the `X-Tenant-ID` / `X-Workspace-ID` headers from inside middleware; the identity middleware relies on them.

`UseMiddleware`/`UseAPIMiddleware` are append-only — each call adds to the list, in the order you call them. There is no remove.

## Lifecycle hooks: `OnStart` / `OnShutdown`

Wire optional setup/teardown to the engine's own boot:

```go
engine.OnStart(func(ctx context.Context) error {
    return warmCache(ctx)
})

engine.OnStart(func(ctx context.Context) error {
    return primeSearchIndex(ctx)
})

engine.OnShutdown(func(ctx context.Context) error {
    return flushMetrics(ctx)
})
```

Rules:

- `OnStart` runs **after** preflight and DI, **before** the HTTP server starts. Synchronous, in registration order. A returned error aborts boot — the engine exits non-zero.
- `OnShutdown` runs **after** the HTTP server stops, **before** cleanup of the DB pool / River workers. Synchronous, in **reverse** registration order (LIFO). Errors are logged but do not block shutdown.
- The `ctx` for shutdown is already cancelled; respect deadlines if your work is bounded.
- Both hook lists are append-only.

Use `OnStart` for: warm caches, validate external connectivity beyond the lib's preflight, register your own `prometheus` collectors, run additional one-off migrations.

Use `OnShutdown` for: flush metrics/traces, drain in-flight outbound work that is NOT tracked by River, close third-party clients with state.

## Logger / `slog`

The engine sets up the default `slog` logger with a JSON handler + the lib's `ContextHandler`, configured to the level in `logging.level`. Everything inside the lib uses `slog.InfoContext` etc. and reads attributes from `context.Context`.

For your own code in injectors, mappers, providers, hooks:

```go
slog.InfoContext(ctx, "rendered greeting", slog.String("workspace", injCtx.WorkspaceCode()))
```

Do **not** call `slog.Info`/`Warn`/`Error` (no context — you lose `tenant_id`, `workspace_id`, `operation_id`). Do **not** instantiate your own logger; the engine's default already routes everything through the `ContextHandler`.

If you need to attach extra attributes for the rest of a request, use the lib's helper:

```go
import "github.com/TetherEducation/doc-assembly/core/internal/infra/logging"
// NOTE: logging is private; if you need WithAttrs in your wrapper,
// either request it be exposed via SDK or use slog.With locally.
ctx = logging.WithAttrs(ctx, slog.String("request_id", id))
```

That import is private and not stable — prefer `slog.With(...)` inside your wrapper code unless you are PR'ing the lib.
