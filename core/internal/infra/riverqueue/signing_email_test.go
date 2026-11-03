package riverqueue

import (
	"encoding/json"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

func TestAddSigningEmailSnapshotsTheDocumentsEmail(t *testing.T) {
	doc := &entity.Document{}
	if err := doc.SetSigningEmail(entity.SigningEmail{Subject: "Anexo", Message: "Para su firma"}); err != nil {
		t.Fatalf("SetSigningEmail() error = %v", err)
	}
	payload := map[string]any{"title": "Anexo"}
	addSigningEmail(payload, doc)

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	// The executor reads these back with payloadString when it creates the envelope.
	if got := payloadString(raw, "emailSubject"); got != "Anexo" {
		t.Fatalf("emailSubject = %q, want %q", got, "Anexo")
	}
	if got := payloadString(raw, "emailMessage"); got != "Para su firma" {
		t.Fatalf("emailMessage = %q, want %q", got, "Para su firma")
	}
}

func TestAddSigningEmailAddsNothingWithoutAnEmail(t *testing.T) {
	for _, doc := range []*entity.Document{{}, {Metadata: []byte(`{"source":"crm"}`)}} {
		payload := map[string]any{"title": "Consent"}
		addSigningEmail(payload, doc)
		if len(payload) != 1 {
			t.Fatalf("payload = %v, want only the title", payload)
		}
	}
}
