package blog

import (
	"context"
	"strings"
	"unicode/utf8"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/imagestore"
)

// ImageChoice is a per-request choice of image provider and model, such as the
// one made for a single post. Empty fields mean "the org's routing, then the
// platform default".
type ImageChoice struct {
	Provider string
	Model    string
}

type imageChoiceKey struct{}

// WithImageChoice carries a provider/model choice down to the illustrator.
// A context value rather than a parameter because the choice must reach every
// image call a cover makes, through several layers that do not otherwise care.
func WithImageChoice(ctx context.Context, c ImageChoice) context.Context {
	if c.Provider == "" && c.Model == "" {
		return ctx
	}
	return context.WithValue(ctx, imageChoiceKey{}, c)
}

// ImageChoiceFrom returns the choice set by WithImageChoice.
func ImageChoiceFrom(ctx context.Context) (ImageChoice, bool) {
	c, ok := ctx.Value(imageChoiceKey{}).(ImageChoice)
	return c, ok
}

// CoverDescriber writes alt text for a picture, normally with a vision model.
type CoverDescriber interface {
	DescribeImage(ctx context.Context, prompt string, png []byte) (string, error)
}

// WithCoverDescriber sets the vision model that writes cover alt text.
func (r *Runner) WithCoverDescriber(d CoverDescriber) *Runner {
	r.describer = d
	return r
}

const altPrompt = "Write alt text for this blog cover image: one plain sentence, under 125 characters, " +
	"describing what is shown for a reader who cannot see it. No leading 'Image of'. Reply with the sentence only."

// maxAltRunes keeps alt text short enough for screen readers and CMS fields.
const maxAltRunes = 200

// describeCover sets a.CoverImageAlt. The vision model is asked when one is
// configured and the picture can be read back from the store; any failure, or
// no model, falls back to text derived from the title, so a cover never ships
// without alt text and a vision outage never fails a publish.
func (r *Runner) describeCover(ctx context.Context, a *GeneratedArticle) {
	a.CoverImageAlt = fallbackAlt(a.Title)
	if r.describer == nil || r.covers == nil {
		return
	}
	key, ok := strings.CutPrefix(strings.TrimPrefix(strings.TrimSpace(a.CoverImageURL), r.PublicBaseURL()), imagestore.URLPrefix)
	if !ok || key == "" {
		return
	}
	png, err := r.covers.Get(ctx, key)
	if err != nil || len(png) == 0 {
		return
	}
	text, err := r.describer.DescribeImage(ctx, altPrompt, png)
	if err != nil {
		if r.logger != nil {
			r.logger.Warn("blog: vision alt text failed; using the title", zap.Error(err))
		}
		return
	}
	text = strings.Join(strings.Fields(strings.Trim(strings.TrimSpace(text), `"`)), " ")
	if text == "" {
		return
	}
	if utf8.RuneCountInString(text) > maxAltRunes {
		text = string([]rune(text)[:maxAltRunes])
	}
	a.CoverImageAlt = text
}

func fallbackAlt(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "Cover illustration"
	}
	return "Cover illustration for: " + title
}
