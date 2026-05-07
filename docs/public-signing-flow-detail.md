# Public Signing Flow

## Objective

Document the public document access and signing flow, including:

- public document URL access (`/public/doc/{documentId}`)
- tokenized link access (`/public/sign/{token}`)
- optional custom domain-auth bypass
- expired magic link recovery
- explicit public UI language selection via `language=en|es`

## Involved Endpoints

- `GET /public/doc/{documentId}`
- `POST /public/doc/{documentId}/request-access`
- `GET /public/sign/{token}`
- `POST /public/sign/{token}`
- `POST /public/sign/{token}/proceed`
- `GET /public/sign/{token}/pdf`
- `GET /public/sign/{token}/download`
- `POST /public/sign/{token}/request-access`
- `POST /public/sign/{token}/complete`
- `GET /public/sign/{token}/refresh`

All public access/signing endpoints accept an optional `language` query parameter for the public UI. Supported values are `en` and `es`; any other explicit value falls back to `en`. When present on `/public/doc/{documentId}` or `POST /api/v1/signing-sessions/{documentId}`, generated direct/session URLs preserve it as `/public/sign/{token}?language=...`.

## Endpoints by Flow Domain

### Public

| Endpoint | Purpose | When it is used |
|---|---|---|
| `GET /public/doc/{documentId}` | Public entry point to the document (email gate) or custom-auth bypass. | First access from email/public channel using `documentId`. |
| `POST /public/doc/{documentId}/request-access` | Request magic link by email from the email gate. | User submitted email in access screen. |

### Tokenized Access

| Endpoint | Purpose | When it is used |
|---|---|---|
| `GET /public/sign/{token}` | Get signing state for a tokenized session. | Initial load/refresh of signing page. |
| `POST /public/sign/{token}` | Submit interactive pre-sign form (if present). | Path B (`PRE_SIGNING` token) with interactive fields. |
| `POST /public/sign/{token}/proceed` | Create/reuse the active signing attempt and enqueue River render/submit work. | After PDF preview in Path A or Path B. Returns `processing` until River prepares provider signing. |
| `GET /public/sign/{token}/pdf` | Render/get preview PDF on demand. | Preview before proceeding to sign. |
| `GET /public/sign/{token}/download` | Download final signed PDF. | Document completed and authorized recipient. |
| `POST /public/sign/{token}/request-access` | Access recovery for expired token/tokenized entry. | User with expired link requests a new email link. |
| `POST /public/sign/{token}/complete` | Mark token as used after embedded signing completion. | Frontend callback after iframe completion. |
| `GET /public/sign/{token}/refresh` | Get new embedded URL when current one expires. | Embedded iframe expires during signing. |
| `GET /public/sign/{token}/signing-callback` | HTML `postMessage` bridge between provider and parent UI. | Signature provider callback/redirection inside iframe. |

### Admin

| Endpoint | Purpose |
|---|---|
| `POST /api/v1/documents/{id}/invalidate-tokens` | Invalidate all active tokens for a document (force link reissue). |

## Flow 1: User Without Domain Authentication

1. User opens public URL: `/public/doc/{documentId}`.
2. Backend returns minimal public document info (title + public status).
3. Frontend asks user email.
4. User submits email to `/public/doc/{documentId}/request-access`.
5. If email/document/status are valid, backend generates token and sends `/public/sign/{token}` by email.
6. User opens `/public/sign/{token}`.
7. Depending on token type, active attempt, and document status:
   - `preview` (PDF or form),
   - `processing` (River is preparing/retrying/reconciling the active attempt),
   - `signing` (iframe),
   - `waiting`,
   - `completed`,
   - `declined`,
   - `document_updated` (old/superseded attempt token),
   - `unavailable` (failed/requires review).
8. In `completed` state, frontend enables download only if `canDownload=true`.

## Flow 2: User With Custom Domain Authentication

1. User opens `/public/doc/{documentId}`.
2. Custom middleware (`PublicDocumentAccessAuthenticator`) evaluates access in `GET /public/doc/:documentId`.
3. If custom auth returns valid claims (with email), backend generates direct token (no email sent) and returns `303` redirect to `/public/sign/{token}`. If the entry URL included `language=en|es`, the redirect preserves it.
4. User goes directly into tokenized flow (`preview/processing/signing/waiting/completed/declined/document_updated/unavailable`).
5. If custom auth does not apply, fails, or provides no useful claims, flow falls back to standard email gate (Flow 1).

## Flow 3: User With Expired Magic Link

1. User opens expired `/public/sign/{token}`.
2. Backend returns token expiration error.
3. Frontend shows email recovery screen.
4. User submits email to `POST /public/sign/{token}/request-access`.
5. Backend tries to resolve `documentId` from that token and, if valid, issues a new token by email.
6. User opens new `/public/sign/{newToken}` and continues normal flow.

## Signing Attempt Execution

After `proceed`, the request path returns quickly. It does not upload to the signing provider inline. Instead, the service creates or observes the document active `SigningAttempt` and River runs the durable phases:

