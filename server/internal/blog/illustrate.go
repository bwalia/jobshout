package blog

import (
	"context"
	"fmt"
	"hash/fnv"
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
// long piece and small enough that the pipeline stays predictable; blocks past
// the limit are dropped rather than queued.
const maxInlineIllustrations = 3

// illustrationFence matches an ```illustration block and captures its body.
//
// The writing prompt offers this to the model as the way to ask for a picture,
// deliberately mirroring the ```mermaid convention it already knows: a fenced
// block whose body is a description rather than code.
var illustrationFence = regexp.MustCompile("(?s)```illustration[ \t]*\r?\n(.*?)```")

// coverPromptStyle is the visual house style pinned onto inline body
// illustrations (not the dark cover template). Covers have to look like a set;
// left to invent its own style, the same model produces a photograph for one
// article and a cartoon for the next.
const coverPromptStyle = "refined flat vector editorial illustration, crisp edges, clean geometric shapes, sharp contrast, teal and coral accents, high clarity"

// coverModel is the only model blog covers may use. z-image-turbo is the
// workstation default: fast enough for retries, strong at short title text,
// and good enough for dark editorial covers when the prompt is structured.
const coverModel = "z-image-turbo"

// coverMaxAttempts bounds how many times a cover may be asked of the image
// service. Three is enough to ride out a WSL-proxy 502 or a busy GPU without
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

// coverVariant is the set of axes a cover is allowed to vary along.
//
// The pinned axes — charcoal navy ground, flat vector style, no lettering
// beyond the title — are what make the covers a set. These four are what stop
// them being the same picture with a different noun in it.
type coverVariant struct {
	accent      string
	composition string
	arrangement string
	lighting    string
}

// The curated option sets. These are deliberately short and hand-picked rather
// than open-ended: the failure mode being avoided is not "too few covers", it
// is the same model producing a photograph for one article and a cartoon for
// the next when left to invent its own treatment.
var (
	coverAccents = []string{
		"teal and cyan",
		"violet and indigo",
		"amber and coral",
		"emerald and mint",
	}
	coverCompositions = []string{
		"Large bold white sans-serif title text on the LEFT side",
		"Large bold white sans-serif title text on the RIGHT side",
		"Large bold white sans-serif title text across the LOWER THIRD",
	}
	coverArrangements = []string{
		"a single large hero object as the focal point",
		"a small cluster of three related objects arranged in a loose triangle",
		"two objects at different depths, one near and softly blurred, one far and sharp",
	}
	coverLightings = []string{
		"lit by a soft rim light from the left",
		"lit from above with a gentle pool of light falling across the subject",
		"backlit with a halo glow behind the subject and long soft shadows forward",
	}
)

// variantFor picks a cover's varying axes deterministically from its topic.
//
// Deterministic rather than random for two reasons: re-running a topic should
// not silently produce a different cover, and the eval suite needs to be able
// to assert distinctiveness without chasing a moving target.
//
// Each axis divides the hash by a different prime before taking its modulus.
// Reusing the raw hash for all four would make them move in lockstep — accent
// and composition would be perfectly correlated — which collapses 108 possible
// treatments back down to 4.
func variantFor(topic string) coverVariant {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(topic))))
	n := uint32(h.Sum32())
	pick := func(opts []string, divisor uint32) string {
		return opts[(n/divisor)%uint32(len(opts))]
	}
	return coverVariant{
		accent:      pick(coverAccents, 1),
		composition: pick(coverCompositions, 7),
		arrangement: pick(coverArrangements, 13),
		lighting:    pick(coverLightings, 29),
	}
}

// normalizeAccent resolves a requested cover accent to its canonical spelling
// in coverAccents, or "" if it matches none. A caller uses "" to mean "keep the
// per-topic default", so an unrecognised request and no request behave alike.
func normalizeAccent(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	for _, a := range coverAccents {
		if strings.EqualFold(a, s) {
			return a
		}
	}
	return ""
}

// stripIllustrationFences removes every illustration block from the markdown,
// for a run that asked for no in-body pictures. Without it a fence the writer
// emitted anyway would render as a raw ```illustration code block.
func stripIllustrationFences(markdown string) string {
	return illustrationFence.ReplaceAllString(markdown, "")
}

// coverPrompt fills the dark-mode editorial cover template for z-image-turbo.
//
// Turbo ignores separate negative prompts, so exclusions and exact title
// strings live in the positive prompt. The topic drives the metaphor and, via
// variantFor, the accent colour, the title's placement, how the focal objects
// are arranged and where the light comes from — so two articles read as the
// same publication without looking like the same cover.
func coverPrompt(title, topic string) string {
	return coverPromptWithAccent(title, topic, "")
}

