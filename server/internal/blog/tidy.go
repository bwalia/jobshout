package blog

import (
	"regexp"
	"strings"
	"time"
)

// Tidying is the last deterministic pass over a finished draft, after every
// model call and before HTML. It removes what a small model reliably leaves
// behind and a reader should never see: illustration requests nobody will
// draw, a section written twice, a paragraph pasted in again further down.
//
// It only removes. Rewording is the reviser's job; a pass that edits prose
// here would be making editorial calls no one reviewed.

// illustrationHeading is a heading introducing a picture ("### Illustration:
// The hiring flow"), which is empty once the picture is gone.
var illustrationHeading = regexp.MustCompile(`(?i)^#{2,6}\s+(illustration|figure|diagram)\b`)

// stripIllustrationFences removes ```illustration blocks, and a heading that
// only introduced one.
//
// With illustration off the writer is not offered the fence, but a small model
// writes one anyway, and nothing downstream drew it: it reached the CMS as a
// <pre> block of raw "participant C as Candidate" lines.
func stripIllustrationFences(markdown string) string {
	if !illustrationFence.MatchString(markdown) {
		return markdown
	}
	stripped := illustrationFence.ReplaceAllString(markdown, "")

	lines := strings.Split(stripped, "\n")
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		if illustrationHeading.MatchString(strings.TrimSpace(line)) && sectionEmptyAfter(lines, i) {
			continue
		}
		out = append(out, line)
	}
	return collapseBlankRuns(strings.Join(out, "\n"))
}

// sectionEmptyAfter reports whether the heading at i is followed only by blank
// lines before the next heading or the end.
func sectionEmptyAfter(lines []string, i int) bool {
	for _, l := range lines[i+1:] {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		return strings.HasPrefix(t, "#")
	}
	return true
}

// dropRepeats removes an H2 section that only repeats an earlier section under
// the same heading, and a paragraph that repeats an earlier one.
//
// Expansion asks a small model to lengthen a short draft, and it pads by
// pasting: a live run came back with two identical Conclusion sections and one
// anecdote four times. Only exact repeats (after whitespace) are dropped, and
// only prose — code, tables and lists repeat legitimately. A repeated heading
// with anything new under it is kept.
func dropRepeats(markdown string) string {
	blocks := splitBlocks(markdown)
	// seenSection maps a heading to its section's text, so a later section
	// under the same heading is dropped when it adds nothing to the first.
	seenSection := map[string]string{}
	seenPara := map[string]bool{}
	out := make([]string, 0, len(blocks))

	for i := 0; i < len(blocks); i++ {
		b := blocks[i]
		if isH2(strings.TrimSpace(b)) {
			// A section is its heading plus the blocks up to the next H2.
			j := i + 1
			for j < len(blocks) && !isH2(strings.TrimSpace(blocks[j])) {
				j++
			}
			heading := normalizeBlock(strings.SplitN(strings.TrimSpace(b), "\n", 2)[0])
			body := normalizeBlock(strings.Join(blocks[i:j], "\n\n"))
			if earlier, ok := seenSection[heading]; ok && strings.Contains(earlier, body) {
				i = j - 1
				continue
			}
			if _, ok := seenSection[heading]; !ok {
				seenSection[heading] = body
			}
			out = append(out, b)
			continue
		}
		if isRepeatableProse(b) {
			key := normalizeBlock(b)
			if seenPara[key] {
				continue
			}
			seenPara[key] = true
		}
		out = append(out, b)
	}
	return strings.Join(out, "\n\n")
}

// splitBlocks splits markdown on blank lines, keeping fenced code whole.
func splitBlocks(markdown string) []string {
	var blocks []string
	var cur []string
	inFence := false
	flush := func() {
		if len(cur) > 0 {
			blocks = append(blocks, strings.Join(cur, "\n"))
			cur = nil
		}
	}
	for _, line := range strings.Split(markdown, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		}
		if !inFence && strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		cur = append(cur, line)
	}
	flush()
	return blocks
}

// isRepeatableProse is a paragraph long enough that repeating it is padding,
// not a heading, code, table, list or quote.
func isRepeatableProse(block string) bool {
	t := strings.TrimSpace(block)
	if len(t) < 80 {
		return false
	}
	switch t[0] {
	case '#', '`', '|', '-', '*', '>':
		return false
	}
	return !(t[0] >= '0' && t[0] <= '9' && strings.Contains(t[:min(4, len(t))], "."))
}

func normalizeBlock(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// promptLabelHeading is a heading that starts with one of the writing prompt's
// own section labels ("## DIAGRAM: The screening flow"). The model echoes the
// label; the reader should see only the title after it.
var promptLabelHeading = regexp.MustCompile(`^(#{2,6}\s+)(?:DIAGRAMS?|TABLES?|ILLUSTRATIONS?|FIGURES?)\s*:\s*(\S.*)$`)

// stripPromptLabels drops prompt labels from headings.
func stripPromptLabels(markdown string) string {
	lines := strings.Split(markdown, "\n")
	inFence := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if !inFence {
			lines[i] = promptLabelHeading.ReplaceAllString(line, "$1$2")
		}
	}
	return strings.Join(lines, "\n")
}

