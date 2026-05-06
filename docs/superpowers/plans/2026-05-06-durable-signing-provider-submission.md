# Durable Signing Provider Submission Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make provider submission in doc-assembly durable, resumable, and visible in UX so a partial Documenso envelope can never leave a signer stuck forever on “Preparing document”.

**Architecture:** Replace the current monolithic `SubmitAttemptDocument` provider call with an attempt-scoped durable submission workflow. Each provider step is persisted, idempotent, reconciliable by correlation key, and can continue after timeout, crash, or provider partial success. Public UX remains simple: `processing` for active healthy work, `signing` when ready, and `unavailable/requires_review` instead of infinite loading when recovery is impossible.

**Tech Stack:** Go 1.25, Gin, River, PostgreSQL 16, pgx, React 19, Vite, Zustand, Testcontainers, Docker, Documenso local, Browser Use in-app browser.

---

## Non-negotiable Requirements

1. Reproduce the failure locally before changing production code behavior.
2. The final implementation must not rely on “just increase timeout”.
3. Provider submission must be resumable from partial Documenso state:
   - envelope exists without recipients
   - envelope exists with recipients but no fields
   - envelope exists with fields but not distributed
   - envelope is distributed but signing references were not persisted
4. No public signing page may poll forever in `processing` for an unrecoverable attempt.
5. All changes must have automated backend tests plus full visual local UI validation in an iframe.
6. The local UI validation must repeat the lifecycle used earlier in the session:
   - generate contract
   - preview
   - request/proceed to signing
   - open signing document in iframe
   - sign
   - complete
   - verify final UI/download

---

## File Structure

### Backend domain and ports

- Modify: `core/internal/core/entity/signing_attempt.go`
  - Add explicit durable provider submit phases.
  - Keep `SigningAttemptStatus` compact and compatible with current public status projection.

- Modify: `core/internal/core/port/signing_provider.go`
  - Add step-level provider methods for ensure/reconcile flow.
  - Add typed `ProviderSubmissionSnapshot` and step-level ensure request/result structs.

### River execution

- Modify: `core/internal/infra/riverqueue/args.go`
  - Add job kind `advance_provider_submission`.
  - Use one job kind with phase stored in DB to avoid job explosion.

- Modify: `core/internal/infra/riverqueue/attempt_workers.go`
  - Add worker timeout and handler for the durable submit advancement.

- Modify: `core/internal/infra/riverqueue/executor.go`
  - Replace monolithic submit logic with state machine advancement.
  - Add stale-state guards and no-infinite-processing behavior.

- Modify: `core/internal/infra/riverqueue/uow.go`
  - Add transition helper that persists next provider phase and enqueues `advance_provider_submission` transactionally.

- Modify: `core/internal/infra/riverqueue/failpoints.go`
  - Add failpoints for each provider boundary so tests can simulate crash/timeout after provider-side success.

### Provider adapters

- Modify: `core/internal/adapters/secondary/signing/documenso/adapter.go`
  - Split current submit flow into idempotent methods:
    - `EnsureProviderDocument`
    - `EnsureProviderRecipients`
    - `EnsureProviderFields`
    - `EnsureProviderDistributed`
    - `FetchProviderSigningReferences`
    - `InspectProviderSubmission`
  - Continue using `externalId` as the canonical correlation key.

- Modify: `core/internal/adapters/secondary/signing/documenso/adapter_test.go`
  - Add unit tests for partial envelopes.

- Modify: `core/internal/adapters/secondary/signing/mock/adapter.go`
  - Add deterministic partial-state support for integration tests.

### Database

- Create: `core/internal/migrations/sql/000020_durable_provider_submission.up.sql`
  - Add nullable JSONB/state columns needed to persist provider step references.

- Create: `core/internal/migrations/sql/000020_durable_provider_submission.down.sql`
  - Reversible migration.

### Public signing UX

- Modify: `core/internal/core/service/document/pre_signing_service.go`
  - Bound `processing` by state freshness/recoverability.
  - Return `unavailable`/recovering copy when attempt is unrecoverable.

- Modify: `app/src/features/public-signing/components/ProcessingState.tsx` or actual equivalent discovered route/component file
  - Show richer copy for long-running recovery.

- Modify: `app/src/features/public-signing/types.ts` or actual equivalent
  - Add optional `processingReason`/`retryAfterSeconds`/`supportCode` only if backend needs it.

### Validation artifacts

- Create: `docs/superpowers/evidence/YYYY-MM-DD-durable-provider-submission/README.md`
  - Record commands, screenshots, DB states, and UI evidence from local validation.

---

## Task 1: Baseline Failure Reproduction Test for Partial Documenso Envelope

**Files:**
- Modify: `core/internal/adapters/secondary/signing/documenso/adapter_test.go`
- Modify: `core/internal/adapters/secondary/signing/documenso/adapter.go`

- [ ] **Step 1: Add a failing unit test that represents the observed production failure**

Append this test to `core/internal/adapters/secondary/signing/documenso/adapter_test.go`:

```go
func TestFindProviderDocumentByCorrelationKeyClassifiesDraftWithoutRecipientsAsPartial(t *testing.T) {
	const correlationKey = "doc-id:attempt-id"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/envelope":
			writeJSON(t, w, map[string]any{
				"data": []map[string]any{{
					"id":         "env_partial",
					"externalId": correlationKey,
					"status":     "DRAFT",
				}},
				"pagination": map[string]any{"page": 1, "perPage": 100, "totalPages": 1, "totalItems": 1},
			})
		case "/api/v2/envelope/env_partial":
			writeJSON(t, w, map[string]any{
				"id":         "env_partial",
				"externalId": correlationKey,
				"status":     "DRAFT",
				"recipients": []map[string]any{},
				"fields":     []map[string]any{},
			})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	adapter, err := New(&Config{APIKey: "api_test", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := adapter.FindProviderDocumentByCorrelationKey(context.Background(), &port.FindProviderDocumentRequest{
		ProviderName:   providerName,
		CorrelationKey: correlationKey,
		Environment:    entity.EnvironmentProd,
	})
	if err != nil {
		t.Fatalf("FindProviderDocumentByCorrelationKey() error = %v", err)
	}

	if !got.Found {
		t.Fatal("Found = false, want true")
	}
	if got.Usable {
		t.Fatal("Usable = true, want false for partial draft")
	}
	if got.ProviderDocumentID != "env_partial" {
		t.Fatalf("ProviderDocumentID = %q, want env_partial", got.ProviderDocumentID)
	}
	if got.Reason != "documenso envelope has no usable recipients" {
		t.Fatalf("Reason = %q", got.Reason)
	}
}
```

- [ ] **Step 2: Run the failing/baseline test**

Run:

```bash
go test -C core ./internal/adapters/secondary/signing/documenso -run TestFindProviderDocumentByCorrelationKeyClassifiesDraftWithoutRecipientsAsPartial -v
```

Expected before durable implementation: the test may already pass on the current coarse unusable classification. If it passes, keep it as a baseline and continue; it proves the adapter can detect the partial envelope but the executor cannot recover from it.

- [ ] **Step 3: Commit only if this test is newly added and passing as baseline**

```bash
git add core/internal/adapters/secondary/signing/documenso/adapter_test.go
git commit -m "test: capture partial documenso envelope classification"
```

---

## Task 2: Add Durable Provider Submission Types

**Files:**
- Modify: `core/internal/core/entity/signing_attempt.go`
- Modify: `core/internal/core/port/signing_provider.go`

- [ ] **Step 1: Add provider submission phases to `signing_attempt.go`**

Add these constants next to existing `ProviderSubmitPhase` constants:

```go
const (
	ProviderSubmitPhaseBeforeRequest          ProviderSubmitPhase = "BEFORE_REQUEST"
	ProviderSubmitPhaseCreateProviderDocument ProviderSubmitPhase = "CREATE_PROVIDER_DOCUMENT"
	ProviderSubmitPhaseAddRecipients          ProviderSubmitPhase = "ADD_RECIPIENTS"
	ProviderSubmitPhaseCreateFields           ProviderSubmitPhase = "CREATE_FIELDS"
	ProviderSubmitPhaseDistributeDocument     ProviderSubmitPhase = "DISTRIBUTE_DOCUMENT"
	ProviderSubmitPhaseFetchSigningReferences ProviderSubmitPhase = "FETCH_SIGNING_REFERENCES"
)
```

If these already exist, do not duplicate them. Instead, add helper functions below the constants:

