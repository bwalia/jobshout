package career

import (
	"context"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/model"
)

const fixtureCV = `# Ada Lovelace

## Experience
- Staff engineer at Acme, 2020–2024. Go, Kubernetes.

## Education
- Cambridge
`

func TestSameOutline(t *testing.T) {
	same := `# Ada Lovelace

## Experience
- Staff engineer at Acme, 2020–2024. Go, Kubernetes, observability.

## Education
- Cambridge
`
	if !SameOutline(fixtureCV, same) {
		t.Fatal("rephrased bullets should keep the outline")
	}
	if SameOutline(fixtureCV, "# Ada Lovelace\n\n## Summary\nhello\n") {
		t.Fatal("different headings must not match")
	}
}

func TestTailorCVKeepsOutline(t *testing.T) {
	kept := `# Ada Lovelace

## Experience
- Staff engineer at Acme, 2020–2024. Go, Kubernetes, observability.

## Education
- Cambridge
`
	gen := func(context.Context, string) (string, error) {
		return `{"body":` + jsonString(kept) + `}`, nil
	}
	out, err := TailorCV(t.Context(), &JobListing{Title: "Head of AI"}, &model.CareerProfile{CVMarkdown: fixtureCV}, &model.CareerEvaluation{Role: "Head of AI", Company: "Northwind"}, gen)
	if err != nil {
		t.Fatal(err)
	}
	if !SameOutline(fixtureCV, out) {
		t.Fatalf("outline drifted:\n%s", out)
	}
}

func TestHeadingOutlineLayoutCV(t *testing.T) {
	src := "Sukhvir Singh\nemail@x.com\nSummary\nA line.\nEducation\nA school.\nInternship Experience\nA job.\nProjects\nA project.\nSkills and Competencies\nGo.\nAchievements\nWon.\n"
	got := HeadingOutline(src)
	want := []string{"summary", "education", "internship experience", "projects", "skills and competencies", "achievements"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestKeepLayoutRejectsExpansion(t *testing.T) {
	grown := fixtureCV + "\n- extra bullet that was not on the source\n"
	if KeepLayout(fixtureCV, grown) {
		t.Fatal("extra bullets must fail the layout gate")
	}
}

func TestTailorCVAppliesReplacements(t *testing.T) {
	gen := func(context.Context, string) (string, error) {
		return `{"note":"Lead with Kubernetes.","replacements":[{"from":"Go, Kubernetes.","to":"Kubernetes, Go."}]}`, nil
	}
	out, err := TailorCV(t.Context(), &JobListing{Title: "Head of AI", Text: "Kubernetes"}, &model.CareerProfile{CVMarkdown: fixtureCV}, &model.CareerEvaluation{Role: "Head of AI", Company: "Northwind"}, gen)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Kubernetes, Go.") {
		t.Fatalf("replacement missing:\n%s", out)
	}
	if strings.Contains(out, "Go, Kubernetes.") {
		t.Fatalf("old phrase still present:\n%s", out)
	}
	if !strings.Contains(out, "Tailored for Head of AI at Northwind") || !strings.Contains(out, "Lead with Kubernetes.") {
		t.Fatalf("expected visible note, got:\n%s", out)
	}
	if !KeepLayout(fixtureCV, out) {
		t.Fatalf("layout drifted:\n%s", out)
	}
}

func TestTailorCVRejectsReformat(t *testing.T) {
	gen := func(context.Context, string) (string, error) {
		return `{"body":"# Summary\nI am a visionary leader.\n"}`, nil
	}
	out, err := TailorCV(t.Context(), &JobListing{Title: "Head of AI"}, &model.CareerProfile{CVMarkdown: fixtureCV}, &model.CareerEvaluation{Role: "Head of AI", Company: "Northwind"}, gen)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "layout unchanged") {
		t.Fatalf("expected fallback note, got:\n%s", out)
	}
	if !SameOutline(fixtureCV, strings.Replace(out, unchangedLayoutNote("Head of AI", "Northwind"), "", 1)) {
		t.Fatalf("fallback must be the original CV:\n%s", out)
	}
}

func jsonString(s string) string {
	b := strings.Builder{}
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