// coverPromptWithAccent is coverPrompt with an optional accent override. An
// override matching one of coverAccents pins that hue; anything else (including
// "") leaves the per-topic default in place, so an unknown value degrades to
// the normal behaviour rather than erroring.
func coverPromptWithAccent(title, topic, accentOverride string) string {
	topic = strings.TrimSpace(topic)
	title = strings.TrimSpace(title)

	subject := topic
	if subject == "" {
		subject = title
	}
	if subject == "" {
		subject = "software engineering"
	}

	headline := coverTitleText(title, topic)
	subtitle := coverSubtitleText(title, topic)
	// Keyed on the subject rather than the title so a retitled article keeps
	// its cover treatment, and so an empty title still varies.
	v := variantFor(subject)
	if a := normalizeAccent(accentOverride); a != "" {
		v.accent = a
	}

	return fmt.Sprintf(
		"A high-quality, ultra-detailed modern editorial blog cover illustration "+
			"about %s on a deep charcoal navy background with a subtle dark gradient. "+
			"%s that says %q, "+
			"with a smaller thin %s subtitle below it that says %q, "+
			"both razor-sharp, crisp, high contrast, and easy to read. "+
			"Away from the text, %s — "+
			"a concrete visual metaphor for the topic using tools, documents, agents, networks or machines as simple shapes — "+
			"emitting soft %s light that gently illuminates the dark surroundings, %s, "+
			"small contrasting accent dots and thin geometric lines floating nearby. "+
			"Flat vector illustration style with finely detailed subtle grain texture and smooth polished gradient shading, "+
			"gentle ambient light falloff, crisp clean edges, "+
			"meticulous professional finish, balanced asymmetric composition, wide 16:9 landscape framing, "+
			"sharp high-resolution rendering, premium dark-mode tech-blog aesthetic, "+
			"uncluttered layout with generous breathing room around the text. "+
			"No logos, no watermarks, no UI chrome, no extra text beyond the title and subtitle.",
		subject, v.composition, headline, v.accent, subtitle,
		v.arrangement, v.accent, v.lighting,
	)
}

