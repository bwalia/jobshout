package blog

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/imagegen"
)

// Illustrator is the slice of the platform's image service this package
// consumes. Declared here, where it is used, so a test can illustrate an
// article without a GPU — the same pattern Researcher and CMSPublisher follow.
type Illustrator interface {
	Generate(ctx context.Context, req IllustrationRequest) (*Illustration, error)
	Enabled() bool
}

// IllustrationRequest asks for one picture.
type IllustrationRequest struct {
	OrgID  uuid.UUID
	Prompt string
	// Model optionally names which image model to use. Empty means the
	// service's configured default (IMAGE_DEFAULT_MODEL).
	Model  string
	Width  int
	Height int
	// Steps is the number of denoising steps. Zero means the provider default.
	Steps  int
	Source string
	// RunID is the article run the picture is for, recorded on the image so
	// it can open its article. Nil outside a run.
	RunID *uuid.UUID
	// ExpectText is every word the picture must show; a picture whose
	// lettering reads back otherwise is redrawn, and then refused.
	ExpectText []string
	// Text is how much readable text the picture carries: it is never drawn
	// by a model that cannot letter that much, because an article without a
	// picture is better than one with misspelt labels.
	Text imagegen.TextNeed
}

// Illustration is what came back.
type Illustration struct {
	URL      string
	Provider string
	Model    string
	Seed     int64
	Width    int
	Height   int
	// PNG is the image itself, when the generator returned it. Publishing uses
	// it to give the CMS the picture rather than a link back into JobShout.
	PNG []byte
}

// coverWidth and coverHeight are 16:9, which is the shape a cover is displayed
// in. Generating square and letting CSS crop would throw away pixels the model
// spent time on. 1536×864 matches typical full-width hero display better than
// 1024×576, which softens when stretched.
const (
	coverWidth  = 1536
	coverHeight = 864
)

// coverSteps is how many denoising passes a cover gets. z-image-turbo is a
// few-step distilled model; eight matches its catalogue default.
const coverSteps = 8

// inlineWidth and inlineHeight are 3:2 — a body illustration sits in the text
// column rather than spanning a hero area, so it wants less extreme a shape.
const (
	inlineWidth  = 1024
	inlineHeight = 688
)

// maxInlineIllustrations bounds how many pictures one article may request.
//
// Each costs tens of seconds on a single shared GPU, so an article that asks
// for nine holds up every other run behind it. Three is enough to illustrate a
// long piece; the writer is asked for 1–2 and the insert pass fills that in.
const maxInlineIllustrations = 3

// illustrationFence matches ```illustration or ```illustration <kind>.
//
// Group 1 is the optional kind (flow, comparison, …). Group 2 is the body —
// the facts the picture must show. The writing prompt teaches this shape so
// a typed request survives a model that copies the example.
var illustrationFence = regexp.MustCompile("(?s)```illustration(?:[ \t]+([A-Za-z][A-Za-z0-9_-]*))?[^\n]*\r?\n(.*?)```")

// coverMaxAttempts bounds how many times a cover may be asked of the image
// service. Three is enough to ride out a Gemini blip or a busy GPU without
// holding a blog run open through a dead image host.
const coverMaxAttempts = 3

// sceneText is every word infographicPrompt asks a picture of scene to show:
// its tag, its labels and its stat, for reading the picture back.
func sceneText(scene coverScene) []string {
	return nonEmpty(append([]string{scene.Eyebrow, scene.Stat}, scene.Labels...))
}

// Openings for infographicPrompt.
const (
	coverOpening  = "A 16:9 cover image for a technical blog article"
	figureOpening = "A figure inside a technical blog article"
)

// figureAccent is the one accent every drawn figure uses, so figures read as
// one set whatever the article's cover accent.
const figureAccent = "ice"

