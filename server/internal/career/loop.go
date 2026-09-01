package career

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jobshout/server/internal/model"
)

const linkedInDraftMax = 300

// FollowupDraft is never sent. Cadence defaults to seven days after apply.
func FollowupDraft(app *model.CareerApplication, due time.Time) string {
	when := due.UTC().Format("2006-01-02")
	return fmt.Sprintf(
		"Draft follow-up (not sent). Checking in on %s at %s. Suggested send date %s. A human sends this.",
		nz(app.Role, "the role"), nz(app.Company, "the company"), when,
	)
}

// OfferPrep walks common clauses and lawyer questions. It is not legal advice.
func OfferPrep(app *model.CareerApplication, profile *model.CareerProfile) model.CareerOfferPrep {
	var b strings.Builder
	fmt.Fprintf(&b, "# Offer prep — %s at %s\n\n", nz(app.Role, "role"), nz(app.Company, "company"))
	b.WriteString("**Not legal advice.** Walk these clauses with a lawyer in the relevant jurisdiction.\n\n")
	b.WriteString("## Clause walk\n\n")
	for _, c := range []string{
		"Base compensation and pay cadence",
		"Equity: grant size, type (ISO/RSU/options), strike, vesting, cliff, acceleration, refresh",
		"Bonus / commission — on-target vs guaranteed",
		"Notice period, garden leave, and start date",
		"IP assignment and prior-invention carve-out",
		"Non-compete / non-solicit (enforceability varies widely)",
		"Benefits, visa/relocation support, probation",
		"Severance and change-of-control",
	} {
		fmt.Fprintf(&b, "- [ ] %s\n", c)
	}
	b.WriteString("\n## Questions for a lawyer (not answered here)\n\n")
	b.WriteString("- Which clauses are unusual for this market and level?\n")
	b.WriteString("- What is actually enforceable where I live and work?\n")
	b.WriteString("- What should I ask to change in writing before I sign?\n")
	if profile != nil && strings.TrimSpace(profile.Narrative) != "" {
		b.WriteString("\n## Negotiation notes from your profile\n\n")
		b.WriteString(clip(profile.Narrative, 1200))
		b.WriteString("\n")
	}
	b.WriteString("\nA human signs. CareerOps drafts the walkthrough.\n")
	return model.CareerOfferPrep{
		Company:        app.Company,
		Role:           app.Role,
		PrepMarkdown:   b.String(),
		NotLegalAdvice: true,
	}
}

// SalaryGap compares desired vs advertised vs actual. Not a compensation opinion.
func SalaryGap(profile *model.CareerProfile, advertised, actual string) model.CareerSalaryGap {
	desired := ""
	if profile != nil {
		desired = profile.Targets.MinComp
	}
	note := "Recorded for calibration. This is not a market valuation or legal advice."
	switch {
	case strings.TrimSpace(desired) == "" && strings.TrimSpace(advertised) == "":
		note = "Neither desired nor advertised compensation is set — fill the profile target or paste the posting band."
	case strings.TrimSpace(advertised) != "" && strings.TrimSpace(desired) != "":
		note = "Compare the advertised band to your target before negotiating. Not legal advice; not a market report."
	}
	return model.CareerSalaryGap{
		Desired:        desired,
		Advertised:     advertised,
		Actual:         actual,
		Note:           note,
		NotLegalAdvice: true,
	}
}

// LinkedInDraft is ≤300 characters, draft-only, facts from the profile.
func LinkedInDraft(name, role, company string, profile *model.CareerProfile) string {
	who := strings.TrimSpace(name)
	if who == "" {
		who = "there"
	}
	me := "I"
	if profile != nil && profile.Identity.FullName != "" {
		me = profile.Identity.FullName
	}
	draft := fmt.Sprintf("Hi %s — %s here. Would you be open to a short note about the %s role at %s? Happy to send a CV if useful.",
		who, me, nz(role, "open"), nz(company, "your team"))
	if utf8.RuneCountInString(draft) <= linkedInDraftMax {
		return draft
	}
	runes := []rune(draft)
	return string(runes[:linkedInDraftMax-1]) + "…"
}

// Upskill extracts repeated JD tokens missing from the CV on sub-4.0 evaluations.
func Upskill(evals []model.CareerEvaluation, cv string) []string {
	cvLow := strings.ToLower(cv)
	counts := map[string]int{}
	for _, ev := range evals {
		if ev.Score.Overall+1e-9 >= model.RecommendApplyMin {
			continue
		}
		for w := range uniqueWords(strings.ToLower(ev.JDText + " " + ev.Role)) {
			if len(w) < 6 {
				continue
			}
			if strings.Contains(cvLow, w) {
				continue
			}
			counts[w]++
		}
	}
	type kv struct {
		w string
		n int
	}
	var list []kv
	for w, n := range counts {
		if n < 1 {
			continue
		}
		list = append(list, kv{w, n})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].n == list[j].n {
			return list[i].w < list[j].w
		}
		return list[i].n > list[j].n
	})
	out := make([]string, 0, 12)
	for _, it := range list {
		out = append(out, it.w)
		if len(out) == 12 {
			break
		}
	}
	return out
}
