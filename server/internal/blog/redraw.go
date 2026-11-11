package blog

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
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
	// The examples come from a bakery so a copied one cannot pass for the
	// article: Gemini put "API gateway: auth service, orders service…" into an
	// API-gateway article that never mentions orders.
	format = `   Write content as short labels in the article's own terms, 1-4 words each,
   naming the real things, never placeholder words like "core" or "step".
   The shapes, shown from an unrelated field whose words must not be reused:
   flow or process as 3-5 steps, e.g. "Mix dough → Proof → Shape loaves → Bake";
   architecture or concept as the centre and its 4-5 parts, e.g.
   "Bakery: oven, flour store, packing line, delivery van"`
	if r.canLetterImages() {
		kinds = "flow, comparison, architecture, process, or concept"
		format += `;
   comparison as the options and what they differ on, e.g.
   "Sourdough vs yeast: rise time, flavour, cost"`
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
		if res.Figures == 0 && figureImage.MatchString(a.Markdown) {
			// None redrawn (on int a full image store turned one figure into
			// none): keep the figures it had, less those it does not support.
			var dropped int
			body, dropped = pruneMisfitFigures(a.Markdown, md)
			if dropped > 0 {
				res.Notes = append(res.Notes, fmt.Sprintf("removed %d old figure(s) the article does not support", dropped))
			}
		}
		switch {
		case len(plan.Figures) == 0:
			res.Notes = append(res.Notes, "the model found no section worth a figure")
		case res.Figures == 0 && figureImage.MatchString(body):
			res.Notes = append(res.Notes, "kept the article's figures: none could be redrawn")
		}
		if body != a.Markdown {
			a.Markdown = tidyMarkdown(body, true)
			if err := a.render(); err != nil {
				return res, err
			}
		}
	}

	// The model's scene, held to the article's own words; an empty one falls
	// back to the heuristic in coverSceneFor.
	a.CoverAccent = ""
	a.CoverScene = r.groundedCover(ctx, a.Title, md, scene)
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
THINGS THE ARTICLE NAMES (take cover and figure labels from these): %s

ARTICLE:
%s

Respond with JSON only: {"cover_scene": {"layout": "...", "labels": ["...", "..."], "stat": "", "eyebrow": "..."}, "figures": [{"kind": "process", "section": "...", "content": "..."}]}`,
		coverLayoutGuide, kinds, format, title, strings.Join(headings, "\n- "), strings.Join(articleTerms(markdown, 12), ", "), body)

	var out struct {
		CoverScene coverScene   `json:"cover_scene"`
		Figures    []figureJSON `json:"figures"`
	}
	if err := r.generateJSON(ctx, r.structuredModel(GenerateRequest{}), "redraw", prompt, 1000, &out); err != nil {
		return nil, coverScene{}, err
	}
	plan := &writePlan{Title: title}
	var misfits []string
	plan.Figures, misfits = fitFigures(out.Figures, markdown)
	if len(plan.Figures) == 0 && len(misfits) > 0 {
		plan.Figures = r.replanFigures(ctx, title, markdown, headings, misfits)
	}
	return plan, out.CoverScene, nil
}

type figureJSON struct {
	Kind    flexibleString `json:"kind"`
	Section flexibleString `json:"section"`
	Content flexibleString `json:"content"`
}

// fitFigures splits planned figures into those the article supports and, for
// the rest, why not.
func fitFigures(in []figureJSON, markdown string) (fit []figureBrief, misfits []string) {
	for _, f := range in {
		if why := figureMisfit(string(f.Content), markdown); why != "" {
			misfits = append(misfits, fmt.Sprintf("%q: %s", string(f.Content), why))
			continue
		}
		fit = append(fit, figureBrief{Kind: string(f.Kind), Section: string(f.Section), Content: string(f.Content)})
	}
	return fit, misfits
}

// replanFigures asks once more, saying why, when no planned figure fits the
// article — groundedCover's retry, for figures. Without it a redraw whose
// plan copied the prompt's example drew nothing and kept the old figures.
func (r *Runner) replanFigures(ctx context.Context, title, markdown string, headings, misfits []string) []figureBrief {
	kinds, format := r.figureKinds()
	body := markdown
	if len(body) > maxRedrawArticleChars {
		body = body[:maxRedrawArticleChars]
	}
	prompt := fmt.Sprintf(`The figures planned for this article cannot be drawn:
- %s

Plan them again: the one or two sections where a figure would teach the reader most. Each figure names a kind (%s), a section, and its content.
%s
Every label is something this article names. "section" is a heading copied exactly from HEADINGS.

TITLE: %s
HEADINGS:
- %s
THINGS THE ARTICLE NAMES: %s