// infographicPrompt is a cover or figure for a model that captions (Nano
// Banana, ERNIE-Image): the article's idea drawn as one labelled diagram, in the
// article's own terms (the cover scene), so the card says what the piece
// argues. The house style is fixed, so covers read as one series on the
// listing page. Every word in the prompt may end up printed — diffusion models
// letter what they read, "do not print" included — so it holds only the words
// the cover should show. The title is not one of them: the site prints it
// under the card, and ERNIE misspelt a 7-word one ("Kubernates").
func infographicPrompt(opening string, scene coverScene, accent string) string {
	var b strings.Builder
	b.WriteString(opening + ". It explains the article at a glance, " +
		"like a well-designed infographic; it is not decoration.\n\n")
	if scene.Eyebrow != "" {
		fmt.Fprintf(&b, "Small uppercase tag, top left, in the accent colour: %q\n", scene.Eyebrow)
	}
	// The guide line is "what it shows (labels = their roles; extras)"; the
	// extras carry examples a diffusion model would letter.
	how, roles := string(scene.Layout), "in this order"
	for _, line := range strings.Split(coverLayoutGuide, "\n") {
		if rest, ok := strings.CutPrefix(line, "- "+how+": "); ok {
			how, roles, _ = strings.Cut(rest, " (labels = ")
			roles, _, _ = strings.Cut(strings.TrimSuffix(roles, ")"), ";")
			roles, _, _ = strings.Cut(roles, ", optionally")
		}
	}
	// A range ("4-6 things") invites the model to draw more parts than there
	// are labels and to fill them with repeats; it is told the exact count.
	how, roles = labelRange.ReplaceAllString(how, ""), labelRange.ReplaceAllString(roles, "")
	fmt.Fprintf(&b, "\nFilling the image, one clear diagram of the article's idea: %s.\n", how)
	labels := scene.Labels
	switch {
	case len(labels) == 0:
	case scene.Layout == layoutHub && len(labels) > 1:
		fmt.Fprintf(&b, "In its centre, a large node labelled %q. Around it, exactly %d smaller nodes, "+
			"no more, each labelled once in large clear type, spelled exactly, with a simple flat icon:\n",
			labels[0], len(labels)-1)
		labels = labels[1:]
	default:
		fmt.Fprintf(&b, "Exactly %d labelled parts, no more, %s, each labelled once in large clear type, "+
			"spelled exactly, with a simple flat icon:\n", len(labels), roles)
	}
	for _, l := range labels {
		fmt.Fprintf(&b, "- %s\n", l)
	}
	if scene.Stat != "" {
		fmt.Fprintf(&b, "A large callout figure in the accent colour: %q\n", scene.Stat)
	}
	pal, ok := fallbackPalettes[accent]
	if !ok {
		accent, pal = "copper", fallbackPalettes["copper"]
	}
	fmt.Fprintf(&b, "\nHouse style, the same on every cover in this series: deep navy background, one %s accent colour "+
		"(#%02x%02x%02x), white and light-grey text, flat vector icons with thin outlines, a faint grid, generous margins, "+
		"crisp edges, high contrast, legible as a small thumbnail.\n", accent, pal[2].R, pal[2].G, pal[2].B)
	b.WriteString("No other text, no numbers beyond those given, no logos, brand marks, watermarks, people or photographs.")
	return b.String()
}

func accentForTopic(title, topic, angle string) string {
	blob := strings.ToLower(title + " " + topic + " " + angle)
	switch {
	case strings.Contains(blob, "security"), strings.Contains(blob, "vault"), strings.Contains(blob, "secret"):
		return "amber"
	case strings.Contains(blob, "resilience"), strings.Contains(blob, "reliability"), strings.Contains(blob, "observ"):
		return "emerald"
	case strings.Contains(blob, "network"), strings.Contains(blob, "gateway"), strings.Contains(blob, "cluster"):
		return "ice"
	case strings.Contains(blob, "agent"), strings.Contains(blob, "mcp"), strings.Contains(blob, "ai"):
		return "violet"
	default:
		return "copper"
	}
}

// labelRange is a count range in the layout guide, such as "4-6 ".
var labelRange = regexp.MustCompile(`\d+-\d+ `)

// citationMark strips [1]-style citation markers so they do not leak into
// an image prompt.
var citationMark = regexp.MustCompile(`\[\d+\]`)

// ensureIllustrationFences adds 1–2 informational figures when the writer
// omitted them and the section has enough facts to letter. Mermaid diagrams
// are left alone: they already teach the path, and replacing one with a
// raster image was how we deleted the only reliable figure in the article.
func ensureIllustrationFences(markdown string, plan *writePlan) string {
	want := 2
	if want > maxInlineIllustrations {
		want = maxInlineIllustrations
	}
	markdown = keepFirstNIllustrations(markdown, maxInlineIllustrations)
	have := len(illustrationFence.FindAllString(markdown, -1))
	if have >= want {
		return markdown
	}
	return insertIllustrations(markdown, plan, want-have)
}

func planTitle(plan *writePlan) string {
	if plan == nil {
		return ""
	}
	return strings.TrimSpace(plan.Title)
}

