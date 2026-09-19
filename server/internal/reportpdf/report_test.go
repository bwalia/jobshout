package reportpdf

import (
	"bytes"
	"testing"
)

func TestRenderPDF(t *testing.T) {
	pdf, err := Render(Doc{
		Title:    "Test Report",
		Subtitle: "Unit test",
		Meta:     []Meta{{Label: "Version", Value: "v1 · app test"}},
		Sections: []Section{{Heading: "Findings", Lines: []string{"None"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(pdf, []byte("%PDF-1.4")) {
		t.Fatalf("not a pdf: %q", pdf[:20])
	}
	if !bytes.Contains(pdf, []byte("%%EOF")) {
		t.Fatal("missing EOF")
	}
}
