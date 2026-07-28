# Getting Started

Create a new project using doc-assembly as a library.

## Prerequisites

- **Go 1.25+** — [go.dev/dl](https://go.dev/dl/)
- **Typst** — [github.com/typst/typst/releases](https://github.com/typst/typst/releases)
- **PostgreSQL 16** — via Docker or local install
- **Docker** (optional) — for the included `docker-compose.yaml`

## 1. Scaffold a New Project

```bash
go run github.com/TetherEducation/doc-assembly/cmd/init@latest my-project \
  --module github.com/myorg/my-project
```

This creates:

```
my-project/
├── main.go                         # Entry point (Engine + extensions)
├── extensions/
│   ├── register.go                 # Register your injectors, mapper, providers
│   └── injectors/
│       └── example.go              # Example injector (replace with your own)
├── settings/
│   ├── app.yaml                    # Server, DB, auth, signing configuration
│   └── injectors.i18n.yaml         # Injector labels and descriptions
├── go.mod                          # Go module (requires doc-assembly)
├── docker-compose.yaml             # PostgreSQL for development
├── Dockerfile                      # Multi-stage production build
├── Makefile                        # Build, run, migrate, lint, docker
├── .env.example                    # Environment variable reference
├── .gitignore
└── .dockerignore
```

## 2. Install Dependencies

```bash
cd my-project
go mod tidy
```

### Local Development Against Unreleased doc-assembly

If working with a local clone of doc-assembly, add a `replace` directive to `go.mod`:

```
replace github.com/TetherEducation/doc-assembly => ../doc-assembly
```

Then re-run `go mod tidy`.

## 3. Start PostgreSQL

```bash
docker compose up -d
```

Default: `localhost:5432`, user `postgres`, password `postgres`, database `doc_assembly`.

> If port 5432 is in use, edit `docker-compose.yaml` ports (e.g., `"5433:5432"`) and update `settings/app.yaml` to match.

## 4. Run Migrations

```bash
go run . migrate
```

Expected output: `Migrations applied successfully (version: 7)`

This creates all schemas, tables, types, and indexes in the database.

## 5. Start the Server

```bash
go run .
```

The scaffolded project starts with dummy auth enabled by default, so no Keycloak is needed for development.

Expected output:

```
  doc-assembly is running

  API:       http://localhost:8080/api/v1
  Swagger:   http://localhost:8080/swagger/index.html
  Health:    http://localhost:8080/health
```

Or use the Makefile shortcut: `make dev` (runs migrations then starts with dummy auth).

## 6. Verify It Works

```bash
# Health check
curl http://localhost:8080/health
# → {"service":"doc-engine","status":"healthy"}

# Client config
curl http://localhost:8080/api/v1/config
# → {"dummyAuth":true}

# Authenticated ping (dummy auth accepts any Bearer token)
curl http://localhost:8080/api/v1/ping -H "Authorization: Bearer dummy"
# → {"message":"pong"}
```

## 7. Create Your First Tenant and Workspace

```bash
# Create a tenant
curl -s -X POST http://localhost:8080/api/v1/system/tenants \
  -H "Authorization: Bearer dummy" \
  -H "Content-Type: application/json" \
  -d '{"name":"My Company","code":"MYCO"}'

# Create a workspace (use the tenant ID from above)
curl -s -X POST http://localhost:8080/api/v1/tenant/workspaces \
  -H "Authorization: Bearer dummy" \
  -H "X-Tenant-ID: <tenant-id>" \
  -H "Content-Type: application/json" \
  -d '{"name":"Contracts","code":"CONTRACTS","type":"CLIENT"}'
```

## 8. Verify Injectors Are Registered

```bash
curl -s http://localhost:8080/api/v1/system/injectables \
  -H "Authorization: Bearer dummy"
```

You should see built-in datetime injectors + your `example_greeting` injector. Injectors are inactive by default; activate them via the admin API:

```bash
curl -s -X PATCH http://localhost:8080/api/v1/system/injectables/example_greeting/activate \
  -H "Authorization: Bearer dummy"
```

---

## Configuration Reference

### settings/app.yaml

All values can be overridden with environment variables using the `DOC_ENGINE_` prefix:

| YAML Path | Env Variable | Default | Description |
|---|---|---|---|
| `server.port` | `DOC_ENGINE_SERVER_PORT` | `8080` | HTTP server port |
| `database.host` | `DOC_ENGINE_DATABASE_HOST` | `localhost` | PostgreSQL host |
| `database.port` | `DOC_ENGINE_DATABASE_PORT` | `5432` | PostgreSQL port |
| `database.password` | `DOC_ENGINE_DATABASE_PASSWORD` | `postgres` | DB password |
| `auth.dummy` | `DOC_ENGINE_AUTH_DUMMY` | `true` | Skip JWT auth (dev only) |
| `signing.provider` | `DOC_ENGINE_SIGNING_PROVIDER` | `mock` | `mock`, `documenso` |
| `storage.provider` | `DOC_ENGINE_STORAGE_PROVIDER` | `local` | `local`, `s3` |
| `typst.bin_path` | `DOC_ENGINE_TYPST_BIN_PATH` | `typst` | Path to typst binary |

### Production Checklist

- [ ] Set `auth.dummy: false` and configure OIDC provider
- [ ] Set a strong `database.password`
- [ ] Configure `signing.provider` with real provider credentials
- [ ] Set `server.cors.allowed_origins` to your frontend domain
- [ ] Set `swagger_ui: false`
- [ ] Use environment variables or secrets manager (never commit `.env`)

---

## Next Steps

- **Add custom injectors** — See [Extensibility Guide](extensibility-guide.md)
- **Embed the frontend** — Build the React SPA and embed it with `engine.SetFrontendFS()`
- **Configure signing** — Set up Documenso or another provider for real signatures
- **Deploy** — Use the generated `Dockerfile` for production builds

---

## Makefile Targets

| Target | Description |
|---|---|
| `make build` | Build binary |
| `make run` | Run the application |
| `make run-dummy` | Run with dummy auth |
| `make dev` | Migrate + run with dummy auth |
| `make migrate` | Run database migrations |
| `make test` | Run tests |
| `make lint` | Run golangci-lint |
| `make docker-build` | Build Docker image |
| `make docker-run` | Build and run Docker image |
| `make clean` | Remove build artifacts |
