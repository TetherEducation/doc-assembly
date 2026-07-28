# doc-assembly

**Multi-tenant document template builder with digital signature delegation.**

Go 1.25 &middot; React 19 &middot; PostgreSQL 16 &middot; Typst

---

## Table of Contents

- [Overview](#overview)
- [Monorepo Structure](#monorepo-structure)
- [Architecture](#architecture)
- [Quick Start](#quick-start)
- [Available Commands](#available-commands)
- [Configuration](#configuration)
- [Digital Signatures](#digital-signatures)
- [Background Workers (River)](#background-workers-river)
- [Database](#database)
- [Deployment](#deployment)
- [Documentation](#documentation)
- [License](#license)

## Overview

doc-assembly lets organizations build reusable document templates with a rich text editor, inject dynamic data through a variable system, render production-quality PDFs via Typst, and delegate digital signatures to external providers.

The platform is multi-tenant by design: a three-level RBAC model (System, Tenant, Workspace) controls access, while the backend enforces tenant isolation across every query.

### Key Features

- Rich text template editor (TipTap) with injectable variables and signature blocks
- PDF rendering powered by Typst (no browser/Chromium dependency)
- Digital signature delegation to Documenso (PandaDoc and DocuSign interfaces planned)
- Three-level RBAC: System, Tenant, and Workspace roles
- Template versioning with publish/archive lifecycle
- Extensibility system via code-generated injectors and mappers
- Internationalization (English & Spanish)
- Folder and tag organization for templates

## Monorepo Structure

```plaintext
doc-assembly/
  core/       Go backend (Hexagonal Architecture, Gin, Wire DI)
  app/        React SPA (TanStack Router, Zustand, TipTap)
  db/         Liquibase migrations (PostgreSQL 16)
  docs/       All project documentation
  scripts/    Tooling (docml2json, etc.)
```

| Component      | Stack                                                    | Docs                              |
| -------------- | -------------------------------------------------------- | --------------------------------- |
| **doc-engine** | Go 1.25, Gin, pgx/v5, Wire                               | [Backend docs](docs/backend/)     |
| **web-client** | React 19, TypeScript, TanStack Router, Zustand, TipTap 3 | [Frontend docs](docs/frontend/)   |
| **db**         | Liquibase, PostgreSQL 16, pgcrypto                       | [DATABASE.md](db/DATABASE.md)     |
| **scripts**    | Python 3 tooling                                         | [docml2json](scripts/docml2json/) |

## Architecture

### Request Flow (Hexagonal)

```plaintext
HTTP Request
 -> Middleware (JWT auth, tenant context, operation ID)
   -> Controller (parse DTO, validate)
     -> UseCase interface
       -> Service (business logic)
         -> Port interface
           -> Repository (SQL via pgx)
             -> PostgreSQL
```

### Multi-Tenant Hierarchy

```plaintext
System
 +-- Tenant A
 |    +-- Workspace 1
 |    +-- Workspace 2
 +-- Tenant B
      +-- Workspace 3
```

### RBAC

Three authorization levels with seven roles:

| Level         | Roles                                            |
| ------------- | ------------------------------------------------ |
| **System**    | `SUPERADMIN`, `PLATFORM_ADMIN`                   |
| **Tenant**    | `OWNER`, `ADMIN`                                 |
| **Workspace** | `OWNER`, `ADMIN`, `EDITOR`, `OPERATOR`, `VIEWER` |

`SUPERADMIN` auto-elevates to `OWNER` in any tenant or workspace.

## Quick Start

### Prerequisites

| Tool          | Version | Install                                                         |
| ------------- | ------- | --------------------------------------------------------------- |
| Go            | 1.25+   | [go.dev/dl](https://go.dev/dl/)                                 |
| Node.js       | 20+     | [nodejs.org](https://nodejs.org/)                               |
| pnpm          | 9+      | `npm i -g pnpm`                                                 |
| Docker        | latest  | [docker.com](https://www.docker.com/)                           |
| Typst         | 0.13+   | [typst.app](https://github.com/typst/typst/releases)            |
| golangci-lint | latest  | [golangci-lint.run](https://golangci-lint.run/welcome/install/) |
| Wire          | latest  | `go install github.com/google/wire/cmd/wire@latest`             |

Run `make doctor` to verify all dependencies are installed.

### 1. Clone and install dependencies

```bash
git clone https://github.com/your-org/doc-assembly.git
cd doc-assembly
pnpm install --dir apps/web-client
go -C apps/doc-engine mod download
```

### 2. Start PostgreSQL

```bash
docker compose -f docker-compose.dev.yml up -d
```

This starts PostgreSQL 16 on port 5432.

### 3. Run database migrations

```bash
cd db && ./run-migrations.sh && cd ..
```

### 4. Start dev servers

```bash
make dev-dummy
```

> [!TIP]
> `dev-dummy` bypasses JWT authentication so you can develop without setting up Keycloak. The backend runs on `:8080` and the frontend on `:3001`.

The app is now available at **<http://localhost:3001>**. The first user to sign up is automatically promoted to `SUPERADMIN`.

## Available Commands

Run `make help` for the full list.

| Command                 | Description                              |
| ----------------------- | ---------------------------------------- |
| `make dev`              | Hot reload backend + frontend            |
| `make dev-dummy`        | Dev with dummy auth (no Keycloak needed) |
| `make build`            | Build backend + frontend                 |
| `make test`             | Run unit tests                           |
| `make test-integration` | Run integration tests (Docker required)  |
| `make lint`             | Lint backend + frontend                  |
| `make gen`              | Codegen: Wire DI + Swagger + Extensions  |
| `make doctor`           | Check system dependencies                |
| `make clean`            | Remove all build artifacts               |

Pass `DUMMY=1` to any target to enable dummy auth: `make run DUMMY=1`.

## Configuration

### Backend

Configuration is loaded from `apps/doc-engine/settings/app.yaml` and can be overridden with environment variables following the pattern `DOC_ENGINE_<SECTION>_<KEY>`.

Copy the example env file and fill in values:

```bash
cp apps/doc-engine/.env.example apps/doc-engine/.env
```

Key variables:

| Variable                          | Required | Description                           |
| --------------------------------- | -------- | ------------------------------------- |
| `DOC_ENGINE_DATABASE_PASSWORD`    | Yes      | PostgreSQL password                   |
| `DOC_ENGINE_AUTH_JWKS_URL`        | Yes\*    | Keycloak JWKS endpoint                |
| `DOC_ENGINE_AUTH_ISSUER`          | Yes\*    | Keycloak issuer URL                   |
| `DOC_ENGINE_AUTH_AUDIENCE`        | Yes\*    | JWT audience claim                    |
| `DOC_ENGINE_AUTH_DUMMY`           | No       | Set `true` to bypass JWT              |
| `DOC_ENGINE_DOCUMENSO_API_KEY`    | No       | Documenso API key (for signing)       |

\* Not required when `DOC_ENGINE_AUTH_DUMMY=true`.

> [!NOTE]
> See [`apps/doc-engine/settings/app.yaml`](apps/doc-engine/settings/app.yaml) for all available options including storage, logging, scheduler, Typst renderer, and notification settings.

### Frontend

```bash
cp apps/web-client/.env.example apps/web-client/.env
```

| Variable                  | Default                 | Description                    |
| ------------------------- | ----------------------- | ------------------------------ |
| `VITE_API_URL`            | `/api/v1`               | Backend API base URL           |
| `VITE_KEYCLOAK_URL`       | `http://localhost:8180` | Keycloak server URL            |
| `VITE_KEYCLOAK_REALM`     | `doc-assembly`          | Keycloak realm                 |
| `VITE_KEYCLOAK_CLIENT_ID` | `web-client`            | Keycloak client ID             |
| `VITE_USE_MOCK_AUTH`      | `true`                  | Bypass Keycloak in development |

## Digital Signatures

doc-assembly delegates digital signatures to external providers. Each document has a **shared public URL** (`/public/doc/{id}`) that recipients use to verify their email and receive a signing link.

```plaintext
Template (published)
  -> Admin creates document
    -> Recipients notified with public URL (/public/doc/{id})
      -> Recipient visits URL, enters email
        -> System verifies email, sends token link (/public/sign/{token})
          -> Path A (no interactive fields): PDF preview -> Sign
          -> Path B (interactive fields): Fill form -> PDF preview -> Sign
            -> Signing provider handles signature -> Webhooks update status -> Sealed PDF stored
```

For detailed flow documentation with sequence diagrams, see **[Public Signing Flow](docs/backend/public-signing-flow.md)**.

### Supported Providers

| Provider                            | Status            |
| ----------------------------------- | ----------------- |
| [Documenso](https://documenso.com/) | Implemented       |
| PandaDoc                            | Interface defined |
| DocuSign                            | Interface defined |

### Local Documenso Setup

```bash
docker compose -f docker-compose.documenso.yml up -d
```

> [!IMPORTANT]
> The compose file includes a `documenso-cert-init` service that auto-generates a self-signed P12 certificate for document sealing. No manual certificate setup is needed.

This starts:

- **Documenso** on `http://localhost:3000`
- **MailPit** (SMTP) on `http://localhost:8025` (web UI) and `:1025` (SMTP)
- **PostgreSQL** for Documenso on port `5433`

Configure the webhook in Documenso to point to `http://host.docker.internal:8080/webhooks/signing/documenso`.

## Background Workers (River)

Signing execution is attempt-scoped and durable. `execution.documents` is the business projection, while `execution.signing_attempts` is the technical source of truth for render, provider submission, retry/reconciliation, cleanup, refresh, and completion dispatch. River runs inside the API process and stores jobs in PostgreSQL; no external broker is required.

### Attempt-scoped jobs

`ProceedToSigning` creates or reuses the active signing attempt and enqueues River work transactionally. Provider upload does **not** happen inline in the public/authenticated request path.

```plaintext
/public/sign/{token}/proceed
  -> create/reuse active SigningAttempt
  -> INSERT river_job(render_attempt_pdf) in same PostgreSQL transaction
  -> frontend receives step=processing and polls

River render_attempt_pdf
  -> render immutable pre-signed PDF
  -> persist PDF path/checksum + enqueue submit_attempt_to_provider in one transaction

River submit_attempt_to_provider
  -> submit PDF snapshot to provider using correlation key {document_id}:{attempt_id}
  -> persist provider IDs + READY_TO_SIGN projection

Provider webhook
  -> resolve active attempt
  -> update attempt + document projection
  -> enqueue dispatch_attempt_completion in the same transaction
```

### Key properties

| Property | Behavior |
|----------|----------|
| **Source of truth** | `SigningAttempt` owns technical signing state; `Document` exposes only the current business projection via `active_attempt_id`. |
| **Atomic enqueue** | Attempt state transitions and next River job enqueue happen in one PostgreSQL transaction. |
| **Idempotency** | Jobs are unique by `attempt_id + phase`; stale jobs no-op when they no longer match the document active attempt. |
| **Recovery** | Transient provider failures retry the same attempt; ambiguous provider results reconcile by correlation key or move to review. |
| **Regeneration** | A new attempt supersedes the old one; historical attempts and late webhooks cannot mutate the active document projection. |
| **Completion dispatch** | Completion events are still delivered through River, but the dispatch job is attempt-aware and checks `document.active_attempt_id`. |

### SDK Usage

```go
import "github.com/TetherEducation/doc-assembly/core/sdk"

handler := func(ctx context.Context, ev sdk.DocumentCompletedEvent) error {
    log.Printf("Document %s completed in tenant %s", ev.DocumentID, ev.TenantCode)
    for _, r := range ev.Recipients {
        log.Printf("  %s (%s) signed at %v", r.Name, r.RoleName, r.SignedAt)
    }
    return nil // return error to retry completion dispatch
}
```

### Configuration

```yaml
worker:
  enabled: false              # DOC_ENGINE_WORKER_ENABLED
  max_workers: 10             # DOC_ENGINE_WORKER_MAX_WORKERS
  runtime_environment: local  # DOC_ENGINE_WORKER_RUNTIME_ENVIRONMENT
  failpoints_enabled: false   # DOC_ENGINE_WORKER_FAILPOINTS_ENABLED (dev/test only)
  failpoints: []              # DOC_ENGINE_WORKER_FAILPOINTS
```

For architecture diagrams, attempt job details, failpoints, and integration test coverage, see **[Worker Queue Guide](docs/backend/worker-queue-guide.md)**.

## Database

PostgreSQL 16 with five schemas:

| Schema      | Purpose                                        |
| ----------- | ---------------------------------------------- |
| `tenancy`   | Tenants, workspaces, memberships               |
| `identity`  | Users, access history                          |
| `organizer` | Folders, tags                                  |
| `content`   | Templates, versions, injectables, signer roles |
| `execution` | Documents, recipients, signing attempts, events |

Migrations are managed with Liquibase:

```bash
cd db && ./run-migrations.sh
```

> [!WARNING]
> Do not modify migration files in `db/src/` directly. Suggest changes and create new changesets instead.

See [`db/DATABASE.md`](db/DATABASE.md) for the complete schema documentation.

## Deployment

### Docker Build

```bash
docker build -f apps/doc-engine/Dockerfile -t doc-engine .
```

The Dockerfile uses a multi-stage build:

1. **Builder**: `golang:1.25-alpine` compiles the binary
2. **Runtime**: `alpine:3.21` with Typst v0.13.1 and ca-certificates

The container exposes port `8080`.

### Required Environment Variables (Production)

| Category     | Variables                                                          |
| ------------ | ------------------------------------------------------------------ |
| **Database** | `DOC_ENGINE_DATABASE_HOST`, `_PORT`, `_USER`, `_PASSWORD`, `_NAME` |
| **Auth**     | `DOC_ENGINE_AUTH_JWKS_URL`, `_ISSUER`, `_AUDIENCE`                 |
| **Signing**  | `DOC_ENGINE_DOCUMENSO_API_URL`, `_API_KEY`, `_WEBHOOK_SECRET`      |
| **Storage**  | `DOC_ENGINE_STORAGE_BUCKET`, `_REGION` (for S3)                    |

## Documentation

| Document                            | Path                                                                                         |
| ----------------------------------- | -------------------------------------------------------------------------------------------- |
| Backend Architecture                | [`docs/backend/architecture.md`](docs/backend/architecture.md)                               |
| Authentication Guide                | [`docs/backend/authentication-guide.md`](docs/backend/authentication-guide.md)               |
| Authorization Matrix                | [`docs/backend/authorization-matrix.md`](docs/backend/authorization-matrix.md)               |
| Public Signing Flow                 | [`docs/backend/public-signing-flow.md`](docs/backend/public-signing-flow.md)                 |
| Template Preview Flow               | [`docs/template-preview-flow.md`](docs/template-preview-flow.md)                             |
| Internal API Document Creation Flow | [`docs/internal-api-document-creation-flow.md`](docs/internal-api-document-creation-flow.md) |
| Public Signing Flow (Flow Detail)   | [`docs/public-signing-flow-detail.md`](docs/public-signing-flow-detail.md)                   |
| Extensibility Guide                 | [`docs/backend/extensibility-guide.md`](docs/backend/extensibility-guide.md)                 |
| Signing Attempts + River Guide      | [`docs/backend/worker-queue-guide.md`](docs/backend/worker-queue-guide.md)                   |
| Frontend Architecture               | [`docs/frontend/architecture.md`](docs/frontend/architecture.md)                             |
| Design System                       | [`docs/frontend/design-system.md`](docs/frontend/design-system.md)                           |
| Database Schema                     | [`db/DATABASE.md`](db/DATABASE.md)                                                           |
| OpenAPI Spec                        | [`core/docs/swagger.yaml`](core/docs/swagger.yaml)                                           |
| docml2json Reference                | [`scripts/docml2json/DOCML-REFERENCIA.md`](scripts/docml2json/DOCML-REFERENCIA.md)           |

## License

This project is licensed under the [MIT License](LICENSE).
