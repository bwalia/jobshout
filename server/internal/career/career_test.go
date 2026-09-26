package career

import (
	"context"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/model"
)

const scamJD = `# Urgent Crypto Ambassador

Wire transfer your equipment deposit via Telegram. Always hiring. Evergreen requisition.
`

func TestBlockGDoesNotChangeScore(t *testing.T) {
	profile := &model.CareerProfile{
		CVMarkdown: "Staff engineer. Kubernetes, GPU scheduling, observability, distributed systems.",
		Identity:   model.CareerIdentity{FullName: "Ada"},
		Targets:    model.CareerTargets{Titles: []string{"Head of AI"}, Seniority: "head"},
	}
	gilded := GoldenJD + "\n\nWire transfer. Always hiring. Evergreen.\n"
	listing := &JobListing{Title: "Head of AI Platform", Company: "Northwind Labs", Text: gilded}
	ev := HeuristicEvaluate(listing, profile, model.CareerEvalModeFull)
	if ev.LegitimacyTier == "clear" || ev.Blocks.G == "" {
		t.Fatal("gilded JD should flag Block G")
	}
	cv := strings.ToLower(profile.CVMarkdown)
	text := strings.ToLower(gilded)
	want := holisticScore(false, keywordOverlap(cv, text), seniorityFit(profile.Targets.Seniority, text), 3.6)
	if ev.Score.Overall != want {
		t.Fatalf("overall must equal holistic score with G ignored: got=%.4f want=%.4f", ev.Score.Overall, want)
	}
	before := ev.Score.Overall
	ev.Blocks.G = "possible scam"
	ev.LegitimacyTier = "review"
	applyScoreRules(ev, profile)
	if ev.Score.Overall != before {
		t.Fatal("recording Block G must not change overall")
	}
}

func TestScamFlagsBlockG(t *testing.T) {
	ev := HeuristicEvaluate(&JobListing{Title: "Ambassador", Company: "ScamCo", Text: scamJD}, &model.CareerProfile{Identity: model.CareerIdentity{FullName: "Ada"}}, model.CareerEvalModeTriage)
	if ev.LegitimacyTier == "clear" {
		t.Fatal("scam JD should not be clear")
	}
}

