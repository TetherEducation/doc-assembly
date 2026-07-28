# Logging Guide

This document describes the logging practices and patterns used in the Doc Engine project.

## Overview

The project uses Go's standard `log/slog` package with a **ContextHandler** that automatically extracts and includes contextual attributes in all log entries.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                        main.go                               │
│  slog.SetDefault(slog.New(logging.NewContextHandler(...)))  │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   ContextHandler                             │
│  Extracts attributes from context.Context automatically      │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│               Operation Middleware                           │
│  Adds: operation_id, method, path, client_ip to context     │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│            Controllers / Services                            │
│  Use slog.InfoContext(ctx, ...) - attrs included auto       │
└─────────────────────────────────────────────────────────────┘
```

## Usage

### Basic Logging

Always use context-aware logging functions:

```go
// Info level - normal business events
slog.InfoContext(ctx, "user created", "user_id", user.ID)

// Error level - errors requiring attention
slog.ErrorContext(ctx, "operation failed", "error", err)

// Warn level - unexpected but handled situations
slog.WarnContext(ctx, "deprecated feature used", "feature", featureName)

// Debug level - detailed debugging info (disabled in production)
slog.DebugContext(ctx, "processing item", "item_id", itemID)
```

### Adding Contextual Attributes

Use `logging.WithAttrs()` to add attributes that will be included in all subsequent logs within that context:

```go
import "github.com/TetherEducation/doc-assembly/core/internal/infra/logging"

// Add attributes to context
ctx = logging.WithAttrs(ctx,
    slog.String("tenant_id", tenantID),
    slog.String("workspace_id", workspaceID),
)

// All logs using this context will include tenant_id and workspace_id
slog.InfoContext(ctx, "template created", "template_id", templateID)
// Output: {"level":"INFO","msg":"template created","tenant_id":"...","workspace_id":"...","template_id":"..."}
```

### Automatic Attributes

The Operation middleware (`internal/adapters/primary/http/middleware/operation.go`) automatically adds these attributes to every request context:

| Attribute | Description |
|-----------|-------------|
| `operation_id` | Unique UUID for the request (also sent in `X-Operation-ID` header) |
| `method` | HTTP method (GET, POST, etc.) |
| `path` | Request path |
| `client_ip` | Client IP address |

## Log Levels

| Level | When to Use | Production |
|-------|-------------|------------|
| `Debug` | Detailed information for troubleshooting specific issues | Disabled |
| `Info` | Normal business events (user created, document generated, etc.) | Enabled |
| `Warn` | Unexpected situations that were handled gracefully | Enabled |
| `Error` | Errors that need attention or investigation | Enabled |

### Examples by Level

```go
// DEBUG - Internal processing details
slog.DebugContext(ctx, "starting document generation",
    "templateID", templateID,
    "operation", operation,
)

// INFO - Business events
slog.InfoContext(ctx, "document created successfully",
    "documentID", doc.ID,
    "recipientCount", len(recipients),
)

// WARN - Handled issues
slog.WarnContext(ctx, "deprecated API version used",
    "version", "v1",
    "recommended", "v2",
)

// ERROR - Failures requiring attention
slog.ErrorContext(ctx, "failed to send document for signing",
    "error", err,
    "documentID", doc.ID,
)
```

## Best Practices

### Do

- **Always use context-aware functions**: `slog.InfoContext(ctx, ...)` instead of `slog.Info(...)`
- **Include relevant identifiers**: user_id, document_id, template_id, etc.
- **Use structured attributes**: Pass key-value pairs, not formatted strings
- **Keep messages concise**: The message should describe what happened
- **Add context early**: Use `logging.WithAttrs()` at the start of request processing

### Don't

- **Don't inject `*slog.Logger` as a dependency** - Use `slog.InfoContext(ctx, ...)` directly
- **Don't call `slog.Default()` in services/controllers** - It defeats the purpose of context-based logging
- **Don't use `slog.Info()` without context** - Always use the `*Context` variants
- **Don't log sensitive data**: passwords, tokens, API keys, PII
- **Don't log entire request/response bodies** in production
- **Don't use string formatting in messages**: Use structured attributes instead

```go
// BAD - string formatting
slog.InfoContext(ctx, fmt.Sprintf("user %s created document %s", userID, docID))

// GOOD - structured attributes
slog.InfoContext(ctx, "document created", "user_id", userID, "document_id", docID)
```

## Key Files

| File | Purpose |
|------|---------|
| `internal/infra/logging/handler.go` | ContextHandler implementation |
| `cmd/api/main.go` | Handler initialization with `slog.SetDefault()` |
| `internal/adapters/primary/http/middleware/operation.go` | Adds request attributes to context |

## Output Format

In production, logs are output as JSON for easy parsing:

```json
{
  "time": "2024-01-15T10:30:00.000Z",
  "level": "INFO",
  "msg": "document created successfully",
  "operation_id": "abc123-...",
  "method": "POST",
  "path": "/api/v1/documents",
  "client_ip": "192.168.1.1",
  "document_id": "doc-456",
  "recipient_count": 2
}
```

## Configuration

The log level is configured in `cmd/api/main.go`:

```go
handler := logging.NewContextHandler(
    slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: slog.LevelInfo, // Change to slog.LevelDebug for debugging
    }),
)
```

To enable debug logging, change `slog.LevelInfo` to `slog.LevelDebug`.
