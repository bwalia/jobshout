package career

import (
	"strings"
	"testing"
)

const groundingCV = `# Harcharan Singh
Senior Platform Engineer

## Experience
### Lead Platform Engineer, Bwalia (2021-2026)
- Built a multi-tenant agent orchestration platform in Go and Next.js serving 104 tables and 100+ tools.
- Cut inference spend 40% by adding per-token cost accounting and per-org budget enforcement.
- Ran Postgres, MinIO and k3s across four environments with Helm.

## Skills
Go, Postgres, Kubernetes, Helm, Next.js, TypeScript, LLM orchestration, RAG
`

func TestUngroundedClaims(t *testing.T) {
	tests := []struct {
		name      string
		candidate string
		wantWords []string // empty means "must be grounded"
	}{
		// The exact fabrication qwen3-coder:30b returned for a Java posting
		// against this CV. Neither word appears anywhere in the source.
		{
			"invented technology from the job description",
			"Designed and implemented scalable backend services using Java and Spring",
			[]string{"backend", "java", "spring"},
		},
		{
			"invented metric",
			"Cut inference spend 90% by adding per-token cost accounting",
			[]string{"90"},
		},
		{
			"invented employer",
			"Lead Platform Engineer, Google (2021-2026)",
			[]string{"google"},
		},
		// Legitimate re-wording: every claim-bearing word is already in the CV,
		// only the emphasis moved.
		{
			"re-ordering existing skills is fine",
			"Built a Go and Postgres platform serving 104 tables",
			nil,
		},
		{
			"generic CV verbs need no grounding",
			"Delivered and maintained the Go orchestration platform",
			nil,
		},
		{
			"exact copy is grounded",
			"Ran Postgres, MinIO and k3s across four environments with Helm.",
			nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UngroundedClaims(groundingCV, tt.candidate)

			if len(tt.wantWords) == 0 {
				if len(got) != 0 {
					t.Fatalf("expected grounded, but these words were flagged: %v", got)
				}
				return
			}
			for _, w := range tt.wantWords {
				if !contains(got, w) {
					t.Errorf("expected %q to be flagged as ungrounded, got %v", w, got)
				}
			}
		})
	}
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// Technology names must not collapse into each other when punctuation is
// stripped, or "C" in the CV would ground a claim of "C++".
func TestClaimWords_KeepsTechnologyPunctuation(t *testing.T) {
	got := claimWords("C++ and C# and Node.js")

	for _, want := range []string{"c++", "c#", "node", "js"} {
		if !contains(got, want) {
			t.Errorf("claimWords lost %q: got %v", want, got)
		}
	}
	if IsGrounded("I know C", "I know C++") {
		t.Error(`"C" in the source must not ground a claim of "C++"`)
	}
}

// The guard has to hold at the point replacements are applied, not only in the
// helper — that is where a fabricated swap would otherwise reach the CV.
func TestApplyReplacements_RejectsUngroundedSwap(t *testing.T) {
	src := groundingCV

	body, n := applyReplacements(src, []tailorSwap{{
		From: "Senior Platform Engineer",
		To:   "Senior Java Engineer",
	}})
	if n != 0 {
		t.Errorf("applied %d ungrounded replacement(s); body now: %q", n, body)
	}
	if strings.Contains(body, "Java") {
		t.Error("a fabricated Java claim reached the CV")
	}

	// A grounded swap still applies, or the guard has made tailoring useless.
	body, n = applyReplacements(src, []tailorSwap{{
		From: "Senior Platform Engineer",
		To:   "Senior Go Engineer",
	}})
	if n != 1 {
		t.Fatalf("grounded replacement was rejected (n=%d)", n)
	}
	if !strings.Contains(body, "Senior Go Engineer") {
		t.Error("grounded replacement did not reach the CV")
	}
}
