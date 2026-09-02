package career

import (
	"bytes"
	"strings"
	"testing"
)

func TestMarkdownToPDFContainsText(t *testing.T) {
	pdf, err := MarkdownToPDF("Ada Lovelace", "# Ada Lovelace\n\nStaff engineer at Northwind.\n")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF")) {
		t.Fatal("not a PDF")
	}
	if !bytes.Contains(pdf, []byte("Ada Lovelace")) {
		t.Fatal("missing title text")
	}
	if !bytes.Contains(pdf, []byte("Staff engineer")) {
		t.Fatal("missing body text")
	}
}

func TestPDFFilename(t *testing.T) {
	got := PDFFilename("Ada Lovelace", "Acme, Inc", "Staff Engineer")
	if !strings.HasSuffix(got, ".pdf") || !strings.Contains(got, "Ada") || strings.Contains(got, " ") {
		t.Fatalf("got %q", got)
	}
}