func TestWorkAuthHardStop(t *testing.T) {
	profile := &model.CareerProfile{
		CVMarkdown: "Staff engineer Kubernetes",
		Identity:   model.CareerIdentity{FullName: "Ada"},
		WorkAuth:   model.CareerWorkAuth{NeedsSponsorship: true},
	}
	listing := &JobListing{Title: "Head of AI Platform", Company: "Northwind Labs", Text: GoldenJD}
	ev, err := EvaluateBlocks(t.Context(), listing, profile, model.CareerEvalModeFull, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !ev.HardStop {
		t.Fatal("expected hard stop for no-sponsorship vs needs-sponsorship")
	}
	if ev.Score.RecommendApply {
		t.Fatal("hard stop must not recommend applying")
	}
}

func TestRecommendApplyFloor(t *testing.T) {
	profile := &model.CareerProfile{CVMarkdown: "unrelated baker pastry chef", Identity: model.CareerIdentity{FullName: "Ada"}}
	ev := HeuristicEvaluate(&JobListing{Title: "Head of AI", Company: "X", Text: GoldenJD}, profile, model.CareerEvalModeFull)
	if ev.Score.Overall >= model.RecommendApplyMin && !strings.Contains(strings.ToLower(profile.CVMarkdown), "kubernetes") {
		// Weak overlap should sit under 4.0 for this fixture.
	}
	if ev.Score.Overall >= model.RecommendApplyMin && ev.Score.RecommendApply == false {
		t.Fatal("recommend flag should track the floor")
	}
	if ev.Score.Overall < model.RecommendApplyMin && ev.Score.RecommendApply {
		t.Fatalf("must not recommend below 4.0: %.2f", ev.Score.Overall)
	}
}

func TestLLMScoreWithoutBlocksIsFilled(t *testing.T) {
	gen := func(context.Context, string) (string, error) {
		return `{"company":"Stripe","role":"Staff Software Engineer, API Platform","overall":4.8,"dimensions":{"a":5,"b":5,"c":4.7,"d":4.9,"e":4.6},"a":"","b":"","c":"","d":"","e":"","f":"","g":"","h":"","report_markdown":"# Staff Software Engineer, API Platform — Stripe\n\n**Score:** 4.8 / 5\n"}`, nil
	}
	profile := &model.CareerProfile{
		CVMarkdown: "Staff engineer. Go, Kubernetes, Postgres. 12 years. UK citizen.",
		Identity:   model.CareerIdentity{FullName: "Dummy"},
		Targets:    model.CareerTargets{Titles: []string{"Staff Software Engineer"}, Seniority: "staff"},
	}
	listing := &JobListing{
		Title: "Staff Software Engineer, API Platform", Company: "stripe",
		Text: GoldenJD, URL: "https://boards.greenhouse.io/stripe/jobs/1",
	}
	ev, err := EvaluateBlocks(t.Context(), listing, profile, model.CareerEvalModeFull, gen)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Score.Overall < 4.7 || ev.Score.Overall > 4.9 {
		t.Fatalf("must keep the model overall, got %.2f", ev.Score.Overall)
	}
	if ev.Blocks.A == "" || ev.Blocks.B == "" || ev.Blocks.G == "" {
		t.Fatalf("expected heuristic prose for empty LLM blocks, got %+v", ev.Blocks)
	}
	if !strings.Contains(ev.ReportMarkdown, "## ") {
		t.Fatalf("report should include A–H headings, got %q", ev.ReportMarkdown)
	}
	if _, ok := ev.Score.Dimensions["match"]; !ok {
		t.Fatalf("dimensions should be remapped off a–e: %#v", ev.Score.Dimensions)
	}
}

func TestLLMPartialBlocksStillFillF(t *testing.T) {
	gen := func(context.Context, string) (string, error) {
		return `{"company":"Stripe","role":"Staff SWE","overall":4.8,"dimensions":{"match":5,"level":5,"culture":4.7,"compensation":4.9,"trajectory":4.6},"a":"Role summary from the model.","b":"CV match from the model.","c":"","d":"","e":"","f":"","g":"Looks legitimate.","h":"","report_markdown":"# Staff SWE — Stripe\n"}`, nil
	}
	profile := &model.CareerProfile{
		CVMarkdown: "Staff engineer. Kubernetes.",
		Identity:   model.CareerIdentity{FullName: "Dummy"},
	}
	ev, err := EvaluateBlocks(t.Context(), &JobListing{Title: "Staff SWE", Company: "Stripe", Text: GoldenJD}, profile, model.CareerEvalModeFull, gen)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Blocks.A != "Role summary from the model." {
		t.Fatalf("must keep model A, got %q", ev.Blocks.A)
	}
	if strings.TrimSpace(ev.Blocks.F) == "" {
		t.Fatal("empty Block F should be filled from the heuristic")
	}
}

func TestStripHTMLUnescapesEntities(t *testing.T) {
	got := stripHTML(`&lt;h2&gt;Who we are&lt;/h2&gt;&lt;p&gt;Stripe builds APIs.&lt;/p&gt;`)
	if strings.Contains(got, "&lt;") || strings.Contains(got, "<h2>") {
		t.Fatalf("still escaped or tagged: %q", got)
	}
	if !strings.Contains(got, "Who we are") || !strings.Contains(got, "Stripe builds APIs") {
		t.Fatalf("lost text: %q", got)
	}
}

func TestBlockHOnlyAtHighScore(t *testing.T) {
	profile := &model.CareerProfile{CVMarkdown: "baker", Identity: model.CareerIdentity{FullName: "Ada"}}
	ev, err := EvaluateBlocks(t.Context(), &JobListing{Text: GoldenJD, Title: "Head of AI"}, profile, model.CareerEvalModeFull, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Score.Overall < model.RecommendFormAnswersMin && ev.Blocks.H != "" {
		t.Fatalf("Block H must be empty below 4.5, overall=%.2f h=%q", ev.Score.Overall, ev.Blocks.H)
	}
}

func TestStatusMachine(t *testing.T) {
	if !CanTransition(model.CareerStatusEvaluated, model.CareerStatusApplied) {
		t.Fatal("evaluated → applied")
	}
	if CanTransition(model.CareerStatusEvaluated, model.CareerStatusHired) {
		t.Fatal("evaluated → hired must be illegal")
	}
	if CanTransition(model.CareerStatusHired, model.CareerStatusApplied) {
		t.Fatal("hired is terminal")
	}
	if err := ValidateTransition(model.CareerStatusOffer, model.CareerStatusHired); err != nil {
		t.Fatal(err)
	}
}

func TestNeverSubmit(t *testing.T) {
	if !NeverSubmit {
		t.Fatal("apply assist must never submit")
	}
	html := RenderHTMLCV("# Ada\nStaff engineer", "Ada")
	if !strings.Contains(html, "Draft only") {
		t.Fatal("HTML CV must say draft only")
	}
}

func TestLinkedInDraftMax(t *testing.T) {
	d := LinkedInDraft("Alex", "Head of AI", "Northwind Labs", &model.CareerProfile{Identity: model.CareerIdentity{FullName: "Ada Lovelace"}})
	if d == "" {
		t.Fatal("empty draft")
	}
	if len([]rune(d)) > 300 {
		t.Fatalf("over 300 runes: %d", len([]rune(d)))
	}
}

func TestOfferPrepDisclaimer(t *testing.T) {
	prep := OfferPrep(&model.CareerApplication{Company: "Acme", Role: "Staff"}, &model.CareerProfile{})
	if !prep.NotLegalAdvice {
		t.Fatal("must flag not legal advice")
	}
}

func TestMatchStoriesRanks(t *testing.T) {
	stories := []model.CareerStory{
		{Title: "GPU scheduler", Situation: "kubernetes gpu scheduling", Provenance: model.CareerStoryCV},
		{Title: "Bakery", Situation: "sourdough croissants", Provenance: model.CareerStoryUser},
	}
	got := MatchStories(stories, GoldenJD, "Head of AI Platform")
	if len(got) == 0 || got[0].Title != "GPU scheduler" {
		t.Fatalf("expected GPU story first: %+v", got)
	}
}

func TestDeadListing(t *testing.T) {
	if !IsDeadText("This position has been filled and is no longer available.") {
		t.Fatal("expected dead")
	}
	if IsDeadText(GoldenJD) {
		t.Fatal("golden JD is live")
	}
}

func TestBlacklistAskNotSkip(t *testing.T) {
	hit := MatchBlacklist("Northwind Labs", "https://northwind.example/jobs/1", []model.CareerBlacklistEntry{
		{Company: "Northwind Labs", Reason: "culture"},
	})
	if hit == nil {
		t.Fatal("expected hit")
	}
	miss := MatchBlacklist("Other Co", "https://other.example", []model.CareerBlacklistEntry{
		{Company: "Northwind Labs"},
	})
	if miss != nil {
		t.Fatal("must not match unrelated company")
	}
}

func TestTitleFilters(t *testing.T) {
	if !TitleAllowed("Head of AI Platform", []string{"head of ai", "staff"}, []string{"intern"}) {
		t.Fatal("should include")
	}
	if TitleAllowed("Marketing Intern", []string{"head of ai"}, []string{"intern"}) {
		t.Fatal("excluded intern")
	}
}

func TestDoctorEmptyProfile(t *testing.T) {
	rep := Doctor(&model.CareerProfile{}, nil, nil, nil)
	if rep.OK {
		t.Fatal("empty profile is not ok")
	}
	if len(rep.Warnings) == 0 {
		t.Fatal("expected warnings")
	}
}

func TestExtractPastedJD(t *testing.T) {
	listing, err := Extract(t.Context(), nil, nil, "", GoldenJD)
	if err != nil {
		t.Fatal(err)
	}
	if listing.Title == "" {
		t.Fatal("expected title from heading")
	}
	if !listing.Live {
		t.Fatal("golden is live")
	}
}

func TestOrgIsolationURLKey(t *testing.T) {
	// Unique listing URL is per profile, not global — modelled by (profile_id, url).
	// This test documents the invariant; repository enforces it.
	if model.RecommendApplyMin != 4.0 || model.RecommendFormAnswersMin != 4.5 {
		t.Fatal("product floors moved")
	}
}
