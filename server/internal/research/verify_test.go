package research

import "testing"

const verifyDoc = `The Gateway API graduated to GA in Kubernetes 1.31, replacing the
long-standing Ingress resource for most new deployments. Unlike Ingress, it separates
the concerns of cluster operators and application developers into distinct resource
types. Adoption has been gradual: the ingress2gateway tool exists specifically to ease
migration for teams with large existing Ingress estates.`

func TestQuoteSupported_AcceptsGenuineQuotes(t *testing.T) {
	tests := []struct {
		name  string
		quote string
	}{
		{
			name:  "verbatim",
			quote: "The Gateway API graduated to GA in Kubernetes 1.31, replacing the long-standing Ingress resource",
		},
		{
			name:  "whitespace collapsed differently",
			quote: "it separates   the concerns of cluster operators\nand application developers",
		},
		{
			name:  "case changed",
			quote: "ADOPTION HAS BEEN GRADUAL: THE INGRESS2GATEWAY TOOL EXISTS SPECIFICALLY TO EASE MIGRATION",
		},
		{
			name:  "curly quotes and em dash normalised",
			quote: "Unlike Ingress — it separates the concerns of cluster operators and application developers",
		},
		{
			name:  "trailing punctuation differs",
			quote: "the ingress2gateway tool exists specifically to ease migration for teams.",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !quoteSupported(verifyDoc, tt.quote) {
				t.Errorf("rejected a genuine quote: %q", tt.quote)
			}
		})
	}
}

// These are the cases the check exists for. A model reconstructing a
// plausible-sounding source rather than reading one produces exactly this: text
// that reads like the document and is not in it.
func TestQuoteSupported_RejectsFabrications(t *testing.T) {
	tests := []struct {
		name  string
		quote string
	}{
		{
			name:  "plausible but absent",
			quote: "The Gateway API was designed from the outset to replace Ingress entirely by the 1.30 release",
		},
		{
			name:  "right topic wrong facts",
			quote: "Gateway API reached general availability in Kubernetes 1.29 after three years in beta",
		},
		{
			name:  "entirely unrelated",
			quote: "Postgres 17 introduces incremental backups and improved vacuum performance for large tables",
		},
		{
			name:  "empty",
			quote: "",
		},
		{
			name:  "too short to be evidence",
			quote: "Gateway API",
		},
		{
			name:  "single common word",
			quote: "the",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if quoteSupported(verifyDoc, tt.quote) {
				t.Errorf("accepted a fabricated quote: %q", tt.quote)
			}
		})
	}
}

func TestQuoteSupported_EmptyDocumentRejects(t *testing.T) {
	if quoteSupported("", "any quote at all here") {
		t.Error("accepted a quote against an empty document")
	}
}

// A quote that is mostly real but has had a clause inserted must fail: that is
// the shape of a claim being sharpened beyond what the source says.
func TestQuoteSupported_RejectsInsertedClause(t *testing.T) {
	quote := "The Gateway API graduated to GA in Kubernetes 1.31, and is now mandatory for all clusters, replacing the long-standing Ingress resource"
	if quoteSupported(verifyDoc, quote) {
		t.Error("accepted a quote with an invented clause spliced into it")
	}
}

func TestNormaliseForCompare(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"lowercases", "Hello World", "hello world"},
		{"collapses whitespace", "a\n\n  b\tc", "a b c"},
		{"punctuation becomes separator", "state-of-the-art", "state of the art"},
		{"curly apostrophe folded", "it’s here", "it s here"},
		{"em dash folded", "a — b", "a b"},
		{"keeps digits", "version 1.31", "version 1 31"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normaliseForCompare(tt.in); got != tt.want {
				t.Errorf("normaliseForCompare(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestShingles(t *testing.T) {
	got := shingles([]string{"a", "b", "c", "d"}, 2)
	want := []string{"a b", "b c", "c d"}

	if len(got) != len(want) {
		t.Fatalf("got %d shingles, want %d", len(got), len(want))
	}
	for _, w := range want {
		if _, ok := got[w]; !ok {
			t.Errorf("missing shingle %q", w)
		}
	}
}

func TestShingles_ShorterThanWindow(t *testing.T) {
	if got := shingles([]string{"a", "b"}, 5); got != nil {
		t.Errorf("got %v, want nil for input shorter than the window", got)
	}
}
