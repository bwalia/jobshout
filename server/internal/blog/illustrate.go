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

// coverPromptStyle is the visual house style pinned onto every cover prompt.
//
// Covers have to look like a set. Left to invent its own style, the same model
// produces a photograph for one article and a cartoon for the next.
const coverPromptStyle = "refined flat vector editorial illustration, soft geometric forms, limited palette of warm amber, cream and deep ink blue-black, subtle paper texture, generous negative space, high clarity"

// coverModel is the only model blog covers may use. Quality matters more than
// speed here: a cover is drawn once per article. Transient upstream failures
// are retried against this same model rather than silently downgraded.
const coverModel = "qwen-image-2512"

// coverMaxAttempts bounds how many times a cover may be asked of the image
// service. Each attempt can take several minutes on a cold qwen load, so this
// is a real budget — high enough to ride out a WSL-proxy 502 or a busy GPU,
// low enough that a permanently broken image host cannot hold a blog run open
// for half an hour.
const coverMaxAttempts = 5

// coverPrompt turns an article title and topic into something worth drawing.
//
// The title alone is a poor prompt: "What the Gateway API Actually Changes"
// describes an argument, not a picture, and diffusion models asked to
// "illustrate" a headline often render the words themselves. The topic is the
// subject; the title is context the model must not letter onto the image.
func coverPrompt(title, topic string) string {
	topic = strings.TrimSpace(topic)
	title = strings.TrimSpace(title)

	subject := topic
	if subject == "" {
		subject = title
	}
	if subject == "" {
		subject = "software engineering"
	}

	headline := title
	if headline == "" || strings.EqualFold(headline, subject) {
		headline = subject
	}

	return fmt.Sprintf(
		"Wide 16:9 magazine cover illustration for a technical essay about %s. "+
			"Depict one concrete visual metaphor — tools, documents, agents, networks or machines as simple shapes — "+
			"that communicates the subject at a glance. Do not illustrate the headline wording literally. "+
			"Headline for context only (never draw it): %q. "+
			"Composition: a single clear focal subject, balanced for a blog hero, readable at small sizes. "+
			"Style: %s. "+
			"Strictly no text, letters, numbers, logos, watermarks, UI chrome, screenshots or diagrams with labels.",
		subject, headline, coverPromptStyle,
	)
}

// generateCover draws an article's cover image with coverModel.
//
// Transient upstream failures (WSL-proxy 502s, busy GPU, timeouts) are retried
// with backoff against the same qwen model. There is deliberately no fallback
// to a faster model: the rings chose qwen for cover quality, and a turbo
// substitute would silently undo that choice.
//
// A failure after every attempt is returned, not swallowed — the caller decides
// whether an article without a picture is still an article. It is: see
// Runner.Generate.
func (r *Runner) generateCover(ctx context.Context, orgID uuid.UUID, a *GeneratedArticle) error {
	prompt := coverPrompt(a.Title, a.Topic)

	var lastErr error
	for attempt := 1; attempt <= coverMaxAttempts; attempt++ {
		img, err := r.images.Generate(ctx, IllustrationRequest{
			OrgID:  orgID,
			Prompt: prompt,
			Model:  coverModel,
			Width:  coverWidth,
			Height: coverHeight,
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
			r.logger.Warn("blog: cover generation failed, retrying with qwen",
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
