package research

import "strings"

// quoteShingleSize is the word-window used for the fuzzy comparison. Five words
// is long enough that matching windows are not coincidental — shared five-word
// runs between an invented quote and an unrelated document are rare — and short
// enough to survive a model tidying up punctuation mid-sentence.
const quoteShingleSize = 5

// quoteOverlapThreshold is the fraction of a quote's word-windows that must
// appear in the document for the quote to count as present.
//
// It is set high on purpose. This check exists to catch fabrication, and the
// asymmetry matters: dropping a real quote costs one finding, while accepting
// an invented one puts a fabricated citation in a published article.
const quoteOverlapThreshold = 0.8

// quoteSupported reports whether quote genuinely appears in docText.
//
// This is the check that makes the difference between a citation the agent
// asserts and one it can be held to. A model asked to copy a passage verbatim
// usually does, but not always — and the cases where it does not are exactly
// the cases where it is reconstructing a plausible-sounding source rather than
// reading one. Because both strings are in hand, that can be settled by
// comparison rather than by asking another model to adjudicate.
//
// Exact matching alone would be too brittle: extraction routinely normalises
// curly quotes, collapses a line break inside a sentence, or drops a stray
// markdown artefact from the middle of a passage. So a normalised substring
// match is tried first, and a windowed-overlap comparison second.
func quoteSupported(docText, quote string) bool {
	doc := normaliseForCompare(docText)
	q := normaliseForCompare(quote)

	if q == "" || doc == "" {
		return false
	}

	// A quote short enough to be a coincidence is not evidence of anything.
	// Requiring a handful of words also stops a single common word from
	// "verifying" a claim.
	qWords := strings.Fields(q)
	if len(qWords) < quoteShingleSize {
		return false
	}

	if strings.Contains(doc, q) {
		return true
	}

	// Fall back to comparing word windows, which tolerates small edits inside
	// the passage while still requiring most of it to be genuinely present.
	docWindows := shingles(strings.Fields(doc), quoteShingleSize)
	if len(docWindows) == 0 {
		return false
	}
	qWindows := shingles(qWords, quoteShingleSize)
	if len(qWindows) == 0 {
		return false
	}

	matched := 0
	for w := range qWindows {
		if _, ok := docWindows[w]; ok {
			matched++
		}
	}
	return float64(matched)/float64(len(qWindows)) >= quoteOverlapThreshold
}

// normaliseForCompare reduces text to what the comparison should be sensitive
// to: the words and their order. Case, whitespace shape, and the punctuation
// that differs between a page and a model's transcription of it are removed.
func normaliseForCompare(s string) string {
	var b strings.Builder
	b.Grow(len(s))

	// Typography is folded first: a curly quote is non-ASCII and would survive
	// the loop below untouched, so replacing it afterwards would leave an
	// apostrophe the loop never got the chance to turn into a separator.
	for _, r := range strings.ToLower(typography.Replace(s)) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r > 127:
			// Keep non-ASCII letters — dropping them would mangle a quote from
			// a non-English source into unrecognisability.
			b.WriteRune(r)
		default:
			// Every ASCII separator and punctuation mark becomes a space, so
			// "state-of-the-art" and "state of the art" compare equal.
			b.WriteRune(' ')
		}
	}

	// Collapse the runs of spaces the substitution above produced.
	return strings.Join(strings.Fields(b.String()), " ")
}

// typography folds the characters that most often differ between a rendered
// page and a model's copy of it into their ASCII equivalents.
var typography = strings.NewReplacer(
	"‘", "'", "’", "'", // curly single quotes
	"“", `"`, "”", `"`, // curly double quotes
	"–", "-", "—", "-", // en and em dashes
	" ", " ", // non-breaking space
	"…", "...", // ellipsis
)

// shingles returns the set of n-word windows in words.
func shingles(words []string, n int) map[string]struct{} {
	if len(words) < n {
		return nil
	}
	out := make(map[string]struct{}, len(words)-n+1)
	for i := 0; i+n <= len(words); i++ {
		out[strings.Join(words[i:i+n], " ")] = struct{}{}
	}
	return out
}
