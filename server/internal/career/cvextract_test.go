package career

import (
	"os"
	"strings"
	"testing"
)

func TestExtractCVMarkdownPDFOnly(t *testing.T) {
	_, err := ExtractCVMarkdown("cv.md", "text/markdown", []byte("# Ada Lovelace\nStaff engineer"))
	if err == nil {
		t.Fatal("markdown must be rejected")
	}
	_, err = ExtractCVMarkdown("cv.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", []byte("PK"))
	if err == nil {
		t.Fatal("docx must be rejected")
	}
	_, err = ExtractCVMarkdown("photo.png", "image/png", []byte("not a cv"))
	if err == nil {
		t.Fatal("expected unsupported type")
	}
}

func TestExtractCVMarkdownPDF(t *testing.T) {
	pdf := []byte("%PDF-1.1\n<< /Length 40 >>\nstream\nBT\n(Ada Lovelace) Tj\n(Staff engineer) Tj\nET\nendstream\n")
	got, err := ExtractCVMarkdown("cv.pdf", "application/pdf", pdf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Ada Lovelace") {
		t.Fatalf("got %q", got)
	}
}

func TestExtractPDFContentStreamsReadsCompressedTJ(t *testing.T) {
	path := "/Users/sukhvirsingh/Desktop/Resume/Sukhvir_Singh_Resume_AI.pdf"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("local resume PDF not present")
	}
	got := extractPDFContentStreams(data)
	if !strings.Contains(got, "Sukhvir") || !strings.Contains(got, "AI Engineer") {
		t.Fatalf("stream extract missed resume text: %q", clip(got, 200))
	}
}