1. `render_attempt_pdf` renders an immutable PDF under `documents/{workspaceID}/{documentID}/attempts/{attemptID}/pre-signed.pdf`.
2. `submit_attempt_to_provider` submits that exact PDF snapshot using correlation key `{document_id}:{attempt_id}`.
3. Provider webhooks update the active attempt and recompute the document projection.
4. Completion enqueues `dispatch_attempt_completion` through River.

Old attempt tokens never receive stale signing URLs; they return `document_updated`. Failed/review attempts return `unavailable`.

## UI States in `/public/sign/{token}`

- `preview`:
  - Path A (`SIGNING` token): PDF preview and "proceed" action.
  - Path B (`PRE_SIGNING` token): interactive form and then PDF preview.
- `processing`: active `SigningAttempt` is rendering, submitting, retrying, or reconciling through River.
- `signing`: embedded iframe (or provider signing URL when embedding is unavailable).
- `waiting`: previous signers are still pending.
- `completed`: signing completed for document.
- `declined`: document was declined.
- `document_updated`: the token is tied to an attempt that was superseded, invalidated, or cancelled by a newer document generation.
- `unavailable`: the active attempt failed permanently or requires manual review.

## Access and Download Rules

- A valid token allows navigating the signing flow.
- Expired token returns expiration error and enables email recovery.
- Used token:
  - can show terminal states (`completed`/`declined`);
  - is rejected for non-terminal states.
- Signed PDF download (`/download`) requires:
  - document in `completed` state,
  - token recipient has signed,
  - and the completed PDF/source can be resolved from the active completed attempt.

## Flow Security Controls

- Anti-enumeration:
  - `POST /public/doc/{id}/request-access` and
  - `POST /public/sign/{token}/request-access`
  always return `200`, even if no link is generated.
- Access-link issuance rate limit by `document + recipient`.
- Custom middleware never blocks on its own: if it does not validate, flow falls back to email gate.

## Sequence Diagrams

### Public Flow (Email Gate)

```mermaid
sequenceDiagram
    participant U as User
    participant FE as Public Frontend
    participant API as Public Controllers
    participant ACC as DocumentAccessService
    participant MAIL as Notification
    participant SIGN as PreSigningService

    U->>FE: GET /public/doc/{documentId}
    FE->>API: GET /public/doc/{documentId}
    API->>ACC: GetPublicDocumentInfo(documentId)
    ACC-->>API: title + status
    API-->>FE: Show email gate

    U->>FE: Enter email
    FE->>API: POST /public/doc/{documentId}/request-access
    API->>ACC: RequestAccess(documentId, email)
    ACC->>MAIL: SendAccessLink(/public/sign/{token})
    API-->>FE: 200 (generic message)

    U->>FE: Open /public/sign/{token}
    FE->>API: GET /public/sign/{token}
    API->>SIGN: GetPublicSigningPage(token)
    SIGN-->>API: step=preview/processing/signing/waiting/completed/declined/document_updated/unavailable
    API-->>FE: Current flow state
```

### Custom Auth Flow (Bypass)

```mermaid
sequenceDiagram
    participant U as User
    participant FE as Public Frontend
    participant API as Public Controllers
    participant AUTH as PublicDocumentAccessAuthenticator
    participant ACC as DocumentAccessService
    participant SIGN as PreSigningService

    U->>FE: GET /public/doc/{documentId}
    FE->>API: GET /public/doc/{documentId}
    API->>AUTH: Authenticate(documentId)

    alt Custom auth OK (claims with email)
        API->>ACC: RequestDirectAccess(documentId, email)
        ACC-->>API: /public/sign/{token}
        API-->>FE: 303 redirect /public/sign/{token}
        FE->>API: GET /public/sign/{token}
        API->>SIGN: GetPublicSigningPage(token)
        SIGN-->>API: step=preview/processing/signing/waiting/completed/declined/document_updated/unavailable
        API-->>FE: Current flow state
    else Fallback
        API-->>FE: Continue with email gate
    end
```

### Recovery Flow (Expired Token)

```mermaid
sequenceDiagram
    participant U as User
    participant FE as Public Frontend
    participant API as Public Controllers
    participant SIGN as PreSigningService
    participant ACC as DocumentAccessService
    participant MAIL as Notification

    U->>FE: GET /public/sign/{expired-token}
    FE->>API: GET /public/sign/{expired-token}
    API->>SIGN: GetPublicSigningPage(token)
    SIGN-->>API: ErrTokenExpired
    API-->>FE: 401 token expired
    FE-->>U: Show email recovery

    U->>FE: Enter email
    FE->>API: POST /public/sign/{expired-token}/request-access
    API->>ACC: RequestAccessByToken(token, email)
    ACC->>MAIL: SendAccessLink(/public/sign/{newToken})
    API-->>FE: 200 (generic message)

    U->>FE: Open /public/sign/{newToken}
    FE->>API: GET /public/sign/{newToken}
    API->>SIGN: GetPublicSigningPage(newToken)
    SIGN-->>API: step=preview/processing/signing/waiting/completed/declined/document_updated/unavailable
    API-->>FE: Current flow state
```
