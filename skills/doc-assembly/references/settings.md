# Configuration — `settings/app.yaml` + `DOC_ENGINE_*` env vars

The engine loads its config via Viper:

1. If you call `sdk.NewWithConfig("path/to/app.yaml")`, that file is loaded.
2. Otherwise `sdk.New()` searches standard locations (typically `./settings/app.yaml`).
3. Env vars with prefix `DOC_ENGINE_` and dots replaced by underscores override file values automatically (e.g. YAML `database.host` ↔ `DOC_ENGINE_DATABASE_HOST`).
4. A handful of explicit env overrides also apply (table at the bottom of this doc).

Boolean envs accept `true`/`false`. Numeric envs are parsed; bad values are ignored.

## Full YAML schema (canonical)

```yaml
# Logical environment label. Used by EnvironmentAliases below and by
# `injectable_sources.env_resolution`.
environment: development

# Map of canonical env keys to alias lists. Lets you treat "staging",
# "qa", and "preprod" as the same canonical env when resolving sources.
environment_aliases:
  prod: ["production", "live"]
  dev:  ["development", "staging", "qa"]

server:
  port: "8080"                           # DOC_ENGINE_SERVER_PORT (also: PORT)
  base_path: ""                          # Mount the API + frontend under a sub-path
  public_url: "https://app.example.com"  # Used in outbound email links — REQUIRED in prod
  read_timeout: 30                       # seconds
  write_timeout: 30
  shutdown_timeout: 10
  swagger_ui: true                       # disable in prod if you do not want /swagger
  cors:
    allowed_origins: ["http://localhost:3000", "http://localhost:5173"]
    allowed_headers: []                  # extra headers; defaults are usually enough
  public_signing_frame_ancestors: []     # CSP frame-ancestors for the signing iframe
                                         # DOC_ENGINE_SERVER_PUBLIC_SIGNING_FRAME_ANCESTORS

database:
  host: localhost
  port: 5432
  user: postgres
  password: "postgres"
  name: doc_assembly
  ssl_mode: disable                      # disable | require | verify-full
  max_pool_size: 10
  min_pool_size: 2
  max_idle_time_seconds: 300

auth:
  dummy: false                           # set true ONLY for local dev
  panel:                                 # OIDC for the embedded admin UI + /api/v1/*
    name: keycloak
    discovery_url: ""                    # one of discovery_url OR (issuer + jwks_url)
    issuer: https://idp.example.com/realms/myrealm
    jwks_url: https://idp.example.com/realms/myrealm/protocol/openid-connect/certs
    audience: web-client
    client_id: web-client
    token_endpoint: ""                   # auto-filled from discovery
    userinfo_endpoint: ""                # auto-filled from discovery
    end_session_endpoint: ""             # auto-filled from discovery

signing_session_auth:
  mode: ""                               # "" | "oidc" | "custom"
  oidc:
    provider: panel                      # which configured OIDC provider to use
    email_claim: email                   # JWT claim that holds the recipient email

internal_api:
  enabled: false                         # mounts /internal/* for service-to-service calls

automation:
  max_body_bytes: 16777216               # DOC_ENGINE_AUTOMATION_MAX_BODY_BYTES — max request body for
                                         # /api/v1/automation; larger requests get 413 (default 16 MiB)

signing:
  provider: documenso                    # mock | documenso (or your own via SetSigningProvider)
  api_key: ""
  base_url: "https://app.documenso.com"
  signing_base_url: "https://app.documenso.com"
  webhook_secret: ""
  webhook_url: "https://app.example.com/webhooks/signing/documenso"

storage:
  enabled: true                          # If false, nothing is persisted; useful for stateless renders
  provider: local                        # local | s3 (or custom via SetStorageAdapter)
  local_dir: "./data/storage"
  bucket: ""
  region: ""
  endpoint: ""                           # custom S3-compatible endpoint (MinIO, Cloudflare R2, …)

logging:
  level: info                            # debug | info | warn | error
  format: json                           # json | text  (json is what the ContextHandler emits)

typst:
  bin_path: typst                        # absolute path or PATH-resolvable
  timeout_seconds: 30                    # per-render compile timeout
  font_dirs: []                          # extra directories to search for fonts
  max_concurrent: 10                     # global concurrent compile cap
  acquire_timeout_seconds: 5             # how long a render waits for a slot
  template_cache_ttl_seconds: 0          # 0 = disabled
  template_cache_max_entries: 0
  image_cache_dir: ""                    # remote image cache root
  image_cache_max_age_seconds: 0
  image_cache_cleanup_interval_seconds: 0

bootstrap:
  enabled: true                          # First user to log in becomes SUPERADMIN
                                         # Disable in prod after onboarding the first admin.

scheduler:
  enabled: true
  polling_interval_sec: 60
  polling_batch_size: 50
  expiration_days: 30                    # access tokens older than this are GC'd
  retry_max_retries: 5
  retry_batch_size: 20
  retry_interval_sec: 120

notification:
  provider: noop                         # noop | smtp | gmail (or custom via SetNotificationProvider)
  from: "no-reply@example.com"
  host: smtp.sendgrid.net
  port: 587
  username: ""
  password: ""

public_access:
  rate_limit_max: 5                      # access requests per recipient per window
  rate_limit_window_min: 15
  token_ttl_hours: 24

worker:
  enabled: true                          # set false to disable River workers in this process
  max_workers: 10
  failpoints_enabled: false              # NEVER true in prod
  failpoints: []                         # e.g. ["render.fail_once", "submit.timeout"]

injectable_sources:
  env_resolution:
    prod:
      order: ["workspace", "global"]     # which sources to consult, in priority order
    dev:
      order: ["workspace", "global", "fixture"]
```

