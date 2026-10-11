package documenso

import (
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// captureEnvelopeCreatePayload runs EnsureProviderDocument against a fake
// Documenso and returns the raw JSON "payload" field sent to /envelope/create.
func captureEnvelopeCreatePayload(t *testing.T, cfg *Config) string {
	t.Helper()
	const correlationKey = "doc-id:attempt-id"
	var payload string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v2/envelope":
			writeJSON(t, w, map[string]any{
				"data":       []map[string]any{},
				"pagination": map[string]any{"page": 1, "perPage": 100, "totalPages": 1, "totalItems": 0},
			})
		case "/api/v2/envelope/create":
			payload = readMultipartPayload(t, r)
			writeJSON(t, w, map[string]any{"id": "env_new"})
		case "/api/v2/envelope/env_new":
			writeJSON(t, w, map[string]any{
				"id":         "env_new",
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

	cfg.APIKey = "api_test"
	cfg.BaseURL = server.URL
	adapter, err := New(cfg)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if _, err := adapter.EnsureProviderDocument(context.Background(), &port.EnsureProviderDocumentRequest{
		CorrelationKey: correlationKey,
		Title:          "Consent",
		PDF:            []byte("%PDF-1.7"),
		Environment:    entity.EnvironmentProd,
	}); err != nil {
		t.Fatalf("EnsureProviderDocument() error = %v", err)
	}
	if payload == "" {
		t.Fatal("envelope/create was not called or payload field missing")
	}
	return payload
}

func readMultipartPayload(t *testing.T, r *http.Request) string {
	t.Helper()
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("parsing content type: %v", err)
	}
	mr := multipart.NewReader(r.Body, params["boundary"])
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			return ""
		}
		if err != nil {
			t.Fatalf("reading multipart: %v", err)
		}
		if part.FormName() == "payload" {
			b, err := io.ReadAll(part)
			if err != nil {
				t.Fatalf("reading payload part: %v", err)
			}
			return string(b)
		}
	}
}

func TestEnvelopeCreatePayloadUnchangedWhenSequentialOrderOff(t *testing.T) {
	got := captureEnvelopeCreatePayload(t, &Config{})

	// Byte-for-byte the payload sent before the setting existed.
	want := `{"externalId":"doc-id:attempt-id","title":"Consent","type":"DOCUMENT"}`
	if got != want {
		t.Fatalf("payload = %s, want %s", got, want)
	}
	if strings.Contains(got, "meta") {
		t.Fatalf("payload must not carry meta when sequential order is off: %s", got)
	}
}

func TestEnvelopeCreatePayloadSetsSequentialSigningOrderWhenOn(t *testing.T) {
	got := captureEnvelopeCreatePayload(t, &Config{SequentialSigningOrder: true})

	var decoded struct {
		Title      string `json:"title"`
		Type       string `json:"type"`
		ExternalID string `json:"externalId"`
		Meta       *struct {
			SigningOrder string `json:"signingOrder"`
		} `json:"meta"`
	}
	if err := json.Unmarshal([]byte(got), &decoded); err != nil {
		t.Fatalf("decoding payload %s: %v", got, err)
	}
	if decoded.Meta == nil || decoded.Meta.SigningOrder != "SEQUENTIAL" {
		t.Fatalf("meta.signingOrder missing or wrong in payload: %s", got)
	}
	if decoded.Title != "Consent" || decoded.Type != "DOCUMENT" || decoded.ExternalID != "doc-id:attempt-id" {
		t.Fatalf("base payload fields changed: %s", got)
	}
}