```go
func (p ProviderSubmitPhase) IsProviderMutationPhase() bool {
	switch p {
	case ProviderSubmitPhaseCreateProviderDocument,
		ProviderSubmitPhaseAddRecipients,
		ProviderSubmitPhaseCreateFields,
		ProviderSubmitPhaseDistributeDocument,
		ProviderSubmitPhaseFetchSigningReferences:
		return true
	default:
		return false
	}
}

func NextProviderSubmitPhase(p ProviderSubmitPhase) ProviderSubmitPhase {
	switch p {
	case ProviderSubmitPhaseCreateProviderDocument:
		return ProviderSubmitPhaseAddRecipients
	case ProviderSubmitPhaseAddRecipients:
		return ProviderSubmitPhaseCreateFields
	case ProviderSubmitPhaseCreateFields:
		return ProviderSubmitPhaseDistributeDocument
	case ProviderSubmitPhaseDistributeDocument:
		return ProviderSubmitPhaseFetchSigningReferences
	default:
		return ""
	}
}
```

- [ ] **Step 2: Add provider step request/result structs to `signing_provider.go`**

Add below `SubmitAttemptDocumentResult`:

```go
type ProviderSubmissionSnapshot struct {
	ProviderDocumentID string
	ProviderName       string
	CorrelationKey     string
	RawStatus          string
	HasDocument        bool
	HasRecipients      bool
	HasFields          bool
	IsDistributed      bool
	Recipients         []RecipientResult
	Reason             string
}

type EnsureProviderDocumentRequest struct {
	AttemptID      string
	DocumentID     string
	CorrelationKey string
	PDF            []byte
	PDFChecksum    string
	Title          string
	Environment    entity.Environment
}

type EnsureProviderDocumentResult struct {
	ProviderDocumentID string
	ProviderName       string
	CorrelationKey     string
	RawStatus          string
}

type EnsureProviderRecipientsRequest struct {
	ProviderDocumentID string
	CorrelationKey     string
	Recipients         []SigningRecipient
	Environment        entity.Environment
}

type EnsureProviderRecipientsResult struct {
	Recipients []RecipientResult
}

type EnsureProviderFieldsRequest struct {
	ProviderDocumentID string
	CorrelationKey     string
	Recipients         []RecipientResult
	SignatureFields    []SignatureFieldPosition
	Environment        entity.Environment
}

type EnsureProviderFieldsResult struct {
	FieldCount int
}

type EnsureProviderDistributedRequest struct {
	ProviderDocumentID string
	CorrelationKey     string
	Environment        entity.Environment
}

type EnsureProviderDistributedResult struct {
	RawStatus string
}

type FetchProviderSigningReferencesRequest struct {
	ProviderDocumentID string
	CorrelationKey     string
	Environment        entity.Environment
}

type FetchProviderSigningReferencesResult struct {
	Recipients []RecipientResult
	Status     entity.SigningAttemptStatus
	RawStatus  string
}
```

- [ ] **Step 3: Extend `SigningProvider` interface**

Replace the interface block with:

```go
type SigningProvider interface {
	SubmitAttemptDocument(ctx context.Context, req *SubmitAttemptDocumentRequest) (*SubmitAttemptDocumentResult, error)
	FindProviderDocumentByCorrelationKey(ctx context.Context, req *FindProviderDocumentRequest) (*ProviderDocumentResult, error)
	InspectProviderSubmission(ctx context.Context, req *FindProviderDocumentRequest) (*ProviderSubmissionSnapshot, error)
	EnsureProviderDocument(ctx context.Context, req *EnsureProviderDocumentRequest) (*EnsureProviderDocumentResult, error)
	EnsureProviderRecipients(ctx context.Context, req *EnsureProviderRecipientsRequest) (*EnsureProviderRecipientsResult, error)
	EnsureProviderFields(ctx context.Context, req *EnsureProviderFieldsRequest) (*EnsureProviderFieldsResult, error)
	EnsureProviderDistributed(ctx context.Context, req *EnsureProviderDistributedRequest) (*EnsureProviderDistributedResult, error)
	FetchProviderSigningReferences(ctx context.Context, req *FetchProviderSigningReferencesRequest) (*FetchProviderSigningReferencesResult, error)
	GetProviderDocumentStatus(ctx context.Context, req *GetProviderDocumentStatusRequest) (*ProviderDocumentStatusResult, error)
	GetAttemptRecipientEmbeddedURL(ctx context.Context, req *GetAttemptRecipientEmbeddedURLRequest) (*GetAttemptRecipientEmbeddedURLResult, error)
	DownloadCompletedPDF(ctx context.Context, req *DownloadCompletedPDFRequest) (*DownloadCompletedPDFResult, error)
	CleanupProviderDocument(ctx context.Context, req *CleanupProviderDocumentRequest) (*CleanupProviderDocumentResult, error)
	ProviderCapabilities() ProviderCapabilities
	ProviderName() string
}
```

- [ ] **Step 4: Run compile to expose required adapter updates**

Run:

```bash
go test -C core ./internal/core/port ./internal/core/entity
```

Expected: PASS for these packages, but broader compile will fail until adapters implement new methods.

- [ ] **Step 5: Commit**

```bash
git add core/internal/core/entity/signing_attempt.go core/internal/core/port/signing_provider.go
git commit -m "feat: define durable provider submission contract"
```

---

## Task 3: Implement Durable Methods in Mock Provider for Tests

**Files:**
- Modify: `core/internal/adapters/secondary/signing/mock/adapter.go`
- Test: `core/internal/adapters/secondary/signing/mock/adapter.go` compile via integration tests

- [ ] **Step 1: Add partial-state fields to mock document struct**

In `core/internal/adapters/secondary/signing/mock/adapter.go`, find the internal mock document struct and ensure it contains:

```go
type providerDocument struct {
	ID             string
	CorrelationKey string
	Status         string
	PDFData        []byte
	Recipients     []string
	FieldsCreated  bool
	Distributed    bool
}
```

If the struct already has different field names, add only missing fields and update call sites.

- [ ] **Step 2: Implement `InspectProviderSubmission`**

Add method:

```go
func (a *Adapter) InspectProviderSubmission(_ context.Context, req *port.FindProviderDocumentRequest) (*port.ProviderSubmissionSnapshot, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, doc := range a.documents {
		if doc.CorrelationKey != req.CorrelationKey {
			continue
		}
		recipients := make([]port.RecipientResult, 0, len(doc.Recipients))
		for _, rid := range doc.Recipients {
			r := a.recipients[rid]
			recipients = append(recipients, port.RecipientResult{
				RoleID:               r.RoleID,
				ProviderRecipientID:  r.ID,
				ProviderSigningToken: r.Token,
				SigningURL:           fmt.Sprintf("http://mock-signing/sign/%s", r.Token),
				Status:               entity.RecipientStatusSent,
			})
		}
		return &port.ProviderSubmissionSnapshot{
			ProviderDocumentID: doc.ID,
			ProviderName:       providerName,
			CorrelationKey:     doc.CorrelationKey,
			RawStatus:          doc.Status,
			HasDocument:        true,
			HasRecipients:      len(doc.Recipients) > 0,
			HasFields:          doc.FieldsCreated,
			IsDistributed:      doc.Distributed,
			Recipients:         recipients,
		}, nil
	}

	return &port.ProviderSubmissionSnapshot{
		ProviderName:   providerName,
		CorrelationKey: req.CorrelationKey,
		HasDocument:    false,
		Reason:         "mock provider document not found",
	}, nil
}
```

- [ ] **Step 3: Implement ensure methods by delegating to existing mock behavior**

Add methods:

```go
func (a *Adapter) EnsureProviderDocument(ctx context.Context, req *port.EnsureProviderDocumentRequest) (*port.EnsureProviderDocumentResult, error) {
	snapshot, err := a.InspectProviderSubmission(ctx, &port.FindProviderDocumentRequest{CorrelationKey: req.CorrelationKey, Environment: req.Environment})
	if err != nil {
		return nil, err
	}
	if snapshot.HasDocument {
		return &port.EnsureProviderDocumentResult{ProviderDocumentID: snapshot.ProviderDocumentID, ProviderName: providerName, CorrelationKey: req.CorrelationKey, RawStatus: snapshot.RawStatus}, nil
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	id := fmt.Sprintf("mock-doc-%d", len(a.documents)+1)
	a.documents[id] = &providerDocument{ID: id, CorrelationKey: req.CorrelationKey, Status: "DRAFT", PDFData: append([]byte(nil), req.PDF...)}
	return &port.EnsureProviderDocumentResult{ProviderDocumentID: id, ProviderName: providerName, CorrelationKey: req.CorrelationKey, RawStatus: "DRAFT"}, nil
}

func (a *Adapter) EnsureProviderRecipients(_ context.Context, req *port.EnsureProviderRecipientsRequest) (*port.EnsureProviderRecipientsResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	doc, ok := a.documents[req.ProviderDocumentID]
	if !ok {
		return nil, fmt.Errorf("mock: document %s not found", req.ProviderDocumentID)
	}
	if len(doc.Recipients) == 0 {
		for _, recipient := range req.Recipients {
			id := fmt.Sprintf("mock-recipient-%d", len(a.recipients)+1)
			token := fmt.Sprintf("token-%s", id)
			a.recipients[id] = &providerRecipient{ID: id, RoleID: recipient.RoleID, Email: recipient.Email, Name: recipient.Name, Token: token, Status: "SENT"}
			doc.Recipients = append(doc.Recipients, id)
		}
	}
	results := make([]port.RecipientResult, 0, len(doc.Recipients))
	for _, rid := range doc.Recipients {
		r := a.recipients[rid]
		results = append(results, port.RecipientResult{RoleID: r.RoleID, ProviderRecipientID: r.ID, ProviderSigningToken: r.Token, SigningURL: fmt.Sprintf("http://mock-signing/sign/%s", r.Token), Status: entity.RecipientStatusSent})
	}
	return &port.EnsureProviderRecipientsResult{Recipients: results}, nil
}

func (a *Adapter) EnsureProviderFields(_ context.Context, req *port.EnsureProviderFieldsRequest) (*port.EnsureProviderFieldsResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	doc, ok := a.documents[req.ProviderDocumentID]
	if !ok {
		return nil, fmt.Errorf("mock: document %s not found", req.ProviderDocumentID)
	}
	doc.FieldsCreated = true
	return &port.EnsureProviderFieldsResult{FieldCount: len(req.SignatureFields)}, nil
}

func (a *Adapter) EnsureProviderDistributed(_ context.Context, req *port.EnsureProviderDistributedRequest) (*port.EnsureProviderDistributedResult, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	doc, ok := a.documents[req.ProviderDocumentID]
	if !ok {
		return nil, fmt.Errorf("mock: document %s not found", req.ProviderDocumentID)
	}
	doc.Distributed = true
	doc.Status = "PENDING"
	return &port.EnsureProviderDistributedResult{RawStatus: doc.Status}, nil
}

func (a *Adapter) FetchProviderSigningReferences(ctx context.Context, req *port.FetchProviderSigningReferencesRequest) (*port.FetchProviderSigningReferencesResult, error) {
	snapshot, err := a.InspectProviderSubmission(ctx, &port.FindProviderDocumentRequest{CorrelationKey: req.CorrelationKey, Environment: req.Environment})
	if err != nil {
		return nil, err
	}
	if !snapshot.HasDocument || !snapshot.HasRecipients || !snapshot.IsDistributed {
		return nil, fmt.Errorf("mock: provider submission is not signing-ready")
	}
	return &port.FetchProviderSigningReferencesResult{Recipients: snapshot.Recipients, Status: entity.SigningAttemptStatusSigningReady, RawStatus: snapshot.RawStatus}, nil
}
```

- [ ] **Step 4: Run mock package compile**

Run:

```bash
go test -C core ./internal/adapters/secondary/signing/mock
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add core/internal/adapters/secondary/signing/mock/adapter.go
git commit -m "test: support durable provider states in mock signing adapter"
```

---

## Task 4: Implement Durable Methods in Documenso Adapter

**Files:**
- Modify: `core/internal/adapters/secondary/signing/documenso/adapter.go`
- Modify: `core/internal/adapters/secondary/signing/documenso/adapter_test.go`

- [ ] **Step 1: Add failing Documenso unit tests for each partial phase**

Add tests to `adapter_test.go`:

```go
func TestInspectProviderSubmissionDetectsPartialDraftWithoutRecipients(t *testing.T) {
	const correlationKey = "doc-id:attempt-id"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/envelope":
			writeJSON(t, w, map[string]any{"data": []map[string]any{{"id": "env_partial", "externalId": correlationKey, "status": "DRAFT"}}, "pagination": map[string]any{"totalPages": 1}})
		case "/api/v2/envelope/env_partial":
			writeJSON(t, w, map[string]any{"id": "env_partial", "externalId": correlationKey, "status": "DRAFT", "recipients": []map[string]any{}, "fields": []map[string]any{}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	adapter, err := New(&Config{APIKey: "api_test", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := adapter.InspectProviderSubmission(context.Background(), &port.FindProviderDocumentRequest{CorrelationKey: correlationKey, Environment: entity.EnvironmentProd})
	if err != nil {
		t.Fatalf("InspectProviderSubmission() error = %v", err)
	}
	if !got.HasDocument || got.HasRecipients || got.HasFields || got.IsDistributed {
		t.Fatalf("snapshot = %+v", got)
	}
}
```

- [ ] **Step 2: Run test and verify it fails**

Run:

```bash
go test -C core ./internal/adapters/secondary/signing/documenso -run TestInspectProviderSubmissionDetectsPartialDraftWithoutRecipients -v
```

Expected: FAIL with `adapter.InspectProviderSubmission undefined`.

- [ ] **Step 3: Add `InspectProviderSubmission` implementation**

Add method to `adapter.go` near `FindProviderDocumentByCorrelationKey`:

```go
func (a *Adapter) InspectProviderSubmission(ctx context.Context, req *port.FindProviderDocumentRequest) (*port.ProviderSubmissionSnapshot, error) {
	found, err := a.FindProviderDocumentByCorrelationKey(ctx, req)
	if err != nil {
		return nil, err
	}
	if !found.Found {
		return &port.ProviderSubmissionSnapshot{ProviderName: providerName, CorrelationKey: req.CorrelationKey, HasDocument: false, Reason: found.Reason}, nil
	}

	detail, err := a.fetchEnvelope(ctx, found.ProviderDocumentID)
	if err != nil {
		return nil, err
	}

	recipients := make([]port.RecipientResult, 0, len(detail.Recipients))
	for _, recipient := range detail.Recipients {
		if strings.TrimSpace(recipient.ExternalID) == "" || strings.TrimSpace(recipient.Token) == "" {
			continue
		}
		recipients = append(recipients, port.RecipientResult{
			RoleID:               recipient.ExternalID,
			ProviderRecipientID:  strconv.Itoa(recipient.ID),
			ProviderSigningToken: recipient.Token,
			SigningURL:           fmt.Sprintf("%s/sign/%s", a.config.SigningBaseURL, recipient.Token),
			Status:               MapRecipientStatus(recipient.Status),
		})
	}

	snapshot := &port.ProviderSubmissionSnapshot{
		ProviderDocumentID: detail.ID,
		ProviderName:       providerName,
		CorrelationKey:     req.CorrelationKey,
		RawStatus:          detail.Status,
		HasDocument:        true,
		HasRecipients:      len(detail.Recipients) > 0,
		HasFields:          envelopeHasSignatureFields(detail),
		IsDistributed:      detail.Status != "DRAFT",
		Recipients:         recipients,
	}
	if !snapshot.HasRecipients {
		snapshot.Reason = "documenso envelope has no recipients"
	} else if !snapshot.HasFields {
		snapshot.Reason = "documenso envelope has no signature fields"
	} else if !snapshot.IsDistributed {
		snapshot.Reason = "documenso envelope is not distributed"
	}
	return snapshot, nil
}
```

Also add helper, adapting field names to the existing `envelopeDetailResponse` type:

```go
func envelopeHasSignatureFields(env *envelopeDetailResponse) bool {
	for _, recipient := range env.Recipients {
		if len(recipient.Fields) > 0 {
			return true
		}
	}
	return len(env.Fields) > 0
}
```

If `recipientResponse` or `envelopeDetailResponse` does not currently expose `Fields`, add JSON fields:

```go
Fields []fieldResponse `json:"fields"`
```

- [ ] **Step 4: Add ensure methods by reusing existing private methods**

Add methods to `adapter.go`:

```go
func (a *Adapter) EnsureProviderDocument(ctx context.Context, req *port.EnsureProviderDocumentRequest) (*port.EnsureProviderDocumentResult, error) {
	snapshot, err := a.InspectProviderSubmission(ctx, &port.FindProviderDocumentRequest{ProviderName: providerName, CorrelationKey: req.CorrelationKey, Environment: req.Environment})
	if err != nil {
		return nil, err
	}
	if snapshot.HasDocument {
		return &port.EnsureProviderDocumentResult{ProviderDocumentID: snapshot.ProviderDocumentID, ProviderName: providerName, CorrelationKey: req.CorrelationKey, RawStatus: snapshot.RawStatus}, nil
	}
	envelopeID, err := a.createEnvelope(ctx, req.Title, req.CorrelationKey, req.PDF)
	if err != nil {
		return nil, err
	}
	return &port.EnsureProviderDocumentResult{ProviderDocumentID: envelopeID, ProviderName: providerName, CorrelationKey: req.CorrelationKey, RawStatus: "DRAFT"}, nil
}

func (a *Adapter) EnsureProviderRecipients(ctx context.Context, req *port.EnsureProviderRecipientsRequest) (*port.EnsureProviderRecipientsResult, error) {
	detail, err := a.fetchEnvelope(ctx, req.ProviderDocumentID)
	if err != nil {
		return nil, err
	}
	if len(detail.Recipients) == 0 {
		if _, err := a.addRecipients(ctx, req.ProviderDocumentID, req.Recipients); err != nil {
			return nil, err
		}
		detail, err = a.fetchEnvelope(ctx, req.ProviderDocumentID)
		if err != nil {
			return nil, err
		}
	}
	return &port.EnsureProviderRecipientsResult{Recipients: recipientResultsFromEnvelope(detail, req.Recipients, a.config.SigningBaseURL)}, nil
}

func (a *Adapter) EnsureProviderFields(ctx context.Context, req *port.EnsureProviderFieldsRequest) (*port.EnsureProviderFieldsResult, error) {
	detail, err := a.fetchEnvelope(ctx, req.ProviderDocumentID)
	if err != nil {
		return nil, err
	}
	if envelopeHasSignatureFields(detail) {
		return &port.EnsureProviderFieldsResult{FieldCount: countEnvelopeFields(detail)}, nil
	}
	if len(req.SignatureFields) > 0 {
		if err := a.createSignatureFieldsFromRecipientResults(ctx, req.ProviderDocumentID, req.Recipients, req.SignatureFields); err != nil {
			return nil, err
		}
	}
	return &port.EnsureProviderFieldsResult{FieldCount: len(req.SignatureFields)}, nil
}

func (a *Adapter) EnsureProviderDistributed(ctx context.Context, req *port.EnsureProviderDistributedRequest) (*port.EnsureProviderDistributedResult, error) {
	detail, err := a.fetchEnvelope(ctx, req.ProviderDocumentID)
	if err != nil {
		return nil, err
	}
	if detail.Status == "DRAFT" {
		if err := a.distributeEnvelope(ctx, req.ProviderDocumentID); err != nil {
			return nil, err
		}
		detail, err = a.fetchEnvelope(ctx, req.ProviderDocumentID)
		if err != nil {
			return nil, err
		}
	}
	return &port.EnsureProviderDistributedResult{RawStatus: detail.Status}, nil
}

func (a *Adapter) FetchProviderSigningReferences(ctx context.Context, req *port.FetchProviderSigningReferencesRequest) (*port.FetchProviderSigningReferencesResult, error) {
	detail, err := a.fetchEnvelope(ctx, req.ProviderDocumentID)
	if err != nil {
		return nil, err
	}
	result := a.buildProviderDocumentResultFromEnvelope(detail, req.CorrelationKey)
	if !result.Usable {
		return nil, &port.ProviderError{Class: entity.ProviderErrorClassPermanent, Phase: entity.ProviderSubmitPhaseFetchSigningReferences, ProviderName: providerName, ProviderDocumentID: &req.ProviderDocumentID, Message: result.Reason}
	}
	return &port.FetchProviderSigningReferencesResult{Recipients: result.Recipients, Status: result.Status, RawStatus: result.RawStatus}, nil
}
```

- [ ] **Step 5: Add helpers used by ensure methods**

Add helpers near `buildUploadResult`:

```go
func recipientResultsFromEnvelope(env *envelopeDetailResponse, originalRecipients []port.SigningRecipient, signingBaseURL string) []port.RecipientResult {
	byExternalID := make(map[string]recipientResponse, len(env.Recipients))
	for _, r := range env.Recipients {
		byExternalID[r.ExternalID] = r
	}
	results := make([]port.RecipientResult, 0, len(originalRecipients))
	for _, orig := range originalRecipients {
		r, ok := byExternalID[orig.RoleID]
		if !ok && len(results) < len(env.Recipients) {
			r = env.Recipients[len(results)]
			ok = true
		}
		if !ok {
			continue
		}
		results = append(results, port.RecipientResult{
			RoleID:               orig.RoleID,
			ProviderRecipientID:  strconv.Itoa(r.ID),
			ProviderSigningToken: r.Token,
			SigningURL:           fmt.Sprintf("%s/sign/%s", signingBaseURL, r.Token),
			Status:               MapRecipientStatus(r.Status),
		})
	}
	return results
}

func countEnvelopeFields(env *envelopeDetailResponse) int {
	count := len(env.Fields)
	for _, r := range env.Recipients {
		count += len(r.Fields)
	}
	return count
}
```

- [ ] **Step 6: Run Documenso adapter tests**

Run:

```bash
go test -C core ./internal/adapters/secondary/signing/documenso -v
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add core/internal/adapters/secondary/signing/documenso/adapter.go core/internal/adapters/secondary/signing/documenso/adapter_test.go
git commit -m "feat: make documenso submission steps idempotent"
```

---

## Task 5: Add Durable River Job and Executor State Machine

**Files:**
- Modify: `core/internal/infra/riverqueue/args.go`
- Modify: `core/internal/infra/riverqueue/attempt_workers.go`
- Modify: `core/internal/infra/riverqueue/uow.go`
- Modify: `core/internal/infra/riverqueue/executor.go`
- Modify: `core/internal/infra/riverqueue/river_integration_test.go`

- [ ] **Step 1: Add failing integration test for timeout after envelope creation**

Append to `river_integration_test.go`:

```go
func TestSigningAttemptExecutor_RecoversPartialProviderEnvelopeWithoutInfiniteProcessing(t *testing.T) {
	ctx := context.Background()
	fx := newAttemptFixture(t, ctx)
	riverSvc, err := riverqueue.New(ctx, fx.pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{DocumentRepo: fx.docRepo, AttemptRepo: fx.attemptRepo})
	require.NoError(t, err)

	attempt, err := riverSvc.SigningExecutionUOW().CreateAttemptAndEnqueueRender(ctx, fx.documentID, fx.recipients(), fx.signerOrders())
	require.NoError(t, err)

	// Simulate render already completed and a provider envelope created before the worker crashed.
	providerName := "mock"
	correlation := fmt.Sprintf("%s:%s", fx.documentID, attempt.ID)
	pdfPath := fmt.Sprintf("documents/%s/%s/attempts/%s/pre-signed.pdf", fx.workspaceID, fx.documentID, attempt.ID)
	checksum := "known-test-checksum"
	_, err = fx.pool.Exec(ctx, `
		UPDATE execution.signing_attempts
		SET status='SUBMISSION_UNKNOWN', provider_name=$2, provider_correlation_key=$3,
		    provider_submit_phase='CREATE_PROVIDER_DOCUMENT', pdf_storage_path=$4, pdf_checksum=$5,
		    pdf_checksum_algorithm='sha256', signature_field_snapshot='[]'::jsonb
		WHERE id=$1`, attempt.ID, providerName, correlation, pdfPath, checksum)
	require.NoError(t, err)

	provider := mocksigning.New()
	_, err = provider.EnsureProviderDocument(ctx, &port.EnsureProviderDocumentRequest{AttemptID: attempt.ID, DocumentID: fx.documentID, CorrelationKey: correlation, PDF: []byte("pdf"), PDFChecksum: checksum, Title: "Attempt UOW", Environment: entity.EnvironmentProd})
	require.NoError(t, err)

	executor := riverqueue.NewSigningAttemptExecutor(riverqueue.SigningAttemptExecutorConfig{
		Pool:            fx.pool,
		Client:          riverSvc.Client(),
		DocumentRepo:    fx.docRepo,
		RecipientRepo:   documentrecipientrepo.New(fx.pool),
		AttemptRepo:     fx.attemptRepo,
		SigningProvider: provider,
	})

	require.NoError(t, executor.ReconcileProviderSubmission(ctx, attempt.ID))

	var status entity.SigningAttemptStatus
	var providerDocID *string
	require.NoError(t, fx.pool.QueryRow(ctx, `SELECT status, provider_document_id FROM execution.signing_attempts WHERE id=$1`, attempt.ID).Scan(&status, &providerDocID))
	require.NotEqual(t, entity.SigningAttemptStatusSubmissionUnknown, status)
	require.NotNil(t, providerDocID)
}
```

Adjust imports in `river_integration_test.go`:

```go
mocksigning "github.com/rendis/doc-assembly/core/internal/adapters/secondary/signing/mock"
documentrecipientrepo "github.com/rendis/doc-assembly/core/internal/adapters/secondary/database/postgres/document_recipient_repo"
```

- [ ] **Step 2: Run failing integration test**

Run:

```bash
go test -C core -tags=integration -run TestSigningAttemptExecutor_RecoversPartialProviderEnvelopeWithoutInfiniteProcessing -v -count=1 ./internal/infra/riverqueue/
```

Expected: FAIL until executor has durable state machine and mock imports are adjusted to actual package path names.

- [ ] **Step 3: Add River args for durable advancement**

In `args.go`, add:

```go
type AdvanceProviderSubmissionArgs AttemptJobArgs

func (AdvanceProviderSubmissionArgs) Kind() string { return "advance_provider_submission" }

func (a AdvanceProviderSubmissionArgs) InsertOpts() river.InsertOpts { return uniqueAttemptPhaseOpts() }
```

- [ ] **Step 4: Add worker**

In `attempt_workers.go`, add:

```go
type AdvanceProviderSubmissionWorker struct {
	river.WorkerDefaults[AdvanceProviderSubmissionArgs]
	executor *SigningAttemptExecutor
}

func (w *AdvanceProviderSubmissionWorker) Work(ctx context.Context, job *river.Job[AdvanceProviderSubmissionArgs]) error {
	return w.executor.AdvanceProviderSubmission(ctx, job.Args.AttemptID)
}
func (w *AdvanceProviderSubmissionWorker) Timeout(_ *river.Job[AdvanceProviderSubmissionArgs]) time.Duration {
	return 2 * time.Minute
}

var _ river.Worker[AdvanceProviderSubmissionArgs] = (*AdvanceProviderSubmissionWorker)(nil)
```

- [ ] **Step 5: Register worker**

In `client.go`, register next to submit/reconcile workers:

```go
river.AddWorker(workers, &AdvanceProviderSubmissionWorker{executor: executor})
```

- [ ] **Step 6: Update enqueue switch**

In `uow.go`, add case:

```go
case port.SigningJobPhaseSubmitAttemptToProvider:
	_, err = u.client.InsertTx(ctx, tx, AdvanceProviderSubmissionArgs{AttemptID: attemptID}, nil)
```

Keep legacy `SubmitAttemptToProviderArgs` only for backwards compatibility tests if needed; otherwise migrate all submit enqueues to `AdvanceProviderSubmissionArgs`.

- [ ] **Step 7: Implement `AdvanceProviderSubmission` in executor**

Add method to `executor.go`:

```go
func (e *SigningAttemptExecutor) AdvanceProviderSubmission(ctx context.Context, attemptID string) error {
	attempt, doc, stale, err := e.loadActiveAttempt(ctx, attemptID,
		entity.SigningAttemptStatusReadyToSubmit,
		entity.SigningAttemptStatusProviderRetryWaiting,
		entity.SigningAttemptStatusSubmittingProvider,
		entity.SigningAttemptStatusSubmissionUnknown,
		entity.SigningAttemptStatusReconcilingProvider)
	if err != nil || stale {
		return err
	}
	if attempt.IsTerminal() {
		return nil
	}
	if attempt.PDFStoragePath == nil || attempt.PDFChecksum == nil || attempt.ProviderCorrelationKey == nil {
		return e.failPermanent(ctx, attempt, attempt.Status, entity.ProviderSubmitPhaseBeforeRequest, fmt.Errorf("attempt provider submission artifacts are missing"))
	}

	phase := entity.ProviderSubmitPhaseCreateProviderDocument
	if attempt.ProviderSubmitPhase != nil && attempt.ProviderSubmitPhase.IsProviderMutationPhase() {
		phase = *attempt.ProviderSubmitPhase
	}

	switch phase {
	case entity.ProviderSubmitPhaseCreateProviderDocument:
		return e.advanceEnsureProviderDocument(ctx, attempt, doc)
	case entity.ProviderSubmitPhaseAddRecipients:
		return e.advanceEnsureProviderRecipients(ctx, attempt)
	case entity.ProviderSubmitPhaseCreateFields:
		return e.advanceEnsureProviderFields(ctx, attempt)
	case entity.ProviderSubmitPhaseDistributeDocument:
		return e.advanceEnsureProviderDistributed(ctx, attempt)
	case entity.ProviderSubmitPhaseFetchSigningReferences:
		return e.advanceFetchSigningReferences(ctx, attempt)
	default:
		return e.failPermanent(ctx, attempt, attempt.Status, entity.ProviderSubmitPhaseBeforeRequest, fmt.Errorf("unknown provider submit phase %q", phase))
	}
}
```

- [ ] **Step 8: Implement phase helpers**

Add helpers in `executor.go`:

```go
func (e *SigningAttemptExecutor) advanceEnsureProviderDocument(ctx context.Context, attempt *entity.SigningAttempt, doc *entity.Document) error {
	pdf, err := e.storageAdapter.Download(ctx, &port.StorageRequest{Key: *attempt.PDFStoragePath, Environment: entity.EnvironmentProd})
	if err != nil {
		return e.failPermanent(ctx, attempt, attempt.Status, entity.ProviderSubmitPhaseBeforeRequest, fmt.Errorf("download attempt PDF: %w", err))
	}
	if !checksumMatches(pdf, *attempt.PDFChecksum) {
		return e.failPermanent(ctx, attempt, attempt.Status, entity.ProviderSubmitPhaseBeforeRequest, fmt.Errorf("attempt PDF checksum mismatch"))
	}
	phase := entity.ProviderSubmitPhaseCreateProviderDocument
	attempt.Status = entity.SigningAttemptStatusSubmittingProvider
	attempt.ProviderSubmitPhase = &phase
	if err := e.transition(ctx, attempt, "ATTEMPT_PROVIDER_ENVELOPE_ENSURE_STARTED", nil, nil); err != nil {
		return err
	}
	result, err := e.signingProvider.EnsureProviderDocument(ctx, &port.EnsureProviderDocumentRequest{AttemptID: attempt.ID, DocumentID: doc.ID, CorrelationKey: deref(attempt.ProviderCorrelationKey), PDF: pdf, PDFChecksum: *attempt.PDFChecksum, Title: documentTitle(doc), Environment: entity.EnvironmentProd})
	if err != nil {
		return e.handleProviderBoundaryError(ctx, attempt, phase, err)
	}
	attempt.ProviderDocumentID = &result.ProviderDocumentID
	attempt.ProviderName = &result.ProviderName
	next := entity.ProviderSubmitPhaseAddRecipients
	attempt.ProviderSubmitPhase = &next
	return e.transition(ctx, attempt, "ATTEMPT_PROVIDER_ENVELOPE_ENSURED", ptrPhase(port.SigningJobPhaseSubmitAttemptToProvider), nil)
}

func (e *SigningAttemptExecutor) advanceEnsureProviderRecipients(ctx context.Context, attempt *entity.SigningAttempt) error {
	if attempt.ProviderDocumentID == nil {
		return e.failPermanent(ctx, attempt, attempt.Status, entity.ProviderSubmitPhaseAddRecipients, fmt.Errorf("provider document id is missing"))
	}
	attemptRecipients, err := e.attemptRepo.FindRecipientsByAttemptID(ctx, attempt.ID)
	if err != nil {
		return err
	}
	phase := entity.ProviderSubmitPhaseAddRecipients
	attempt.ProviderSubmitPhase = &phase
	if err := e.transition(ctx, attempt, "ATTEMPT_PROVIDER_RECIPIENTS_ENSURE_STARTED", nil, nil); err != nil {
		return err
	}
	result, err := e.signingProvider.EnsureProviderRecipients(ctx, &port.EnsureProviderRecipientsRequest{ProviderDocumentID: *attempt.ProviderDocumentID, CorrelationKey: deref(attempt.ProviderCorrelationKey), Recipients: signingRecipientsFromAttempts(attemptRecipients), Environment: entity.EnvironmentProd})
	if err != nil {
		return e.handleProviderBoundaryError(ctx, attempt, phase, err)
	}
	if err := e.persistAttemptRecipientProviderRefs(ctx, attempt.ID, result.Recipients); err != nil {
		return err
	}
	next := entity.ProviderSubmitPhaseCreateFields
	attempt.ProviderSubmitPhase = &next
	return e.transition(ctx, attempt, "ATTEMPT_PROVIDER_RECIPIENTS_ENSURED", ptrPhase(port.SigningJobPhaseSubmitAttemptToProvider), nil)
}

func (e *SigningAttemptExecutor) advanceEnsureProviderFields(ctx context.Context, attempt *entity.SigningAttempt) error {
	if attempt.ProviderDocumentID == nil {
		return e.failPermanent(ctx, attempt, attempt.Status, entity.ProviderSubmitPhaseCreateFields, fmt.Errorf("provider document id is missing"))
	}
	recipients, err := e.attemptRepo.FindRecipientsByAttemptID(ctx, attempt.ID)
	if err != nil {
		return err
	}
	providerRecipients := recipientResultsFromAttemptRecipients(recipients)
	sigFields, err := decodeSignatureFields(attempt.SignatureFieldSnapshot)
	if err != nil {
		return e.failPermanent(ctx, attempt, attempt.Status, entity.ProviderSubmitPhaseBeforeRequest, err)
	}
	phase := entity.ProviderSubmitPhaseCreateFields
	attempt.ProviderSubmitPhase = &phase
	if err := e.transition(ctx, attempt, "ATTEMPT_PROVIDER_FIELDS_ENSURE_STARTED", nil, nil); err != nil {
		return err
	}
	_, err = e.signingProvider.EnsureProviderFields(ctx, &port.EnsureProviderFieldsRequest{ProviderDocumentID: *attempt.ProviderDocumentID, CorrelationKey: deref(attempt.ProviderCorrelationKey), Recipients: providerRecipients, SignatureFields: sigFields, Environment: entity.EnvironmentProd})
	if err != nil {
		return e.handleProviderBoundaryError(ctx, attempt, phase, err)
	}
	next := entity.ProviderSubmitPhaseDistributeDocument
	attempt.ProviderSubmitPhase = &next
	return e.transition(ctx, attempt, "ATTEMPT_PROVIDER_FIELDS_ENSURED", ptrPhase(port.SigningJobPhaseSubmitAttemptToProvider), nil)
}

func (e *SigningAttemptExecutor) advanceEnsureProviderDistributed(ctx context.Context, attempt *entity.SigningAttempt) error {
	if attempt.ProviderDocumentID == nil {
		return e.failPermanent(ctx, attempt, attempt.Status, entity.ProviderSubmitPhaseDistributeDocument, fmt.Errorf("provider document id is missing"))
	}
	phase := entity.ProviderSubmitPhaseDistributeDocument
	attempt.ProviderSubmitPhase = &phase
	if err := e.transition(ctx, attempt, "ATTEMPT_PROVIDER_DISTRIBUTE_STARTED", nil, nil); err != nil {
		return err
	}
	_, err := e.signingProvider.EnsureProviderDistributed(ctx, &port.EnsureProviderDistributedRequest{ProviderDocumentID: *attempt.ProviderDocumentID, CorrelationKey: deref(attempt.ProviderCorrelationKey), Environment: entity.EnvironmentProd})
	if err != nil {
		return e.handleProviderBoundaryError(ctx, attempt, phase, err)
	}
	next := entity.ProviderSubmitPhaseFetchSigningReferences
	attempt.ProviderSubmitPhase = &next
	return e.transition(ctx, attempt, "ATTEMPT_PROVIDER_DISTRIBUTED", ptrPhase(port.SigningJobPhaseSubmitAttemptToProvider), nil)
}

func (e *SigningAttemptExecutor) advanceFetchSigningReferences(ctx context.Context, attempt *entity.SigningAttempt) error {
	if attempt.ProviderDocumentID == nil {
		return e.failPermanent(ctx, attempt, attempt.Status, entity.ProviderSubmitPhaseFetchSigningReferences, fmt.Errorf("provider document id is missing"))
	}
	phase := entity.ProviderSubmitPhaseFetchSigningReferences
	attempt.ProviderSubmitPhase = &phase
	if err := e.transition(ctx, attempt, "ATTEMPT_PROVIDER_SIGNING_REFERENCES_FETCH_STARTED", nil, nil); err != nil {
		return err
	}
	result, err := e.signingProvider.FetchProviderSigningReferences(ctx, &port.FetchProviderSigningReferencesRequest{ProviderDocumentID: *attempt.ProviderDocumentID, CorrelationKey: deref(attempt.ProviderCorrelationKey), Environment: entity.EnvironmentProd})
	if err != nil {
		return e.handleProviderBoundaryError(ctx, attempt, phase, err)
	}
	attempt.Status = result.Status
	if attempt.Status == "" {
		attempt.Status = entity.SigningAttemptStatusSigningReady
	}
	attempt.ProviderSubmitPhase = nil
	attempt.RetryCount = 0
	attempt.LastErrorClass = nil
	attempt.LastErrorMessage = nil
	return e.persistProviderSuccess(ctx, attempt, result.Recipients)
}
```

