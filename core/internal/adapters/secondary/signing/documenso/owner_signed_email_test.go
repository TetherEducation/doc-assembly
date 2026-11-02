package documenso

import (
	"encoding/json"
	"testing"
)

type envelopeCreateEmailSettings struct {
	Title      string `json:"title"`
	Type       string `json:"type"`
	ExternalID string `json:"externalId"`
	Meta       *struct {
		Language      string          `json:"language"`
		SigningOrder  string          `json:"signingOrder"`
		EmailSettings map[string]bool `json:"emailSettings"`
	} `json:"meta"`
}

func decodeEnvelopeCreateEmailSettings(t *testing.T, payload string) envelopeCreateEmailSettings {
	t.Helper()
	var decoded envelopeCreateEmailSettings
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("decoding payload %s: %v", payload, err)
	}
	return decoded
}

func TestEnvelopeCreatePayloadUnchangedWhenOwnerSignedEmailSettingOff(t *testing.T) {
	got := captureEnvelopeCreatePayload(t, &Config{}, "")

	// Byte-for-byte the payload sent before the setting existed.
	want := `{"externalId":"doc-id:attempt-id","title":"Consent","type":"DOCUMENT"}`
	if got != want {
		t.Fatalf("payload = %s, want %s", got, want)
	}
}

func TestEnvelopeCreatePayloadTurnsOffOwnerSignedEmail(t *testing.T) {
	got := captureEnvelopeCreatePayload(t, &Config{SuppressOwnerSignedEmail: true}, "")

	decoded := decodeEnvelopeCreateEmailSettings(t, got)
	if decoded.Meta == nil {
		t.Fatalf("meta missing in payload: %s", got)
	}
	// Only recipientSigned is sent; Documenso defaults every other email to on.
	if len(decoded.Meta.EmailSettings) != 1 {
		t.Fatalf("meta.emailSettings = %v, want only recipientSigned: %s", decoded.Meta.EmailSettings, got)
	}
	if v, ok := decoded.Meta.EmailSettings["recipientSigned"]; !ok || v {
		t.Fatalf("meta.emailSettings.recipientSigned missing or true in payload: %s", got)
	}
	if decoded.Meta.SigningOrder != "" || decoded.Meta.Language != "" {
		t.Fatalf("unrelated meta keys set: %s", got)
	}
	if decoded.Title != "Consent" || decoded.Type != "DOCUMENT" || decoded.ExternalID != "doc-id:attempt-id" {
		t.Fatalf("base payload fields changed: %s", got)
	}
}

func TestEnvelopeCreatePayloadCombinesOwnerSignedEmailWithOtherMeta(t *testing.T) {
	got := captureEnvelopeCreatePayload(t, &Config{
		SequentialSigningOrder:   true,
		DocumentLanguage:         true,
		SuppressOwnerSignedEmail: true,
	}, "es")

	decoded := decodeEnvelopeCreateEmailSettings(t, got)
	if decoded.Meta == nil {
		t.Fatalf("meta missing in payload: %s", got)
	}
	if decoded.Meta.SigningOrder != "SEQUENTIAL" || decoded.Meta.Language != "es" {
		t.Fatalf("signingOrder/language lost alongside emailSettings: %s", got)
	}
	if v, ok := decoded.Meta.EmailSettings["recipientSigned"]; !ok || v {
		t.Fatalf("meta.emailSettings.recipientSigned missing or true in payload: %s", got)
	}
}
