package blog

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/imagestore"
	"github.com/jobshout/server/internal/model"
)

// CoverStore is where covers live and are served from (an imagestore.Store).
// Publishing reads a cover back to prove it exists before giving its URL to the
// CMS, and writes a fallback cover into it when none could be drawn.
type CoverStore interface {
	PutFile(ctx context.Context, orgID uuid.UUID, data []byte, ext, contentType string) (string, error)
	Get(ctx context.Context, key string) ([]byte, error)
}

// WithCoverStore sets the store covers are verified against and fallback
// covers are written to. Without one, publishing trusts a recorded cover URL
// and cannot fall back when there is none.
func (r *Runner) WithCoverStore(s CoverStore) *Runner {
	r.covers = s
	return r
}

// maxFeaturedImageURL is opsapi's featured_image_url column (varchar(255)): a
// longer value fails the whole post rather than just the picture.
const maxFeaturedImageURL = 255

// ensureCover gives a the cover the public website will show and returns the
// absolute URL to file as the CMS featured image. opsapi stores that value
// verbatim and the site links it as-is, so whatever is sent here is what
// readers see — a missing or dead link is a broken card on the live site.
//
// In order:
//  1. the article's recorded cover, if its bytes really are in the store;
//  2. a cover drawn now from the article — the relevant picture;
//  3. a fallback rendered here, needing no GPU, provider or network: the
//     article's diagram, or failing that a branded gradient.
//
// An error means there is no usable image at all, and the caller must not file
// the post: a draft without a picture is one click from a live page without one.
func (r *Runner) ensureCover(ctx context.Context, orgID uuid.UUID, a *GeneratedArticle, progress ProgressFunc) (string, error) {
	if r.coverStored(ctx, a.CoverImageURL) {
		return r.featuredImageURL(a.CoverImageURL)
	}
	if a.CoverImageURL != "" {
		r.logger.Warn("blog: recorded cover is not in image storage; replacing it",
			zap.String("title", a.Title), zap.String("cover", a.CoverImageURL))
		a.CoverImageURL = ""
	}
	if r.canDrawCover() {
		report(progress, model.BlogStepIllustrating,
			fmt.Sprintf("Drawing a cover for %q before publishing", a.Title),
			model.AgentNameArticleWriter)
		err := r.drawCover(ctx, orgID, a)
		if err == nil && r.coverStored(ctx, a.CoverImageURL) {
			return r.featuredImageURL(a.CoverImageURL)
		}
		if err == nil {
			err = errors.New("the drawn cover could not be read back from storage")
		}
		r.logger.Warn("blog: could not draw a cover; filing the branded fallback",
			zap.String("title", a.Title), zap.Error(err))
		a.CoverImageURL = ""
	}
	if err := r.storeFallbackCover(ctx, orgID, a); err != nil {
		return "", fmt.Errorf("blog: %q has no featured image and none could be stored: %w", a.Title, err)
	}
	return r.featuredImageURL(a.CoverImageURL)
}

// coverStored reports whether url is a cover in this ring's image store whose
// bytes can be read back — not merely a URL someone recorded. Only our own
// covers count: anything else (a hand-pointed website asset that may never
// have been deployed) is redrawn rather than trusted.
func (r *Runner) coverStored(ctx context.Context, url string) bool {
	url = strings.TrimSpace(url)
	if base := r.PublicBaseURL(); base != "" {
		url = strings.TrimPrefix(url, base)
	}
	key, ours := strings.CutPrefix(url, imagestore.URLPrefix)
	if !ours || key == "" {
		return false
	}
	if r.covers == nil {
		return true
	}
	data, err := r.covers.Get(ctx, key)
	return err == nil && len(data) > 0
}

