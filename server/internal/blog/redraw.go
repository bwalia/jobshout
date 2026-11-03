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
	format = `   Write content as short labels in the article's own terms, 1-4 words each,
   naming the real things, never placeholder words like "core" or "step":
   flow or process as 3-5 steps, e.g. "Commit → Build image → Run tests → Deploy";
   architecture or concept as the centre and its 4-5 parts, e.g.
   "API gateway: auth service, orders service, billing service, Redis cache"`
	if r.canLetterImages() {
		kinds = "flow, comparison, architecture, process, or concept"
		format += `;
   comparison as the options and what they differ on, e.g.
   "Polling vs webhooks: latency, cost, complexity"`
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

// RedrawResult is what a redraw changed, so the caller can say so honestly
// rather than reporting success for a redraw that kept everything.
type RedrawResult struct {
	Notes   []string
	Figures int  // figures drawn into the body
	Cover   bool // a new cover was stored
}

// RedrawImages redraws a finished article's cover and in-body figures from its
// content. One model call reads the article and decides both: the cover layout
// whose meaning matches the article's central claim, labelled with its own
// terms, and the one or two sections a figure teaches most in. The figures it
// had are kept when none can be drawn; the cover is kept when no new one can
// be stored.
func (r *Runner) RedrawImages(ctx context.Context, orgID uuid.UUID, a *GeneratedArticle) (RedrawResult, error) {
	var res RedrawResult
	if !r.canLetterFigures() && !r.canDrawCover() {
		return res, errors.New("blog: no image model or image store to draw with")
	}
	md := figureImage.ReplaceAllString(a.Markdown, "")
	plan, scene, err := r.planRedraw(ctx, a.Title, md)
	if err != nil {
		return res, fmt.Errorf("blog: plan the redraw: %w", err)
	}

	if r.canLetterFigures() {
		body, notes := r.illustrateBody(ctx, orgID, a.RunID, insertPlannedFigures(md, plan))
		res.Notes = notes
		res.Figures = len(figureImage.FindAllString(body, -1))
		switch {
		case len(plan.Figures) == 0:
			res.Notes = append(res.Notes, "the model found no section worth a figure")
		case res.Figures == 0 && figureImage.MatchString(a.Markdown):
			// On int a full image store turned one figure into none.
			res.Notes = append(res.Notes, "kept the article's figures: none could be redrawn")
		}
		if res.Figures > 0 || !figureImage.MatchString(a.Markdown) {
			a.Markdown = tidyMarkdown(body, true)
			if err := a.render(); err != nil {
				return res, err
			}
		}
	}

	// The model's scene, held to the article's own words; an empty one falls
	// back to the heuristic in coverSceneFor.
	a.CoverMetaphor, a.CoverObjects, a.CoverAccent = "", "", ""
	a.CoverScene = groundScene(scene, a.Markdown)
	if strings.TrimSpace(a.CoverAngle) == "" {
		a.CoverAngle = strings.TrimSpace(a.Excerpt)
	}
	if err := r.redrawCover(ctx, orgID, a); err != nil {
		res.Notes = append(res.Notes, "kept the cover: "+truncateRunes(err.Error(), 200))
	} else {
		res.Cover = true
	}
	return res, nil
}

// planRedraw asks the structured model, once, for a finished article's cover
// diagram and figures.
func (r *Runner) planRedraw(ctx context.Context, title, markdown string) (*writePlan, coverScene, error) {
	headings := collectH2s(markdown)
	kinds, format := r.figureKinds()
	body := markdown
	if len(body) > maxRedrawArticleChars {
		body = body[:maxRedrawArticleChars]
	}
	prompt := fmt.Sprintf(`Read this finished article and plan its pictures. Use only what the article says; skip the references.

1. A cover diagram of the article's central claim. Pick the one layout below whose meaning matches what the article argues. Do not default to flow: use flow only when the article's point is a sequence of steps. Give 2-6 labels of 1-3 words each, copied from the article's own terms (the things it names, not section titles or generic words), a "stat" only when the article states that exact figure (otherwise ""), and an "eyebrow" of 1-3 words naming the field.
%s

2. The one or two sections where a figure would teach the reader most. Each figure names a kind (%s), a section, and its content.
%s
"section" is a heading copied exactly from HEADINGS.

TITLE: %s
HEADINGS:
- %s

ARTICLE:
%s

Respond with JSON only: {"cover_scene": {"layout": "...", "labels": ["...", "..."], "stat": "", "eyebrow": "..."}, "figures": [{"kind": "process", "section": "...", "content": "..."}]}`,
		coverLayoutGuide, kinds, format, title, strings.Join(headings, "\n- "), body)

	var out struct {
		CoverScene coverScene `json:"cover_scene"`
		Figures    []struct {
			Kind    flexibleString `json:"kind"`
			Section flexibleString `json:"section"`
			Content flexibleString `json:"content"`
		} `json:"figures"`
	}
	if err := r.generateJSON(ctx, r.structuredModel(GenerateRequest{}), "redraw", prompt, 1000, &out); err != nil {
		return nil, coverScene{}, err
	}
	plan := &writePlan{Title: title}
	for _, f := range out.Figures {
		plan.Figures = append(plan.Figures, figureBrief{Kind: string(f.Kind), Section: string(f.Section), Content: string(f.Content)})
	}
	return plan, out.CoverScene, nil
}

// groundScene keeps the cover labels the article supports: each must share a
// word of four or more letters with it, so the cover cannot claim what the
// piece does not say. A stat stays only when the article states it. With
// fewer than two labels left the scene is dropped and the cover is derived
// from the article instead.
func groundScene(s coverScene, markdown string) coverScene {
	text := strings.ToLower(markdown)
	var kept []string
	for _, l := range s.Labels {
		for _, w := range strings.Fields(strings.ToLower(l)) {
			if w = strings.Trim(w, ".,:;()\"'"); len([]rune(w)) >= 4 && strings.Contains(text, w) {
				kept = append(kept, l)
				break
			}
		}
	}
	if len(kept) < 2 {
		return coverScene{}
	}
	s.Labels = kept
	if stat := strings.TrimLeft(strings.TrimSpace(s.Stat), "+-−"); stat != "" && !strings.Contains(markdown, stat) {
		s.Stat = ""
	}
	return s
}

// insertPlannedFigures puts a fence after the first paragraph of each section
// the plan drew a figure for, and nowhere else: a section-prose figure is what
// RedrawImages is replacing.
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
