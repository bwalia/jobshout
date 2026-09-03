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

// Designer-exported CVs often use hex glyph strings + ToUnicode CMaps. The
// naive stream scrape returns empty; the pure-Go reader must still recover the
// name even when poppler is unavailable.
func TestExtractCVMarkdownCustomEncodedPDF(t *testing.T) {
	path := "/Users/balinderwalia/Downloads/Balinder_Walia_CV.pdf"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("local Balinder CV PDF not present")
	}

	if got := strings.TrimSpace(extractPDFContentStreams(data)); strings.Contains(got, "Balinder") {
		t.Fatalf("stream scrape unexpectedly read custom-encoded CV; test assumption broken: %q", clip(got, 120))
	}

	lib := strings.TrimSpace(extractViaPDFLib(data))
	if !strings.Contains(lib, "Balinder") || !strings.Contains(lib, "DevOps") {
		t.Fatalf("pdf lib missed CV text: %q", clip(lib, 200))
	}

	got, err := ExtractCVMarkdown("Balinder_Walia_CV.pdf", "application/pdf", data)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "Balinder") || !strings.Contains(got, "DevOps") {
		t.Fatalf("extract missed CV text: %q", clip(got, 200))
	}
}
