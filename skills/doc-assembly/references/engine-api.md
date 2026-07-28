# Engine API Reference

The `Engine` is the only object you talk to. Construct → configure (fluent) → `Run()`.

```go
import "github.com/TetherEducation/doc-assembly/core/sdk"

engine := sdk.New()                           // default config search
engine := sdk.NewWithConfig("settings/app.yaml") // explicit path
```

Every method below returns `*Engine`, so configuration can be chained. They must be called **before** `Run()` — there is no hot-reconfiguration after start.

## Lifecycle

| Method | Notes |
|---|---|
| `Run() error` | Loads config → preflight → manual DI → `OnStart` hooks → scheduler → River workers → HTTP server. Blocks on `SIGINT`/`SIGTERM`. |
| `RunMigrations() error` | Loads config and applies pending SQL migrations embedded in the lib. Use as `go run . migrate` in your CLI flow. Does not start the server. |
| `OnStart(fn func(ctx context.Context) error)` | Hook fired AFTER preflight, BEFORE the HTTP server starts. Synchronous, registration order. Return error → boot aborts. |
| `OnShutdown(fn func(ctx context.Context) error)` | Hook fired AFTER server stops, BEFORE cleanup. Synchronous, **LIFO** order. Errors are logged but do not block shutdown. |

## Configuration / runtime

| Method | Notes |
|---|---|
| `SetI18nFilePath(path string)` | Path to your `injectors.i18n.yaml` (overrides built-in translations and labels for your custom injectors). The scaffolder writes one to `settings/injectors.i18n.yaml`. |

## Extensibility — data shaping

See [extensibility.md](extensibility.md) for full code samples.

| Method | Cardinality | Purpose |
|---|---|---|
| `RegisterInjector(inj sdk.Injector)` | many | Adds a custom injector. The `Code()` of each injector must be unique across the whole registry. |
| `SetMapper(m sdk.RequestMapper)` | one | Transforms incoming render request bodies into your typed payload. Only ONE mapper is allowed; route to multiple document types inside its `Map`. |
| `SetInitFunc(fn sdk.InitFunc)` | one | Runs once per render request before all injectors. Whatever you return is exposed to every injector via `injCtx.InitData()`. |
| `SetTemplateResolver(r sdk.TemplateResolver)` | one | Custom template version selection for the internal create flow. Return `(nil, nil)` to fall back to the default resolver. |
| `SetProcessResolver(r sdk.ProcessResolver)` | one | Lists / validates business processes per tenant. |
| `SetWorkspaceInjectableProvider(p sdk.WorkspaceInjectableProvider)` | one | Supplies dynamic, workspace-scoped injectables (e.g. tenant-defined fields). The provider handles its own i18n. |
| `GetTemplateResolver()` / `GetProcessResolver()` / `GetPublicDocumentAccessAuthenticator()` / `GetSigningSessionAuthenticator()` | — | Inspectors used mostly by tests. |

## External systems — providers and adapters

See [signing.md](signing.md) and [adapters.md](adapters.md).

| Method | Default | Override when |
|---|---|---|
| `SetSigningProvider(sp sdk.SigningProvider)` | auto-built from `signing.provider` config (`mock`/`documenso`) | You need a custom provider (PandaDoc/DocuSign/in-house). |
| `SetWebhookHandlers(handlers map[string]sdk.WebhookHandler)` | auto-built per provider | You implement custom webhook parsing. |
| `SetStorageAdapter(sa sdk.StorageAdapter)` | auto-built from `storage.provider` config | You need S3/GCS/your-own. |
| `SetNotificationProvider(np sdk.NotificationProvider)` | auto-built from `notification.provider` | You ship via SES/SendGrid/Slack/etc. |

## Auth hooks

| Method | Affects |
|---|---|
| `SetPublicDocumentAccessAuthenticator(auth sdk.PublicDocumentAccessAuthenticator)` | `/public/doc/:documentId` — bypass the email gate when your auth succeeds. |
| `SetSigningSessionAuthenticator(auth sdk.SigningSessionAuthenticator)` | `/api/v1/signing-sessions/:documentId` — only used when `signing_session_auth.mode == "custom"`. |
| `SetSigningSessionAuthMode(mode string)` | Overrides `signing_session_auth.mode` from config. |
| `SetLegacyDocumentHandler(handler sdk.LegacyDocumentHandler)` | Mounts `POST /api/v1/legacy-documents/proxy` for host-owned legacy document access negotiation. No handler means no route. |

## Legacy Document Proxy

Use `SetLegacyDocumentHandler` only for documents outside the current doc-assembly document lifecycle. It is not a custom route framework and must not be used as an alternate access path for Doc Assembly Documents.

The library validates only:

- method is `POST`
- `X-Workspace-Code` is present
- `X-Environment` is present and is exactly `dev` or `prod`
- request body is within `legacy_documents.max_body_bytes`

The handler owns authentication, authorization, request parsing, legacy lookup, and JSON response semantics.

## HTTP middleware

| Method | Where it runs |
|---|---|
| `UseMiddleware(mw gin.HandlerFunc)` | Global. Order: Recovery → Logger → CORS → **your global** → routes. |
| `UseAPIMiddleware(mw gin.HandlerFunc)` | Only `/api/v1/*`. Order: Operation → Auth → Identity → Roles → **your API** → controller. |

Use these for request enrichment (custom headers, audit trails, tenant resolution overrides). Do not use them for auth — there are dedicated authenticators.

## Customization (look & feel, embedded UI)

| Method | Purpose |
|---|---|
| `SetDesignTokens(tokens sdk.TypstDesignTokens)` | Fonts/colors/spacing/heading styles for Typst-rendered PDFs. Falls back to `sdk.DefaultDesignTokens()` when not set. |
| `SetFrontendFS(fsys fs.FS)` | Replace the embedded React SPA. Pass `nil` to disable frontend serving entirely (API-only deployment). |

## Worker / completion

| Method | Purpose |
|---|---|
| `OnDocumentCompleted(fn sdk.DocumentCompletedHandler)` | Callback invoked from within the River `dispatch_attempt_completion` worker when a document reaches `COMPLETED`. Return an error to retry with backoff. See [completion-events.md](completion-events.md). |

## Default resolution rules

When a `Set*` for an external system is **not** called:
- `SigningProvider` is built from `signing.provider` (`mock`/`documenso` today). `mock` is dev-only.
- `StorageAdapter` is built from `storage.provider` (`local`/`s3`).
- `NotificationProvider` is built from `notification.provider` (`noop`/`smtp`/`gmail`).
- `WebhookHandler` is auto-paired with the active `SigningProvider`.

When you `Set*` an explicit override the corresponding config key is ignored, but the **other** config (e.g. `signing.api_key`, `storage.bucket`) is still loaded and may be consumed by your implementation if you choose to read it via `sdk.NewWithConfig` and your own helpers — usually you do not need to.
