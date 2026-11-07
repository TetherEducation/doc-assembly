package documenso

import (
	"io"
	"mime"
	"mime/multipart"
	"strings"
	"testing"
)

func TestEnvelopeFileName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Mutual NDA", "Mutual NDA"},
		{"Anexo de Renovación - Colegio Rech", "Anexo de Renovación - Colegio Rech"},
		{"Contrato v2.1", "Contrato v2.1"},
		{"Offer Letter.pdf", "Offer Letter"},
		{"Offer Letter.PDF ", "Offer Letter"},
		{"scan.pdf.pdf", "scan"},
		{`The "Big" Deal`, "The Big Deal"},
		{"A/B\\C", "A-B-C"},
		{"Line\nbreak\ttab", "Linebreaktab"},
		{"  padded  ", "padded"},
		{"", "document.pdf"},
		{"   ", "document.pdf"},
		{".pdf", "document.pdf"},
		{`""`, "document.pdf"},
		{strings.Repeat("é", 200), strings.Repeat("é", 150)},
	}
	for _, tc := range cases {
		if got := envelopeFileName(tc.in); got != tc.want {
			t.Errorf("envelopeFileName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// uploadedFileName returns the filename of the "files" part of an
// envelope-create body.
func uploadedFileName(t *testing.T, cfg *Config, title string) string {
	t.Helper()
	fileName := defaultEnvelopeFileName
	if cfg.TitleAsFileName {
		fileName = envelopeFileName(title)
	}
	buf, contentType, err := buildEnvelopeBody(title, fileName, "doc-id:attempt-id", []byte("%PDF-1.7"), false, "")
	if err != nil {
		t.Fatalf("buildEnvelopeBody() error = %v", err)
	}
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		t.Fatalf("parsing content type: %v", err)
	}
	reader := multipart.NewReader(buf, params["boundary"])
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			t.Fatal("no files part in envelope body")
		}
		if err != nil {
			t.Fatalf("reading part: %v", err)
		}
		if part.FormName() == "files" {
			return part.FileName()
		}
	}
}

func TestEnvelopeBodyFileNameDefaultsToDocumentPDF(t *testing.T) {
	if got := uploadedFileName(t, &Config{}, "Mutual NDA"); got != "document.pdf" {
		t.Fatalf("filename = %q, want document.pdf", got)
	}
}

func TestEnvelopeBodyFileNameFromTitle(t *testing.T) {
	cfg := &Config{TitleAsFileName: true}
	cases := map[string]string{
		"Mutual NDA":                 "Mutual NDA",
		"Anexo de Renovación - Rech": "Anexo de Renovación - Rech",
		`Say "hi"`:                   "Say hi",
		"":                           "document.pdf",
	}
	for title, want := range cases {
		if got := uploadedFileName(t, cfg, title); got != want {
			t.Errorf("title %q: filename = %q, want %q", title, got, want)
		}
	}
}

func TestEnvelopeCreatePayloadUnchangedByFileNameSetting(t *testing.T) {
	got := captureEnvelopeCreatePayload(t, &Config{TitleAsFileName: true}, "")

	// Only the file part changes; the JSON payload stays as before.
	want := `{"externalId":"doc-id:attempt-id","title":"Consent","type":"DOCUMENT"}`
	if got != want {
		t.Fatalf("payload = %s, want %s", got, want)
	}
}
