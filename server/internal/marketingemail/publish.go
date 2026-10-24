package marketingemail

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/jobshout/server/internal/integration/adapters/opsapi"
)

// pageTemplate is the CMS template the website keys on: it serves
// /email/{slug} only for pages with this template.
const pageTemplate = "email"

// maxSlug is the longest slug filed.
const maxSlug = 80

// maxSlugAttempts bounds the search for a free slug.
const maxSlugAttempts = 25

// Slugify makes a kebab-case ASCII slug of at most maxSlug characters.
func Slugify(s string) string {
	var b strings.Builder
	dash := true
	for _, r := range strings.ToLower(s) {
		switch {
		case r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			b.WriteRune(r)
			dash = false
		case !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > maxSlug {
		out = strings.Trim(out[:maxSlug], "-")
	}
	return out
}

// pageInput is everything a page upsert files.
type pageInput struct {
	Title, Slug, Excerpt, HTML, HeroImageURL, SEOTitle string
	// Live files the page as published; otherwise as a draft.
	Live bool
}

func (in pageInput) status() string {
	if in.Live {
		return opsapi.StatusPublished
	}
	return opsapi.StatusDraft
}

func (in pageInput) request() opsapi.CreatePageRequest {
	off := false
	return opsapi.CreatePageRequest{
		Title:            in.Title,
		Slug:             in.Slug,
		Excerpt:          in.Excerpt,
		ContentHTML:      in.HTML,
		FeaturedImageURL: in.HeroImageURL,
		Status:           in.status(),
		Template:         pageTemplate,
		ShowInNav:        &off,
		SEOTitle:         in.SEOTitle,
		SEODescription:   in.Excerpt,
	}
}

// uniqueSlug returns base, or base-2, base-3 … — the first no page uses.
func uniqueSlug(ctx context.Context, pages Pages, base string) (string, error) {
	if base == "" {
		return "", errors.New("marketingemail: empty slug")
	}
	for i := 1; i <= maxSlugAttempts; i++ {
		cand := base
		if i > 1 {
			suffix := fmt.Sprintf("-%d", i)
			if len(cand)+len(suffix) > maxSlug {
				cand = strings.Trim(cand[:maxSlug-len(suffix)], "-")
			}
			cand += suffix
		}
		p, err := pages.FindPageBySlug(ctx, cand)
		if err != nil {
			return "", err
		}
		if p == nil {
			return cand, nil
		}
	}
	return "", fmt.Errorf("marketingemail: no free slug near %q", base)
}

// upsertPage files the email as a CMS page: an update of the page already at
// in.Slug, else a new page. It never files to /cms/posts.
//
// An update the key may not make (403, wrapping opsapi.ErrUpdateForbidden) is
// returned as is — a duplicate page is never created to get round it. A page at
// the slug that is not an email page is refused rather than overwritten.
func upsertPage(ctx context.Context, pages Pages, in pageInput) (*opsapi.Page, error) {
	existing, err := pages.FindPageBySlug(ctx, in.Slug)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return pages.CreatePage(ctx, in.request())
	}
	if existing.Template != pageTemplate {
		return nil, fmt.Errorf("marketingemail: a page already uses the slug %q and is not an email page (template %q); not overwriting it",
			in.Slug, existing.Template)
	}
	page, err := pages.UpdatePage(ctx, existing.UUID, in.request())
	if err != nil {
		return nil, err
	}
	if page.Slug == "" {
		page.Slug = in.Slug
	}
	return page, nil
}
