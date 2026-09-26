package career

import (
	"fmt"
	"strings"

	"github.com/jobshout/server/internal/model"
)

// The apply sequence: what the agent does for one job once it has been scored.
//
// It stops one step short of submitting. NeverSubmit (artifacts.go) is the
// product rule and there is no code path that posts to a portal — this file
// assembles what a human would paste and says plainly that nothing was sent.
// A dry run is therefore not a simulation of the real thing; it is the whole
// of the real thing, because preparation is all this agent does.

// RecommendFloor is the CareerOps score below which applying is never
// recommended. Callers may prepare materials under a lower threshold when they
// ask for one explicitly, but the evaluation's own recommend_apply is left
// alone: preparing is not recommending.
const RecommendFloor = 4.0

// Stages one job can reach. Anything short of ApplyStagePrepared carries a
// reason.
const (
	ApplyStageSkipped  = "skipped"
	ApplyStagePrepared = "prepared"
	ApplyStageFailed   = "failed"
)

// ApplyDecision is whether to spend model calls preparing materials for a job.
type ApplyDecision struct {
	Proceed bool
	Reason  string
}

// DecideApply gates preparation on the rules the package doc calls
// non-negotiable: a hard stop (no sponsorship, blacklist) ends it whatever the
// score, and a score under the threshold is not worth the tokens.
func DecideApply(ev *model.CareerEvaluation, minScore float64) ApplyDecision {
	if ev == nil {
		return ApplyDecision{Reason: "no evaluation was produced"}
	}
	if ev.HardStop {
		reason := strings.TrimSpace(ev.HardStopReason)
		if reason == "" {
			reason = "hard stop"
		}
		return ApplyDecision{Reason: "hard stop: " + reason}
	}
	if ev.Score.Overall < minScore {
		return ApplyDecision{Reason: fmt.Sprintf(
			"score %.2f is below the %.2f threshold", ev.Score.Overall, minScore)}
	}
	return ApplyDecision{Proceed: true}
}

// CVWasTailored reports whether tailoring actually rewrote the CV for this job,
// rather than falling back to the stored one.
//
// Both TailorCV paths append a visible "Tailored for <role> at <company>" note —
// the real rewrite and the fallback alike — so comparing the raw strings marks
// every fallback as a success. stripTailorChrome removes that note (it is what
// CVForPDF uses to keep it off the printed page), which leaves the actual
// document to compare. A fallback strips back to exactly the stored CV; a real
// rewrite does not.
func CVWasTailored(base, tailored string) bool {
	body := strings.TrimSpace(stripTailorChrome(tailored))
	return body != "" && body != strings.TrimSpace(stripTailorChrome(base))
}

// SubmissionPackage renders everything a human needs to complete the
// application by hand, in one artifact: where it goes, who is applying, the CV
// tailored for this posting, and the cover letter.
//
// It opens with what did NOT happen, because this document is the thing most
// likely to be mistaken for a submitted application.
func SubmissionPackage(
	ev *model.CareerEvaluation,
	profile *model.CareerProfile,
	cv, cover string,
) string {
	var b strings.Builder

	b.WriteString("# Application package — NOT SUBMITTED\n\n")
	b.WriteString("Career Agent prepared this. It did not send it, and it cannot: ")
	b.WriteString("there is no code path in this system that posts to a job portal. ")
	b.WriteString("A human submits, then marks the tracker row `applied`.\n\n")

	b.WriteString("## Where it goes\n\n")
	if ev != nil {
		row(&b, "Company", ev.Company)
		row(&b, "Role", ev.Role)
		row(&b, "Posting", ev.ListingURL)
		row(&b, "Score", fmt.Sprintf("%.2f / 5", ev.Score.Overall))
		if ev.Score.Recommendation != "" {
			row(&b, "Recommendation", ev.Score.Recommendation)
		}
		if ev.LegitimacyTier != "" {
			row(&b, "Legitimacy", ev.LegitimacyTier)
		}
	}

	if profile != nil {
		b.WriteString("\n## Applicant details for the form\n\n")
		row(&b, "Name", profile.Identity.FullName)
		row(&b, "Email", profile.Identity.Email)
		row(&b, "Phone", profile.Identity.Phone)
		if len(profile.Identity.Links) > 0 {
			row(&b, "Links", strings.Join(profile.Identity.Links, ", "))
		}
		if profile.WorkAuth.NeedsSponsorship {
			row(&b, "Sponsorship", "required — check the posting allows it")
		} else if len(profile.WorkAuth.Countries) > 0 {
			row(&b, "Work authorisation", strings.Join(profile.WorkAuth.Countries, ", "))
		}
	}

	if strings.TrimSpace(cv) != "" {
		b.WriteString("\n## CV, tailored to this posting\n\n")
		b.WriteString(strings.TrimSpace(cv))
		b.WriteString("\n")
	}
	if strings.TrimSpace(cover) != "" {
		b.WriteString("\n## Cover letter\n\n")
		b.WriteString(strings.TrimSpace(cover))
		b.WriteString("\n")
	}

	b.WriteString("\n---\n\nNothing above was sent to anyone.\n")
	return b.String()
}

func row(b *strings.Builder, label, value string) {
	if strings.TrimSpace(value) == "" {
		return
	}
	fmt.Fprintf(b, "- **%s:** %s\n", label, value)
}
