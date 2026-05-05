# Adapters — Storage, Notifications, Auth

These plug *external systems* into the engine. None of them shape data (that is [extensibility.md](extensibility.md)). For signature providers see [signing.md](signing.md).

## StorageAdapter

Object storage for rendered PDFs, completed PDFs, gallery assets, etc. The engine talks to a single adapter; you decide where bytes go.

Default behavior: built from `storage.provider` config (`local` or `s3`). Override:

```go
engine.SetStorageAdapter(&MyS3Adapter{client: ...})
```

### Interface

```go
type StorageAdapter interface {
    Upload(ctx context.Context, req *sdk.StorageUploadRequest) error
    Download(ctx context.Context, req *sdk.StorageRequest) ([]byte, error)
    GetURL(ctx context.Context, req *sdk.StorageRequest) (string, error)
    Delete(ctx context.Context, req *sdk.StorageRequest) error
    Exists(ctx context.Context, req *sdk.StorageRequest) (bool, error)
}
```

`StorageUploadRequest.Key` is opaque to your adapter; treat it as a logical path. `Environment` is `dev` or `prod` — segregate the bucket prefix per environment to avoid cross-leak between sandbox and production data.

`GetURL` may return a presigned URL or a stable public URL — your choice. The engine never assumes the URL is signed.

## NotificationProvider

Outbound email (signing invitations, access tokens, completion notices). Single provider per engine.

Default: built from `notification.provider` (`noop` / `smtp` / `gmail`). Override:

```go
engine.SetNotificationProvider(&SESProvider{client: ...})
```

### Interface

```go
type NotificationProvider interface {
    Send(ctx context.Context, req *sdk.NotificationRequest) error
}
```

`NotificationRequest` carries `To`, `Subject`, `HTMLBody`, `TextBody`, `ReplyTo`, `Environment`, and `Attachments []NotificationAttachment{Filename, ContentType, Data}`. Honor `ReplyTo` even if your transport would default to the `From` address.

Failures bubble up to the caller. The engine retries idempotent flows (e.g. resending an access link from a re-poll) but does not retry from inside `Send`. Surface transient errors so the engine can decide.

## PublicDocumentAccessAuthenticator

Custom auth for `/public/doc/:documentId` so the recipient can skip the email gate. Useful when your wrapper already has a session/JWT/SSO that proves who the visitor is.

```go
engine.SetPublicDocumentAccessAuthenticator(&MyPubAuth{})
```

### Interface

```go
type PublicDocumentAccessAuthenticator interface {
    Authenticate(c *gin.Context, req *sdk.AuthenticateRequest) (*sdk.PublicDocumentAccessClaims, error)
}
```

Return rules:

| Return | Meaning |
|---|---|
| `(nil, nil)` | Fall back to the email gate (anonymous visitor). |
| `(claims, nil)` | Direct access; engine will mint a tokenized signing URL for `claims.Email`. |
| `(nil, err)` | Auth failed; falls back to the email gate. |

`PublicDocumentAccessClaims.Email` MUST match a recipient on the document, otherwise the engine treats it as anonymous. Use `Provider`/`Subject`/`Extra` for audit logging.

## SigningSessionAuthenticator

Custom auth for the authenticated signing endpoint `/api/v1/signing-sessions/:documentId` — only invoked when `signing_session_auth.mode == "custom"` (or you call `SetSigningSessionAuthMode("custom")`).

```go
engine.SetSigningSessionAuthMode("custom")
engine.SetSigningSessionAuthenticator(&MyAuth{})
```

### Interface

```go
type SigningSessionAuthenticator interface {
    Authenticate(c *gin.Context, req *sdk.SigningSessionAuthenticateRequest) (*sdk.SigningSessionAuthClaims, error)
}
```

`SigningSessionAuthClaims.Email` is matched against document recipients. Other fields (`Subject`, `Provider`, `Extra`) are passed through for logging / downstream use.

The other modes are:

| Mode | Behavior |
|---|---|
| `oidc` | Engine validates JWT against the OIDC provider in `signing_session_auth.oidc.provider` and uses `email_claim` to extract the recipient email. No SDK code needed. |
| `custom` | Calls your `SigningSessionAuthenticator`. |
| (empty) | Endpoint disabled — public flow only. |

## Webhook secret / public URL

Signing providers and notifications need stable public URLs the outside world can reach:

| Setting | Used for |
|---|---|
| `server.public_url` | Base URL the engine uses when generating public links inside email bodies. Critical when running behind a reverse proxy or in containers. |
| `signing.webhook_url` | Public URL of the engine's signing webhook endpoint (`/webhooks/signing/{providerName}`). Configure the same value in the provider's webhook setup. |
| `signing.webhook_secret` | Shared secret used to validate inbound webhook signatures in your `WebhookHandler.ParseWebhook`. |
| `server.public_signing_frame_ancestors` | Domains allowed to embed the signing iframe via CSP `frame-ancestors`. Override via `DOC_ENGINE_SERVER_PUBLIC_SIGNING_FRAME_ANCESTORS`. |