// generateCover draws an article's cover image with coverModel.
//
// Transient upstream failures (WSL-proxy 502s, busy GPU, timeouts) are retried
// with backoff against the same z-image-turbo model. There is deliberately no
// silent swap to another model.
//
// A failure after every attempt is returned, not swallowed — the caller decides
// whether an article without a picture is still an article. It is: see
// Runner.Generate.
func (r *Runner) generateCover(ctx context.Context, req GenerateRequest, a *GeneratedArticle) error {
	orgID := req.OrgID
	prompt := coverPromptWithAccent(a.Title, a.Topic, req.CoverStyle)

	var lastErr error
	for attempt := 1; attempt <= coverMaxAttempts; attempt++ {
		img, err := r.images.Generate(ctx, IllustrationRequest{
			OrgID:  orgID,
			Prompt: prompt,
			Model:  coverModel,
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
			r.logger.Warn("blog: cover generation failed, retrying with z-image-turbo",
				zap.String("model", coverModel),
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
	return fmt.Errorf("blog: cover with %s failed after %d attempts: %w", coverModel, coverMaxAttempts, lastErr)
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
		description := strings.TrimSpace(markdown[m[2]:m[3]])

		out.WriteString(markdown[last:start])
		last = end

		if description == "" {
			notes = append(notes, "dropped an illustration block with no description")
			continue
		}
		if drawn >= maxInlineIllustrations {
			notes = append(notes, fmt.Sprintf(
				"dropped an illustration block beyond the limit of %d", maxInlineIllustrations))
			continue
		}

		img, err := r.images.Generate(ctx, IllustrationRequest{
			OrgID:  orgID,
			Prompt: description + ". Style: " + coverPromptStyle + ". Strictly no text, letters, logos or watermarks.",
			Width:  inlineWidth,
			Height: inlineHeight,
			Source: "blog_inline",
		})
		if err != nil || img.URL == "" {
			notes = append(notes, fmt.Sprintf("dropped an illustration that could not be drawn: %v", err))
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

// illustrationRequirement is the requirements-list bullet that tells the writer
// pictures are available at all.
//
// The ILLUSTRATIONS block further down the prompt already explains the syntax,
// but a rule that only appears in an appendix is a rule a model skims past: a
// live run against llama3 read the whole block and requested none. The checklist
// is where the model looks for what it must produce, so the ask goes there too.
// Empty when this run cannot draw, so a text-only pipeline never promises a
// picture it has no way to make.
func (r *Runner) illustrationRequirement() string {
	if !r.canIllustrate() {
		return ""
	}
	return "- Include at least one ILLUSTRATION — see the illustration rules below.\n"
}

// illustrationPlacement is one picture the model wants, and where it belongs.
type illustrationPlacement struct {
	// AfterHeading is the heading text the picture should follow, copied from
	// the article. Asking for a heading rather than a line number means a wrong
	// answer is one we can detect and drop, instead of one that silently lands
	// the image in the middle of a paragraph.
	AfterHeading string `json:"after_heading"`
	// Scene is the description handed to the image model, and the alt text.
	Scene string `json:"scene"`
}

// markdownHeading matches an ATX heading and captures its text.
var markdownHeading = regexp.MustCompile(`(?m)^(#{1,6})[ \t]+(.+?)[ \t]*$`)

// ensureIllustrations gives an article that asked for no pictures one chance to
// get some.
//
// Checked rather than trusted, for the same reason the word count is: the draft
// prompt offers illustrations and a live run against a local model produced an
// article with none anyway. Rather than re-draft — which would put the citations
// and the diagrams back at risk to add a picture — this asks only where the
// pictures go, and inserts the fences itself. The worst outcome of a bad answer
// is a placement we cannot match and therefore drop.
//
// One pass, and never fatal: an article without a picture is still an article.
func (r *Runner) ensureIllustrations(
	ctx context.Context, modelName string, plan *writePlan, markdown string,
) (string, []string) {
	if !r.canIllustrate() {
		return markdown, nil
	}
	if len(illustrationFence.FindAllStringIndex(markdown, 1)) > 0 {
		return markdown, nil
	}
	headings := bodyHeadings(markdown)
	if len(headings) == 0 {
		return markdown, []string{"no section headings to place an illustration after"}
	}

	prompt := fmt.Sprintf(`This article has no illustrations. Choose where one or two would help a
reader, and describe the picture for each.

TITLE: %s

SECTION HEADINGS (copy one of these EXACTLY as "after_heading"):
%s

A good illustration is a CONCRETE SCENE — something that could be photographed:
"A machinist watching a spindle spin down in a quiet workshop" works. "The
concept of reliability" does not. Do not ask for text, labels, charts, diagrams
or UI; image models render lettering badly. Never put two in adjacent sections,
and never describe the same scene twice. Write each description as a sentence a
screen-reader user would find useful, because it becomes the alt text.

Pick at most %d. Respond with JSON only, in exactly this shape:
{"illustrations": [{"after_heading": "...", "scene": "..."}]}`,
		plan.Title, strings.Join(headings, "\n"), maxInlineIllustrations)

	var out struct {
		Illustrations []illustrationPlacement `json:"illustrations"`
	}
	if err := r.generateJSON(ctx, modelName, "illustrations", prompt, maxPlanTokens, &out); err != nil {
		return markdown, []string{fmt.Sprintf("could not choose illustration placements: %v", err)}
	}
	return insertIllustrations(markdown, out.Illustrations)
}

// bodyHeadings lists the H2/H3 headings a picture may be placed under.
//
// The H1 is excluded because the cover already sits above it, and the reference
// list because an image among the citations is noise.
func bodyHeadings(markdown string) []string {
	var out []string
	for _, m := range markdownHeading.FindAllStringSubmatch(markdown, -1) {
		if len(m[1]) < 2 {
			continue
		}
		text := strings.TrimSpace(m[2])
		if text == "" || strings.EqualFold(text, "References") {
			continue
		}
		out = append(out, text)
	}
	return out
}

// insertIllustrations writes an illustration fence after each named heading.
//
// The rules the prompt states are enforced here rather than assumed, which is
// the same division of labour illustrateBody keeps: a prompt is advice, and
// this is where a model that ignored it stops mattering. A placement naming a
// heading the article does not have is dropped, because the alternative —
// guessing at the nearest match — puts a picture somewhere nobody asked for.
func insertIllustrations(markdown string, places []illustrationPlacement) (string, []string) {
	if len(places) == 0 {
		return markdown, []string{"the model chose no illustration placements"}
	}

	var notes []string
	used := map[string]bool{}
	scenes := map[string]bool{}
	added := 0

	for _, p := range places {
		heading := strings.TrimSpace(p.AfterHeading)
		scene := strings.TrimSpace(p.Scene)
		switch {
		case scene == "":
			notes = append(notes, "dropped an illustration placement with no description")
			continue
		case added >= maxInlineIllustrations:
			notes = append(notes, fmt.Sprintf(
				"dropped an illustration placement beyond the limit of %d", maxInlineIllustrations))
			continue
		case used[strings.ToLower(heading)]:
			notes = append(notes, fmt.Sprintf("dropped a second illustration under %q", heading))
			continue
		case scenes[strings.ToLower(scene)]:
			notes = append(notes, "dropped an illustration that repeats an earlier scene")
			continue
		}

		next, ok := insertAfterHeading(markdown, heading, "```illustration\n"+scene+"\n```")
		if !ok {
			notes = append(notes, fmt.Sprintf(
				"dropped an illustration for %q, which is not a heading in the article", heading))
			continue
		}
		markdown = next
		used[strings.ToLower(heading)] = true
		scenes[strings.ToLower(scene)] = true
		added++
	}

	if added > 0 {
		notes = append(notes, fmt.Sprintf("added %d illustration request(s) the draft left out", added))
	}
	return markdown, notes
}

// insertAfterHeading puts block immediately below the named heading, reporting
// whether the heading was found.
func insertAfterHeading(markdown, heading, block string) (string, bool) {
	for _, m := range markdownHeading.FindAllStringSubmatchIndex(markdown, -1) {
		if !strings.EqualFold(strings.TrimSpace(markdown[m[4]:m[5]]), heading) {
			continue
		}
		// After the heading's line, not after its whole section: the image is
		// the section's opening beat, and the prose below it then unpacks.
		end := m[1]
		return markdown[:end] + "\n\n" + block + markdown[end:], true
	}
	return markdown, false
}
