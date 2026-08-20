package blog

import (
	"context"
	"fmt"
	"regexp"
	"strings"

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
// spent time on.
const (
	coverWidth  = 1024
	coverHeight = 576
)

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

// coverPromptStyle is appended to every generated prompt.
//
// A house style is applied here rather than left to the model because the
// covers have to look like a set. Left to write its own style, the same model
// produces a photograph for one article and a cartoon for the next, and a blog
// whose header images disagree about what kind of publication it is looks
// worse than one with no images at all.
const coverPromptStyle = ", flat vector editorial illustration, minimal, warm amber and deep ink tones, no text, no words, no lettering"

// coverPrompt turns an article title and topic into something worth drawing.
//
// The title alone is a poor prompt: "What the Gateway API Actually Changes"
// describes an argument, not a picture. Naming the subject and then pinning the
// style gives the model something concrete to draw and keeps the result on
// house style.
func coverPrompt(title, topic string) string {
	subject := strings.TrimSpace(title)
	if subject == "" {
		subject = strings.TrimSpace(topic)
	}
	return "An illustration representing: " + subject + coverPromptStyle
}

// coverFallbackModel is what a cover falls back to when the configured default
// cannot be drawn. qwen-image-2512 is the quality default (~3.5 min) but a
// cold load or a WSL-proxy blip returns 502 within seconds and used to leave
// the article with no cover at all. z-image-turbo answers in ~40s and is the
// model that already produced working covers in int before the qwen cutover.
const coverFallbackModel = "z-image-turbo"

// generateCover draws an article's cover image.
//
// A failure is returned, not swallowed — the caller decides whether an article
// without a picture is still an article. It is: see Runner.Generate.
//
// Transient upstream failures are retried once on the default model, then once
// on coverFallbackModel. A cover is worth a couple of extra attempts; an
// article with no picture because the proxy hiccuped once is not.
func (r *Runner) generateCover(ctx context.Context, orgID uuid.UUID, a *GeneratedArticle) error {
	prompt := coverPrompt(a.Title, a.Topic)

	img, err := r.drawCover(ctx, orgID, prompt, "")
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("blog: cover with default model failed, trying fallback",
				zap.String("fallback", coverFallbackModel), zap.Error(err))
		}
		img, err = r.drawCover(ctx, orgID, prompt, coverFallbackModel)
		if err != nil {
			return err
		}
	}
	if img.URL == "" {
		// Generated but unstored. A cover with no URL is not a cover — the CMS
		// would receive an empty src — so this counts as a failure rather than
		// a success with a blank field.
		return fmt.Errorf("blog: cover image was generated but there is nowhere to serve it from")
	}

	a.CoverImageURL = img.URL
	a.CoverImagePrompt = prompt
	a.CoverImageProvider = img.Provider
	a.CoverImageModel = img.Model
	a.CoverImageSeed = img.Seed
	a.CoverImageWidth = img.Width
	a.CoverImageHeight = img.Height
	return nil
}

// drawCover asks the illustrator for one cover, retrying once on a failure that
// looks like a transient upstream problem (502/503/timeout) rather than a
// permanent misconfiguration.
func (r *Runner) drawCover(ctx context.Context, orgID uuid.UUID, prompt, model string) (*Illustration, error) {
	req := IllustrationRequest{
		OrgID:  orgID,
		Prompt: prompt,
		Model:  model,
		Width:  coverWidth,
		Height: coverHeight,
		Source: "blog_cover",
	}
	img, err := r.images.Generate(ctx, req)
	if err == nil {
		return img, nil
	}
	if !transientImageErr(err) {
		return nil, err
	}
	if r.logger != nil {
		r.logger.Warn("blog: transient cover failure, retrying once",
			zap.String("model", model), zap.Error(err))
	}
	return r.images.Generate(ctx, req)
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
			Prompt: description + coverPromptStyle,
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
