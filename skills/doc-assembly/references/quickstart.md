# Quickstart — building a doc-assembly wrapper

Goal: a Go binary that imports `github.com/rendis/doc-assembly/core/sdk`, registers your extensions, and serves the engine.

## 1. Scaffold

There is a generator embedded in the lib that produces a complete wrapper:

```bash
go run github.com/rendis/doc-assembly/cmd/init@latest my-wrapper \
  --module github.com/myorg/my-wrapper
```

It writes (skipping any pre-existing file unless `--force`):

| Path | Purpose |
|---|---|
| `main.go` | calls `sdk.New()`, `extensions.Register(engine)`, `engine.Run()` |
| `extensions/register.go` | central place for all your `engine.Register*/Set*` calls |
| `extensions/injectors/example.go` | example custom injector |
| `settings/app.yaml` | full config — copy and adjust |
| `settings/injectors.i18n.yaml` | UI labels for your injectors (locale → label/description) |
| `Makefile` | `build`, `run`, `run-dummy`, `dev`, `migrate`, `lint`, `docker-*` |
| `Dockerfile` | multi-stage build with Typst preinstalled |
| `docker-compose.yaml` | local Postgres for dev |
| `.env.example` | `DOC_ENGINE_*` overrides |
| `go.mod` | requires `github.com/rendis/doc-assembly` |

After scaffolding:

```bash
cd my-wrapper
go mod tidy
docker compose up -d        # local Postgres
go run . migrate            # apply lib-owned migrations to your DB
go run .                    # boots the engine
```

You should see a startup banner with API + Swagger + Frontend URLs.

## 2. Manual setup (if you do not use the scaffolder)

Minimum `main.go`:

```go
package main

import (
    "fmt"
    "os"

    "github.com/rendis/doc-assembly/core/sdk"
)

func main() {
    if len(os.Args) > 1 && os.Args[1] == "migrate" {
        if err := sdk.New().RunMigrations(); err != nil {
            fmt.Fprintln(os.Stderr, err)
            os.Exit(1)
        }
        return
    }

    engine := sdk.New()
    // engine.RegisterInjector(...) etc. — see references/extensibility.md
    if err := engine.Run(); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
```

You also need:

- A `settings/app.yaml` (or pass the path to `sdk.NewWithConfig("…")`).
- A reachable Postgres 16 instance.
- Typst on `PATH` (or set `typst.bin_path` in the config).

## 3. The two commands you actually run

| Command | What it does |
|---|---|
| `go run . migrate` | Loads config + applies pending lib-owned SQL migrations. Idempotent — safe to re-run. Run before every deploy whose lib version bumped. |
| `go run .` (or built binary) | Loads config → preflight → manual DI → `OnStart` hooks → scheduler → River workers → HTTP server → blocks on `SIGINT`/`SIGTERM` → `OnShutdown` hooks (LIFO) → cleanup. |

## 4. Local development with dummy auth

The lib supports a dummy-auth mode that bypasses JWT verification — convenient for first-run smoke tests.

```bash
DOC_ENGINE_AUTH_DUMMY=true go run .
# or, using the scaffolded Makefile:
make run-dummy
```

The first user that signs up via the embedded UI is auto-promoted to `SUPERADMIN`. **Never enable this in production** — see [pitfalls.md](pitfalls.md).

## 5. Local signing provider (Documenso)

If your wrapper sets `signing.provider: documenso`, run a local Documenso alongside Postgres:

```bash
# From the doc-assembly repo (or copy this compose file into your wrapper):
docker compose -f docker-compose.documenso.yml up -d
```

Then in your wrapper's `settings/app.yaml`:

```yaml
signing:
  provider: documenso
  base_url: http://localhost:3000
  signing_base_url: http://localhost:3000
  api_key: <token>             # from Documenso settings UI
  webhook_secret: <secret>
  webhook_url: http://host.docker.internal:8080/webhooks/signing/documenso
```

Configure the webhook in Documenso to point to the same `webhook_url`.

For other providers (PandaDoc, DocuSign, your own): implement [`sdk.SigningProvider`](signing.md) and register it via `engine.SetSigningProvider(...)`. You no longer need `signing.provider` in config (the override wins).

## 6. Working against a local clone of doc-assembly

In your wrapper's `go.mod`:

```go
replace github.com/rendis/doc-assembly => ../doc-assembly
```

Run `go mod tidy` to refresh. Remember to remove the replace before publishing.

## 7. Verification before "ready"

| Check | How |
|---|---|
| Wrapper compiles | `go build ./...` |
| Lib code is not referenced beyond `core/sdk` | `grep -rE 'doc-assembly/core/(internal|cmd)' .` should return nothing in your wrapper |
| Migrations apply | `go run . migrate` exits 0 |
| Engine boots | `go run .` prints the banner; `GET /health` returns 200 |
| Custom injector reachable | render a template that uses its `Code()` and confirm the value appears in the resulting PDF |
| Completion handler invoked (if used) | sign a document end-to-end and confirm your `OnDocumentCompleted` callback fires |
