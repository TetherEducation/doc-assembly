# Document Completion Events

When a document reaches `COMPLETED` status (all recipients signed), the engine fires your registered handler from inside the River `dispatch_attempt_completion` worker.

## Registering a handler

```go
import (
    "context"
    "log/slog"

    "github.com/TetherEducation/doc-assembly/core/sdk"
)

engine.OnDocumentCompleted(func(ctx context.Context, ev sdk.DocumentCompletedEvent) error {
    slog.InfoContext(ctx, "document completed",
        slog.String("document_id", ev.DocumentID),
        slog.String("tenant", ev.TenantCode),
        slog.Int("recipients", len(ev.Recipients)),
    )
    return notifyDownstream(ctx, ev) // return error to retry
})
```

You can register only **one** handler per engine. Calling `OnDocumentCompleted` twice replaces the previous one.

## Event payload

```go
type DocumentCompletedEvent struct {
    DocumentID       string
    ExternalID       *string
    Title            *string
    Status           sdk.DocumentStatus
    WorkspaceCode    string
    DocumentTypeCode *string
    TenantCode       string
    Environment      sdk.Environment
    CreatedAt        time.Time
    UpdatedAt        *time.Time
    ExpiresAt        *time.Time
    Metadata         map[string]string
    Recipients       []sdk.CompletedRecipient
}

type CompletedRecipient struct {
    RoleName    string
    SignerOrder int
    Name        string
    Email       string
    Status      sdk.RecipientStatus
    SignedAt    *time.Time
}
```

`ExternalID` is whatever your wrapper passed during creation (the field you use to correlate with your own domain). `Metadata` is the bag persisted on the document — it survives the entire lifecycle. Use either of those to find the matching record in your own system.

## Retry semantics

The handler runs inside a River worker:

| Return | Engine reaction |
|---|---|
| `nil` | Job ack'd. Done. |
| `error` | River retries with exponential backoff according to its default policy. Same `(DocumentID, attempt_id)` will be re-delivered. |

This means **your handler MUST be idempotent**. The same event can be delivered twice (after a transient failure on first attempt). De-dupe by `(DocumentID, AttemptID-from-event-context)` or by a unique key in your downstream system. A naïve "always insert a new audit row" implementation will produce duplicates after the first retry.

Long-running work: keep the handler short. If you need to do heavy work, enqueue your own job onto your own queue from inside the handler and return `nil` once the enqueue commits.

## Order vs other side effects

By the time your handler fires:

- The document projection is `COMPLETED`.
- The signed PDF is available via the engine's storage adapter (or via the provider's `DownloadCompletedPDF` if your provider returned it lazily).
- Recipient statuses are persisted.
- The webhook from the signing provider has been processed and acknowledged.

What is **not** guaranteed:

- That all email notifications already went out (notifications are best-effort).
- The order of completion handler vs your own scheduler-driven cleanup jobs — they run in their own River workers.

If your handler depends on the signed PDF bytes, fetch them from the storage adapter using the `DocumentID`; do not assume any specific download URL is in `Metadata`.

## What if you do not register a handler?

Nothing breaks — the lib still updates the document projection, persists everything, and fires `dispatch_attempt_completion` to a no-op. The handler is optional.

## Testing

In integration tests for your wrapper:

1. Stand up the engine with a real `OnDocumentCompleted` (use a channel-backed handler).
2. Drive a document through completion (mock signing provider returns success, post a webhook).
3. Assert the channel received the expected event.

Avoid asserting on River internals (job IDs, timestamps) — those are private.

## See also

- [signing.md](signing.md) — what happens before completion (attempts, webhook, dispatch).
- [pitfalls.md](pitfalls.md) — common completion-handler mistakes (non-idempotent, blocking calls, swallowing errors).