- [ ] **Step 9: Add missing helper functions**

Add to `executor.go`:

```go
func recipientResultsFromAttemptRecipients(recipients []*entity.SigningAttemptRecipient) []port.RecipientResult {
	results := make([]port.RecipientResult, 0, len(recipients))
	for _, r := range recipients {
		results = append(results, port.RecipientResult{
			RoleID:               r.TemplateVersionRoleID,
			ProviderRecipientID:  deref(r.ProviderRecipientID),
			ProviderSigningToken: deref(r.ProviderSigningToken),
			SigningURL:           deref(r.SigningURL),
			Status:               r.Status,
		})
	}
	return results
}

func (e *SigningAttemptExecutor) persistAttemptRecipientProviderRefs(ctx context.Context, attemptID string, recipients []port.RecipientResult) error {
	attemptRecipients, err := e.attemptRepo.FindRecipientsByAttemptID(ctx, attemptID)
	if err != nil {
		return err
	}
	byRole := make(map[string]port.RecipientResult, len(recipients))
	for _, r := range recipients {
		byRole[r.RoleID] = r
	}
	for _, attemptRecipient := range attemptRecipients {
		result, ok := byRole[attemptRecipient.TemplateVersionRoleID]
		if !ok {
			continue
		}
		attemptRecipient.ProviderRecipientID = ptrString(result.ProviderRecipientID)
		attemptRecipient.ProviderSigningToken = ptrString(result.ProviderSigningToken)
		attemptRecipient.SigningURL = ptrString(result.SigningURL)
		attemptRecipient.Status = result.Status
		if err := e.attemptRepo.UpdateRecipient(ctx, attemptRecipient); err != nil {
			return err
		}
	}
	return nil
}

func ptrString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}
```

`SigningAttemptRepository` already exposes `UpdateRecipient`; use it for this helper.

- [ ] **Step 10: Change legacy `SubmitAttemptToProvider` to delegate**

Replace the body of `SubmitAttemptToProvider` with:

```go
func (e *SigningAttemptExecutor) SubmitAttemptToProvider(ctx context.Context, attemptID string) error {
	return e.AdvanceProviderSubmission(ctx, attemptID)
}
```

- [ ] **Step 11: Update reconciliation to resume partial states**

In `ReconcileProviderSubmission`, replace unusable-found branch:

```go
attempt.Status = entity.SigningAttemptStatusRequiresReview
msg := found.Reason
attempt.LastErrorMessage = &msg
return e.transition(ctx, attempt, "ATTEMPT_REQUIRES_REVIEW", nil, &old)
```

with:

```go
snapshot, inspectErr := e.signingProvider.InspectProviderSubmission(ctx, &port.FindProviderDocumentRequest{ProviderName: *attempt.ProviderName, CorrelationKey: *attempt.ProviderCorrelationKey, Environment: entity.EnvironmentProd})
if inspectErr != nil {
	return inspectErr
}
if snapshot.HasDocument {
	attempt.ProviderDocumentID = &snapshot.ProviderDocumentID
	attempt.Status = entity.SigningAttemptStatusSubmittingProvider
	next := phaseFromProviderSnapshot(snapshot)
	attempt.ProviderSubmitPhase = &next
	return e.transition(ctx, attempt, "ATTEMPT_PROVIDER_PARTIAL_RECOVERED", ptrPhase(port.SigningJobPhaseSubmitAttemptToProvider), &old)
}
attempt.Status = entity.SigningAttemptStatusRequiresReview
msg := found.Reason
attempt.LastErrorMessage = &msg
return e.transition(ctx, attempt, "ATTEMPT_REQUIRES_REVIEW", nil, &old)
```

Add helper:

```go
func phaseFromProviderSnapshot(snapshot *port.ProviderSubmissionSnapshot) entity.ProviderSubmitPhase {
	switch {
	case snapshot == nil || !snapshot.HasDocument:
		return entity.ProviderSubmitPhaseCreateProviderDocument
	case !snapshot.HasRecipients:
		return entity.ProviderSubmitPhaseAddRecipients
	case !snapshot.HasFields:
		return entity.ProviderSubmitPhaseCreateFields
	case !snapshot.IsDistributed:
		return entity.ProviderSubmitPhaseDistributeDocument
	default:
		return entity.ProviderSubmitPhaseFetchSigningReferences
	}
}
```

- [ ] **Step 12: Run targeted integration test**

Run:

```bash
go test -C core -tags=integration -run TestSigningAttemptExecutor_RecoversPartialProviderEnvelopeWithoutInfiniteProcessing -v -count=1 ./internal/infra/riverqueue/
```

Expected: PASS.

- [ ] **Step 13: Run River integration suite**

Run:

```bash
go test -C core -tags=integration -run TestSigningAttempt -v -count=1 ./internal/infra/riverqueue/
```

Expected: PASS.

- [ ] **Step 14: Commit**

```bash
git add core/internal/infra/riverqueue/args.go core/internal/infra/riverqueue/attempt_workers.go core/internal/infra/riverqueue/client.go core/internal/infra/riverqueue/uow.go core/internal/infra/riverqueue/executor.go core/internal/infra/riverqueue/river_integration_test.go
git commit -m "feat: resume provider submission by durable phase"
```