// dropRepeatedDiagrams removes a mermaid diagram identical to an earlier one,
// and a heading left introducing nothing. Code blocks are left alone — the
// same command can belong in two steps — but the same diagram twice is
// padding: a live run drew one sequence diagram under two headings.
func dropRepeatedDiagrams(markdown string) string {
	seen := map[string]bool{}
	dropped := false
	out := mermaidFence.ReplaceAllStringFunc(markdown, func(fence string) string {
		key := normalizeBlock(fence)
		if seen[key] {
			dropped = true
			return ""
		}
		seen[key] = true
		return fence
	})
	if !dropped {
		return markdown
	}
	return removeEmptyHeadings(out)
}

// removeEmptyHeadings drops H3+ headings with nothing under them, and H2
// headings with nothing before the next H2.
func removeEmptyHeadings(markdown string) string {
	lines := strings.Split(markdown, "\n")
	out := make([]string, 0, len(lines))
	for i, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "## ") || strings.HasPrefix(t, "### ") || strings.HasPrefix(t, "#### ") {
			level := len(t) - len(strings.TrimLeft(t, "#"))
			if headingEmpty(lines[i+1:], level) {
				continue
			}
		}
		out = append(out, line)
	}
	return collapseBlankRuns(strings.Join(out, "\n"))
}

// headingEmpty reports whether nothing but blank lines comes before the next
// heading at the same or a higher level.
func headingEmpty(rest []string, level int) bool {
	for _, l := range rest {
		t := strings.TrimSpace(l)
		if t == "" {
			continue
		}
		if strings.HasPrefix(t, "#") {
			next := len(t) - len(strings.TrimLeft(t, "#"))
			if next <= level {
				return true
			}
		}
		return false
	}
	return true
}

// tidyMarkdown is the pass finalizeArticle runs. illustrated says whether the
// illustrator already turned fences into images.
func tidyMarkdown(markdown string, illustrated bool) string {
	if !illustrated {
		markdown = stripIllustrationFences(markdown)
	}
	markdown = stripPromptLabels(markdown)
	markdown = dropRepeatedDiagrams(markdown)
	return dropRepeats(markdown)
}

// modelReferencesHeading is a heading for a reference list the model wrote.
var modelReferencesHeading = regexp.MustCompile(`(?i)^#{2,3}\s+(references|sources|bibliography|citations|works cited)\s*$`)

// stripModelReferences removes a reference section the model wrote, from its
// heading to the next heading of the same or a higher level.
//
// The pipeline builds the reference list from what the article cites; a list
// the model writes is at best a duplicate and at worst — as on a live run — a
// dump of research findings with their quotes and two sources the model
// invented ("from: Nature").
func stripModelReferences(markdown string) string {
	lines := strings.Split(markdown, "\n")
	out := make([]string, 0, len(lines))
	skipping, level, inFence := false, 0, false
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(t, "#") {
			l := len(t) - len(strings.TrimLeft(t, "#"))
			if skipping && l <= level {
				skipping = false
			}
			if !skipping && modelReferencesHeading.MatchString(t) {
				skipping, level = true, l
				continue
			}
		}
		if !skipping {
			out = append(out, line)
		}
	}
	return collapseBlankRuns(strings.Join(out, "\n"))
}

// landscapeDate matches the dated opener the Insights reader asks for.
var landscapeDate = regexp.MustCompile(`(?i)(landscape reviewed:\s*)[^*_\n]+`)

// stampLandscapeDate writes the run's own date into the "AI landscape
// reviewed:" line. A live run dated its article 2023-09-28, three years before
// it was written.
func stampLandscapeDate(markdown string, now time.Time) string {
	return landscapeDate.ReplaceAllString(markdown, "${1}"+now.Format("2006-01-02"))
}

// trimIncompleteTail cuts a reply that stopped at the token ceiling back to
// its last complete block: an unclosed code fence goes, then a final paragraph
// that does not end a sentence, then any heading left with nothing under it.
func trimIncompleteTail(markdown string) string {
	// An odd number of fences means the last one was never closed.
	if strings.Count(markdown, "```")%2 == 1 {
		markdown = markdown[:strings.LastIndex(markdown, "```")]
	}
	blocks := splitBlocks(markdown)
	for len(blocks) > 0 && !blockComplete(blocks[len(blocks)-1]) {
		blocks = blocks[:len(blocks)-1]
	}
	return strings.TrimSpace(removeEmptyHeadings(strings.Join(blocks, "\n\n")))
}

// listItem matches a bullet or numbered line; list items rarely end a sentence.
var listItem = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s+\S`)

// blockComplete reports whether a block reads as finished. Headings, fenced
// code, tables and lists are taken whole; prose must end a sentence.
func blockComplete(block string) bool {
	t := strings.TrimSpace(block)
	if t == "" {
		return false
	}
	last := t[strings.LastIndex(t, "\n")+1:]
	switch {
	case strings.HasPrefix(t, "#"), strings.HasSuffix(t, "```"):
		return true
	case strings.HasPrefix(strings.TrimSpace(last), "|"):
		// A table row is complete when it closes its last cell.
		return strings.HasSuffix(strings.TrimSpace(last), "|")
	}
	if listItem.MatchString(last) {
		return true
	}
	end := strings.TrimRight(t, "*_)\"'`] ")
	if end == "" {
		return false
	}
	switch end[len(end)-1] {
	case '.', '!', '?', ':':
		return true
	}
	// A citation closes a sentence too: "…at a lower cost [2]".
	return citationMark.MatchString(t[max(0, len(t)-6):]) && strings.HasSuffix(t, "]")
}
