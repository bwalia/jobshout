package blog

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/imagestore"
)

// figureKinds is the figure kinds this runner can draw and the format to write
// them in. Every figure is a list of short labels, even for a lettering model:
// that is what a good figure shows, and it is what figureScene needs to draw it
// as a diagram when the model fails (int's Gemini has no image quota). A
// comparison has no faithful diagram, so it is offered only with a model.
func (r *Runner) figureKinds() (kinds, format string) {
	kinds = "flow, architecture, process, or concept"
	format = `   Write content as short labels in the article's own terms, 1-4 words each:
   flow or process as 3-5 steps "step → step → step"; architecture or concept
   as a core and 4-5 parts "core: part, part, part, part"`
	if r.canLetterImages() {
		kinds = "flow, comparison, architecture, process, or concept"
		format += `; comparison as
   "option vs option: aspect, aspect"`
	}
	return kinds, format + "."
}

// figureImage is a body figure this package wrote: a Markdown image on its own
// line, served from the image store at a relative or absolute URL.
var figureImage = regexp.MustCompile(`(?m)^!\[[^\]\n]*\]\((?:https?://[^\s)]*)?` +
	regexp.QuoteMeta(imagestore.URLPrefix) + `[^\s)]*\)[ \t]*\n?`)

// maxRedrawArticleChars bounds the article text a figure plan reads, so a long
// piece still fits a local model's context.
const maxRedrawArticleChars = 12000

// RedrawFigures replaces a finished article's in-body figures: the ones it has
// are removed, the model picks the sections a figure would teach most in and
// what to put on it, and each is drawn again. Notes say what was dropped.
func (r *Runner) RedrawFigures(ctx context.Context, orgID uuid.UUID, a *GeneratedArticle) ([]string, error) {
	if !r.canLetterFigures() {
		return nil, errors.New("blog: no image model or image store to draw figures with")
	}
	md := figureImage.ReplaceAllString(a.Markdown, "")
	plan, err := r.planFigures(ctx, a.Title, md)
	if err != nil {
		return nil, fmt.Errorf("blog: plan figures: %w", err)
	}
	md, notes := r.illustrateBody(ctx, orgID, a.RunID, insertPlannedFigures(md, plan))
	if len(plan.Figures) == 0 {
		notes = append(notes, "the model found no section worth a figure")
	}
	a.Markdown = tidyMarkdown(md, true)
	return notes, a.render()
}

// planFigures asks the structured model which sections of a finished article
// to illustrate, in the format the configured drawing path can draw.
func (r *Runner) planFigures(ctx context.Context, title, markdown string) (*writePlan, error) {
	headings := collectH2s(markdown)
	kinds, format := r.figureKinds()
	body := markdown
	if len(body) > maxRedrawArticleChars {
		body = body[:maxRedrawArticleChars]
	}
	prompt := fmt.Sprintf(`Choose the one or two sections of this article where a figure would teach the reader most, and say what the figure shows. Use only facts from the article; skip the references.

Each figure names a kind (%s), a section, and its content.
%s
"section" is a heading copied exactly from HEADINGS.

TITLE: %s
HEADINGS:
- %s

ARTICLE:
%s

Respond with JSON only: {"figures": [{"kind": "process", "section": "...", "content": "..."}]}`,
		kinds, format, title, strings.Join(headings, "\n- "), body)

	var out struct {
		Figures []struct {
			Kind    flexibleString `json:"kind"`
			Section flexibleString `json:"section"`
			Content flexibleString `json:"content"`
		} `json:"figures"`
	}
	if err := r.generateJSON(ctx, r.structuredModel(GenerateRequest{}), "figures", prompt, 800, &out); err != nil {
		return nil, err
	}
	plan := &writePlan{Title: title}
	for _, f := range out.Figures {
		plan.Figures = append(plan.Figures, figureBrief{Kind: string(f.Kind), Section: string(f.Section), Content: string(f.Content)})
	}
	return plan, nil
}

// insertPlannedFigures puts a fence after the first paragraph of each section
// the plan drew a figure for, and nowhere else: a section-prose figure is what
// RedrawFigures is replacing.
func insertPlannedFigures(markdown string, plan *writePlan) string {
	for range maxInlineIllustrations {
		lines := strings.Split(markdown, "\n")
		inserted := false
		for i, line := range lines {
			t := strings.TrimSpace(line)
			if !isH2(t) || sectionHasIllustrationSoon(lines, i) {
				continue
			}
			if kind, content, ok := plannedFigure(plan, strings.TrimSpace(strings.TrimPrefix(t, "## "))); ok {
				markdown = insertFenceAfter(lines, lineAfterFirstParagraph(lines, i), kind, content)
				inserted = true
				break
			}
		}
		if !inserted {
			break
		}
	}
	return markdown
}
