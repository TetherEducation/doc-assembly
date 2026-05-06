package documenso

import (
	"context"
	"encoding/json"
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