// featuredImageURL is the absolute URL the CMS stores for a cover.
func (r *Runner) featuredImageURL(cover string) (string, error) {
	abs := publicImageURL(r.cfg.PublicBaseURL, cover)
	switch {
	case abs == "":
		return "", fmt.Errorf("blog: cover %q cannot be made absolute — FRONTEND_BASE_URL is unset, and the CMS cannot load a relative image", cover)
	case len(abs) > maxFeaturedImageURL:
		return "", fmt.Errorf("blog: cover URL is %d characters; opsapi stores at most %d", len(abs), maxFeaturedImageURL)
	}
	return abs, nil
}

// Cover styles (Config.CoverStyle).
const (
	// CoverStyleDiagram draws the article's idea as a vector diagram in the
	// house style. No GPU or provider: it only needs the cover store.
	CoverStyleDiagram = "diagram"
	// CoverStylePicture asks the image service for a generated picture.
	CoverStylePicture = "picture"
)

func (r *Runner) coverPictures() bool {
	return strings.EqualFold(strings.TrimSpace(r.cfg.CoverStyle), CoverStylePicture)
}

// canDrawCover reports whether this run can give an article a real cover in
// the configured style.
func (r *Runner) canDrawCover() bool {
	if r.coverPictures() {
		return r.canIllustrate()
	}
	return r.covers != nil
}

// drawCover gives a its cover in the configured style.
func (r *Runner) drawCover(ctx context.Context, orgID uuid.UUID, a *GeneratedArticle) error {
	if r.coverPictures() {
		return r.generateCover(ctx, orgID, a)
	}
	return r.storeDiagramCover(ctx, orgID, a)
}

// storeDiagramCover draws a's cover diagram and records it as a's cover. The
// scene is kept as the prompt, so the article shows what was drawn and why.
func (r *Runner) storeDiagramCover(ctx context.Context, orgID uuid.UUID, a *GeneratedArticle) error {
	if r.covers == nil {
		return errors.New("no image store is configured")
	}
	scene := coverSceneFor(a)
	accent := a.CoverAccent
	if strings.TrimSpace(accent) == "" {
		accent = accentForTopic(a.Title, a.Topic, a.CoverAngle)
	}
	seed := a.Slug
	if seed == "" {
		seed = a.Title
	}
	img, err := drawDiagramCover(scene, accent, seed)
	if err != nil {
		return err
	}
	url, err := r.covers.PutFile(ctx, orgID, img, "png", "image/png")
	if err != nil {
		return err
	}
	spec, _ := json.Marshal(scene)
	a.CoverScene = scene
	a.CoverImageURL = url
	a.CoverImagePrompt = string(spec)
	a.CoverImageProvider = "diagram"
	a.CoverImageModel = "scene:" + string(scene.Layout)
	a.CoverImageSeed = 0
	a.CoverImageWidth, a.CoverImageHeight = int(designW), int(designH)
	return nil
}

// storeFallbackCover records a cover for an article none could be drawn for.
// The article's diagram comes first — it needs nothing that a failed picture
// needed, and it is still about the article. The branded gradient is the
// last resort; its provider "fallback" marks it for regenerate-cover once
// drawing is healthy again.
func (r *Runner) storeFallbackCover(ctx context.Context, orgID uuid.UUID, a *GeneratedArticle) error {
	if r.covers == nil {
		return errors.New("no image store is configured")
	}
	err := r.storeDiagramCover(ctx, orgID, a)
	if err == nil {
		return nil
	}
	r.logger.Warn("blog: could not draw a diagram cover; using the branded gradient",
		zap.String("title", a.Title), zap.Error(err))
	seed := a.Slug
	if seed == "" {
		seed = a.Title
	}
	img, err := fallbackCover(seed, accentForTopic(a.Title, a.Topic, a.CoverAngle))
	if err != nil {
		return err
	}
	url, err := r.covers.PutFile(ctx, orgID, img, "png", "image/png")
	if err != nil {
		return err
	}
	a.CoverImageURL = url
	a.CoverImagePrompt = ""
	a.CoverImageProvider = "fallback"
	a.CoverImageModel = "branded-gradient"
	a.CoverImageSeed = 0
	a.CoverImageWidth, a.CoverImageHeight = coverWidth, coverHeight
	r.logger.Warn("blog: filed a branded fallback cover; regenerate it once image generation is back",
		zap.String("title", a.Title), zap.String("cover", url))
	return nil
}

