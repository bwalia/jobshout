package career

import "strings"

// Grounding: a tailored CV may re-word what the CV already says. It may not add
// anything the CV does not say.
//
// This exists because the tailor prompt already asks for exactly that — "role
// keywords that already appear somewhere in the source" — and nothing enforced
// it. A capable model asked to tailor a Go/Postgres CV for a Java posting will
// cheerfully return "Designed and implemented scalable backend services using
// Java and Spring", which is a fabricated claim on a real person's CV. The
// layout gate (KeepLayout) cannot catch it: the fabrication is the same shape
// and length as the truth.
//
// The rule: every claim-bearing word in a replacement must already appear in
// the source CV. Ordinary English and generic CV verbs are exempt, so genuine
// re-wording still works; technology names, employers, and numbers are not, so
// invented experience and invented metrics are rejected.

// genericWords are exempt from grounding: function words, and the ordinary
// verbs and adjectives of CV prose. None of them asserts a fact about the
// candidate on its own — "delivered" claims nothing until it is followed by a
// noun, and the noun still has to be grounded.
//
// Digits are deliberately absent. An invented metric is the most damaging
// fabrication of all, so every number must already appear in the CV.
var genericWords = map[string]bool{
	"a": true, "an": true, "and": true, "as": true, "at": true, "by": true,
	"for": true, "from": true, "in": true, "into": true, "of": true, "on": true,
	"or": true, "over": true, "the": true, "to": true, "with": true, "within": true,
	"across": true, "per": true, "via": true, "using": true, "used": true,
	"is": true, "are": true, "was": true, "were": true, "be": true, "been": true,
	"my": true, "our": true, "their": true, "its": true, "this": true, "that": true,
	"which": true, "who": true, "while": true, "when": true, "than": true, "then": true,

	// Ordinary CV verbs and adjectives.
	"built": true, "build": true, "building": true, "designed": true, "design": true,
	"delivered": true, "deliver": true, "developed": true, "develop": true,
	"implemented": true, "implement": true, "created": true, "create": true,
	"led": true, "lead": true, "leading": true, "managed": true, "manage": true,
	"owned": true, "own": true, "ran": true, "run": true, "running": true,
	"improved": true, "improve": true, "reduced": true, "reduce": true,
	"increased": true, "increase": true, "cut": true, "scaled": true, "scale": true,
	"maintained": true, "maintain": true, "shipped": true, "ship": true,
	"worked": true, "work": true, "supported": true, "support": true,
	"engineered": true, "architected": true, "optimised": true, "optimized": true,
	"scalable": true, "reliable": true, "robust": true, "high": true, "large": true,
	"end": true, "production": true, "team": true, "teams": true, "systems": true,
	"system": true, "services": true, "service": true, "platform": true,
	"experience": true, "senior": true, "engineer": true, "engineering": true,
}

// claimWords splits text into comparable words: lowercased, punctuation
// stripped, but keeping the characters that carry meaning inside technology
// names so "c++" and "c#" do not collapse into "c".
func claimWords(s string) []string {
	var out []string
	var cur strings.Builder

	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '+', r == '#':
			cur.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return out
}

func wordSet(s string) map[string]bool {
	set := make(map[string]bool)
	for _, w := range claimWords(s) {
		set[w] = true
	}
	return set
}

// UngroundedClaims returns the words in candidate that assert something the
// source does not, in order and without duplicates. An empty result means the
// candidate is safe to use.
func UngroundedClaims(source, candidate string) []string {
	have := wordSet(source)

	var out []string
	seen := make(map[string]bool)
	for _, w := range claimWords(candidate) {
		if have[w] || genericWords[w] || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	return out
}

// IsGrounded reports whether candidate only restates what source already says.
func IsGrounded(source, candidate string) bool {
	return len(UngroundedClaims(source, candidate)) == 0
}
