package pdfpages

import (
	"errors"
	"os"
	"testing"

	"github.com/TetherEducation/doc-assembly/core/internal/core/pdfpages/pdfpagestest"
)

const catalog = "<< /Type /Catalog /Pages 2 0 R >>"

func mustParse(t *testing.T, pdf []byte) []Size {
	t.Helper()
	pages, err := Parse(pdf)
	if err != nil {
		t.Fatal(err)
	}
	return pages
}

func assertSizes(t *testing.T, got []Size, want ...Size) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d pages %+v, want %d %+v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("page %d = %+v, want %+v (all: %+v)", i+1, got[i], want[i], got)
		}
	}
}

func TestParseLiteralMediaBox(t *testing.T) {
	assertSizes(t, mustParse(t, pdfpagestest.Pages([2]float64{612, 792})), Size{612, 792})
}

func TestParseInheritedMediaBox(t *testing.T) {
	pdf := pdfpagestest.Build(catalog,
		"<< /Type /Pages /Count 3 /MediaBox [0 0 595 842] /Kids [3 0 R 4 0 R 5 0 R] >>",
		"<< /Type /Page /Parent 2 0 R >>",
		"<< /Type /Page /Parent 2 0 R >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 1008] >>")
	assertSizes(t, mustParse(t, pdf), Size{595, 842}, Size{595, 842}, Size{612, 1008})
}

// Pages come in page-tree order, not file order, and a page object that is
// not in the tree (left behind by an editor or an incremental save) is not
// counted. The old byte scan got both wrong.
func TestParsePageOrderAndOrphans(t *testing.T) {
	pdf := pdfpagestest.Build(catalog,
		"<< /Type /Pages /Count 2 /Kids [5 0 R 3 0 R] >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
		"<< /Type /Page /MediaBox [0 0 100 100] >>", // orphan
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] >>")
	assertSizes(t, mustParse(t, pdf), Size{595, 842}, Size{612, 792})
}

func TestParseCropBox(t *testing.T) {
	pdf := pdfpagestest.Build(catalog,
		"<< /Type /Pages /Count 3 /Kids [3 0 R 4 0 R 5 0 R] >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /CropBox [0 0 523 720] >>",
		// A CropBox larger than the MediaBox: Documenso uses the MediaBox.
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /CropBox [0 0 700 900] >>",
		// Corners may come in any order.
		"<< /Type /Page /Parent 2 0 R /MediaBox [612 792 0 0] >>")
	assertSizes(t, mustParse(t, pdf), Size{523, 720}, Size{612, 792}, Size{612, 792})
}

// Documenso stamps from the user-space origin, so an offset visible area
// would shift every field. Such pages are refused, not misplaced.
func TestParseRejectsOffsetVisibleArea(t *testing.T) {
	for name, page := range map[string]string{
		"offset CropBox":  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /CropBox [36 36 559 756] >>",
		"offset MediaBox": "<< /Type /Page /Parent 2 0 R /MediaBox [18 18 630 810] >>",
	} {
		t.Run(name, func(t *testing.T) {
			pdf := pdfpagestest.Build(catalog, "<< /Type /Pages /Count 1 /Kids [3 0 R] >>", page)
			if _, err := Parse(pdf); !errors.Is(err, ErrOffsetPage) {
				t.Fatalf("err = %v, want %v", err, ErrOffsetPage)
			}
		})
	}
}

func TestParseRotation(t *testing.T) {
	pdf := pdfpagestest.Build(catalog,
		"<< /Type /Pages /Count 4 /Rotate 270 /MediaBox [0 0 612 792] /Kids [3 0 R 4 0 R 5 0 R 6 0 R] >>",
		"<< /Type /Page /Parent 2 0 R >>",
		"<< /Type /Page /Parent 2 0 R /Rotate 0 >>",
		"<< /Type /Page /Parent 2 0 R /Rotate -90 >>",
		"<< /Type /Page /Parent 2 0 R /Rotate 180 >>")
	assertSizes(t, mustParse(t, pdf), Size{792, 612}, Size{612, 792}, Size{792, 612}, Size{612, 792})
}

func TestParseRejectsOddRotation(t *testing.T) {
	pdf := pdfpagestest.Build(catalog,
		"<< /Type /Pages /Count 1 /Kids [3 0 R] >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Rotate 45 >>")
	if _, err := Parse(pdf); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseFixtures(t *testing.T) {
	letter, a4, legal := Size{612, 792}, Size{595, 842}, Size{612, 1008}
	for name, want := range map[string][]Size{
		// Object streams and an xref stream, as DocuSign and LaTeX write them.
		"objstm.pdf": {letter, a4, legal},
		// AES-256 with an empty user password: read by the literal fallback.
		"permissions-only.pdf": {letter, letter},
	} {
		t.Run(name, func(t *testing.T) {
			pdf, err := os.ReadFile("testdata/" + name)
			if err != nil {
				t.Fatal(err)
			}
			assertSizes(t, mustParse(t, pdf), want...)
		})
	}
}

func TestParseRejectsUserPassword(t *testing.T) {
	pdf, err := os.ReadFile("testdata/user-password.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(pdf); !errors.Is(err, errPassword) {
		t.Fatalf("err = %v, want %v", err, errPassword)
	}
}

func TestParseRejectsNonPDF(t *testing.T) {
	for _, data := range [][]byte{[]byte("hello"), []byte("%PDF-1.7 truncated")} {
		if _, err := Parse(data); err == nil {
			t.Fatalf("expected error for %q", data)
		}
	}
}
