package documenso

import (
	"encoding/json"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

type envelopeCreateEmailMeta struct {
	Meta *struct {
		Subject      string `json:"subject"`
		Message      string `json:"message"`
		Language     string `json:"language"`
		SigningOrder string `json:"signingOrder"`
	} `json:"meta"`
}

func decodeEnvelopeCreateEmailMeta(t *testing.T, payload string) envelopeCreateEmailMeta {
	t.Helper()
	var decoded envelopeCreateEmailMeta
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("decoding payload %s: %v", payload, err)
	}
	return decoded
}

func TestEnvelopeCreatePayloadCarriesEmailSubjectAndMessage(t *testing.T) {
	got := captureEnvelopeCreatePayloadWithEmail(t, &Config{}, "", port.ProviderEmail{
		Subject: "Anexo de Renovación 2026",
		Message: "Estimada Directora:\nLe enviamos el anexo para su firma.",
	})

	decoded := decodeEnvelopeCreateEmailMeta(t, got)
	if decoded.Meta == nil {
		t.Fatalf("meta missing from payload: %s", got)
	}
	if decoded.Meta.Subject != "Anexo de Renovación 2026" || decoded.Meta.Message != "Estimada Directora:\nLe enviamos el anexo para su firma." {
		t.Fatalf("meta.subject/meta.message wrong in payload: %s", got)
	}
	if decoded.Meta.Language != "" || decoded.Meta.SigningOrder != "" {
		t.Fatalf("an email must not switch on language or signing order: %s", got)
	}
}

func TestEnvelopeCreatePayloadSendsOnlyTheSetEmailField(t *testing.T) {
	got := captureEnvelopeCreatePayloadWithEmail(t, &Config{}, "", port.ProviderEmail{Message: "  Hola  "})

	want := `{"externalId":"doc-id:attempt-id","meta":{"message":"Hola"},"title":"Consent","type":"DOCUMENT"}`
	if got != want {
		t.Fatalf("payload = %s, want %s", got, want)
	}
}

func TestEnvelopeCreatePayloadCarriesEmailWithLanguageAndOrder(t *testing.T) {
	got := captureEnvelopeCreatePayloadWithEmail(t, &Config{SequentialSigningOrder: true, DocumentLanguage: true}, "es",
		port.ProviderEmail{Subject: "Contrato", Message: "Adjunto"})

	decoded := decodeEnvelopeCreateEmailMeta(t, got)
	if decoded.Meta == nil || decoded.Meta.Subject != "Contrato" || decoded.Meta.Message != "Adjunto" ||
		decoded.Meta.Language != "es" || decoded.Meta.SigningOrder != "SEQUENTIAL" {
		t.Fatalf("meta must carry subject, message, language and signingOrder: %s", got)
	}
}

// Documents without an email (every admissions document) send exactly the
// payload they sent before the field existed.
func TestEnvelopeCreatePayloadUnchangedWithoutEmail(t *testing.T) {
	for _, email := range []port.ProviderEmail{{}, {Subject: "  ", Message: "\n"}} {
		got := captureEnvelopeCreatePayloadWithEmail(t, &Config{}, "", email)
		want := `{"externalId":"doc-id:attempt-id","title":"Consent","type":"DOCUMENT"}`
		if got != want {
			t.Fatalf("payload = %s, want %s", got, want)
		}
	}
}
