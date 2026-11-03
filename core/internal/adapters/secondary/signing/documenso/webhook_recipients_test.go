package documenso

import (
	"context"
	"testing"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

func parseWebhook(t *testing.T, body string) *port.WebhookEvent {
	t.Helper()
	adapter, err := New(&Config{APIKey: "api_test", BaseURL: "https://sign.example.test"})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	event, err := adapter.ParseWebhook(context.Background(), &port.ParseWebhookRequest{Body: []byte(body)})
	if err != nil {
		t.Fatalf("ParseWebhook() error = %v", err)
	}
	return event
}

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.UTC()
}

// Signer 2's webhook lists signer 1 as SIGNED too. The event belongs to signer
// 2, and both signers' own times come through (tools-team-signing#35).
func TestParseWebhookCreditsTheLatestSignerAndKeepsEveryTime(t *testing.T) {
	event := parseWebhook(t, `{"event":"DOCUMENT_SIGNED","payload":{"id":7,"externalId":"doc:attempt","recipients":[
		{"id":11,"signingStatus":"SIGNED","signedAt":"2026-10-30T17:13:42.000Z"},
		{"id":12,"signingStatus":"SIGNED","signedAt":"2026-10-30T17:14:09.123Z"}]}}`)

	if event.ProviderRecipientID != "12" {
		t.Fatalf("ProviderRecipientID = %q, want the latest signer 12", event.ProviderRecipientID)
	}
	if len(event.Recipients) != 2 {
		t.Fatalf("Recipients = %+v, want 2", event.Recipients)
	}
	want := map[string]time.Time{
		"11": mustTime(t, "2026-10-30T17:13:42Z"),
		"12": mustTime(t, "2026-10-30T17:14:09.123Z"),
	}
	for _, r := range event.Recipients {
		if !r.Signed || r.SignedAt == nil || !r.SignedAt.Equal(want[r.ProviderRecipientID]) {
			t.Fatalf("recipient %s = signed %v at %v, want %v", r.ProviderRecipientID, r.Signed, r.SignedAt, want[r.ProviderRecipientID])
		}
	}
}

func TestParseWebhookFirstSignerOnly(t *testing.T) {
	event := parseWebhook(t, `{"event":"DOCUMENT_SIGNED","payload":{"id":7,"recipients":[
		{"id":11,"signingStatus":"SIGNED","signedAt":"2026-10-30T15:59:12Z"},
		{"id":12,"signingStatus":"NOT_SIGNED","signedAt":null}]}}`)

	if event.ProviderRecipientID != "11" {
		t.Fatalf("ProviderRecipientID = %q, want 11", event.ProviderRecipientID)
	}
	if r := event.Recipients[1]; r.Signed || r.SignedAt != nil {
		t.Fatalf("signer 2 = %+v, want not signed and no time", r)
	}
}

// Earlier behaviour is kept when the provider's time can't be read: the first
// signer listed gets the event.
func TestParseWebhookUnreadableTimeFallsBackToFirstSigner(t *testing.T) {
	event := parseWebhook(t, `{"event":"DOCUMENT_SIGNED","payload":{"id":7,"recipients":[
		{"id":11,"signingStatus":"SIGNED","signedAt":"yesterday"},
		{"id":12,"signingStatus":"SIGNED","signedAt":"also yesterday"}]}}`)

	if event.ProviderRecipientID != "11" {
		t.Fatalf("ProviderRecipientID = %q, want 11", event.ProviderRecipientID)
	}
	if event.Recipients[0].SignedAt != nil {
		t.Fatalf("an unreadable time must not be guessed: %+v", event.Recipients[0])
	}
}
