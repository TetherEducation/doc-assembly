package pdfpages

import "testing"

func TestParseLiteralMediaBox(t *testing.T) {
	pdf := []byte("%PDF-1.4\n1 0 obj << /Type /Catalog /Pages 2 0 R >> endobj\n" +
		"2 0 obj << /Type /Pages /Count 1 /Kids [3 0 R] >> endobj\n" +
		"3 0 obj << /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >> endobj\n%%EOF")
	pages, err := Parse(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 1 || pages[0].Width != 612 || pages[0].Height != 792 {
		t.Fatalf("pages = %+v", pages)
	}
}

func TestParseRejectsNonPDF(t *testing.T) {
	if _, err := Parse([]byte("hello")); err == nil {
		t.Fatal("expected error")
	}
}
