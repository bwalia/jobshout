package career

import (
	"strings"
	"testing"

	"github.com/jobshout/server/internal/model"
)

func evalWith(score float64, hardStop bool, reason string) *model.CareerEvaluation {
	return &model.CareerEvaluation{
		Company:        "SumUp",
		Role:           "Senior Platform Engineer",
		ListingURL:     "https://sumup.com/careers/positions/1",
		HardStop:       hardStop,
		HardStopReason: reason,
		Score:          model.CareerScore{Overall: score},
	}
}

func TestDecideApply(t *testing.T) {
	tests := []struct {
		name        string
		ev          *model.CareerEvaluation
		minScore    float64
		wantProceed bool
		wantReason  string
	}{
		{"above the floor proceeds", evalWith(4.5, false, ""), RecommendFloor, true, ""},
		{"exactly at the floor proceeds", evalWith(4.0, false, ""), RecommendFloor, true, ""},
		{"below the floor is skipped", evalWith(2.95, false, ""), RecommendFloor, false, "below"},
		// The hard stop is the rule the package doc says never relaxes: it has
		// to win even when the score is excellent.
		{"hard stop beats a high score", evalWith(4.9, true, "no sponsorship"), RecommendFloor, false, "no sponsorship"},
		{"a lowered threshold lets a weak job through", evalWith(2.95, false, ""), 2.0, true, ""},
		{"missing evaluation is skipped, not crashed", nil, RecommendFloor, false, "no evaluation"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecideApply(tt.ev, tt.minScore)
			if got.Proceed != tt.wantProceed {
				t.Errorf("Proceed = %v, want %v (reason %q)", got.Proceed, tt.wantProceed, got.Reason)
			}
			if tt.wantReason != "" && !strings.Contains(got.Reason, tt.wantReason) {
				t.Errorf("Reason = %q, want it to mention %q", got.Reason, tt.wantReason)
			}
			if tt.wantProceed && got.Reason != "" {
				t.Errorf("a proceeding decision carried a reason: %q", got.Reason)
			}
		})
	}
}

// TailorCV falls back to the stored CV when the model is unavailable, so the
// existence of an artifact proves nothing. This is what the run reports on.
func TestCVWasTailored(t *testing.T) {
	base := "# Harcharan Singh\nPlatform Engineer\n"

	tests := []struct {
		name     string
		tailored string
		want     bool
	}{
		{"genuinely rewritten", "# Harcharan Singh\nPlatform Engineer for SumUp\n", true},
		{"identical fallback", base, false},
		{"identical but for whitespace", "\n  " + base + "  \n", false},
		{"empty output is not tailoring", "", false},
		// The case seen in a live run: TailorCV fell back to the stored CV and
		// appended its "Tailored for …" note. The note alone must not count —
		// reporting that as a tailored CV is exactly the lie this guards.
		{"fallback plus its note is not tailoring",
			base + unchangedLayoutNote("Senior Backend Engineer (Java)", "Moniepoint Inc."), false},
		{"a real rewrite still counts with the note attached",
			"# Harcharan Singh\nJava Backend Engineer\n" + unchangedLayoutNote("Senior Backend Engineer", "Moniepoint"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CVWasTailored(base, tt.tailored); got != tt.want {
				t.Errorf("CVWasTailored = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSubmissionPackage(t *testing.T) {
	ev := evalWith(4.3, false, "")
	profile := &model.CareerProfile{
		Identity: model.CareerIdentity{FullName: "Harcharan Singh", Email: "dev@local.test"},
		WorkAuth: model.CareerWorkAuth{Countries: []string{"UK"}},
	}

	got := SubmissionPackage(ev, profile, "TAILORED CV BODY", "COVER BODY")

	// The banner is the point of the document: it is the artifact most likely
	// to be mistaken for a submitted application.
	if !strings.HasPrefix(got, "# Application package — NOT SUBMITTED") {
		t.Errorf("package does not open with the not-submitted banner:\n%s", got[:min(120, len(got))])
	}
	for _, want := range []string{
		"SumUp", "Senior Platform Engineer", "Harcharan Singh",
		"dev@local.test", "TAILORED CV BODY", "COVER BODY",
		"4.30 / 5", "Nothing above was sent to anyone.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("package is missing %q", want)
		}
	}
}

// A profile with blank fields must not render empty bullets into the form
// section — a human reads this and pastes from it.
func TestSubmissionPackage_SkipsEmptyFields(t *testing.T) {
	got := SubmissionPackage(evalWith(4.1, false, ""), &model.CareerProfile{}, "CV", "")

	if strings.Contains(got, "**Phone:**") {
		t.Error("rendered an empty Phone row")
	}
	if strings.Contains(got, "## Cover letter") {
		t.Error("rendered a Cover letter heading with no cover letter")
	}
	if !strings.Contains(got, "CV") {
		t.Error("dropped the CV body")
	}
}

// NeverSubmit is the product rule the whole feature rests on. If it is ever
// flipped, this test is the thing that should stop the commit.
func TestNeverSubmitStands(t *testing.T) {
	if !NeverSubmit {
		t.Fatal("NeverSubmit is false — apply assist must never submit an application")
	}
}
