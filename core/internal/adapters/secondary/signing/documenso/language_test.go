package documenso

import (
	"encoding/json"
	"testing"
)

func TestDocumensoLanguage(t *testing.T) {
	cases := []struct{ in, want string }{
		{"es", "es"},
		{"en", "en"},
		{"ES", "es"},
		{" es ", "es"},
		{"es-CL", "es"},
		{"en_US", "en"},
		{"pt-BR", "pt-BR"},
		{"pt-br", "pt-BR"},
		{"pt", ""},
		{"xx", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := documensoLanguage(tc.in); got != tc.want {
			t.Errorf("documensoLanguage(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

type envelopeCreateMeta struct {
	Meta *struct {
		Language     string `json:"language"`
		SigningOrder string `json:"signingOrder"`
	} `json:"meta"`
}

func decodeEnvelopeCreateMeta(t *testing.T, payload string) envelopeCreateMeta {
	t.Helper()
	var decoded envelopeCreateMeta
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("decoding payload %s: %v", payload, err)
	}
	return decoded
}

func TestEnvelopeCreatePayloadIgnoresLanguageWhenSettingOff(t *testing.T) {
	got := captureEnvelopeCreatePayload(t, &Config{}, "es")

	// Byte-for-byte the payload sent before the setting existed.
	want := `{"externalId":"doc-id:attempt-id","title":"Consent","type":"DOCUMENT"}`
	if got != want {
		t.Fatalf("payload = %s, want %s", got, want)
	}
}

func TestEnvelopeCreatePayloadSetsDocumentLanguage(t *testing.T) {
	got := captureEnvelopeCreatePayload(t, &Config{DocumentLanguage: true}, "es")

	decoded := decodeEnvelopeCreateMeta(t, got)
	if decoded.Meta == nil || decoded.Meta.Language != "es" {
		t.Fatalf("meta.language missing or wrong in payload: %s", got)
	}
	if decoded.Meta.SigningOrder != "" {
		t.Fatalf("meta.signingOrder must stay unset when sequential order is off: %s", got)
	}
}

func TestEnvelopeCreatePayloadCarriesLanguageWithSequentialOrder(t *testing.T) {
	got := captureEnvelopeCreatePayload(t, &Config{SequentialSigningOrder: true, DocumentLanguage: true}, "en")

	decoded := decodeEnvelopeCreateMeta(t, got)
	if decoded.Meta == nil || decoded.Meta.Language != "en" || decoded.Meta.SigningOrder != "SEQUENTIAL" {
		t.Fatalf("meta must carry both language and signingOrder: %s", got)
	}
}

func TestEnvelopeCreatePayloadOmitsUnsupportedLanguage(t *testing.T) {
	got := captureEnvelopeCreatePayload(t, &Config{DocumentLanguage: true}, "xx")

	// Documenso would reject the envelope, so the payload stays as it was before.
	want := `{"externalId":"doc-id:attempt-id","title":"Consent","type":"DOCUMENT"}`
	if got != want {
		t.Fatalf("payload = %s, want %s", got, want)
	}
}
