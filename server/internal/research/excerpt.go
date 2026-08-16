package research

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Extraction sees a bounded slice of each document rather than the whole thing.
// Which slice matters more than it looks: taking the first N characters assumes
// a page opens with its substance, and fetched pages routinely do not. The
// reader output for ebpf.io spends its first 6,000 characters on a navigation
// menu and a table of contents repeated twice — an extractor handed that window
// is being asked to find citable facts in a list of links, and reports back
// that the source established nothing. The failure is silent and looks
// identical to a genuinely irrelevant source.
//
// So the window is chosen rather than assumed: the densest run of prose in the
// document, measured by how much of each line survives once markdown link and
// image syntax is removed.

var (
	// ![alt](url) — dropped entirely; alt text is not prose.
	markdownImage = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	// [text](url) — the URL is markup, the text may be prose.
	markdownLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
)

// A line counts as prose only if it is long enough and has enough words to be a
// sentence rather than a label. Both thresholds are deliberately modest: the
// point is to separate "Get Started" and "What is eBPF?" from actual writing,
// not to grade the writing.
const (
	proseMinChars = 40
	proseMinWords = 8
)

// readableText strips the markup from a line, leaving what a person would read.
func readableText(line string) string {
	s := markdownImage.ReplaceAllString(line, "")
	s = markdownLink.ReplaceAllString(s, "$1")
	// Leading list bullets, heading hashes, blockquote markers and table pipes
	// are structure, not content.
	s = strings.TrimLeft(strings.TrimSpace(s), "#*->+|=_ \t")
	return strings.TrimSpace(s)
}

// proseScore is how much readable prose a line contributes. Non-prose lines
// score zero rather than a little, so a long run of links cannot outweigh a
// short run of sentences.
func proseScore(line string) int {
	s := readableText(line)
	if utf8.RuneCountInString(s) < proseMinChars {
		return 0
	}
	if len(strings.Fields(s)) < proseMinWords {
		return 0
	}
	return utf8.RuneCountInString(s)
}

// proseExcerpt returns the contiguous run of at most limit runes containing the
// most prose.
//
// The window is contiguous, and that is a requirement rather than a convenience:
// extraction must quote the source verbatim, and those quotes are later checked
// against the document. Assembling an excerpt out of non-adjacent lines would
// let the model quote across a seam — a passage that reads as continuous here
// but appears nowhere in the real page, and so fails verification through no
// fault of the model.
//
// When nothing in the document looks like prose the head of the document is
// returned, which is what the caller would have used anyway.
func proseExcerpt(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(text) <= limit {
		return text
	}

	lines := strings.Split(text, "\n")
	size := make([]int, len(lines))
	score := make([]int, len(lines))
	for i, ln := range lines {
		size[i] = utf8.RuneCountInString(ln) + 1 // the newline rejoining it
		score[i] = proseScore(ln)
	}

	// Widen the window one line at a time, pulling the tail forward whenever it
	// no longer fits. Ties keep the earliest window: a page's opening is more
	// often its thesis than its appendix.
	best, bestStart, bestEnd := 0, 0, 0
	sum, runes, start := 0, 0, 0
	for end := range lines {
		sum += score[end]
		runes += size[end]
		for start < end && runes > limit {
			sum -= score[start]
			runes -= size[start]
			start++
		}
		if sum > best {
			best, bestStart, bestEnd = sum, start, end
		}
	}

	if best == 0 {
		return truncate(text, limit)
	}
	window := trimLinkEdges(lines[bestStart : bestEnd+1])
	return truncate(strings.TrimSpace(strings.Join(window, "\n")), limit)
}

// isLinkLine reports whether a line is navigation rather than content: it
// carries a link and contributes no prose. Headings are deliberately not
// included — a heading scores zero but tells the extractor what the passage
// below it is about, which is worth the handful of characters.
func isLinkLine(line string) bool {
	return proseScore(line) == 0 && markdownLink.MatchString(line)
}

// trimLinkEdges drops navigation lines from both ends of the chosen window.
//
// The window is picked to maximise the prose inside it, which leaves whatever
// non-prose happened to sit within budget at its edges. Those lines are noise
// in the prompt and characters taken from the budget, and removing them from
// the ends keeps the window contiguous.
func trimLinkEdges(lines []string) []string {
	start, end := 0, len(lines)
	for start < end && (strings.TrimSpace(lines[start]) == "" || isLinkLine(lines[start])) {
		start++
	}
	for end > start && (strings.TrimSpace(lines[end-1]) == "" || isLinkLine(lines[end-1])) {
		end--
	}
	return lines[start:end]
}
