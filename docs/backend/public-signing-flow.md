# Public Signing Flow

This document describes the complete flow for public document signing, from document creation through email verification to digital signature completion.

## Table of Contents

- [Overview](#overview)
- [Architecture](#architecture)
- [Flow 1: Document Creation](#flow-1-document-creation)
- [Flow 2: Email Verification Gate](#flow-2-email-verification-gate)
- [Flow 2B: Authenticated Signing Session (Iframe)](#flow-2b-authenticated-signing-session-iframe)
- [Flow 3: Token-Based Signing](#flow-3-token-based-signing)
- [Flow 4: Admin Token Invalidation](#flow-4-admin-token-invalidation)
- [Endpoints Reference](#endpoints-reference)
- [Security](#security)
- [Configuration](#configuration)

---

## Overview

Documents use a **single shared public URL** per document instead of per-recipient signing URLs. Recipients visit the public URL, verify their email, receive a token-based signing link, and proceed through the signing flow.

There are now two strictly separated entry flows:

- Public flow (`/public/doc/{documentId}`): email verification gate, anonymous-safe anti-enumeration semantics.
- Authenticated iframe flow (`POST /api/v1/signing-sessions/{documentId}`): bearer-authenticated session bootstrap for CRM embeds, no email sending.

Optionally, a custom public-access middleware can validate an already-authenticated user (domain/JWT/etc.) and bypass the email gate by issuing a standard `/public/sign/{token}` redirect.

```
Create Document
  -> Notify recipients (email with /public/doc/{id})
    -> Recipient visits public URL
      -> Enters email -> Receives token link via email
        -> Accesses /public/sign/{token}
          -> Path A: PDF preview -> Proceed -> processing -> Sign
          -> Path B: Fill form -> PDF preview -> Proceed -> processing -> Sign
```

There are two signing paths based on whether the document template has interactive fields:

| Path | Token Type | Interactive Fields | Flow |
|------|-----------|-------------------|------|
| **A** | `SIGNING` | No | Preview PDF -> create/reuse active attempt -> River prepares provider signing |
| **B** | `PRE_SIGNING` | Yes | Fill form -> Preview PDF -> create/reuse active attempt -> River prepares provider signing |

---

## Architecture

### High-Level Flow

```mermaid
flowchart TB
    subgraph Admin["Admin (Authenticated)"]
        A1[Create Document] --> A2[NotifyDocumentCreated]
        A3[Invalidate Tokens]
    end

    subgraph Public["Public (No Auth)"]
        B1[GET /public/doc/:id] --> B1a{Custom middleware grants access?}
        B1a -->|Yes| B1b[Redirect to /public/sign/:token]
        B1a -->|No| B2[Show email form]
        B2 --> B3[POST /public/doc/:id/request-access]
        B3 --> B4{Email matches recipient?}
        B4 -->|Yes| B5[Generate token + send email]
        B4 -->|No| B6[Return 200 anyway]
    end

    subgraph Signing["Token-Based Signing"]
        C1[GET /public/sign/:token] --> C2{Token type?}
        C2 -->|SIGNING| C3[PDF Preview]
        C2 -->|PRE_SIGNING| C4{Form submitted?}
        C4 -->|No| C5[Show form]
        C4 -->|Yes| C3
        C5 --> C6[Submit form]
        C6 --> C3
        C3 --> C7[Proceed creates/reuses attempt]
        C7 --> C8[processing: River render/submit]
        C8 --> C9[Embedded signing iframe]
        C9 --> C10[Complete]
    end

    A2 -->|Email with /public/doc/:id| B1
    B1b --> C1
    B5 -->|Email with /public/sign/:token| C1
    A3 -.->|Invalidates tokens| Signing
```

### Component Map

```
Controller Layer                    Service Layer                    Repository Layer
---------------------              -------------------              ------------------
PublicDocAccessCtrl  ------------> DocumentAccessService ----------> DocumentRepository
  GET  /public/doc/:id                GetPublicDocumentInfo          DocumentRecipientRepo
  POST /public/doc/:id/request-access RequestAccess                  DocumentAccessTokenRepo
                                        |                            TemplateVersionRepo
                                        v
                                   NotificationService
                                        SendAccessLink
                                        NotifyDocumentCreated

PublicSigningCtrl  ----------------> PreSigningService -------------> DocumentRepository
  GET  /public/sign/:token             GetPublicSigningPage           DocumentAccessTokenRepo
  POST /public/sign/:token             SubmitPreSigningForm           FieldResponseRepo
  POST /public/sign/:token/request-access DocumentAccessService       DocumentAccessTokenRepo
  POST /public/sign/:token/proceed     ProceedToSigning               SigningExecutionUnitOfWork
  GET  /public/sign/:token/pdf         RenderPreviewPDF               PDFRenderer
  GET  /public/sign/:token/download    DownloadCompletedPDF           SigningProvider/StorageAdapter
  POST /public/sign/:token/complete    CompleteEmbeddedSigning        River dispatch already handles document completion
  GET  /public/sign/:token/refresh     RefreshEmbeddedURL             SigningProvider

River attempt workers -------------> SigningAttemptExecutor --------> PDFRenderer
  render_attempt_pdf                   Render immutable attempt PDF    StorageAdapter
  submit_attempt_to_provider           Submit provider document        SigningProvider
  reconcile_provider_submission        Resolve ambiguous submission    SigningProvider
  cleanup_provider_attempt             Best-effort provider cleanup    SigningProvider
  dispatch_attempt_completion          SDK completion event            DocumentCompletedHandler

DocumentCtrl (authenticated) -----> PreSigningService
  POST /api/v1/documents/:id/         InvalidateTokens
       invalidate-tokens
```

---

## Flow 1: Document Creation

When an admin creates a document, the system notifies all recipients with the public document URL.

```mermaid
sequenceDiagram
    participant Admin
    participant API as DocumentController
    participant Svc as DocumentService
    participant Notif as NotificationService
    participant Email as Email Provider
    participant Recipient

    Admin->>API: POST /api/v1/documents
    API->>Svc: CreateAndSendDocument()
    Svc->>Svc: validateTemplateAndRoles()
    Svc->>Svc: createDocument() [status: DRAFT]
    Svc->>Svc: createRecipients()
    Svc->>Svc: transitionToAwaitingInput() [status: AWAITING_INPUT]
    Svc->>Notif: NotifyDocumentCreated(documentID)
    Notif->>Notif: buildSigningURL() per recipient
    Note over Notif: No tokens exist yet -> fallback to /public/doc/{id}
    Notif->>Email: Send "Document ready for signature" email
    Email->>Recipient: Email with link to /public/doc/{documentID}
    API-->>Admin: 200 OK (document created)
```

**Key files:**
- `core/internal/core/service/document/document_service.go` — `CreateAndSendDocument()`
- `core/internal/core/service/document/notification_service.go` — `NotifyDocumentCreated()`, `buildSigningURL()`
- `core/internal/core/service/document/templates/signing_request.html` — email template

---

## Flow 2: Email Verification Gate

The public document page acts as an email verification gate. Recipients enter their email, and if it matches a document recipient, a token is generated and sent via email.

When custom middleware is configured and grants access, the user skips this gate and is redirected directly to `/public/sign/{token}`.

```mermaid
sequenceDiagram
    participant Signer as Recipient
    participant Frontend as PublicDocumentAccessPage
    participant API as PublicDocAccessController
    participant Svc as DocumentAccessService
    participant Notif as NotificationService
    participant Email as Email Provider

    Signer->>Frontend: Visit /public/doc/{documentID}
    Frontend->>API: GET /public/doc/{documentID}
    API->>Svc: GetPublicDocumentInfo()
    Svc-->>Frontend: {documentTitle, status: "active"}

    Signer->>Frontend: Enter email + click "Send Link"
    Frontend->>API: POST /public/doc/{documentID}/request-access {email}
    API->>Svc: RequestAccess(documentID, email)

    Note over Svc: Always returns 200 (anti-enumeration)

    Svc->>Svc: validateAccessRequest()
    alt Document not found or terminal (except COMPLETED)
        Svc->>Svc: Log + return nil
    else Email doesn't match any recipient
        Svc->>Svc: Log + return nil
    else Rate limit exceeded
        Svc->>Svc: Log + return nil
    else Valid request
        Svc->>Svc: resolveTokenType()
        Note over Svc: Interactive fields? PRE_SIGNING : SIGNING
        Svc->>Svc: generateAccessToken() [128-char hex]
        Svc->>Svc: Create DocumentAccessToken in DB
        Svc->>Notif: SendAccessLink(recipient, doc, token)
        Notif->>Email: Send "Your signing link" email
        Email->>Signer: Email with /public/sign/{token}
    end

    API-->>Frontend: 200 {message: "If your email matches..."}
    Frontend->>Frontend: Show "Check your email" + cooldown timer
```

**Key files:**
- `core/internal/core/service/document/document_access_service.go` — `RequestAccess()`, `validateAccessRequest()`, `generateAndSendToken()`
- `core/internal/adapters/primary/http/controller/public_document_access_controller.go` — routes
- `core/internal/core/service/document/templates/access_link.html` — email template
- `app/src/features/public-signing/components/PublicDocumentAccessPage.tsx` — frontend

---

## Flow 2B: Authenticated Signing Session (Iframe)

This flow is used by authenticated CRM embeds and does not call `POST /public/doc/{id}/request-access`.

```mermaid
sequenceDiagram
    participant Front as applications-front
    participant API as SigningSessionController
    participant Auth as OIDC or Custom Authenticator
    participant Svc as SigningSessionService
    participant Public as PreSigningService

    Front->>API: POST /api/v1/signing-sessions/{documentId}?language=en + Bearer
    API->>Auth: validate JWT/custom claims
    Auth-->>API: {email, subject, provider}
    API->>Svc: CreateOrGetSession(documentId, principal)
    Svc->>Svc: Resolve recipient by email/subject
    Svc->>Svc: If PRE_SIGNING responses missing, return form/preview only
    Svc->>Svc: If signing can start, create/reuse active attempt through River UoW
    Svc->>Public: GetPublicSigningPage(token)
    Public-->>Svc: {step, canSign, canDownload, downloadUrl}
    Svc-->>API: {sessionUrl: /public/sign/{token}?language=en, step, ...}
    API-->>Front: 200 response
    Front->>Front: Load sessionUrl in iframe or show processing/current state
```

The returned `sessionUrl` always points to `/public/sign/{token}`. If the request includes `language=en|es`, the URL includes the same query parameter; unsupported explicit values fall back to `language=en`. That page remains the single UX for sign, view, waiting, and download states.

---

## Flow 3: Token-Based Signing

After receiving the token URL via email, the recipient accesses the signing page. The flow depends on the token type.

### Path A: Direct Signing (SIGNING token, no interactive fields)

```mermaid
sequenceDiagram
    participant Signer as Recipient
    participant Frontend as PublicSigningPage
    participant API as PublicSigningController
    participant Svc as PreSigningService
    participant Provider as Signing Provider

    Signer->>Frontend: Click /public/sign/{token} from email
    Frontend->>API: GET /public/sign/{token}
    API->>Svc: GetPublicSigningPage(token)
    Svc->>Svc: validateToken()
    Svc-->>Frontend: {step: "preview", pdfUrl: "/public/sign/{token}/pdf"}

    Frontend->>API: GET /public/sign/{token}/pdf
    API->>Svc: RenderPreviewPDF(token)
    Svc-->>Frontend: PDF bytes (rendered on-demand)

    Signer->>Frontend: Click "Proceed to Sign"
    Frontend->>API: POST /public/sign/{token}/proceed
    API->>Svc: ProceedToSigning(token)
    Svc->>Svc: Create/reuse active SigningAttempt
    Svc->>Svc: Enqueue render_attempt_pdf in same DB transaction
    Svc-->>Frontend: {step: "processing"}

    Note over Svc,Provider: River renders PDF, submits attempt to provider, persists signing refs
    Frontend->>API: Poll GET /public/sign/{token}
    Svc->>Provider: GetAttemptRecipientEmbeddedURL(attempt recipient)
    Svc-->>Frontend: {step: "signing", embeddedSigningUrl: "..."}

    Frontend->>Frontend: Load signing iframe
    Signer->>Provider: Sign document in iframe
    Provider->>Frontend: Redirect to /public/sign/{token}/signing-callback?status=signed
    Frontend->>Frontend: postMessage to parent
    Frontend->>API: POST /public/sign/{token}/complete
    API->>Svc: CompleteEmbeddedSigning(token)
    Svc->>Svc: Mark token as used
```

### Path B: Form + Signing (PRE_SIGNING token, interactive fields)

```mermaid
sequenceDiagram
    participant Signer as Recipient
    participant Frontend as PublicSigningPage
    participant API as PublicSigningController
    participant Svc as PreSigningService
    participant Provider as Signing Provider

    Signer->>Frontend: Click /public/sign/{token} from email
    Frontend->>API: GET /public/sign/{token}
    API->>Svc: GetPublicSigningPage(token)
    Svc-->>Frontend: {step: "preview", form: {fields: [...]}}

    Signer->>Frontend: Fill interactive fields
    Frontend->>API: POST /public/sign/{token} {responses: [...]}
    API->>Svc: SubmitPreSigningForm(token, responses)
    Svc->>Svc: Validate responses
    Svc->>Svc: Save field responses to DB
    Svc-->>Frontend: {step: "preview", pdfUrl: "/public/sign/{token}/pdf"}

    Frontend->>API: GET /public/sign/{token}/pdf
    Svc-->>Frontend: PDF bytes (with form values injected)

    Note over Signer,Provider: From here, same as Path A
    Signer->>Frontend: Click "Proceed to Sign"
    Frontend->>API: POST /public/sign/{token}/proceed
    Svc->>Svc: Create/reuse active SigningAttempt + enqueue River render
    Svc-->>Frontend: {step: "processing"}
    Note over Svc,Provider: River renders stored PDF snapshot and submits attempt to provider
    Frontend->>API: Poll GET /public/sign/{token}
    Svc-->>Frontend: {step: "signing", embeddedSigningUrl: "..."}
    Signer->>Provider: Sign in iframe
```

### Signing Page States

```mermaid
stateDiagram-v2
    [*] --> preview: Token valid
    preview --> form: PRE_SIGNING + no responses
    form --> preview: Submit form responses
    preview --> processing: Proceed creates/reuses active attempt
    processing --> signing: River reaches SIGNING_READY
    processing --> unavailable: River reaches FAILED_PERMANENT/REQUIRES_REVIEW
    signing --> completed: Provider completion webhook
    signing --> declined: Provider decline webhook

    [*] --> waiting: Earlier signers pending
    waiting --> signing: Previous signers done

    [*] --> completed: Token used + doc completed
    [*] --> declined: Token used + doc declined
    [*] --> document_updated: Token attempt was superseded/invalidated
    [*] --> unavailable: Active attempt cannot safely continue
    [*] --> error: Invalid/expired token

    error --> preview: POST /public/sign/:token/request-access (new link by email)
```


Public responses intentionally expose user-safe steps only: `processing` while River is preparing the active attempt, `document_updated` when a token is tied to a superseded/invalidated attempt, and `unavailable` when the active attempt failed permanently or requires manual review. Internal attempt statuses are not shown to signers.

**Key files:**
- `core/internal/core/service/document/pre_signing_service.go` — all signing methods
- `core/internal/adapters/primary/http/controller/public_signing_controller.go` — routes
- `app/src/features/public-signing/components/PublicSigningPage.tsx` — frontend

---

## Flow 4: Admin Token Invalidation

Administrators can invalidate all active tokens for a document. Recipients with invalidated tokens cannot access the signing page and must request a new token via the email verification gate.

```mermaid
sequenceDiagram
    participant Admin
    participant Frontend as SigningDetailPage
    participant API as DocumentController
    participant Svc as PreSigningService
    participant DB as Database

    Admin->>Frontend: Click "Invalidate Tokens"
    Frontend->>Frontend: Show confirmation dialog
    Admin->>Frontend: Confirm
    Frontend->>API: POST /api/v1/documents/{id}/invalidate-tokens
    API->>Svc: InvalidateTokens(documentID)
    Svc->>Svc: Validate document is AWAITING_INPUT
    Svc->>DB: SET used_at = NOW() WHERE document_id = ? AND used_at IS NULL
    Svc-->>API: OK
    API-->>Frontend: 200 {message: "Tokens invalidated"}
    Frontend->>Frontend: Toast "Tokens invalidated successfully"
```

After invalidation, the recipient can still access `/public/doc/{documentID}` and request a new token via email.

**Key files:**
- `core/internal/core/service/document/pre_signing_service.go` — `InvalidateTokens()`
- `core/internal/adapters/primary/http/controller/document_controller.go` — route registration
- `app/src/features/signing/components/SigningDetailPage.tsx` — admin UI

---

## Endpoints Reference

### Public Endpoints (No Authentication)

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/public/doc/{documentId}` | Get document info (title, status); optional `language=en|es` is preserved on custom-auth direct redirects |
| `POST` | `/public/doc/{documentId}/request-access` | Request access link (email verification); optional `language=en|es` is preserved on custom-auth direct responses |
| `GET` | `/public/sign/{token}` | Get signing page state |
| `POST` | `/public/sign/{token}` | Submit pre-signing form (Path B) |
| `POST` | `/public/sign/{token}/request-access` | Request a new link from expired token entrypoint |
| `POST` | `/public/sign/{token}/proceed` | Create/reuse active attempt and enqueue River render/submit work |
| `GET` | `/public/sign/{token}/pdf` | Render PDF preview (on-demand) |
| `GET` | `/public/sign/{token}/download` | Download completed/signed PDF (authorized recipient only) |
| `POST` | `/public/sign/{token}/complete` | Mark token as used after signing |
| `GET` | `/public/sign/{token}/refresh` | Refresh expired embedded URL |
| `GET` | `/public/sign/{token}/signing-callback` | Callback bridge (postMessage to parent) |

### Authenticated Endpoints

| Method | Path | Role | Purpose |
|--------|------|------|---------|
| `POST` | `/api/v1/signing-sessions/{documentId}` | Authenticated user | Create/reuse iframe signing session and return `/public/sign/{token}` URL; optional `language=en|es` is preserved |
| `POST` | `/api/v1/documents` | Operator | Create document + send notifications |
| `POST` | `/api/v1/documents/{id}/invalidate-tokens` | Operator | Invalidate all active tokens |

---

## Security

### Anti-Email Enumeration

`POST /public/doc/{id}/request-access` and `POST /public/sign/{token}/request-access` **always return HTTP 200** regardless of email/token validity. Attackers cannot determine valid email addresses or token ownership.

### Rate Limiting

Token generation is rate-limited per document + recipient pair. If the limit is exceeded, the request silently succeeds (HTTP 200) without generating a token.

| Config | Default | Description |
|--------|---------|-------------|
| `rate_limit_max` | 3 | Max tokens per window |
| `rate_limit_window_min` | 60 | Window duration (minutes) |

### Token Security

- **Random**: 64 cryptographic random bytes, hex-encoded (128 characters)
- **Single-use**: marked with `used_at` timestamp after signing completion
- **Expiring**: configurable TTL (default 48 hours)
- **Database-backed**: no client-side state or cookies

### Signing Order

Multi-signer documents enforce signing order. If earlier signers haven't signed, the current signer sees a "waiting" state with their position in the queue.

---

## Configuration

Signing access settings in `core/settings/app.yaml`:

```yaml
public_access:
  rate_limit_max: 3            # Max token requests per window
  rate_limit_window_min: 60    # Rate limit window (minutes)
  token_ttl_hours: 48          # Token expiration (hours)

signing_session_auth:
  mode: oidc                   # required: "oidc" or "custom"
  oidc:
    provider: panel            # "panel" or provider name (if configured)
    email_claim: email         # claim used to resolve recipient
```

Environment variable overrides:

| Variable | Description |
|----------|-------------|
| `DOC_ENGINE_PUBLIC_ACCESS_RATE_LIMIT_MAX` | Max token requests per window |
| `DOC_ENGINE_PUBLIC_ACCESS_RATE_LIMIT_WINDOW_MIN` | Rate limit window in minutes |
| `DOC_ENGINE_PUBLIC_ACCESS_TOKEN_TTL_HOURS` | Token TTL in hours |
| `DOC_ENGINE_SIGNING_SESSION_AUTH_MODE` | Auth mode for `/api/v1/signing-sessions/{id}` (`oidc` or `custom`) |
| `DOC_ENGINE_SIGNING_SESSION_AUTH_OIDC_PROVIDER` | OIDC provider name for signing session auth |
| `DOC_ENGINE_SIGNING_SESSION_AUTH_OIDC_EMAIL_CLAIM` | Email claim key for recipient resolution |