// fallbackPalettes are the dark base and glow colour per cover accent
// (accentForTopic). Dark bases keep a white site header and card text legible
// over the picture, the same way the drawn covers are prompted to.
var fallbackPalettes = map[string][3]color.RGBA{
	// base top-left, base bottom-right, glow
	"amber":   {{18, 16, 30, 255}, {44, 28, 14, 255}, {245, 166, 35, 255}},
	"emerald": {{10, 22, 28, 255}, {12, 44, 38, 255}, {52, 211, 153, 255}},
	"ice":     {{10, 18, 36, 255}, {18, 40, 64, 255}, {125, 211, 252, 255}},
	"violet":  {{16, 14, 36, 255}, {40, 20, 66, 255}, {167, 139, 250, 255}},
	"copper":  {{20, 16, 24, 255}, {52, 30, 24, 255}, {234, 140, 85, 255}},
}

// fallbackCover renders a branded abstract cover (coverWidth × coverHeight PNG)
// for an article no image provider could draw. It needs no network, GPU or
// font, so it cannot fail for the reasons a drawn cover does — it is the
// guarantee that a post never reaches the CMS without a featured image.
//
// It is deterministic per seed (the article's slug or title): republishing
// gives the same picture, and two articles never share one, because the glow
// positions come from a hash of the seed.
func fallbackCover(seed, accent string) ([]byte, error) {
	pal, ok := fallbackPalettes[accent]
	if !ok {
		pal = fallbackPalettes["copper"]
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(seed))
	sum := h.Sum64()
	// Three glows placed from different bits of the hash.
	type glow struct{ x, y, r, a float64 }
	glows := make([]glow, 3)
	for i := range glows {
		bits := sum >> (uint(i) * 20)
		glows[i] = glow{
			x: float64(bits%1000) / 1000 * coverWidth,
			y: float64((bits/1000)%1000) / 1000 * coverHeight,
			r: coverHeight * (0.35 + float64((bits/1_000_000)%100)/400),
			a: 0.75 - float64(i)*0.15,
		}
	}

	img := image.NewRGBA(image.Rect(0, 0, coverWidth, coverHeight))
	lerp := func(a, b uint8, t float64) float64 { return float64(a) + (float64(b)-float64(a))*t }
	for y := 0; y < coverHeight; y++ {
		for x := 0; x < coverWidth; x++ {
			// Diagonal base gradient.
			t := (float64(x)/coverWidth + float64(y)/coverHeight) / 2
			r := lerp(pal[0].R, pal[1].R, t)
			g := lerp(pal[0].G, pal[1].G, t)
			b := lerp(pal[0].B, pal[1].B, t)
			// Soft accent glows.
			for _, gl := range glows {
				d := math.Hypot(float64(x)-gl.x, float64(y)-gl.y) / gl.r
				if d < 1 {
					w := gl.a * (1 - d) * (1 - d)
					r += (float64(pal[2].R) - r) * w
					g += (float64(pal[2].G) - g) * w
					b += (float64(pal[2].B) - b) * w
				}
			}
			// A faint grid, so it reads as a designed cover, not a blur.
			if x%96 == 0 || y%96 == 0 {
				r, g, b = r+10, g+10, b+12
			}
			img.SetRGBA(x, y, color.RGBA{clamp8(r), clamp8(g), clamp8(b), 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func clamp8(v float64) uint8 {
	switch {
	case v < 0:
		return 0
	case v > 255:
		return 255
	default:
		return uint8(v)
	}
}
