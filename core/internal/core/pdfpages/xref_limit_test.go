package pdfpages

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"runtime"
	"testing"
)

// xrefStreamPDF builds a one-page PDF whose only cross-reference section is an
// xref stream (/W [1 4 2]) followed by `padding` zero bytes inside the same
// stream. Zeros compress about 1000:1, so a few dozen KiB on disk inflate to
// tens of MiB: the crafted upload from tools-team-signing#38.
func xrefStreamPDF(t *testing.T, padding int) []byte {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	offsets := map[int]int{}
	object := func(n int, body string) {
		offsets[n] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", n, body)
	}
	object(1, "<< /Type /Catalog /Pages 2 0 R >>")
	object(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	object(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>")
	xrefOffset := b.Len()

	var entries bytes.Buffer
	entry := func(kind byte, offset uint32, gen uint16) {
		entries.WriteByte(kind)
		_ = binary.Write(&entries, binary.BigEndian, offset)
		_ = binary.Write(&entries, binary.BigEndian, gen)
	}
	entry(0, 0, 65535)
	for n := 1; n <= 3; n++ {
		entry(1, uint32(offsets[n]), 0)
	}
	entry(1, uint32(xrefOffset), 0)
	entries.Write(make([]byte, padding))

	var compressed bytes.Buffer
	zw, err := zlib.NewWriterLevel(&compressed, zlib.BestCompression)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := zw.Write(entries.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	fmt.Fprintf(&b, "4 0 obj\n<< /Type /XRef /Size 5 /W [1 4 2] /Root 1 0 R /Filter /FlateDecode /Length %d >>\nstream\n", compressed.Len())
	b.Write(compressed.Bytes())
	b.WriteString("\nendstream\nendobj\n")
	fmt.Fprintf(&b, "startxref\n%d\n%%%%EOF\n", xrefOffset)
	return b.Bytes()
}

// The control: the same structure without padding is a valid one-page PDF, so
// the next test fails for its padding and not for a broken file.
func TestParseReadsAnXRefStreamPDF(t *testing.T) {
	sizes, err := Parse(xrefStreamPDF(t, 0))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(sizes) != 1 || sizes[0] != (Size{Width: 612, Height: 792}) {
		t.Fatalf("sizes = %+v, want one Letter page", sizes)
	}
}

// tools-team-signing#38: pdfcpu v0.15 decoded xref streams under its 512 MiB
// default instead of our 1 MiB limit, so this file allocated its full 64 MiB
// (and a 0.5 MB upload about 1 GiB). Since v0.16.4 the decode stops at the
// limit and the upload is refused.
func TestParseRefusesAnXRefStreamThatInflatesPastTheDecodeLimit(t *testing.T) {
	const padding = 64 << 20
	data := xrefStreamPDF(t, padding)
	if len(data) > 1<<20 {
		t.Fatalf("crafted file is %d bytes; it should be small on disk", len(data))
	}

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := Parse(data)
	runtime.ReadMemStats(&after)

	if err == nil {
		t.Fatal("Parse() accepted an xref stream that inflates past the decode limit")
	}
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 16<<20 {
		t.Fatalf("Parse() allocated %d MiB reading a %d KiB file; the decode limit is 1 MiB", allocated>>20, len(data)>>10)
	}
}