ARTICLE:
%s

Respond with JSON only: {"figures": [{"kind": "process", "section": "...", "content": "..."}]}`,
		strings.Join(misfits, "\n- "), kinds, format, title, strings.Join(headings, "\n- "), strings.Join(articleTerms(markdown, 12), ", "), body)
	var out struct {
		Figures []figureJSON `json:"figures"`
	}
	if err := r.generateJSON(ctx, r.structuredModel(GenerateRequest{}), "redraw-figures", prompt, 600, &out); err != nil {
		return nil
	}
	fit, _ := fitFigures(out.Figures, markdown)
	return fit
}

// pruneMisfitFigures drops the article's figures whose alt text — the
// description each was drawn from — the article does not support.
func pruneMisfitFigures(markdown, text string) (string, int) {
	n := 0
	out := figureImage.ReplaceAllStringFunc(markdown, func(img string) string {
		alt := img[2:strings.Index(img, "](")]
		if figureMisfit(alt, text) == "" {
			return img
		}
		n++
		return ""
	})
	return out, n
}

// layoutMinLabels is how many real labels each layout needs before it fills
// the rest with its own wording ("teams", "data", "input"): a model scene with
// fewer would put words on the cover the article never uses.
var layoutMinLabels = map[coverLayout]int{
	layoutFlow: 3, layoutHub: 4, layoutLoop: 3, layoutBars: 3, layoutVersus: 2,
	layoutRipple: 3, layoutTree: 1, layoutIceberg: 4, layoutHierarchy: 4, layoutThreshold: 4,
}

// groundScene keeps the cover labels the article supports: each must share a
// word of four or more letters with it, so the cover cannot claim what the
// piece does not say, and a stat stays only when the article states it. ok is
// false when the layout is unknown or too few labels are left to draw it
// without filler; rejected are the labels the article does not contain.
func groundScene(s coverScene, markdown string) (scene coverScene, rejected []string, ok bool) {
	text := strings.ToLower(markdown)
	var kept []string
	for _, l := range s.Labels {
		grounded := false
		for _, w := range strings.Fields(strings.ToLower(l)) {
			if w = strings.Trim(w, ".,:;()\"'"); len([]rune(w)) >= 4 && strings.Contains(text, w) {
				grounded = true
				break
			}
		}
		if grounded {
			kept = append(kept, l)
		} else {
			rejected = append(rejected, l)
		}
	}
	s.Labels = kept
	if stat := strings.TrimLeft(strings.TrimSpace(s.Stat), "+-−"); stat != "" && !strings.Contains(markdown, stat) {
		s.Stat = ""
	}
	need, known := layoutMinLabels[s.Layout]
	return s, rejected, known && len(kept) >= need
}

// groundedCover is the model's cover scene held to the article. When it cannot
// be drawn without filler, the model is asked once more, told why; when that
// fails too, the cover is a hub of the things the article names, and with too
// few of those the scene is empty and coverSceneFor derives one.
func (r *Runner) groundedCover(ctx context.Context, title, markdown string, scene coverScene) coverScene {
	s, rejected, ok := groundScene(scene, markdown)
	if ok {
		return s
	}
	why := "its layout is not one of those listed"
	if need, known := layoutMinLabels[s.Layout]; known {
		why = fmt.Sprintf("the %s layout needs at least %d labels found in the article, and %d were", s.Layout, need, len(s.Labels))
		if len(rejected) > 0 {
			why += fmt.Sprintf(" (not in the article: %s)", strings.Join(rejected, ", "))
		}
	}
	body := markdown
	if len(body) > maxRedrawArticleChars {
		body = body[:maxRedrawArticleChars]
	}
	prompt := fmt.Sprintf(`The cover diagram planned for this article cannot be drawn: %s. Plan it again.

Pick the one layout below whose meaning matches what the article argues (flow only when its point is a sequence of steps), and give the labels that layout needs, chosen only from THINGS THE ARTICLE NAMES, a "stat" only when the article states that exact figure, and an "eyebrow" of 1-3 words naming the field.
%s

TITLE: %s
THINGS THE ARTICLE NAMES: %s
ARTICLE:
%s

