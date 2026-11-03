package entity

import (
	"errors"
	"strings"
	"testing"
)

func TestSetSigningEmailStoresTrimmedValues(t *testing.T) {
	doc := &Document{}
	if err := doc.SetSigningEmail(SigningEmail{Subject: "  Anexo de Renovación ", Message: "\nEstimada Directora:\n"}); err != nil {
		t.Fatalf("SetSigningEmail() error = %v", err)
	}
	got := doc.SigningEmail()
	want := SigningEmail{Subject: "Anexo de Renovación", Message: "Estimada Directora:"}
	if got != want {
		t.Fatalf("SigningEmail() = %+v, want %+v", got, want)
	}
}

func TestSetSigningEmailKeepsOtherMetadata(t *testing.T) {
	doc := &Document{Metadata: []byte(`{"source":"crm","count":3}`)}
	if err := doc.SetSigningEmail(SigningEmail{Message: "Hola"}); err != nil {
		t.Fatalf("SetSigningEmail() error = %v", err)
	}
	for _, key := range []string{`"source":"crm"`, `"count":3`, `"signingEmail":{"message":"Hola"}`} {
		if !strings.Contains(string(doc.Metadata), key) {
			t.Fatalf("metadata %s lacks %s", doc.Metadata, key)
		}
	}
}

// An empty email leaves the document untouched, so callers that never send
// one (admissions) keep exactly the metadata they had.
func TestSetSigningEmailEmptyIsNoOp(t *testing.T) {
	for _, metadata := range [][]byte{nil, []byte(`{"source":"crm"}`)} {
		doc := &Document{Metadata: metadata}
		if err := doc.SetSigningEmail(SigningEmail{Subject: "  ", Message: ""}); err != nil {
			t.Fatalf("SetSigningEmail() error = %v", err)
		}
		if string(doc.Metadata) != string(metadata) || doc.UpdatedAt != nil {
			t.Fatalf("metadata = %s (updated %v), want %s unchanged", doc.Metadata, doc.UpdatedAt, metadata)
		}
	}
}

func TestSetSigningEmailRejectsTooLong(t *testing.T) {
	cases := []SigningEmail{
		{Subject: strings.Repeat("á", MaxSigningEmailSubjectLength+1)},
		{Message: strings.Repeat("x", MaxSigningEmailMessageLength+1)},
	}
	for _, email := range cases {
		doc := &Document{}
		if err := doc.SetSigningEmail(email); !errors.Is(err, ErrFieldTooLong) {
			t.Fatalf("SetSigningEmail() error = %v, want ErrFieldTooLong", err)
		}
		if doc.Metadata != nil {
			t.Fatalf("metadata written despite the error: %s", doc.Metadata)
		}
	}
	// Exactly at the limit is fine: an accented letter counts 1, as in JavaScript.
	doc := &Document{}
	if err := doc.SetSigningEmail(SigningEmail{Subject: strings.Repeat("á", MaxSigningEmailSubjectLength)}); err != nil {
		t.Fatalf("SetSigningEmail() at the limit error = %v", err)
	}
	// An emoji counts 2, so half the limit in emoji fits and one more doesn't.
	if err := (&Document{}).SetSigningEmail(SigningEmail{Subject: strings.Repeat("😀", MaxSigningEmailSubjectLength/2)}); err != nil {
		t.Fatalf("SetSigningEmail() with %d emoji error = %v", MaxSigningEmailSubjectLength/2, err)
	}
	if err := (&Document{}).SetSigningEmail(SigningEmail{Subject: strings.Repeat("😀", MaxSigningEmailSubjectLength/2+1)}); !errors.Is(err, ErrFieldTooLong) {
		t.Fatalf("SetSigningEmail() with %d emoji error = %v, want ErrFieldTooLong", MaxSigningEmailSubjectLength/2+1, err)
	}
}

func TestSigningEmailUnreadableMetadata(t *testing.T) {
	for _, metadata := range [][]byte{nil, []byte(`not json`), []byte(`{"signingEmail":"text"}`), []byte(`{"other":1}`)} {
		doc := &Document{Metadata: metadata}
		if got := doc.SigningEmail(); got != (SigningEmail{}) {
			t.Fatalf("SigningEmail() with metadata %s = %+v, want zero", metadata, got)
		}
	}
}
