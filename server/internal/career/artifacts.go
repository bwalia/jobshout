package career

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
)

// NeverSubmit is hard-coded: apply assist may prefill, never submit.
const NeverSubmit = true

type draftBody struct {
	Body    string `json:"body"`
	Subject string `json:"subject"`
}

// DraftCoverLetter writes a cover from profile + evaluation. Empty generate → template.
func DraftCoverLetter(ctx context.Context, listing *JobListing, profile *model.CareerProfile, ev *model.CareerEvaluation, generate Generator) (string, error) {
	if generate != nil {
		var out draftBody
		if err := llm.GenerateJSON(ctx, "career-cover", coverPrompt(listing, profile, ev), &out, generate, nil); err == nil && strings.TrimSpace(out.Body) != "" {
			return out.Body, nil
		}
	}
	return templateCover(profile, ev), nil
}

// TailorCV rewrites CV markdown without inventing claims. Empty generate → lightly annotated original.
func TailorCV(ctx context.Context, listing *JobListing, profile *model.CareerProfile, ev *model.CareerEvaluation, generate Generator) (string, error) {
	src := strings.TrimSpace(profile.CVMarkdown)
	if src == "" {
		return "", fmt.Errorf("career: cannot tailor an empty CV")
	}
	if generate != nil {
		var out draftBody
		if err := llm.GenerateJSON(ctx, "career-cv", tailorPrompt(listing, profile, ev), &out, generate, nil); err == nil && strings.TrimSpace(out.Body) != "" {
			return out.Body, nil
		}
	}
	return src + "\n\n<!-- tailored for " + ev.Role + " at " + ev.Company + " — claims unchanged -->\n", nil
}

// DraftEmail is draft-only. Sending is Mail Agent + human approve.
func DraftEmail(ctx context.Context, profile *model.CareerProfile, ev *model.CareerEvaluation, generate Generator) (subject, body string, err error) {
	if generate != nil {
		var out draftBody
		if e := llm.GenerateJSON(ctx, "career-email", emailPrompt(ev, profile), &out, generate, nil); e == nil && out.Body != "" {
			return nz(out.Subject, "Application: "+ev.Role), out.Body, nil
		}
	}
	subj := "Application: " + ev.Role
	body = "Hello,\n\nPlease find my application for " + ev.Role + " at " + ev.Company + ".\n\nThis is a draft. A human sends it.\n"
	return subj, body, nil
}

// RenderHTMLCV wraps CV markdown in a printable HTML document. PDF generation
// stays on the python-sidecar when that route exists; until then the HTML is
// the ATS-friendly artifact. NeverSubmit remains true.
func RenderHTMLCV(markdown, name string) string {
	title := strings.TrimSpace(name)
	if title == "" {
		title = "CV"
	}
	return fmt.Sprintf(
		"<!DOCTYPE html><html><head><meta charset=\"utf-8\"><title>%s</title></head><body><pre>%s</pre><p>Draft only. A human submits.</p></body></html>",
		html.EscapeString(title), html.EscapeString(markdown),
	)
}

func templateCover(profile *model.CareerProfile, ev *model.CareerEvaluation) string {
	name := profile.Identity.FullName
	if name == "" {
		name = "Applicant"
	}
	return fmt.Sprintf("Dear hiring team,\n\nI am applying for %s at %s. "+
		"This draft uses only claims from my JobShout career profile; nothing here was invented for keywords.\n\n"+
		"%s\n\nSincerely,\n%s\n",
		nz(ev.Role, "this role"), nz(ev.Company, "your company"),
		clip(profile.ProofPoints, 800), name)
}
