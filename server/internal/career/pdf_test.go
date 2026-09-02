package career

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/model"
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

func TestMarkdownToPDFOnePageResume(t *testing.T) {
	var b strings.Builder
	b.WriteString("                              Jane Doe\n")
	b.WriteString("                         jane@example.com | GitHub\n")
	b.WriteString("Summary\n")
	b.WriteString("Staff engineer building LLM systems, RAG pipelines, and agents with Python and Go.\n")
	b.WriteString("Education\n")
	b.WriteString("Some Institute | B.Tech in Computer Science                                          2023 – 2027\n")
	b.WriteString("Internship Experience\n")
	b.WriteString("AI Engineer Intern, Example Co                                                        Jan 2026 – July 2026\n")
	b.WriteString("Python, LangChain, LangGraph, PostgreSQL, Docker\n")
	for i := 0; i < 4; i++ {
		b.WriteString("• Built production RAG and agent workflows with evaluation, tracing, and fallbacks.\n")
	}
	b.WriteString("Projects\n")
	b.WriteString("Orbit AI - Autonomous Agent                                                                                 GitHub\n")
	b.WriteString("• Built a multi-step agent with RAG, tool execution, and regression tests.\n")
	b.WriteString("Skills and Competencies\n")
	b.WriteString("AI / LLM: GPT-4, Claude, Gemini, RAG, Prompt Engineering\n")
	b.WriteString("Achievements\n")
	b.WriteString("Winner - Hackathon (200+ participants) | Winner - Another (300+ participants)\n")
	b.WriteString(visibleTailorNote("AI Engineer", "Northwind Labs", "Summary already led with RAG and agents."))
	pdf, err := MarkdownToPDF("Jane Doe", b.String())
	if err != nil {
		t.Fatal(err)
	}
	if n := pdfPageCount(pdf); n != 1 {
		t.Fatalf("pages = %d, want 1", n)
	}
	if bytes.Contains(pdf, []byte("<!--")) {
		t.Fatal("HTML comments must not appear in the PDF")
	}
	if !bytes.Contains(pdf, []byte("Jane Doe")) {
		t.Fatal("missing name")
	}
	if bytes.Contains(pdf, []byte("Tailored for")) {
		t.Fatal("tailor note belongs on the web, not the PDF")
	}
}

func TestMarkdownToPDFWinAnsiDash(t *testing.T) {
	pdf, err := MarkdownToPDF("Ada", "Education\nCambridge 2020 – 2024\n")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(pdf, []byte("2020 ? 2024")) || bytes.Contains(pdf, []byte("2020?2024")) {
		t.Fatal("en-dash must not become a question mark")
	}
}

func TestPDFFilename(t *testing.T) {
	got := PDFFilename("Ada Lovelace", "Acme, Inc", "Staff Engineer")
	if !strings.HasSuffix(got, ".pdf") || !strings.Contains(got, "Ada") || strings.Contains(got, " ") {
		t.Fatalf("got %q", got)
	}
}

func TestRealResumePDFStaysOnePage(t *testing.T) {
	path := "/Users/sukhvirsingh/Desktop/Resume/Sukhvir_Singh_Resume_AI.pdf"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("local resume PDF not present")
	}
	text, err := ExtractCVMarkdown("cv.pdf", "application/pdf", data)
	if err != nil {
		t.Fatal(err)
	}
	body := text + visibleTailorNote("AI Engineer", "Northwind Labs", "Summary and internships already lead with RAG, agents, and evaluation.")
	pdf, err := MarkdownToPDF("Sukhvir Singh", body)
	if err != nil {
		t.Fatal(err)
	}
	if n := pdfPageCount(pdf); n != 1 {
		t.Fatalf("pages = %d, want 1 (source was 1 page)", n)
	}
	if !bytes.Contains(pdf, []byte("Sukhvir")) || !bytes.Contains(pdf, []byte("Summary")) {
		t.Fatal("missing resume text")
	}
	if bytes.Contains(pdf, []byte("<!--")) {
		t.Fatal("HTML comments must not appear")
	}
	_ = os.WriteFile("/tmp/sukhvir-tailored-cv.pdf", pdf, 0o644)
}

func TestRealResumeTailorReplacementsStayOnePage(t *testing.T) {
	path := "/Users/sukhvirsingh/Desktop/Resume/Sukhvir_Singh_Resume_AI.pdf"
	data, err := os.ReadFile(path)
	if err != nil {
		t.Skip("local resume PDF not present")
	}
	text, err := ExtractCVMarkdown("cv.pdf", "application/pdf", data)
	if err != nil {
		t.Fatal(err)
	}
	gen := func(context.Context, string) (string, error) {
		return `{
			"note": "Summary now leads with RAG, agents, and LangGraph already on this CV.",
			"replacements": [
				{"from": "experienced in building production LLM systems: RAG pipelines, AI agents", "to": "experienced in production LLM systems: RAG pipelines, AI agents, LangGraph"}
			]
		}`, nil
	}
	out, err := TailorCV(context.Background(), &JobListing{Title: "AI Engineer", Text: "RAG LangGraph agents evaluation"}, &model.CareerProfile{CVMarkdown: text}, &model.CareerEvaluation{Role: "AI Engineer", Company: "Northwind Labs"}, gen)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "AI agents, LangGraph") || !strings.Contains(out, "Tailored for AI Engineer at Northwind Labs") {
		t.Fatalf("expected surgical edit and note:\n%s", out[:min(len(out), 500)])
	}
	if strings.Contains(out, "<!--") {
		t.Fatal("HTML comments must not appear")
	}
	if !KeepLayout(text, out) {
		t.Fatal("layout drifted after replacements")
	}
	pdf, err := MarkdownToPDF("Sukhvir Singh", out)
	if err != nil {
		t.Fatal(err)
	}
	if n := pdfPageCount(pdf); n != 1 {
		t.Fatalf("pages = %d, want 1", n)
	}
	if bytes.Contains(pdf, []byte("Tailored for")) {
		t.Fatal("tailor note belongs on the web, not the PDF")
	}
	_ = os.WriteFile("/tmp/sukhvir-tailored-cv.pdf", pdf, 0o644)
}