func keepFirstNIllustrations(markdown string, n int) string {
	if n < 1 {
		n = 1
	}
	matches := illustrationFence.FindAllStringIndex(markdown, -1)
	if len(matches) <= n {
		return markdown
	}
	var b strings.Builder
	last := 0
	for i, m := range matches {
		if i < n {
			b.WriteString(markdown[last:m[1]])
			last = m[1]
			continue
		}
		b.WriteString(markdown[last:m[0]])
		last = m[1]
	}
	b.WriteString(markdown[last:])
	return collapseBlankRuns(b.String())
}

func insertIllustrations(markdown string, plan *writePlan, n int) string {
	for i := 0; i < n; i++ {
		next := insertOneIllustration(markdown, plan)
		if next == markdown {
			break
		}
		markdown = next
	}
	return markdown
}

func insertOneIllustration(markdown string, plan *writePlan) string {
	title := planTitle(plan)
	headings := collectH2s(markdown)
	if len(headings) == 0 && plan != nil {
		headings = plan.Sections
	}

	lines := strings.Split(markdown, "\n")
	type candidate struct {
		lineIdx int
		heading string
	}
	var preferred, fallback []candidate
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if !isH2(trim) {
			continue
		}
		c := candidate{i, strings.TrimSpace(strings.TrimPrefix(trim, "## "))}
		if sectionHasIllustrationSoon(lines, i) {
			continue
		}
		if sectionHasMermaidSoon(lines, i) {
			fallback = append(fallback, c)
		} else {
			preferred = append(preferred, c)
		}
	}
	try := append(append([]candidate{}, preferred...), fallback...)
	for _, c := range try {
		prose := firstParagraph(sectionTextFrom(lines, c.lineIdx))
		kind, desc := figureForSection(plan, c.heading, prose, title)
		_, _, planned := plannedFigure(plan, c.heading)
		if !figureWorthInserting(kind, c.heading, prose, planned) {
			continue
		}
		return insertFenceAfter(lines, lineAfterFirstParagraph(lines, c.lineIdx), kind, desc)
	}
	if illustrationFence.MatchString(markdown) {
		return markdown
	}
	heading := ""
	if len(headings) > 0 {
		heading = headings[0]
	}
	prose := firstParagraph(markdown)
	kind, desc := figureForSection(plan, heading, prose, title)
	_, _, planned := plannedFigure(plan, heading)
	if !figureWorthInserting(kind, heading, prose, planned) {
		return markdown
	}
	return strings.TrimRight(markdown, "\n") + "\n\n" + formatIllustrationFence(kind, desc) + "\n"
}

func insertFenceAfter(lines []string, insertAt int, kind illustrationKind, desc string) string {
	var b strings.Builder
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
		if i == insertAt {
			b.WriteString("\n\n")
			b.WriteString(formatIllustrationFence(kind, desc))
		}
	}
	return b.String()
}

func collectH2s(markdown string) []string {
	var out []string
	for _, line := range strings.Split(markdown, "\n") {
		trim := strings.TrimSpace(line)
		if isH2(trim) {
			out = append(out, strings.TrimSpace(strings.TrimPrefix(trim, "## ")))
		}
	}
	return out
}

func isH2(trim string) bool {
	return strings.HasPrefix(trim, "## ") && !strings.HasPrefix(trim, "### ")
}

func sectionHasIllustrationSoon(lines []string, headingIdx int) bool {
	for i := headingIdx + 1; i < len(lines) && i <= headingIdx+24; i++ {
		trim := strings.TrimSpace(lines[i])
		if isH2(trim) {
			return false
		}
		if strings.HasPrefix(trim, "```illustration") {
			return true
		}
	}
	return false
}

func sectionHasMermaidSoon(lines []string, headingIdx int) bool {
	for i := headingIdx + 1; i < len(lines) && i <= headingIdx+24; i++ {
		trim := strings.TrimSpace(lines[i])
		if isH2(trim) {
			return false
		}
		if strings.HasPrefix(trim, "```mermaid") {
			return true
		}
	}
	return false
}

func sectionTextFrom(lines []string, headingIdx int) string {
	var b strings.Builder
	for i := headingIdx; i < len(lines); i++ {
		if i > headingIdx && isH2(strings.TrimSpace(lines[i])) {
			break
		}
		if i > headingIdx {
			b.WriteByte('\n')
		}
		b.WriteString(lines[i])
	}
	return b.String()
}

