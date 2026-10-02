package blog

import (
	"os"
	"strings"
	"testing"
	"time"
)

// A live business-audience run on llama3.1:8b with illustration off: a leaked
// illustration fence, two identical Conclusion sections and one anecdote
// pasted in repeatedly.
func TestTidy_LiveBusinessDraft(t *testing.T) {
	raw, err := os.ReadFile("testdata/live_business_padded.md")
	if err != nil {
		t.Fatal(err)
	}
	out := tidyMarkdown(string(raw), false)

	if strings.Contains(out, "```illustration") || strings.Contains(out, "participant C as Candidate") {
		t.Error("an undrawn illustration fence survived")
	}
	if strings.Contains(out, "### Illustration:") {
		t.Error("the heading that introduced the dropped illustration survived")
	}
	if n := strings.Count(out, "## Conclusion"); n != 1 {
		t.Errorf("Conclusion sections = %d, want 1", n)
	}
	anecdote := "For example, a recruitment agency used an AI agent to help assess candidate qualifications and experience. The AI agent was able to provide a more comprehensive and accurate assessment of the candidate's qualifications, reducing the time spent on each application by 30%. However"
	if n := strings.Count(out, anecdote); n != 1 {
		t.Errorf("repeated paragraph appears %d times, want 1", n)
	}
	// What must stay.
	for _, keep := range []string{"# Cutting Time-to-Hire", "## References", "| **Modular Design** |", "## Costs, Risks, and Timeline"} {
		if !strings.Contains(out, keep) {
			t.Errorf("tidy removed %q", keep)
		}
	}
}

func TestTidy_KeepsFencesWhenIllustrated(t *testing.T) {
	md := "# T\n\n## A\n\nText.\n\n```illustration flow\nA then B\n```\n"
	if out := tidyMarkdown(md, true); !strings.Contains(out, "```illustration flow") {
		t.Error("fence removed although the illustrator handles it")
	}
}

func TestTidy_LeavesRepeatedCodeAndShortLines(t *testing.T) {
	code := "```bash\nkubectl apply -f gateway.yaml --namespace production --server-side --force-conflicts\n```"
	md := "# T\n\n## A\n\n" + code + "\n\nSee above.\n\n## B\n\n" + code + "\n\nSee above.\n"
	out := tidyMarkdown(md, false)
	if strings.Count(out, "kubectl apply") != 2 || strings.Count(out, "See above.") != 2 {
		t.Errorf("tidy removed legitimate repeats:\n%s", out)
	}
}

func TestTidy_SameHeadingDifferentBodyKept(t *testing.T) {
	md := "# T\n\n## Example\n\nFirst example body.\n\n## Example\n\nA different second example.\n"
	if out := tidyMarkdown(md, false); strings.Count(out, "## Example") != 2 {
		t.Errorf("a section with different content was dropped:\n%s", out)
	}
}

// A live keyword run echoed the prompt's DIAGRAMS/TABLES labels as headings
// and drew one sequence diagram twice.
func TestTidy_LiveKeywordDraft(t *testing.T) {
	raw, err := os.ReadFile("testdata/live_keyword_labels.md")
	if err != nil {
		t.Fatal(err)
	}
	out := tidyMarkdown(string(raw), false)

	if strings.Contains(out, "DIAGRAM:") || strings.Contains(out, "TABLE:") {
		t.Error("a prompt label survived in a heading")
	}
	if !strings.Contains(out, "## Comparison of Bias Audit Methods") {
		t.Error("the heading text after the label was lost")
	}
	if n := strings.Count(out, "sequenceDiagram"); n != 1 {
		t.Errorf("sequence diagrams = %d, want 1", n)
	}
	if strings.Contains(out, "## AI Candidate Screening System") {
		t.Error("the heading of the dropped duplicate diagram survived with nothing under it")
	}
	if n := strings.Count(out, "import weft"); n != 1 {
		t.Errorf("code block count changed: %d", n)
	}
}

func TestTidy_LabelInsideCodeIsLeftAlone(t *testing.T) {
	md := "# T\n\n## A\n\n```text\n## TABLE: literal\n```\n"
	if out := tidyMarkdown(md, false); !strings.Contains(out, "## TABLE: literal") {
		t.Error("tidy edited the inside of a code block")
	}
}

// A live trending run wrote its own References section — findings, quotes and
// two invented sources — ahead of the pipeline's, and dated itself 2023.
func TestLiveTrendingDraft_ReferencesAndDate(t *testing.T) {
	raw, err := os.ReadFile("testdata/live_trending_refs.md")
	if err != nil {
		t.Fatal(err)
	}
	// The fixture is the finished article; drop the pipeline's list (the last
	// section) to get back what the model wrote.
	md := string(raw)
	md = md[:strings.LastIndex(md, "## References")]

	out := stampLandscapeDate(stripModelReferences(md), time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC))

	if strings.Contains(out, "## References") || strings.Contains(out, "supporting quote") {
		t.Error("the model-written reference dump survived")
	}
	if strings.Contains(out, "from: Nature") {
		t.Error("an invented source survived")
	}
	if !strings.Contains(out, "AI landscape reviewed: 2026-09-30") || strings.Contains(out, "2023-09-28") {
		t.Errorf("date line not restamped:\n%s", out[:200])
	}
	if !strings.Contains(out, "## Limitations of GPT-6.1 Sol") {
		t.Error("the section before the model's references was removed")
	}
}

func TestStripModelReferences_StopsAtNextSection(t *testing.T) {
	md := "# T\n\n## Sources\n\n- a\n\n## Next steps\n\nKeep this.\n"
	out := stripModelReferences(md)
	if strings.Contains(out, "## Sources") || !strings.Contains(out, "Keep this.") {
		t.Errorf("got:\n%s", out)
	}
}