Respond with JSON only: {"layout": "...", "labels": ["...", "..."], "stat": "", "eyebrow": "..."}`,
		why, coverLayoutGuide, title, strings.Join(articleTerms(markdown, 12), ", "), body)
	var retry coverScene
	if err := r.generateJSON(ctx, r.structuredModel(GenerateRequest{}), "redraw-cover", prompt, 400, &retry); err == nil {
		if s, _, ok := groundScene(retry, markdown); ok {
			return s
		}
	}
	// Still nothing drawable: a hub of what the article names, which cannot
	// be off-topic, rather than its headings.
	if s, ok := termsHub(title, markdown); ok {
		return s
	}
	return coverScene{}
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

var (
	termRun = regexp.MustCompile(`[A-Z][A-Za-z0-9+]*(?:[-/][A-Za-z0-9+]+)*(?: [A-Z][A-Za-z0-9+]*(?:[-/][A-Za-z0-9+]+)*){0,2}`)
	mdLink  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	wordRe  = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+]*(?:[-/][A-Za-z0-9+]+)*`)
)

// articleTerms lists the things an article names — products, technologies,
// concepts such as "Kusk Gateway", "ReplicaSet", "OpenAPI" — most mentioned
// first, from its prose (not headings, code or references). A lone
// capitalised word counts only when it is CamelCase or an acronym, or when the
// article never writes it in lower case and uses it mid-sentence at least
// once: that keeps sentence starters like "Create" or "However" out.
func articleTerms(markdown string, max int) []string {
	text := articleProse(markdown)
	plain := map[string]bool{}
	for _, w := range wordRe.FindAllString(text, -1) {
		if w == strings.ToLower(w) {
			plain[w] = true
		}
	}
	count := map[string]int{}
	mid := map[string]bool{}
	var order []string
	for _, loc := range termRun.FindAllStringIndex(text, -1) {
		at, words := loc[0], strings.Fields(text[loc[0]:loc[1]])
		// Drop a starter glued to a name: "Use Kusk Gateway" is Kusk Gateway.
		for len(words) > 1 && (labelStop[strings.ToLower(words[0])] || plain[strings.ToLower(words[0])]) {
			at += len(words[0]) + 1
			words = words[1:]
		}
		term := strings.Join(words, " ")
		if len(words) == 1 {
			w := words[0]
			camel := len(w) > 1 && strings.ToLower(w[1:]) != w[1:]
			if !camel && (plain[strings.ToLower(w)] || labelStop[strings.ToLower(w)]) {
				continue
			}
		}
		if count[term] == 0 {
			order = append(order, term)
		}
		count[term]++
		if !sentenceStart(text, at) {
			mid[term] = true
		}
	}
	var out []string
	for _, t := range order {
		if base := strings.TrimSuffix(t, "s"); base != t && count[base] > 0 {
			count[base] += count[t] // "APIs" is "API"
			continue
		}
		single := !strings.Contains(t, " ")
		if single && strings.ToLower(t[1:]) == t[1:] && !mid[t] {
			continue
		}
		out = append(out, t)
	}
	sort.SliceStable(out, func(i, j int) bool { return count[out[i]] > count[out[j]] })
	if len(out) > max {
		out = out[:max]
	}
	return out
}

// articleProse is the article's running text: no headings, code, images or
// references, with links reduced to their text.
func articleProse(markdown string) string {
	var prose []string
	inCode := false
	for _, line := range strings.Split(markdown, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(t, "```"):
			inCode = !inCode
		case inCode, strings.HasPrefix(t, "!["):
		case strings.HasPrefix(t, "#"):
			if strings.EqualFold(strings.TrimSpace(strings.TrimLeft(t, "# ")), "references") {
				return strings.Join(prose, "\n")
			}
		default:
			prose = append(prose, mdLink.ReplaceAllString(t, "$1"))
		}
	}
	return strings.Join(prose, "\n")
}

// sentenceStart reports whether the word at i opens a sentence or list item.
func sentenceStart(text string, i int) bool {
	for i > 0 {
		c := text[i-1]
		switch {
		case c == ' ' || c == '\t' || c == '*' || c == '"' || c == '(':
			i--
		case c == '\n' || c == '.' || c == '!' || c == '?' || c == ':':
			return true
		case c >= '0' && c <= '9' || c == '-':
			i-- // "1. " and "- " list markers
		default:
			return false
		}
	}
	return true
}

// termsHub is a cover of what an article names: the term its title names at
// the centre (else the most mentioned), the next most mentioned around it.
// Grounded by construction. False when the article names fewer than four.
func termsHub(title, markdown string) (coverScene, bool) {
	terms := articleTerms(markdown, 8)
	if len(terms) < layoutMinLabels[layoutHub] {
		return coverScene{}, false
	}
	centre, lt := 0, strings.ToLower(title)
	for i, t := range terms {
		if strings.Contains(lt, strings.ToLower(t)) {
			centre = i
			break
		}
	}
	labels := []string{terms[centre]}
	for i, t := range terms {
		if i != centre && len(labels) < 6 {
			labels = append(labels, t)
		}
	}
	return coverScene{Layout: layoutHub, Labels: labels}, true
}