func lineAfterFirstParagraph(lines []string, headingIdx int) int {
	i := headingIdx + 1
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) {
		return headingIdx
	}
	trim := strings.TrimSpace(lines[i])
	if strings.HasPrefix(trim, "```") {
		i++
		for i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```") {
			i++
		}
		if i < len(lines) {
			i++
		}
		for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
			i++
		}
	}
	if i >= len(lines) || isH2(strings.TrimSpace(lines[i])) {
		return headingIdx
	}
	last := i
	for i < len(lines) {
		trim := strings.TrimSpace(lines[i])
		if trim == "" || isH2(trim) || strings.HasPrefix(trim, "```") {
			break
		}
		last = i
		i++
	}
	return last
}

func firstParagraph(s string) string {
	var buf []string
	inFence := false
	for _, line := range strings.Split(s, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if trim == "" || strings.HasPrefix(trim, "#") {
			if len(buf) > 0 {
				break
			}
			continue
		}
		buf = append(buf, trim)
		if len(strings.Join(buf, " ")) > 280 {
			break
		}
	}
	return strings.TrimSpace(citationMark.ReplaceAllString(strings.Join(buf, " "), ""))
}

func firstSentence(s string) string {
	s = strings.TrimSpace(s)
	for i := 0; i < len(s); i++ {
		if (s[i] == '.' || s[i] == '?' || s[i] == '!') && (i+1 == len(s) || s[i+1] == ' ') {
			return strings.TrimSpace(s[:i+1])
		}
	}
	if len(s) <= 220 {
		return s
	}
	cut := s[:220]
	if j := strings.LastIndex(cut, " "); j > 80 {
		cut = cut[:j]
	}
	return strings.TrimSpace(cut)
}

