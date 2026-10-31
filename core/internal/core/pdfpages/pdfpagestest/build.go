// Package pdfpagestest builds small, valid PDFs for tests.
package pdfpagestest

import (
	"bytes"
	"fmt"
)

// Build writes objects as "1 0 obj", "2 0 obj", ... with a correct xref
// table. Object 1 must be the catalog.
func Build(objects ...string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objects))
	for i, obj := range objects {
		offsets[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objects)+1)
	for _, off := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return b.Bytes()
}

// Pages builds a PDF with one page per size, each with a literal MediaBox.
func Pages(sizes ...[2]float64) []byte {
	kids := ""
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", ""}
	for i, s := range sizes {
		kids += fmt.Sprintf("%d 0 R ", i+3)
		objects = append(objects, fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %g %g] >>", s[0], s[1]))
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Count %d /Kids [%s] >>", len(sizes), kids)
	return Build(objects...)
}
