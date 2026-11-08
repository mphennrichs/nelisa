package officialtext

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

// minimalPDF monta um PDF de verdade, com uma página por texto (fonte
// Helvetica, só ASCII).
func minimalPDF(pages ...string) []byte {
	var b bytes.Buffer
	var offsets []int
	obj := func(body string) {
		offsets = append(offsets, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", len(offsets), body)
	}
	b.WriteString("%PDF-1.4\n")
	n := len(pages)
	kids := make([]string, n)
	for i := range pages {
		kids[i] = fmt.Sprintf("%d 0 R", 4+2*i)
	}
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), n))
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	for i, text := range pages {
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", 5+2*i))
		stream := fmt.Sprintf("BT /F1 12 Tf 72 720 Td (%s) Tj ET", text)
		obj(fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(offsets)+1)
	for _, o := range offsets {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets)+1, xref)
	return b.Bytes()
}

func TestPDFiumExtractsEachPage(t *testing.T) {
	if testing.Short() {
		t.Skip("sobe o runtime WebAssembly")
	}
	p := NewPDFium()
	defer p.Close()
	pages, err := p.Pages(context.Background(), minimalPDF("Art. 1 Esta Lei protege nascentes.", "Art. 2 Vigora ja."))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 || !strings.Contains(pages[0], "Art. 1 Esta Lei protege nascentes.") || !strings.Contains(pages[1], "Art. 2") {
		t.Fatalf("pages = %q", pages)
	}
	if _, err := p.Pages(context.Background(), []byte("%PDF-1.4 lixo")); err == nil {
		t.Fatal("PDF quebrado deveria falhar")
	}
}