---

## Task 6: Add Public UX Anti-Infinite-Processing Behavior

**Files:**
- Modify: `core/internal/core/usecase/document/pre_signing_usecase.go`
- Modify: `core/internal/core/service/document/pre_signing_service.go`
- Modify: `app/src/features/public-signing/**` actual component files discovered by `rg "Preparing document|processing|PublicSigning" app/src`

- [ ] **Step 1: Find actual frontend component files**

Run:

```bash
rg -n "Preparing document|processing|PublicSigning|StepProcessing|public-signing" app/src
```

Expected: identify exact component/type files. Use those exact files in following steps.

- [ ] **Step 2: Add backend response fields**

In `pre_signing_usecase.go`, add optional fields to `PublicSigningResponse`:

```go
ProcessingReason  string `json:"processingReason,omitempty"`
RetryAfterSeconds int    `json:"retryAfterSeconds,omitempty"`
SupportCode       string `json:"supportCode,omitempty"`
```

- [ ] **Step 3: Add stale processing helper**

In `pre_signing_service.go`, add constants and helper:

```go
const publicProcessingSoftLimit = 10 * time.Minute

func attemptProcessingAge(attempt *entity.SigningAttempt) time.Duration {
	if attempt == nil {
		return 0
	}
	if attempt.UpdatedAt != nil {
		return time.Since(*attempt.UpdatedAt)
	}
	return time.Since(attempt.CreatedAt)
}

func isLongRunningProcessingAttempt(attempt *entity.SigningAttempt) bool {
	return attemptProcessingAge(attempt) > publicProcessingSoftLimit
}
```

- [ ] **Step 4: Bound public processing response**

In `buildAttemptSigningResponse`, change the processing case to:

```go
case entity.SigningAttemptStatusCreated, entity.SigningAttemptStatusRendering, entity.SigningAttemptStatusPDFReady,
	entity.SigningAttemptStatusReadyToSubmit, entity.SigningAttemptStatusSubmittingProvider,
	entity.SigningAttemptStatusProviderRetryWaiting, entity.SigningAttemptStatusSubmissionUnknown, entity.SigningAttemptStatusReconcilingProvider:
	resp := s.buildProcessingResponse(doc, recipient, accessToken.Token)
	resp.RetryAfterSeconds = 5
	resp.ProcessingReason = "preparing_document"
	if isLongRunningProcessingAttempt(attempt) {
		resp.ProcessingReason = "recovering_provider_submission"
		resp.SupportCode = attempt.ID
	}
	return resp, nil
```

Do not expose internal error messages to public signer.

- [ ] **Step 5: Keep unrecoverable terminal attempts unavailable**

Ensure this branch remains:

```go
case entity.SigningAttemptStatusFailedPermanent, entity.SigningAttemptStatusRequiresReview:
	return s.buildUnavailableResponse(doc, recipient, accessToken.Token), nil
```

- [ ] **Step 6: Update frontend copy**

In the discovered processing component, implement equivalent logic:

```tsx
const title = response.processingReason === 'recovering_provider_submission'
  ? t('publicSigning.recoveringDocumentTitle', 'Still preparing your document')
  : t('publicSigning.preparingDocumentTitle', 'Preparing document')

const description = response.processingReason === 'recovering_provider_submission'
  ? t('publicSigning.recoveringDocumentDescription', 'This is taking longer than usual. We are safely recovering your signing session and this page will update automatically.')
  : t('publicSigning.preparingDocumentDescription', 'Your signing session is being prepared. This page will update automatically.')
```

- [ ] **Step 7: Run frontend checks**

Run:

```bash
pnpm -C app lint
pnpm -C app build
```

Expected: PASS.

- [ ] **Step 8: Run backend focused tests**

Run:

```bash
go test -C core ./internal/core/service/document ./internal/adapters/primary/http/controller
```

Expected: PASS.

- [ ] **Step 9: Commit**

```bash
git add core/internal/core/usecase/document/pre_signing_usecase.go core/internal/core/service/document/pre_signing_service.go app/src
git commit -m "feat: prevent infinite public signing processing state"
```

---

## Task 7: Local Failure Reproduction with Docker and Visual Harness

**Files:**
- Create: `docs/superpowers/evidence/2026-05-06-durable-provider-submission/README.md`
- Create if needed: temporary untracked local script under `.tmp/visual-e2e/` only; do not commit temp scripts unless they become reusable.

- [ ] **Step 1: Start local dependencies**

Run the same local environment used earlier in this session. Record exact commands in evidence README. Expected services:

```text
app Postgres: localhost:5432
Documenso: localhost:3000
Documenso DB: localhost:5433
Mailpit: localhost:8025 / localhost:1025
doc-assembly backend: localhost:8080 or configured local port
iframe harness: localhost:8099
```

- [ ] **Step 2: Build backend and frontend before reproduction**

Run:

```bash
make -C core build
pnpm -C app build
```

Expected: PASS.

- [ ] **Step 3: Enable failpoint that simulates production failure**

Set non-production env vars for local backend:

```bash
export DOC_ENGINE_WORKER_ENABLED=true
export DOC_ENGINE_WORKER_RUNTIME_ENVIRONMENT=development
export DOC_ENGINE_WORKER_FAILPOINTS_ENABLED=true
export DOC_ENGINE_WORKER_FAILPOINTS=submit_after_provider_before_commit
```

If existing failpoint does not create an envelope before failing, add a new failpoint in Task 8 before proceeding:

```text
submit_after_envelope_before_recipients
```

- [ ] **Step 4: Run visual flow until failure is reproduced**

Using Browser Use in-app browser:

1. open iframe harness at `http://localhost:8099/`
2. generate contract
3. preview contract
4. proceed to signing
5. verify UI shows processing while provider has partial state

Expected before fix, or with legacy code: public page remains `processing` and DB shows a partial provider artifact.

- [ ] **Step 5: Capture evidence**

Add to evidence README:

```markdown
## Failure Reproduction

- Date/time:
- Backend commit:
- Failpoint:
- Document ID:
- Attempt ID:
- Browser screenshot path:
- doc_assembly attempt status:
- Documenso provider state:
- Public API response:
```

Run DB query and paste sanitized output:

```sql
select id, status, provider_submit_phase, provider_document_id, provider_correlation_key, updated_at
from execution.signing_attempts
where document_id = '<document-id>'
order by sequence;
```

- [ ] **Step 6: Commit evidence only if repository convention accepts evidence artifacts**

If evidence artifacts are normally committed:

```bash
git add docs/superpowers/evidence/2026-05-06-durable-provider-submission/README.md
git commit -m "test: document local provider partial failure reproduction"
```

If evidence artifacts are not committed, keep them untracked and mention in final verification.

---

## Task 8: Add Fine-Grained Failpoints for Provider Step Boundaries

**Files:**
- Modify: `core/internal/infra/riverqueue/failpoints.go`
- Modify: `core/internal/infra/riverqueue/executor.go`
- Modify: `core/internal/infra/riverqueue/river_integration_test.go`

- [ ] **Step 1: Add failpoint names**

In `failpoints.go`, add constants:

```go
const (
	failpointSubmitAfterEnvelopeBeforeRecipients = "submit_after_envelope_before_recipients"
	failpointSubmitAfterRecipientsBeforeFields   = "submit_after_recipients_before_fields"
	failpointSubmitAfterFieldsBeforeDistribute   = "submit_after_fields_before_distribute"
	failpointSubmitAfterDistributeBeforeRefs     = "submit_after_distribute_before_refs"
)
```

- [ ] **Step 2: Trigger failpoints in phase helpers**

In `advanceEnsureProviderDocument`, after provider document is persisted and before enqueueing next phase:

```go
if e.failpoints.Enabled(failpointSubmitAfterEnvelopeBeforeRecipients) {
	return failpointErr(failpointSubmitAfterEnvelopeBeforeRecipients)
}
```

In `advanceEnsureProviderRecipients`, after `persistAttemptRecipientProviderRefs`:

```go
if e.failpoints.Enabled(failpointSubmitAfterRecipientsBeforeFields) {
	return failpointErr(failpointSubmitAfterRecipientsBeforeFields)
}
```

In `advanceEnsureProviderFields`, after `EnsureProviderFields` succeeds:

```go
if e.failpoints.Enabled(failpointSubmitAfterFieldsBeforeDistribute) {
	return failpointErr(failpointSubmitAfterFieldsBeforeDistribute)
}
```

In `advanceEnsureProviderDistributed`, after `EnsureProviderDistributed` succeeds:

```go
if e.failpoints.Enabled(failpointSubmitAfterDistributeBeforeRefs) {
	return failpointErr(failpointSubmitAfterDistributeBeforeRefs)
}
```

- [ ] **Step 3: Add integration tests for every failpoint**

