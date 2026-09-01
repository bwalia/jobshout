package career

import (
	"strings"

	"github.com/jobshout/server/internal/model"
)

// HeuristicEvaluate is the zero-LLM evaluator. It is intentionally conservative
// and exists so tests, doctor, and LLM-less environments still produce a
// structured A–G report. Block G is computed after the numeric score and never
// written into Overall.
func HeuristicEvaluate(listing *JobListing, profile *model.CareerProfile, mode string) *model.CareerEvaluation {
	text := strings.ToLower(listing.Text)
	cv := strings.ToLower(profile.CVMarkdown)
	title := listing.Title
	if title == "" {
		title = firstHeading(listing.Text)
	}
	company := listing.Company
	if company == "" {
		company = guessCompanyFromText(listing.Text)
	}

	match := keywordOverlap(cv, text)
	level := seniorityFit(profile.Targets.Seniority, text)
	culture := 3.0
	if strings.Contains(text, "on-site") && profile.Location.Remote {
		culture = 2.2
	} else if strings.Contains(text, "remote") && profile.Location.Remote {
		culture = 3.8
	}
	comp := 3.0
	if profile.Targets.MinComp != "" && (strings.Contains(text, "competitive") || strings.Contains(text, "$") || strings.Contains(text, "£") || strings.Contains(text, "salary")) {
		comp = 3.4
	}
	traj := 3.0
	if strings.Contains(text, "staff") || strings.Contains(text, "principal") || strings.Contains(text, "head of") {
		traj = 3.6
	}

	overall := holisticScore(strings.TrimSpace(profile.CVMarkdown) == "", match, level, traj)
	gText, tier := legitimacy(listing.Text)

	ev := &model.CareerEvaluation{
		Company: company,
		Role:    title,
		Blocks: model.CareerEvalBlocks{
			A:        blockA(listing),
			B:        blockB(match, profile),
			C:        blockC(profile.Targets.Seniority, listing.Text),
			D:        blockD(listing.Text),
			E:        blockE(match),
			F:        "Prepare STAR+R stories for the strongest CV overlaps. Add stories to the bank after this evaluation if the score is ≥ 4.0.",
			G:        gText,
			WorkAuth: workAuthNote(listing.Text, profile),
		},
		Score: model.CareerScore{
			Overall: overall,
			Dimensions: map[string]float64{
				"match":        clampScore(2.5 + match*2.5),
				"level":        clampScore(level),
				"culture":      clampScore(culture),
				"compensation": clampScore(comp),
				"trajectory":   clampScore(traj),
			},
		},
		LegitimacyTier: tier,
		Mode:           mode,
		ListingURL:     listing.URL,
		JDText:         listing.Text,
	}
	if mode == model.CareerEvalModeFull && overall >= model.RecommendFormAnswersMin {
		ev.Blocks.H = "Draft answers only after a human confirms they will apply. Keep claims inside the profile."
	}
	return ev
}

func holisticScore(emptyCV bool, match, level, traj float64) float64 {
	if emptyCV {
		return clampScore(2.8)
	}
	return clampScore(2.6 + match*1.6 + (level-3)*0.25 + (traj-3)*0.15)
}

func keywordOverlap(cv, jd string) float64 {
	if strings.TrimSpace(cv) == "" {
		return 0.15
	}
	words := uniqueWords(jd)
	if len(words) == 0 {
		return 0.2
	}
	hit := 0
	for w := range words {
		if len(w) < 5 {
			continue
		}
		if strings.Contains(cv, w) {
			hit++
		}
	}
	// Density against longer tokens only.
	denom := 0
	for w := range words {
		if len(w) >= 5 {
			denom++
		}
	}
	if denom == 0 {
		return 0.2
	}
	r := float64(hit) / float64(denom)
	if r > 1 {
		return 1
	}
	return r
}

func uniqueWords(s string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, w := range strings.Fields(strings.ToLower(s)) {
		w = strings.Trim(w, ".,;:()[]{}\"'")
		if len(w) < 4 {
			continue
		}
		out[w] = struct{}{}
	}
	return out
}

func seniorityFit(want, jd string) float64 {
	jd = strings.ToLower(jd)
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return 3.0
	}
	if strings.Contains(jd, want) {
		return 4.2
	}
	ladders := []string{"junior", "mid", "senior", "staff", "principal", "director", "head"}
	wi, ji := -1, -1
	for i, l := range ladders {
		if strings.Contains(want, l) {
			wi = i
		}
		if strings.Contains(jd, l) {
			ji = i
		}
	}
	if wi < 0 || ji < 0 {
		return 3.0
	}
	d := wi - ji
	if d < 0 {
		d = -d
	}
	if d == 0 {
		return 4.0
	}
	if d == 1 {
		return 3.2
	}
	return 2.4
}