// generateCover draws an article's cover as a picture: its idea as a labelled
// diagram (infographicPrompt).
//
// The model is left unset so the platform default applies: Gemini first when
// a key is set, then the workstation, but only a workstation model that
// captions (imagegen.TextCaptions). Steps suit a turbo model (ERNIE-Image-Turbo);
// Gemini ignores them.
//
// Transient upstream failures (WSL-proxy 502s, busy GPU, timeouts) are retried
// with backoff. A failure after every attempt is returned, not swallowed — the
// caller decides whether an article without a picture is still an article.
func (r *Runner) generateCover(ctx context.Context, orgID uuid.UUID, a *GeneratedArticle) error {
	if strings.TrimSpace(a.CoverAccent) == "" {
		a.CoverAccent = accentForTopic(a.Title, a.Topic, a.CoverAngle)
	}
	a.CoverScene = coverSceneFor(a)
	prompt := infographicPrompt(coverOpening, a.CoverScene, a.CoverAccent)

	var lastErr error
	for attempt := 1; attempt <= coverMaxAttempts; attempt++ {
		img, err := r.images.Generate(ctx, IllustrationRequest{
			OrgID:      orgID,
			Prompt:     prompt,
			Width:      coverWidth,
			Height:     coverHeight,
			Steps:      coverSteps,
			Source:     "blog_cover",
			RunID:      a.RunID,
			Text:       imagegen.TextCaptions,
			ExpectText: sceneText(a.CoverScene),
		})
		if err == nil && img != nil && img.URL != "" {
			a.CoverImageURL = img.URL
			a.CoverImagePrompt = prompt
			a.CoverImageProvider = img.Provider
			a.CoverImageModel = img.Model
			a.CoverImageSeed = img.Seed
			a.CoverImageWidth = img.Width
			a.CoverImageHeight = img.Height
			if attempt > 1 && r.logger != nil {
				r.logger.Info("blog: cover succeeded after retry",
					zap.String("model", img.Model), zap.Int("attempt", attempt))
			}
			return nil
		}

		switch {
		case err != nil:
			lastErr = err
		case img == nil || img.URL == "":
			// Generated but unstored. A cover with no URL is not a cover — the
			// CMS would receive an empty src. This is a configuration problem
			// (no object store), not a blip worth retrying.
			return fmt.Errorf("blog: cover image was generated but there is nowhere to serve it from")
		}

		if !transientImageErr(lastErr) {
			return lastErr
		}
		if attempt == coverMaxAttempts {
			break
		}
		wait := coverRetryWaitFn(attempt)
		if r.logger != nil {
			r.logger.Warn("blog: cover generation failed, retrying",
				zap.Int("attempt", attempt),
				zap.Duration("backoff", wait),
				zap.Error(lastErr))
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return fmt.Errorf("blog: cover failed after %d attempts: %w", coverMaxAttempts, lastErr)
}

// RegenerateCover redraws an article's cover from its finished content.
//
// Used when a published piece has no featured image (or a stale one) and the
// operator wants a new picture grounded in title, angle, excerpt and body —
// not a re-run of the whole writing pipeline. Best-effort callers treat a
// failure the same way generateCover does: the article stays publishable.
func (r *Runner) RegenerateCover(ctx context.Context, orgID uuid.UUID, a *GeneratedArticle) error {
	if a == nil {
		return fmt.Errorf("blog: regenerate cover: no article")
	}
	// Force a fresh brief from the written piece so a missing or generic plan
	// brief cannot keep producing the same fallback scene.
	a.CoverAccent = ""
	a.CoverScene = coverScene{}
	if strings.TrimSpace(a.CoverAngle) == "" {
		a.CoverAngle = strings.TrimSpace(a.Excerpt)
	}
	return r.redrawCover(ctx, orgID, a)
}

// redrawCover draws a's cover from what a holds, keeping a real cover already
// in storage when drawing fails and storing the branded fallback for an
// article that has none.
func (r *Runner) redrawCover(ctx context.Context, orgID uuid.UUID, a *GeneratedArticle) error {
	err := fmt.Errorf("cover drawing is not available")
	if r.canDrawCover() {
		if err = r.drawCover(ctx, orgID, a); err == nil {
			return nil
		}
	}
	// Could not draw. A real cover already in storage is kept — never swapped
	// for the fallback; an article with none gets the branded fallback, so its
	// post is not left without a featured image.
	if r.coverStored(ctx, a.CoverImageURL) {
		return fmt.Errorf("blog: regenerate cover: %w", err)
	}
	if ferr := r.storeFallbackCover(ctx, orgID, a); ferr != nil {
		return fmt.Errorf("blog: regenerate cover: %w; no fallback could be stored either: %v", err, ferr)
	}
	return nil
}

// coverRetryWaitFn is the pause before the next cover attempt. Tests replace it
// so a retry suite does not sleep for real.
var coverRetryWaitFn = coverRetryWait

// coverRetryWait is the pause before attempt n+1. Short at first (a proxy blip),
// then longer so a busy GPU has time to finish whatever is ahead of us.
func coverRetryWait(attempt int) time.Duration {
	switch attempt {
	case 1:
		return 5 * time.Second
	case 2:
		return 15 * time.Second
	case 3:
		return 30 * time.Second
	default:
		return 60 * time.Second
	}
}

// transientImageErr reports whether an image-generation failure is worth
// retrying. Gateway HTML 502s and "busy" answers come and go; auth failures and
// bad prompts do not.
func transientImageErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, needle := range []string{
		"502", "503", "504",
		"timeout", "deadline exceeded",
		"connection reset", "connection refused",
		"busy", "server error", "wsl proxy",
	} {
		if strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

// illustrateBody replaces the article's ```illustration blocks with generated
// images, returning the new markdown and a note for each block it acted on.
//
// A block whose image cannot be drawn is removed rather than left in place, for
// the same reason an unrenderable Mermaid diagram is removed: the reader would
// otherwise be shown a fenced block of prose describing a picture that is not
// there, which reads as a bug in the article.
func (r *Runner) illustrateBody(ctx context.Context, orgID uuid.UUID, runID *uuid.UUID, markdown string) (string, []string) {
	matches := illustrationFence.FindAllStringSubmatchIndex(markdown, -1)
	if len(matches) == 0 {
		return markdown, nil
	}

	var notes []string
	var out strings.Builder
	last := 0
	drawn := 0
	prose := illustrationFence.ReplaceAllString(markdown, "")

	for _, m := range matches {
		start, end := m[0], m[1]
		kind, description := illustrationFromMatch(markdown, m)

		out.WriteString(markdown[last:start])
		last = end

		var ok bool
		kind, description, ok = salvageIllustration(kind, description)
		description = dropTemplateLabel(description)
		if !ok {
			notes = append(notes, "dropped an illustration block with no facts to letter")
			continue
		}
		if why := figureMisfit(description, prose); why != "" {
			notes = append(notes, "dropped an illustration: "+why)
			continue
		}
		if drawn >= maxInlineIllustrations {
			notes = append(notes, fmt.Sprintf(
				"dropped an illustration block beyond the limit of %d", maxInlineIllustrations))
			continue
		}

		url, note := r.drawFigure(ctx, orgID, runID, kind, description)
		if url == "" {
			notes = append(notes, note)
			continue
		}

		drawn++
		// Absolute, because the CMS posts this HTML to another site, where a
		// /api/v1/images/… path would resolve against that site and 404.
		if abs := publicImageURL(r.PublicBaseURL(), url); abs != "" {
			url = abs
		}
		// The description becomes the alt text: it is a sentence describing the
		// picture, written for this picture, which is exactly what alt text is.
		out.WriteString(fmt.Sprintf("![%s](%s)", escapeAlt(description), url))
	}

	out.WriteString(markdown[last:])
	return out.String(), notes
}

// drawFigure returns the URL of one body figure, or "" and the note saying
// why there is none. An image model that letters labels goes first; without
// one, or when it fails, the figure is drawn as a vector diagram from its own
// terms, which letters them exactly and needs no GPU or key.
func (r *Runner) drawFigure(ctx context.Context, orgID uuid.UUID, runID *uuid.UUID, kind illustrationKind, description string) (string, string) {
	note := "dropped an illustration: no image model can letter labels"
	width, height := inlineSize(kind)
	if r.canLetterImages() {
		img, err := r.images.Generate(ctx, IllustrationRequest{
			OrgID:  orgID,
			Prompt: inlineImagePrompt(kind, description),
			Width:  width,
			Height: height,
			Source: "blog_inline",
			RunID:  runID,
			Text:   imagegen.TextLabels,
		})
		switch {
		case err != nil || img == nil || img.URL == "":
			// A provider error can run to a page (Gemini's quota message does).
			note = "dropped an illustration that could not be drawn: " + truncateRunes(fmt.Sprint(err), 200)
		case !imageRendersLabels(img.Provider, img.Model):
			note = "dropped an illustration from a model that cannot letter labels"
		default:
			return img.URL, ""
		}
	}
	// A flow or a hub is drawn as a cover is, by a model that only captions
	// (ERNIE on the workstation): a few labels, each once. That prompt came
	// out clean on int where the full figure prompt repeated and misspelt its
	// boxes. A table or prose has no scene, and needs a model that letters.
	scene, ok := figureScene(kind, description)
	if ok && r.canLetter(imagegen.TextCaptions) {
		img, err := r.images.Generate(ctx, IllustrationRequest{
			OrgID:      orgID,
			Prompt:     infographicPrompt(figureOpening, scene, figureAccent),
			Width:      width,
			Height:     height,
			Steps:      coverSteps,
			Source:     "blog_inline",
			RunID:      runID,
			Text:       imagegen.TextCaptions,
			ExpectText: sceneText(scene),
		})
		switch {
		case err != nil || img == nil || img.URL == "":
			note = "dropped an illustration that could not be drawn: " + truncateRunes(fmt.Sprint(err), 200)
		case !imagegen.Letters(img.Provider, img.Model, imagegen.TextCaptions):
			note = "dropped an illustration from a model that cannot letter labels"
		default:
			return img.URL, ""
		}
	}
	if r.covers == nil {
		return "", note
	}
	if !ok {
		return "", note + "; it is not a list of steps or parts to draw as a diagram"
	}
	pic, err := drawDiagramCover(scene, "", description)
	if err == nil {
		var url string
		if url, err = r.covers.PutFile(ctx, orgID, pic, "png", "image/png"); err == nil {
			return url, ""
		}
	}
	return "", fmt.Sprintf("%s; drawing it as a diagram failed: %v", note, err)
}

// illustrationFromMatch reads kind (group 1, optional) and body (group 2)
// out of a submatch-index slice. An unmatched kind group is -1, -1 — slicing
// that would panic, which is why this is not done inline.
func illustrationFromMatch(markdown string, m []int) (illustrationKind, string) {
	kindRaw := ""
	if len(m) >= 4 && m[2] >= 0 && m[3] >= 0 {
		kindRaw = markdown[m[2]:m[3]]
	}
	body := ""
	if len(m) >= 6 && m[4] >= 0 && m[5] >= 0 {
		body = markdown[m[4]:m[5]]
	}
	return parseIllustration(kindRaw, body)
}

// escapeAlt makes a description safe to sit inside markdown alt-text brackets.
// Only the characters that would end the alt text early are touched; the rest
// is left as the model wrote it.
func escapeAlt(s string) string {
	s = strings.ReplaceAll(s, "[", "(")
	s = strings.ReplaceAll(s, "]", ")")
	// Alt text is one line. A description spanning several would break the
	// image out of its own markdown.
	s = strings.Join(strings.Fields(s), " ")
	return s
}
