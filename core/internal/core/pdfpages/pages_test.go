package pdfpages

import (
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

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
		"<< /Type /Page /Parent 2 0 R /Rotate 90 >>",
		"<< /Type /Page /Parent 2 0 R /Rotate 180 >>")
	assertSizes(t, mustParse(t, pdf), Size{792, 612}, Size{612, 792}, Size{792, 612}, Size{612, 792})
}

// Documenso swaps width and height only for a literal 90 or 270 and does not
// normalise other values, so anything outside 0, 90, 180, 270 is refused.
func TestParseRejectsOtherRotations(t *testing.T) {
	for _, rotate := range []string{"45", "-90", "450", "360"} {
		t.Run(rotate, func(t *testing.T) {
			pdf := pdfpagestest.Build(catalog,
				"<< /Type /Pages /Count 1 /Kids [3 0 R] >>",
				"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Rotate "+rotate+" >>")
			if _, err := Parse(pdf); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// pdf-lib (Documenso) counts only /Type /Page leaves and inherits through
// /Parent, so a tree it would read differently is refused.
func TestParseRejectsTreesPdfLibReadsDifferently(t *testing.T) {
	for name, objects := range map[string][]string{
		"leaf without /Type": {catalog,
			"<< /Type /Pages /Count 2 /MediaBox [0 0 612 792] /Kids [3 0 R 4 0 R] >>",
			"<< /Type /Page /Parent 2 0 R >>",
			"<< /Parent 2 0 R >>"},
		"node with Kids but no /Type": {catalog,
			"<< /Type /Pages /Count 1 /MediaBox [0 0 612 792] /Kids [3 0 R] >>",
			"<< /Parent 2 0 R /Kids [4 0 R] >>",
			"<< /Type /Page /Parent 3 0 R >>"},
		"/Parent points elsewhere": {catalog,
			"<< /Type /Pages /Count 1 /MediaBox [0 0 612 792] /Kids [3 0 R] >>",
			"<< /Type /Page /Parent 4 0 R >>",
			"<< /Type /Pages /Count 0 /MediaBox [0 0 100 100] /Kids [] >>"},
		"no /Parent": {catalog,
			"<< /Type /Pages /Count 1 /MediaBox [0 0 612 792] /Kids [3 0 R] >>",
			"<< /Type /Page >>"},
		"direct kid": {catalog,
			"<< /Type /Pages /Count 1 /MediaBox [0 0 612 792] /Kids [<< /Type /Page /Parent 2 0 R >>] >>"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(pdfpagestest.Build(objects...)); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// Each object stream is decoded once and kept, so the number of distinct
// streams the walk may touch is capped.
func TestParseObjectStreamBudget(t *testing.T) {
	pages := mustParse(t, fixture(t, "objstm-64-streams.pdf"))
	if len(pages) != maxObjectStreams {
		t.Fatalf("got %d pages", len(pages))
	}
	if _, err := Parse(fixture(t, "objstm-65-streams.pdf")); !errors.Is(err, errLimit) {
		t.Fatalf("err = %v, want %v", err, errLimit)
	}
}

// pdf-lib reads a real /Rotate, so "90.0" must rotate, not count as 0.
func TestParseRealRotation(t *testing.T) {
	pdf := pdfpagestest.Build(catalog,
		"<< /Type /Pages /Count 1 /Kids [3 0 R] >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Rotate 90.0 >>")
	assertSizes(t, mustParse(t, pdf), Size{792, 612})
	odd := pdfpagestest.Build(catalog,
		"<< /Type /Pages /Count 1 /Kids [3 0 R] >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Rotate 90.5 >>")
	if _, err := Parse(odd); err == nil {
		t.Fatal("expected error for /Rotate 90.5")
	}
}

func TestParseFixtures(t *testing.T) {
	letter, a4, legal := Size{612, 792}, Size{595, 842}, Size{612, 1008}
	for name, want := range map[string][]Size{
		// Object streams and an xref stream, as DocuSign and LaTeX write them.
		"objstm.pdf": {letter, a4, legal},
		// AES-256 with an empty user password opens like any other file.
		"permissions-only.pdf": {letter, letter},
	} {
		t.Run(name, func(t *testing.T) {
			assertSizes(t, mustParse(t, fixture(t, name)), want...)
		})
	}
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	pdf, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return pdf
}

func TestParseRejectsUserPassword(t *testing.T) {
	for name, want := range map[string]error{
		"user-password.pdf":        errPassword, // RC4-128
		"aes256-user-password.pdf": errPassword,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(fixture(t, name)); !errors.Is(err, want) {
				t.Fatalf("err = %v, want %v", err, want)
			}
		})
	}
}

func TestParsePDF20Header(t *testing.T) {
	pdf := pdfpagestest.Pages([2]float64{612, 792})
	copy(pdf, "%PDF-2.0")
	assertSizes(t, mustParse(t, pdf), Size{612, 792})
}

// /Count is untrusted: it is ignored, so a huge value allocates nothing and a
// low value does not hide pages.
func TestParseIgnoresCount(t *testing.T) {
	for name, count := range map[string]string{"huge": "4000000000", "low": "1", "negative": "-5"} {
		t.Run(name, func(t *testing.T) {
			pdf := pdfpagestest.Build(catalog,
				"<< /Type /Pages /Count "+count+" /MediaBox [0 0 612 792] /Kids [3 0 R 4 0 R 5 0 R] >>",
				"<< /Type /Page /Parent 2 0 R >>",
				"<< /Type /Page /Parent 2 0 R >>",
				"<< /Type /Page /Parent 2 0 R >>")
			assertSizes(t, mustParse(t, pdf), Size{612, 792}, Size{612, 792}, Size{612, 792})
		})
	}
}

func TestParseRejectsHostileTrees(t *testing.T) {
	deep := []string{catalog}
	for i := 2; i <= maxTreeDepth+3; i++ {
		deep = append(deep, fmt.Sprintf("<< /Type /Pages /Kids [%d 0 R] /MediaBox [0 0 612 792] >>", i+1))
	}
	deep = append(deep, "<< /Type /Page >>")
	for name, pdf := range map[string][]byte{
		"self-cycle": pdfpagestest.Build(catalog, "<< /Type /Pages /Count 5 /Kids [2 0 R] >>"),
		"two-node cycle": pdfpagestest.Build(catalog,
			"<< /Type /Pages /Kids [3 0 R] >>",
			"<< /Type /Pages /Kids [2 0 R 2 0 R] >>"),
		"too deep": pdfpagestest.Build(deep...),
	} {
		t.Run(name, func(t *testing.T) {
			start := time.Now()
			if _, err := Parse(pdf); err == nil {
				t.Fatal("expected error")
			}
			if d := time.Since(start); d > 2*time.Second {
				t.Fatalf("took %v", d)
			}
		})
	}
}

func TestParsePageLimit(t *testing.T) {
	sizes := make([][2]float64, MaxPages)
	for i := range sizes {
		sizes[i] = [2]float64{612, 792}
	}
	start := time.Now()
	pages := mustParse(t, pdfpagestest.Pages(sizes...))
	if len(pages) != MaxPages {
		t.Fatalf("got %d pages", len(pages))
	}
	// One pass over the tree: a flat tree at the limit stays fast.
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("%d pages took %v", MaxPages, d)
	}
	if _, err := Parse(pdfpagestest.Pages(append(sizes, [2]float64{612, 792})...)); err == nil {
		t.Fatal("expected error above the page limit")
	}
}

func TestParseRejectsNonPDF(t *testing.T) {
	for _, data := range [][]byte{[]byte("hello"), []byte("%PDF-1.7 truncated")} {
		if _, err := Parse(data); err == nil {
			t.Fatalf("expected error for %q", data)
		}
	}
}
