package blog

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
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
	// NoFallback skips the workstation (mflux) path. Labeled figures become
	// unreadable lettering on z-image-turbo; an article without a picture is
	// better than one with that picture.
	NoFallback bool
}

// Illustration is what came back.
type Illustration struct {
	URL      string
	Provider string
	Model    string
	Seed     int64
	Width    int
	Height   int
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

// coverTitleMaxWords caps on-image title lettering. Z-Image-Turbo renders short
// quoted titles reliably; long headlines turn into gibberish.
const coverTitleMaxWords = 5

// coverSubtitleMaxWords keeps the subtitle thin and readable under the title.
const coverSubtitleMaxWords = 6

// coverTitleText is the large on-image headline: at most coverTitleMaxWords,
// uppercased for cover contrast.
func coverTitleText(title, topic string) string {
	s := strings.TrimSpace(title)
	if s == "" {
		s = strings.TrimSpace(topic)
	}
	if s == "" {
		s = "Tech Briefing"
	}
	words := strings.Fields(s)
	if len(words) > coverTitleMaxWords {
		words = words[:coverTitleMaxWords]
	}
	return strings.ToUpper(strings.Join(words, " "))
}

// coverSubtitleText is the thin accent line under the title.
func coverSubtitleText(title, topic string) string {
	topic = strings.TrimSpace(topic)
	title = strings.TrimSpace(title)
	s := topic
	if s == "" || strings.EqualFold(s, title) {
		s = "A closer look"
	}
	words := strings.Fields(s)
	if len(words) > coverSubtitleMaxWords {
		words = words[:coverSubtitleMaxWords]
	}
	// Title-case-ish without pulling in cases: leave as written for readability.
	return strings.Join(words, " ")
}

// coverPrompt fills the dark-mode editorial cover template for z-image-turbo.
//
// Turbo ignores separate negative prompts, so exclusions and exact title
// strings live in the positive prompt. The topic drives the metaphor; the
// title (trimmed) is lettered on the left.
func coverPrompt(title, topic string, metaphor, objects, accent string) string {
	topic = strings.TrimSpace(topic)
	title = strings.TrimSpace(title)
	metaphor = strings.TrimSpace(metaphor)
	objects = strings.TrimSpace(objects)
	accent = strings.TrimSpace(accent)

	subject := topic
	if subject == "" {
		subject = title
	}
	if subject == "" {
		subject = "software engineering"
	}

	headline := coverTitleText(title, topic)
	subtitle := coverSubtitleText(title, topic)

	focal := "a concrete visual metaphor for the topic"
	if metaphor != "" {
		focal = metaphor
	}
	if objects != "" {
		focal += " — " + objects
	} else if metaphor == "" {
		focal += " using tools, documents, agents, networks or machines as simple shapes"
	}
	accentClause := "small coral accent dots and thin geometric lines floating nearby"
	if accent != "" {
		accentClause = "small " + accent + " and coral accent dots and thin geometric lines floating nearby"
	}

	return fmt.Sprintf(
		"A high-quality, ultra-detailed modern editorial blog cover illustration "+
			"about %s on a deep charcoal navy background with a subtle dark gradient. "+
			"Large bold white sans-serif title text on the LEFT side that says %q, "+
			"with a smaller thin teal subtitle below it that says %q, "+
			"both razor-sharp, crisp, high contrast, and easy to read. "+
			"On the opposite side, one or two glowing visual objects as the focal point — "+
			"%s — "+
			"emitting soft teal and cyan light that gently illuminates the dark surroundings, "+
			"%s. "+
			"Flat vector illustration style with finely detailed subtle grain texture and smooth polished gradient shading, "+
			"soft rim glow around the subject, gentle ambient light falloff, crisp clean edges, "+
			"meticulous professional finish, balanced asymmetric composition, wide 16:9 landscape framing, "+
			"sharp high-resolution rendering, premium dark-mode tech-blog aesthetic, "+
			"uncluttered layout with generous breathing room around the text. "+
			"No logos, no watermarks, no UI chrome, no extra text beyond the title and subtitle.",
		subject, headline, subtitle, focal, accentClause,
	)
}

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

// generateCover draws an article's cover image.
//
// The model is left unset so the platform default applies: Gemini first when
// a key is set, then mflux on the workstation. Naming z-image-turbo here used
// to skip Gemini. Steps stay set so a workstation fallback still gets the
// turbo step count.
//
// Transient upstream failures (WSL-proxy 502s, busy GPU, timeouts) are retried
// with backoff. A failure after every attempt is returned, not swallowed — the
// caller decides whether an article without a picture is still an article.
func (r *Runner) generateCover(ctx context.Context, orgID uuid.UUID, a *GeneratedArticle) error {
	prompt := coverPrompt(a.Title, a.Topic, a.CoverMetaphor, a.CoverObjects, a.CoverAccent)

	var lastErr error
	for attempt := 1; attempt <= coverMaxAttempts; attempt++ {
		img, err := r.images.Generate(ctx, IllustrationRequest{
			OrgID:  orgID,
			Prompt: prompt,
			Width:  coverWidth,
			Height: coverHeight,
			Steps:  coverSteps,
			Source: "blog_cover",
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
func (r *Runner) illustrateBody(ctx context.Context, orgID uuid.UUID, markdown string) (string, []string) {
	matches := illustrationFence.FindAllStringSubmatchIndex(markdown, -1)
	if len(matches) == 0 {
		return markdown, nil
	}

	var notes []string
	var out strings.Builder
	last := 0
	drawn := 0

	for _, m := range matches {
		start, end := m[0], m[1]
		kind, description := illustrationFromMatch(markdown, m)

		out.WriteString(markdown[last:start])
		last = end

		var ok bool
		kind, description, ok = salvageIllustration(kind, description)
		if !ok {
			notes = append(notes, "dropped an illustration block with no facts to letter")
			continue
		}
		if drawn >= maxInlineIllustrations {
			notes = append(notes, fmt.Sprintf(
				"dropped an illustration block beyond the limit of %d", maxInlineIllustrations))
			continue
		}

		width, height := inlineSize(kind)
		img, err := r.images.Generate(ctx, IllustrationRequest{
			OrgID:      orgID,
			Prompt:     inlineImagePrompt(kind, description),
			Width:      width,
			Height:     height,
			Source:     "blog_inline",
			NoFallback: true,
		})
		if err != nil || img == nil || img.URL == "" {
			notes = append(notes, fmt.Sprintf("dropped an illustration that could not be drawn: %v", err))
			continue
		}
		if !imageRendersLabels(img.Provider, img.Model) {
			notes = append(notes, "dropped an illustration from a model that cannot letter labels")
			continue
		}

		drawn++
		// The description becomes the alt text: it is a sentence describing the
		// picture, written for this picture, which is exactly what alt text is.
		out.WriteString(fmt.Sprintf("![%s](%s)", escapeAlt(description), img.URL))
	}

	out.WriteString(markdown[last:])
	return out.String(), notes
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
