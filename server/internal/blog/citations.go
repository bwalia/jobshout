package blog

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
)

// citationPattern matches an inline citation marker like [3].
//
// The negative lookahead cannot be expressed in Go's regexp, so a trailing "("
// is captured and checked by the caller instead: "[2](https://…)" is a markdown
// link whose text happens to be a number, not a citation, and rewriting it
// would break the link.
var citationPattern = regexp.MustCompile(`\[(\d+)\](\(?)`)

// resolveCitations rewrites a draft's citation markers into a contiguous
// sequence and returns the reference list they point to.
//
// Three things happen here, and all three are corrections to what the model
// produced rather than formatting:
//
//   - Citations to a source number that does not exist are removed. A model
//     that writes [9] when eight sources were offered has cited nothing, and
//     leaving the marker would imply a reference that cannot be printed.
//   - Sources that are never cited do not appear in the reference list. A
//     reference list is what the article rests on; padding it with everything
//     the researcher read is how "references" stop meaning anything.
//   - The surviving citations are renumbered in order of first appearance, so
//     the article reads [1], [2], [3] rather than [2], [7], [4].
//
// It returns the rewritten markdown and the references, in citation order.
func resolveCitations(markdown string, brief *research.Brief) (string, []model.BlogReference) {
	if brief == nil || len(brief.Findings) == 0 {
		return stripAllCitations(markdown), nil
	}

	// Map each source URL to the reference describing it, so several findings
	// drawn from the same page collapse to one entry.
	byURL := make(map[string]model.BlogReference, len(brief.Sources))
	for _, s := range brief.Sources {
		byURL[s.URL] = model.BlogReference{
			URL:         s.URL,
			Title:       s.Title,
			Site:        s.Site,
			PublishedAt: s.PublishedAt,
		}
	}

	var (
		refs     []model.BlogReference
		newIndex = make(map[string]int) // source URL → its number in the output
	)

	rewritten := citationPattern.ReplaceAllStringFunc(markdown, func(match string) string {
		parts := citationPattern.FindStringSubmatch(match)
		// A trailing "(" means this is a markdown link, not a citation.
		if parts[2] == "(" {
			return match
		}

		n, err := strconv.Atoi(parts[1])
		if err != nil || n < 1 || n > len(brief.Findings) {
			return "" // cites a source that was never offered
		}

		url := brief.Findings[n-1].SourceURL
		ref, known := byURL[url]
		if !known {
			return ""
		}

		if idx, seen := newIndex[url]; seen {
			return fmt.Sprintf("[%d]", idx)
		}
		idx := len(refs) + 1
		newIndex[url] = idx
		refs = append(refs, ref)
		return fmt.Sprintf("[%d]", idx)
	})

	return tidySpacing(rewritten), refs
}

// modelRefsHeading matches a References / Further Reading / Bibliography heading
// the model wrote itself, and everything after it to the end of the document.
//
// The heading text has to be exactly one of those phrases, so a section called
// "References to prior art" is left alone. It is matched anywhere rather than
// only at the start of a line because a local model routinely glues the heading
// to the end of the preceding sentence — "...into L1 cache. ## References".
var modelRefsHeading = regexp.MustCompile(`(?is)#{1,6}[ \t]*(?:references|further reading|bibliography)[ \t]*(?:\r?\n|$).*$`)

// stripModelReferences removes a references section the model produced against
// instructions.
//
// The draft prompt forbids a hand-written reference list — the real one is
// generated from resolved citations and appended by the caller. A model that
// writes one anyway leaves the article with two "## References" headings, the
// first full of markers and raw URLs the model invented. Cutting it here, before
// citations resolve, keeps those invented markers out of the renumbering too.
func stripModelReferences(markdown string) string {
	loc := modelRefsHeading.FindStringIndex(markdown)
	if loc == nil {
		return markdown
	}
	return strings.TrimRight(markdown[:loc[0]], " \t\r\n")
}

// stripAllCitations removes every citation marker, used when there is no brief
// to resolve them against. Markdown links are left alone.
func stripAllCitations(markdown string) string {
	return tidySpacing(citationPattern.ReplaceAllStringFunc(markdown, func(match string) string {
		parts := citationPattern.FindStringSubmatch(match)
		if parts[2] == "(" {
			return match
		}
		return ""
	}))
}

// referencesMarkdown renders the reference list appended to an article.
//
// It is generated rather than written by the model for the same reason the
// citations are numbers: a model asked to write out a URL will produce one that
// looks plausible. Every URL here came back from a successful fetch.
func referencesMarkdown(refs []model.BlogReference) string {
	if len(refs) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n## References\n\n")
	for i, ref := range refs {
		title := strings.TrimSpace(ref.Title)
		if title == "" {
			title = ref.URL
		}
		fmt.Fprintf(&b, "%d. [%s](%s)", i+1, title, ref.URL)
		if ref.Site != "" {
			fmt.Fprintf(&b, " — %s", ref.Site)
		}
		if ref.PublishedAt != nil {
			fmt.Fprintf(&b, " (%s)", ref.PublishedAt.Format("2 January 2006"))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// spacingPattern matches the whitespace left where a citation was removed:
// a space before punctuation, or a doubled space mid-sentence.
var (
	spaceBeforePunct = regexp.MustCompile(`[ \t]+([.,;:!?)])`)
	doubledSpace     = regexp.MustCompile(`([^\n]) {2,}`)
)

// tidySpacing repairs the gaps left by removed citation markers, so a dropped
// citation does not leave "the API was released ." behind.
func tidySpacing(s string) string {
	s = spaceBeforePunct.ReplaceAllString(s, "$1")
	s = doubledSpace.ReplaceAllString(s, "$1 ")
	return s
}

// countCitations reports how many resolvable citation markers a draft contains.
// Used to tell an article that cited its sources from one that ignored them.
func countCitations(markdown string) int {
	n := 0
	for _, m := range citationPattern.FindAllStringSubmatch(markdown, -1) {
		if m[2] != "(" {
			n++
		}
	}
	return n
}