Anything not listed here is private to the lib; do not rely on it.

## All `DOC_ENGINE_*` env overrides

Two layers of env support:

### 1. Generic Viper mapping (any leaf key)

`DOC_ENGINE_<SECTION>_<KEY>` overrides `<section>.<key>`. Nested structs use underscores per level. So all of these work even though they are not in the explicit table:

- `DOC_ENGINE_SERVER_PORT`
- `DOC_ENGINE_SERVER_PUBLIC_URL`
- `DOC_ENGINE_LOGGING_LEVEL`
- `DOC_ENGINE_TYPST_BIN_PATH`
- `DOC_ENGINE_TYPST_TIMEOUT_SECONDS`
- `DOC_ENGINE_SCHEDULER_ENABLED`
- `DOC_ENGINE_NOTIFICATION_FROM`
- `DOC_ENGINE_PUBLIC_ACCESS_TOKEN_TTL_HOURS`
- …etc.

Slices (`server.cors.allowed_origins`) accept comma-separated values via Viper, e.g. `DOC_ENGINE_SERVER_CORS_ALLOWED_ORIGINS=http://a,http://b`.

### 2. Explicit overrides (read directly by the lib)

These are the env vars the lib `Getenv`s explicitly — they always win over YAML and over the Viper mapping, and most exist because they are common ops knobs:

| Env var | YAML key | Notes |
|---|---|---|
| `PORT` | `server.port` | Cloud-run / Heroku style fallback when set. |
| `DOC_ENGINE_AUTH_DUMMY` | `auth.dummy` | `true` forces dummy auth. |
| `DOC_ENGINE_DUMMY_AUTH_USER_ID` | `(runtime)` | Pre-seeded user ID for dummy mode. |
| `DOC_ENGINE_DATABASE_HOST` | `database.host` | |
| `DOC_ENGINE_DATABASE_PORT` | `database.port` | |
| `DOC_ENGINE_DATABASE_USER` | `database.user` | |
| `DOC_ENGINE_DATABASE_PASSWORD` | `database.password` | |
| `DOC_ENGINE_DATABASE_NAME` | `database.name` | |
| `DOC_ENGINE_DATABASE_SSL_MODE` | `database.ssl_mode` | |
| `DOC_ENGINE_AUTH_PANEL_NAME` | `auth.panel.name` | |
| `DOC_ENGINE_AUTH_PANEL_DISCOVERY_URL` | `auth.panel.discovery_url` | |
| `DOC_ENGINE_AUTH_PANEL_ISSUER` | `auth.panel.issuer` | |
| `DOC_ENGINE_AUTH_PANEL_AUDIENCE` | `auth.panel.audience` | |
| `DOC_ENGINE_AUTH_PANEL_CLIENT_ID` | `auth.panel.client_id` | |
| `DOC_ENGINE_SIGNING_SESSION_AUTH_MODE` | `signing_session_auth.mode` | |
| `DOC_ENGINE_SIGNING_SESSION_AUTH_OIDC_PROVIDER` | `signing_session_auth.oidc.provider` | |
| `DOC_ENGINE_SIGNING_SESSION_AUTH_OIDC_EMAIL_CLAIM` | `signing_session_auth.oidc.email_claim` | |
| `DOC_ENGINE_SIGNING_PROVIDER` | `signing.provider` | |
| `DOC_ENGINE_SIGNING_API_KEY` | `signing.api_key` | |
| `DOC_ENGINE_SIGNING_BASE_URL` | `signing.base_url` | |
| `DOC_ENGINE_SIGNING_SIGNING_BASE_URL` | `signing.signing_base_url` | |
| `DOC_ENGINE_SIGNING_WEBHOOK_SECRET` | `signing.webhook_secret` | |
| `DOC_ENGINE_SIGNING_WEBHOOK_URL` | `signing.webhook_url` | |
| `DOC_ENGINE_STORAGE_ENABLED` | `storage.enabled` | |
| `DOC_ENGINE_STORAGE_PROVIDER` | `storage.provider` | |
| `DOC_ENGINE_STORAGE_LOCAL_DIR` | `storage.local_dir` | |
| `DOC_ENGINE_STORAGE_BUCKET` | `storage.bucket` | |
| `DOC_ENGINE_STORAGE_REGION` | `storage.region` | |
| `DOC_ENGINE_STORAGE_ENDPOINT` | `storage.endpoint` | |
| `DOC_ENGINE_BOOTSTRAP_ENABLED` | `bootstrap.enabled` | `false` disables the first-user auto-promotion. |
| `DOC_ENGINE_WORKER_ENABLED` | `worker.enabled` | |
| `DOC_ENGINE_WORKER_MAX_WORKERS` | `worker.max_workers` | |
| `DOC_ENGINE_WORKER_FAILPOINTS_ENABLED` | `worker.failpoints_enabled` | Refuses to enable when `environment` is prod. |
| `DOC_ENGINE_WORKER_FAILPOINTS` | `worker.failpoints` | Comma-separated. |
| `DOC_ENGINE_SERVER_PUBLIC_SIGNING_FRAME_ANCESTORS` | `server.public_signing_frame_ancestors` | Comma-separated origins. |

Any change to the above table belongs upstream in the lib; do not maintain a parallel list in your wrapper.

## What you typically put in YAML vs env

| Layer | Keep in YAML | Keep in env |
|---|---|---|
| Local dev | `settings/app.yaml` (committed) | `.env` (gitignored) for secrets only |
| Container | image ships YAML defaults | All secrets + per-env overrides via the platform's secret manager |
| Production | reasonable static defaults | DB password, `signing.api_key`, `signing.webhook_secret`, OIDC, `notification.password`, S3 creds |

Never commit secrets to YAML even in dev — the scaffolder's `.gitignore` already excludes `.env`, but `settings/app.yaml` is committed by default.

## Minimum config for first boot

```yaml
environment: development
server:
  port: "8080"
database:
  host: localhost
  port: 5432
  user: postgres
  password: postgres
  name: doc_assembly
  ssl_mode: disable
auth:
  dummy: true
typst:
  bin_path: typst
storage:
  provider: local
  local_dir: ./data/storage
notification:
  provider: noop
worker:
  enabled: true
bootstrap:
  enabled: true
```

Anything not set falls back to zero values; the engine logs warnings for missing required fields during preflight.