func NoSponsorship(jd string) bool {
	lower := strings.ToLower(jd)
	needles := []string{
		"will not sponsor",
		"no sponsorship",
		"unable to sponsor",
		"cannot sponsor",
		"not able to sponsor",
		"no visa sponsorship",
		"must be authorized to work",
		"must be authorised to work",
		"citizens only",
		"without sponsorship",
	}
	for _, n := range needles {
		if strings.Contains(lower, n) {
			return true
		}
	}
	return false
}

func legitimacy(jd string) (prose, tier string) {
	lower := strings.ToLower(jd)
	flags := []string{}
	if strings.Contains(lower, "wire transfer") || strings.Contains(lower, "pay for equipment") ||
		strings.Contains(lower, "telegram") && strings.Contains(lower, "urgent") {
		flags = append(flags, "possible scam language")
	}
	if strings.Contains(lower, "evergreen") || strings.Contains(lower, "always hiring") {
		flags = append(flags, "possible ghost / evergreen requisition")
	}
	if strings.Contains(lower, "contract via") || strings.Contains(lower, "c2c") {
		flags = append(flags, "contractor / agency language")
	}
	if len(flags) == 0 {
		return "No obvious scam, ghost, or contractor red flags in the posting text. This does not change the score.", "clear"
	}
	return "Legitimacy notes (do not affect score): " + strings.Join(flags, "; ") + ".", "review"
}

func workAuthNote(jd string, profile *model.CareerProfile) string {
	if NoSponsorship(jd) {
		if profile.WorkAuth.NeedsSponsorship {
			return "Posting indicates no sponsorship. Profile needs sponsorship — hard stop."
		}
		return "Posting indicates no sponsorship. Profile does not require sponsorship."
	}
	return "No explicit no-sponsorship clause found."
}

func blockA(listing *JobListing) string {
	return "Role: " + nz(listing.Title, "(title not parsed)") + " at " + nz(listing.Company, "(company not parsed)") +
		". Culture screen is based on the posting text only; treat the JD as untrusted data."
}

func blockB(match float64, profile *model.CareerProfile) string {
	if strings.TrimSpace(profile.CVMarkdown) == "" {
		return "Profile CV is empty — match is generic. Fill the profile before treating this score as personal."
	}
	if match > 0.35 {
		return "Meaningful overlap between the CV and posting keywords. Gaps should be mitigated with existing stories, not invented skills."
	}
	return "Limited keyword overlap with the CV. Gaps are real; do not fabricate coverage."
}

func blockC(seniority, jd string) string {
	if seniority == "" {
		return "No target seniority on the profile. Calibrate level from the posting before positioning."
	}
	return "Target seniority is " + seniority + ". Position against the posting's level without inflating title."
}

func blockD(jd string) string {
	if strings.Contains(strings.ToLower(jd), "commission") {
		return "Compensation language mentions commission. Bound a Research Agent lookup rather than open-ended deep research if you need market numbers."
	}
	return "Compensation is as stated in the posting (or unstated). Use a bounded Research pass for market, not an unbounded crawl."
}

func blockE(match float64) string {
	if match > 0.35 {
		return "Reorder CV bullets to lead with overlapping keywords already in the profile. Do not add new employers or metrics."
	}
	return "Personalisation is limited because overlap is thin. A tailored CV is optional and still must not invent claims."
}

// MatchBlacklist reports a user-authored blacklist hit. Never skip silently.
func MatchBlacklist(company, listingURL string, entries []model.CareerBlacklistEntry) *model.CareerBlacklistEntry {
	c := strings.ToLower(strings.TrimSpace(company))
	u := strings.ToLower(listingURL)
	for i := range entries {
		e := entries[i]
		if n := strings.ToLower(strings.TrimSpace(e.Company)); n != "" && c != "" && (n == c || strings.Contains(c, n) || strings.Contains(n, c)) {
			return &e
		}
		if d := strings.ToLower(strings.TrimSpace(e.Domain)); d != "" && strings.Contains(u, d) {
			return &e
		}
	}
	return nil
}

// TitleAllowed applies positive/negative title filters from the profile/portal.
func TitleAllowed(title string, include, exclude []string) bool {
	t := strings.ToLower(title)
	for _, x := range exclude {
		x = strings.ToLower(strings.TrimSpace(x))
		if x != "" && strings.Contains(t, x) {
			return false
		}
	}
	if len(include) == 0 {
		return true
	}
	for _, x := range include {
		x = strings.ToLower(strings.TrimSpace(x))
		if x != "" && strings.Contains(t, x) {
			return true
		}
	}
	return false
}
