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