Add table test:

```go
func TestSigningAttemptExecutor_ResumesAfterProviderStepFailpoints(t *testing.T) {
	cases := []struct {
		name      string
		failpoint string
	}{
		{"after envelope", "submit_after_envelope_before_recipients"},
		{"after recipients", "submit_after_recipients_before_fields"},
		{"after fields", "submit_after_fields_before_distribute"},
		{"after distribute", "submit_after_distribute_before_refs"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			fx := newAttemptFixture(t, ctx)
			provider := mocksigning.New()
			riverSvc, err := riverqueue.New(ctx, fx.pool, config.WorkerConfig{Enabled: false}, riverqueue.Dependencies{DocumentRepo: fx.docRepo, AttemptRepo: fx.attemptRepo})
			require.NoError(t, err)
			attempt, err := riverSvc.SigningExecutionUOW().CreateAttemptAndEnqueueRender(ctx, fx.documentID, fx.recipients(), fx.signerOrders())
			require.NoError(t, err)

			executorWithFailpoint := newExecutorForAttemptFixture(t, fx, riverSvc, provider, riverqueue.AttemptFailpointsFromList(true, []string{tc.failpoint}))
			err = executorWithFailpoint.AdvanceProviderSubmission(ctx, attempt.ID)
			require.Error(t, err)

			executorRecovered := newExecutorForAttemptFixture(t, fx, riverSvc, provider, riverqueue.AttemptFailpoints{})
			require.NoError(t, executorRecovered.AdvanceProviderSubmission(ctx, attempt.ID))

			var status entity.SigningAttemptStatus
			require.NoError(t, fx.pool.QueryRow(ctx, `SELECT status FROM execution.signing_attempts WHERE id=$1`, attempt.ID).Scan(&status))
			require.NotEqual(t, entity.SigningAttemptStatusSubmissionUnknown, status)
		})
	}
}
```

Add helper in test file:

```go
func newExecutorForAttemptFixture(t *testing.T, fx *attemptFixture, riverSvc *riverqueue.RiverService, provider port.SigningProvider, failpoints riverqueue.AttemptFailpoints) *riverqueue.SigningAttemptExecutor {
	t.Helper()
	return riverqueue.NewSigningAttemptExecutor(riverqueue.SigningAttemptExecutorConfig{
		Pool:              fx.pool,
		Client:            riverSvc.Client(),
		DocumentRepo:      fx.docRepo,
		RecipientRepo:     documentrecipientrepo.New(fx.pool),
		AttemptRepo:       fx.attemptRepo,
		VersionRepo:       templateversionrepo.New(fx.pool),
		SignerRoleRepo:    signerversionrolerepo.New(fx.pool),
		FieldResponseRepo: documentfieldresponserepo.New(fx.pool),
		SigningProvider:   provider,
		Failpoints:        failpoints,
	})
}
```

Use actual repo package names discovered under `core/internal/adapters/secondary/database/postgres/`.

- [ ] **Step 4: Run targeted tests**

Run:

```bash
go test -C core -tags=integration -run TestSigningAttemptExecutor_ResumesAfterProviderStepFailpoints -v -count=1 ./internal/infra/riverqueue/
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add core/internal/infra/riverqueue/failpoints.go core/internal/infra/riverqueue/executor.go core/internal/infra/riverqueue/river_integration_test.go
git commit -m "test: cover provider submission recovery boundaries"
```

---

## Task 9: Full Local UI Validation After Fix

**Files:**
- Modify: `docs/superpowers/evidence/2026-05-06-durable-provider-submission/README.md`

- [ ] **Step 1: Rebuild all local artifacts**

Run:

```bash
make -C core build
pnpm -C app build
```

Expected: PASS.

- [ ] **Step 2: Start local environment with failure failpoint enabled**

Run backend with:

```bash
export DOC_ENGINE_WORKER_ENABLED=true
export DOC_ENGINE_WORKER_RUNTIME_ENVIRONMENT=development
export DOC_ENGINE_WORKER_FAILPOINTS_ENABLED=true
export DOC_ENGINE_WORKER_FAILPOINTS=submit_after_envelope_before_recipients
make -C core run
```

Expected: backend starts, River workers enabled, failpoint active.

- [ ] **Step 3: Run Browser Use visual flow to reproduce and observe recovery**

Use Browser Use in-app browser. Execute:

1. Navigate to local iframe harness.
2. Generate a contract.
3. Preview PDF inside iframe.
4. Proceed to signing.
5. Observe transient processing/recovery UI.
6. Disable failpoint and restart backend.
7. Trigger/poll same public signing URL.
8. Verify it resumes into Documenso signing UI, not stuck processing.

Expected: after failpoint removal, same attempt resumes from the persisted phase and reaches `SIGNING_READY`.

- [ ] **Step 4: Complete visual signing**

In Browser Use:

1. Open embedded signing document.
2. Fill or accept signature field.
3. Submit/finalize Documenso signing.
4. Replay or wait for webhook if local Documenso sends it.
5. Poll iframe until completed.
6. Verify completed page and download button.

Expected: full visual lifecycle passes inside iframe host, matching earlier session validation.

- [ ] **Step 5: Capture screenshots and DB evidence**

Save screenshots under:

```text
docs/superpowers/evidence/2026-05-06-durable-provider-submission/
```

Record:

```sql
select id, sequence, status, provider_submit_phase, provider_document_id, provider_correlation_key, retry_count, reconciliation_count
from execution.signing_attempts
where document_id = '<document-id>'
order by sequence;

select event_type, old_status, new_status, provider_document_id, correlation_key, created_at
from execution.signing_attempt_events
where document_id = '<document-id>'
order by created_at;
```

- [ ] **Step 6: Run no-failpoint happy path UI validation**

Restart backend with failpoints disabled:

```bash
unset DOC_ENGINE_WORKER_FAILPOINTS
export DOC_ENGINE_WORKER_FAILPOINTS_ENABLED=false
make -C core run
```

Run the full iframe lifecycle again for a fresh document.

Expected: happy path still passes visually.

- [ ] **Step 7: Commit evidence if accepted by repo convention**

```bash
git add docs/superpowers/evidence/2026-05-06-durable-provider-submission/README.md docs/superpowers/evidence/2026-05-06-durable-provider-submission/*.png
git commit -m "test: validate durable provider submission visually"
```

---

## Task 10: Full Verification Suite

**Files:**
- No source changes unless verification exposes a bug.

- [ ] **Step 1: Run backend generation**

```bash
make -C core wire
make -C core swagger
```

Expected: PASS. If generated files change, inspect and commit them.

- [ ] **Step 2: Run backend build**

```bash
make -C core build
```

Expected: PASS.

- [ ] **Step 3: Run backend unit tests**

```bash
make -C core test
```

Expected: PASS.

- [ ] **Step 4: Run backend lint**

```bash
make -C core lint
```

Expected: PASS.

- [ ] **Step 5: Compile integration tests**

```bash
cd core && go build -tags=integration ./...
```

Expected: PASS.

- [ ] **Step 6: Run integration tests**

```bash
make -C core test-integration
```

Expected: PASS.

- [ ] **Step 7: Run frontend checks**

```bash
pnpm -C app lint
pnpm -C app build
```

Expected: PASS.

- [ ] **Step 8: Inspect final diff**

```bash
git diff --stat
git diff -- core/internal/core/entity/signing_attempt.go core/internal/core/port/signing_provider.go core/internal/infra/riverqueue core/internal/adapters/secondary/signing app/src
```

Expected: diff only contains durable provider submission, UX anti-loop, tests, migrations, and evidence/docs.

- [ ] **Step 9: Final commit**

```bash
git status --short
git add core app docs
git commit -m "feat: make signing provider submission durable"
```

Expected: commit succeeds with no AI attribution.

---

## Self-Review

### Spec coverage

- Reproduces local failure first: Task 7.
- Implements durable provider phases: Tasks 2, 4, 5.
- Handles partial Documenso envelopes: Tasks 1, 4, 5, 8.
- Prevents infinite UX processing: Task 6.
- Validates full visual iframe lifecycle: Task 9.
- Runs broad verification: Task 10.

### Placeholder scan

The plan intentionally avoids placeholder markers. Steps that depend on discovering actual frontend component filenames include the exact discovery command and require replacing only the discovered file paths in execution notes.

### Type consistency

The plan defines provider structs in Task 2 before using them in Tasks 3-5. River job naming uses `AdvanceProviderSubmissionArgs` and `advance_provider_submission` consistently.

---

## Execution Notes

- Prefer subagent-driven implementation, one task at a time.
- Do not write production code before Task 1 establishes/baselines the failure mode.
- Do not skip UI validation. The definition of done includes Browser Use visual evidence.
- Do not store session-only DB passwords in code, docs, commits, screenshots, or memory.
