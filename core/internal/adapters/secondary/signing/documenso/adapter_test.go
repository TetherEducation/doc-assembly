package documenso

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rendis/doc-assembly/core/internal/core/entity"
	"github.com/rendis/doc-assembly/core/internal/core/port"
)

func TestProviderCapabilitiesCanFindByCorrelationKey(t *testing.T) {
	adapter, err := New(&Config{APIKey: "api_test", BaseURL: "https://sign.example.test"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if !adapter.ProviderCapabilities().CanFindByCorrelationKey {
		t.Fatal("CanFindByCorrelationKey = false, want true")
	}
}

func TestNewUsesLongDocumensoHTTPTimeout(t *testing.T) {
	adapter, err := New(&Config{APIKey: "api_test", BaseURL: "https://sign.example.test"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if adapter.httpClient.Timeout != documensoHTTPTimeout {
		t.Fatalf("http timeout = %s, want %s", adapter.httpClient.Timeout, documensoHTTPTimeout)
	}
}

func TestFindProviderDocumentByCorrelationKeyFindsUsableEnvelopeByExternalID(t *testing.T) {
	const correlationKey = "doc-id:attempt-id"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "api_test" {
			t.Fatalf("Authorization header = %q, want api_test", got)
		}

		switch r.URL.Path {
		case "/api/v2/envelope":
			if got := r.URL.Query().Get("type"); got != "DOCUMENT" {
				t.Fatalf("type query = %q, want DOCUMENT", got)
			}
			writeJSON(t, w, map[string]any{
				"data": []map[string]any{
					{"id": "env_other", "externalId": "other", "status": "PENDING"},
					{"id": "env_match", "externalId": correlationKey, "status": "PENDING"},
				},
				"pagination": map[string]any{"page": 1, "perPage": 100, "totalPages": 1, "totalItems": 2},
			})
		case "/api/v2/envelope/env_match":
			writeJSON(t, w, map[string]any{
				"id":         "env_match",
				"externalId": correlationKey,
				"status":     "PENDING",
				"recipients": []map[string]any{
					{"id": 123, "externalId": "guardian", "token": "sign-token", "status": "SENT"},
				},
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

	if !got.Found || !got.Usable {
		t.Fatalf("Found/Usable = %v/%v, want true/true (reason: %s)", got.Found, got.Usable, got.Reason)
	}
	if got.ProviderDocumentID != "env_match" {
		t.Fatalf("ProviderDocumentID = %q, want env_match", got.ProviderDocumentID)
	}
	if got.Status != entity.SigningAttemptStatusSigningReady {
		t.Fatalf("Status = %q, want %q", got.Status, entity.SigningAttemptStatusSigningReady)
	}
	if len(got.Recipients) != 1 {
		t.Fatalf("Recipients len = %d, want 1", len(got.Recipients))
	}
	recipient := got.Recipients[0]
	if recipient.RoleID != "guardian" || recipient.ProviderRecipientID != "123" || recipient.SigningURL != server.URL+"/sign/sign-token" || recipient.Status != entity.RecipientStatusSent {
		t.Fatalf("recipient = %+v", recipient)
	}
}

func TestFindProviderDocumentByCorrelationKeyReturnsUnusableWhenRecipientRefsMissing(t *testing.T) {
	const correlationKey = "doc-id:attempt-id"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/envelope":
			writeJSON(t, w, map[string]any{
				"data":       []map[string]any{{"id": "env_match", "externalId": correlationKey, "status": "DRAFT"}},
				"pagination": map[string]any{"page": 1, "perPage": 100, "totalPages": 1, "totalItems": 1},
			})
		case "/api/v2/envelope/env_match":
			writeJSON(t, w, map[string]any{
				"id":         "env_match",
				"externalId": correlationKey,
				"status":     "DRAFT",
				"recipients": []map[string]any{{"id": 123, "status": "PENDING"}},
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
	})
	if err != nil {
		t.Fatalf("FindProviderDocumentByCorrelationKey() error = %v", err)
	}

	if !got.Found || got.Usable {
		t.Fatalf("Found/Usable = %v/%v, want true/false", got.Found, got.Usable)
	}
	if got.Reason == "" {
		t.Fatal("Reason is empty")
	}
}

func TestFindProviderDocumentByCorrelationKeyReturnsNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, map[string]any{
			"data":       []map[string]any{{"id": "env_other", "externalId": "other", "status": "PENDING"}},
			"pagination": map[string]any{"page": 1, "perPage": 100, "totalPages": 1, "totalItems": 1},
		})
	}))
	defer server.Close()

	adapter, err := New(&Config{APIKey: "api_test", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := adapter.FindProviderDocumentByCorrelationKey(context.Background(), &port.FindProviderDocumentRequest{
		ProviderName:   providerName,
		CorrelationKey: "doc-id:attempt-id",
	})
	if err != nil {
		t.Fatalf("FindProviderDocumentByCorrelationKey() error = %v", err)
	}

	if got.Found || got.Usable {
		t.Fatalf("Found/Usable = %v/%v, want false/false", got.Found, got.Usable)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, body any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("encoding response: %v", err)
	}
}

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

func TestInspectProviderSubmissionDetectsPartialDraftWithoutRecipients(t *testing.T) {
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

	got, err := adapter.InspectProviderSubmission(context.Background(), &port.FindProviderDocumentRequest{
		ProviderName:   providerName,
		CorrelationKey: correlationKey,
		Environment:    entity.EnvironmentProd,
	})
	if err != nil {
		t.Fatalf("InspectProviderSubmission() error = %v", err)
	}

	if !got.HasDocument {
		t.Fatal("HasDocument = false, want true")
	}
	if got.ProviderDocumentID != "env_partial" {
		t.Fatalf("ProviderDocumentID = %q, want env_partial", got.ProviderDocumentID)
	}
	if got.HasRecipients {
		t.Fatal("HasRecipients = true, want false")
	}
	if got.HasFields {
		t.Fatal("HasFields = true, want false")
	}
	if got.IsDistributed {
		t.Fatal("IsDistributed = true, want false")
	}
	if got.Reason == "" {
		t.Fatal("Reason is empty")
	}
}

func TestEnsureProviderDocumentReusesExistingEnvelopeByExternalID(t *testing.T) {
	const correlationKey = "doc-id:attempt-id"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/envelope":
			writeJSON(t, w, map[string]any{
				"data": []map[string]any{{
					"id":         "env_existing",
					"externalId": correlationKey,
					"status":     "DRAFT",
				}},
				"pagination": map[string]any{"page": 1, "perPage": 100, "totalPages": 1, "totalItems": 1},
			})
		case "/api/v2/envelope/env_existing":
			writeJSON(t, w, map[string]any{
				"id":         "env_existing",
				"externalId": correlationKey,
				"status":     "DRAFT",
				"recipients": []map[string]any{},
				"fields":     []map[string]any{},
			})
		case "/api/v2/envelope/create":
			t.Fatal("EnsureProviderDocument posted to create despite existing matching externalId")
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	adapter, err := New(&Config{APIKey: "api_test", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := adapter.EnsureProviderDocument(context.Background(), &port.EnsureProviderDocumentRequest{
		CorrelationKey: correlationKey,
		Title:          "Consent",
		PDF:            []byte("%PDF-1.7"),
		Environment:    entity.EnvironmentProd,
	})
	if err != nil {
		t.Fatalf("EnsureProviderDocument() error = %v", err)
	}

	if got.ProviderDocumentID != "env_existing" {
		t.Fatalf("ProviderDocumentID = %q, want env_existing", got.ProviderDocumentID)
	}
	if got.ProviderName != providerName {
		t.Fatalf("ProviderName = %q, want %q", got.ProviderName, providerName)
	}
	if got.CorrelationKey != correlationKey {
		t.Fatalf("CorrelationKey = %q, want %q", got.CorrelationKey, correlationKey)
	}
	if got.RawStatus != "DRAFT" {
		t.Fatalf("RawStatus = %q, want DRAFT", got.RawStatus)
	}
}

func TestFetchProviderSigningReferencesReturnsUsableRecipientRefs(t *testing.T) {
	const correlationKey = "doc-id:attempt-id"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/envelope/env_ready":
			writeJSON(t, w, map[string]any{
				"id":         "env_ready",
				"externalId": correlationKey,
				"status":     "PENDING",
				"recipients": []map[string]any{
					{"id": 123, "externalId": "guardian", "token": "guardian-token", "status": "SENT"},
					{"id": 456, "externalId": "student", "token": "student-token", "status": "SENT"},
				},
				"fields": []map[string]any{
					{"id": 1, "recipientId": 123, "type": "SIGNATURE"},
				},
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

	got, err := adapter.FetchProviderSigningReferences(context.Background(), &port.FetchProviderSigningReferencesRequest{
		ProviderDocumentID: "env_ready",
		CorrelationKey:     correlationKey,
		Environment:        entity.EnvironmentProd,
	})
	if err != nil {
		t.Fatalf("FetchProviderSigningReferences() error = %v", err)
	}

	if got.Status != entity.SigningAttemptStatusSigningReady {
		t.Fatalf("Status = %q, want %q", got.Status, entity.SigningAttemptStatusSigningReady)
	}
	if got.RawStatus != "PENDING" {
		t.Fatalf("RawStatus = %q, want PENDING", got.RawStatus)
	}
	if len(got.Recipients) != 2 {
		t.Fatalf("Recipients len = %d, want 2", len(got.Recipients))
	}
	if got.Recipients[0].RoleID != "guardian" ||
		got.Recipients[0].ProviderRecipientID != "123" ||
		got.Recipients[0].ProviderSigningToken != "guardian-token" ||
		got.Recipients[0].SigningURL != server.URL+"/sign/guardian-token" {
		t.Fatalf("first recipient = %+v", got.Recipients[0])
	}
}

func TestEnsureProviderRecipientsAddsRecipientsWhenEnvelopeIsEmpty(t *testing.T) {
	fetchCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/envelope/env_empty":
			fetchCount++
			recipients := []map[string]any{}
			if fetchCount > 1 {
				recipients = []map[string]any{{
					"id":         123,
					"email":      "guardian@example.test",
					"externalId": "guardian",
					"token":      "guardian-token",
					"status":     "PENDING",
				}}
			}
			writeJSON(t, w, map[string]any{
				"id":         "env_empty",
				"externalId": "doc-id:attempt-id",
				"status":     "DRAFT",
				"recipients": recipients,
				"fields":     []map[string]any{},
			})
		case "/api/v2/envelope/recipient/create-many":
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			writeJSON(t, w, map[string]any{
				"data": []map[string]any{{
					"id":    123,
					"email": "guardian@example.test",
					"token": "guardian-token",
				}},
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

	got, err := adapter.EnsureProviderRecipients(context.Background(), &port.EnsureProviderRecipientsRequest{
		ProviderDocumentID: "env_empty",
		CorrelationKey:     "doc-id:attempt-id",
		Recipients: []port.SigningRecipient{{
			Email:  "guardian@example.test",
			Name:   "Guardian",
			RoleID: "guardian",
		}},
		Environment: entity.EnvironmentProd,
	})
	if err != nil {
		t.Fatalf("EnsureProviderRecipients() error = %v", err)
	}

	if len(got.Recipients) != 1 {
		t.Fatalf("Recipients len = %d, want 1", len(got.Recipients))
	}
	if got.Recipients[0].RoleID != "guardian" || got.Recipients[0].ProviderRecipientID != "123" {
		t.Fatalf("recipient = %+v", got.Recipients[0])
	}
}

func TestEnsureProviderFieldsReturnsExistingFieldCountWithoutPost(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/envelope/env_fields":
			writeJSON(t, w, map[string]any{
				"id":         "env_fields",
				"externalId": "doc-id:attempt-id",
				"status":     "DRAFT",
				"recipients": []map[string]any{},
				"fields": []map[string]any{
					{"id": 1, "recipientId": 123, "type": "SIGNATURE"},
				},
			})
		case "/api/v2/envelope/field/create-many":
			t.Fatal("EnsureProviderFields posted to create-many despite existing fields")
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	adapter, err := New(&Config{APIKey: "api_test", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := adapter.EnsureProviderFields(context.Background(), &port.EnsureProviderFieldsRequest{
		ProviderDocumentID: "env_fields",
		CorrelationKey:     "doc-id:attempt-id",
		Recipients: []port.RecipientResult{{
			RoleID:              "guardian",
			ProviderRecipientID: "123",
		}},
		SignatureFields: []port.SignatureFieldPosition{{
			RoleID: "guardian",
			Page:   1,
		}},
		Environment: entity.EnvironmentProd,
	})
	if err != nil {
		t.Fatalf("EnsureProviderFields() error = %v", err)
	}

	if got.FieldCount != 1 {
		t.Fatalf("FieldCount = %d, want 1", got.FieldCount)
	}
}

func TestEnsureProviderDistributedSkipsAlreadyDistributedEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/envelope/env_sent":
			writeJSON(t, w, map[string]any{
				"id":         "env_sent",
				"externalId": "doc-id:attempt-id",
				"status":     "PENDING",
				"recipients": []map[string]any{},
				"fields":     []map[string]any{},
			})
		case "/api/v2/envelope/distribute":
			t.Fatal("EnsureProviderDistributed posted to distribute despite non-draft status")
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	adapter, err := New(&Config{APIKey: "api_test", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := adapter.EnsureProviderDistributed(context.Background(), &port.EnsureProviderDistributedRequest{
		ProviderDocumentID: "env_sent",
		CorrelationKey:     "doc-id:attempt-id",
		Environment:        entity.EnvironmentProd,
	})
	if err != nil {
		t.Fatalf("EnsureProviderDistributed() error = %v", err)
	}

	if got.RawStatus != "PENDING" {
		t.Fatalf("RawStatus = %q, want PENDING", got.RawStatus)
	}
}

func TestEnsureProviderDistributedRejectsCorrelationMismatchBeforeMutation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/envelope/env_stale":
			writeJSON(t, w, map[string]any{
				"id":         "env_stale",
				"externalId": "other-attempt",
				"status":     "DRAFT",
				"recipients": []map[string]any{},
				"fields":     []map[string]any{},
			})
		case "/api/v2/envelope/distribute":
			t.Fatal("EnsureProviderDistributed mutated a correlation-mismatched envelope")
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	adapter, err := New(&Config{APIKey: "api_test", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = adapter.EnsureProviderDistributed(context.Background(), &port.EnsureProviderDistributedRequest{
		ProviderDocumentID: "env_stale",
		CorrelationKey:     "expected-attempt",
		Environment:        entity.EnvironmentProd,
	})
	if err == nil {
		t.Fatal("EnsureProviderDistributed() error = nil, want correlation mismatch error")
	}

	assertProviderError(t, err, entity.ProviderSubmitPhaseDistributeDocument, entity.ProviderErrorClassConflictStale)
}

func TestFetchProviderSigningReferencesRejectsCorrelationMismatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/envelope/env_stale":
			writeJSON(t, w, map[string]any{
				"id":         "env_stale",
				"externalId": "other-attempt",
				"status":     "PENDING",
				"recipients": []map[string]any{
					{"id": 123, "externalId": "guardian", "token": "guardian-token", "status": "SENT"},
				},
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

	_, err = adapter.FetchProviderSigningReferences(context.Background(), &port.FetchProviderSigningReferencesRequest{
		ProviderDocumentID: "env_stale",
		CorrelationKey:     "expected-attempt",
		Environment:        entity.EnvironmentProd,
	})
	if err == nil {
		t.Fatal("FetchProviderSigningReferences() error = nil, want correlation mismatch error")
	}

	assertProviderError(t, err, entity.ProviderSubmitPhaseFetchSigningReferences, entity.ProviderErrorClassConflictStale)
}

func TestEnsureProviderRecipientsRejectsPartialExistingRecipients(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/envelope/env_partial":
			writeJSON(t, w, map[string]any{
				"id":         "env_partial",
				"externalId": "doc-id:attempt-id",
				"status":     "DRAFT",
				"recipients": []map[string]any{
					{"id": 123, "externalId": "guardian", "token": "guardian-token", "status": "PENDING"},
				},
				"fields": []map[string]any{},
			})
		case "/api/v2/envelope/recipient/create-many":
			t.Fatal("EnsureProviderRecipients attempted unsafe partial recipient repair")
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	adapter, err := New(&Config{APIKey: "api_test", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = adapter.EnsureProviderRecipients(context.Background(), &port.EnsureProviderRecipientsRequest{
		ProviderDocumentID: "env_partial",
		CorrelationKey:     "doc-id:attempt-id",
		Recipients: []port.SigningRecipient{
			{Email: "guardian@example.test", Name: "Guardian", RoleID: "guardian"},
			{Email: "student@example.test", Name: "Student", RoleID: "student"},
		},
		Environment: entity.EnvironmentProd,
	})
	if err == nil {
		t.Fatal("EnsureProviderRecipients() error = nil, want partial recipient error")
	}

	assertProviderError(t, err, entity.ProviderSubmitPhaseAddRecipients, entity.ProviderErrorClassPermanent)
}

func TestEnsureProviderFieldsIgnoresNonSignatureFieldsWhenCheckingCompleteness(t *testing.T) {
	createManyCalled := false

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/envelope/env_text_field":
			writeJSON(t, w, map[string]any{
				"id":         "env_text_field",
				"externalId": "doc-id:attempt-id",
				"status":     "DRAFT",
				"recipients": []map[string]any{},
				"fields": []map[string]any{
					{"id": 1, "recipientId": 123, "type": "TEXT"},
				},
			})
		case "/api/v2/envelope/field/create-many":
			createManyCalled = true
			if r.Method != http.MethodPost {
				t.Fatalf("method = %s, want POST", r.Method)
			}
			writeJSON(t, w, map[string]any{"data": []map[string]any{{"id": 2}}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	adapter, err := New(&Config{APIKey: "api_test", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := adapter.EnsureProviderFields(context.Background(), &port.EnsureProviderFieldsRequest{
		ProviderDocumentID: "env_text_field",
		CorrelationKey:     "doc-id:attempt-id",
		Recipients: []port.RecipientResult{{
			RoleID:              "guardian",
			ProviderRecipientID: "123",
		}},
		SignatureFields: []port.SignatureFieldPosition{{
			RoleID: "guardian",
			Page:   1,
		}},
		Environment: entity.EnvironmentProd,
	})
	if err != nil {
		t.Fatalf("EnsureProviderFields() error = %v", err)
	}
	if !createManyCalled {
		t.Fatal("EnsureProviderFields did not create required signature field")
	}
	if got.FieldCount != 1 {
		t.Fatalf("FieldCount = %d, want 1 required signature field", got.FieldCount)
	}
}

func TestEnsureProviderFieldsCreatesOnlyMissingRequiredSignatureFields(t *testing.T) {
	var createdFields []fieldPayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/envelope/env_partial_fields":
			writeJSON(t, w, map[string]any{
				"id":         "env_partial_fields",
				"externalId": "doc-id:attempt-id",
				"status":     "DRAFT",
				"recipients": []map[string]any{},
				"fields": []map[string]any{
					{"id": 1, "recipientId": 123, "type": "SIGNATURE"},
				},
			})
		case "/api/v2/envelope/field/create-many":
			var req fieldsRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatalf("decoding create-many request: %v", err)
			}
			createdFields = req.Data
			writeJSON(t, w, map[string]any{"data": []map[string]any{{"id": 2}}})
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	adapter, err := New(&Config{APIKey: "api_test", BaseURL: server.URL})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	got, err := adapter.EnsureProviderFields(context.Background(), &port.EnsureProviderFieldsRequest{
		ProviderDocumentID: "env_partial_fields",
		CorrelationKey:     "doc-id:attempt-id",
		Recipients: []port.RecipientResult{
			{RoleID: "guardian", ProviderRecipientID: "123"},
			{RoleID: "student", ProviderRecipientID: "456"},
		},
		SignatureFields: []port.SignatureFieldPosition{
			{RoleID: "guardian", Page: 1},
			{RoleID: "student", Page: 1},
		},
		Environment: entity.EnvironmentProd,
	})
	if err != nil {
		t.Fatalf("EnsureProviderFields() error = %v", err)
	}

	if got.FieldCount != 2 {
		t.Fatalf("FieldCount = %d, want 2 required signature fields", got.FieldCount)
	}
	if len(createdFields) != 1 {
		t.Fatalf("created fields len = %d, want 1", len(createdFields))
	}
	if createdFields[0].RecipientID != 456 || createdFields[0].Type != "SIGNATURE" {
		t.Fatalf("created field = %+v", createdFields[0])
	}
}

func assertProviderError(t *testing.T, err error, phase entity.ProviderSubmitPhase, class entity.ProviderErrorClass) {
	t.Helper()

	var providerErr *port.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("error type = %T, want *port.ProviderError", err)
	}
	if providerErr.Phase != phase {
		t.Fatalf("ProviderError.Phase = %q, want %q", providerErr.Phase, phase)
	}
	if providerErr.Class != class {
		t.Fatalf("ProviderError.Class = %q, want %q", providerErr.Class, class)
	}
}
