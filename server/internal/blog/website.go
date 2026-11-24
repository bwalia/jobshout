package blog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/imagestore"
	"github.com/jobshout/server/internal/integration/adapters/opsapi"
	"github.com/jobshout/server/internal/model"
)

// How a writer's posts are filed on its own website (model.PostSettings):
// byline, categories, tags, and whether the images live on the website.

// CategoryLister is implemented by the opsapi client: the website's blog
// categories, so a writer's category names can be filed as uuids.
type CategoryLister interface {
	ListCategories(ctx context.Context) ([]opsapi.Category, error)
}

// MediaUploader is implemented by the opsapi client: adds an image to the
// website's media library and returns its public URL.
type MediaUploader interface {
	UploadMedia(ctx context.Context, filename, contentType string, data []byte, title, alt string) (*opsapi.Media, error)
}

// resolveCategories turns a writer's category names (or slugs) into the
// website's category uuids, in the order given. A name the website does not
// have fails the publish and lists the ones it does: filing the post with no
// category would leave it off the site's category pages without anyone
// noticing, and creating one would let a typo invent a public category.
func resolveCategories(ctx context.Context, cms CMSPublisher, names []string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	lister, ok := cms.(CategoryLister)
	if !ok {
		return nil, fmt.Errorf("blog: this website's client cannot list categories, so writer categories cannot be filed")
	}
	cats, err := lister.ListCategories(ctx)
	if err != nil {
		return nil, fmt.Errorf("blog: list categories in %s: %w", cms.Namespace(), err)
	}
	out := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		var found *opsapi.Category
		for i := range cats {
			if strings.EqualFold(cats[i].Name, n) || strings.EqualFold(cats[i].Slug, n) {
				found = &cats[i]
				break
			}
		}
		if found == nil {
			have := make([]string, 0, len(cats))
			for _, c := range cats {
				have = append(have, c.Name)
			}
			return nil, fmt.Errorf("blog: category %q does not exist in %s (it has: %s) — fix the writer's categories or create it in the CMS",
				n, cms.Namespace(), orNone(strings.Join(have, ", ")))
		}
		if !seen[found.UUID] {
			seen[found.UUID] = true
			out = append(out, found.UUID)
		}
	}
	return out, nil
}

// postTags is what a post is tagged with: the writer's own tags when it has
// any, else who the piece was written for and its industry.
func postTags(a GeneratedArticle, post model.PostSettings) []string {
	if len(post.Tags) > 0 {
		return append([]string(nil), post.Tags...)
	}
	return articleTags(a)
}

// postAuthor is the byline: the writer's own, else the ring's.
func (r *Runner) postAuthor(post model.PostSettings) string {
	if a := strings.TrimSpace(post.Author); a != "" {
		return a
	}
	return r.cfg.AuthorName
}

// mermaidInkHost serves the diagrams in an article body (see mermaidToImg).
const mermaidInkHost = "mermaid.ink"

// imgSrc matches the src of an <img> tag in rendered article HTML.
var imgSrc = regexp.MustCompile(`(<img\b[^>]*?\bsrc=")([^"]+)(")`)

// imageFetchTimeout bounds fetching one image to upload it.
const imageFetchTimeout = 30 * time.Second

// HostImages copies an article's cover and the images in its body to the
// website's media library and points the post at those copies, so the website
// serves its own pictures instead of loading them from this ring (and from
// mermaid.ink for diagrams). It returns the body and featured image to file.
//
// Only images this ring stored, and mermaid.ink diagrams, are copied; any other
// URL is left alone rather than fetched. The cover must be copied — a featured
// image is required — while a body image that cannot be fetched or uploaded is
// left pointing where it was, with a warning. A website without image storage
// fails the whole call: the writer asked for its images on the website.
func (r *Runner) HostImages(ctx context.Context, cms CMSPublisher, title, html, featured string) (string, string, error) {
	up, ok := cms.(MediaUploader)
	if !ok {
		return "", "", fmt.Errorf("blog: this website's client cannot upload images")
	}
	hosted := map[string]string{}
	copyOne := func(src, name, alt string) (string, error) {
		if u, ok := hosted[src]; ok {
			return u, nil
		}
		data, ctype, err := r.fetchImage(ctx, src)
		if err != nil {
			return "", err
		}
		if ext := extFor(ctype, src); ext != "" {
			name += ext
		}
		m, err := up.UploadMedia(ctx, name, ctype, data, title, alt)
		if err != nil {
			return "", err
		}
		hosted[src] = m.URL
		return m.URL, nil
	}

	slug := slugify(title)
	if featured = strings.TrimSpace(featured); featured != "" {
		u, err := copyOne(featured, slug+"-cover", title)
		if err != nil {
			return "", "", fmt.Errorf("blog: copy cover to %s: %w", cms.Namespace(), err)
		}
		featured = u
	}

	var hostErr error
	n := 0
	html = imgSrc.ReplaceAllStringFunc(html, func(tag string) string {
		m := imgSrc.FindStringSubmatch(tag)
		src := m[2]
		if hostErr != nil || !r.copyable(src) {
			return tag
		}
		n++
		u, err := copyOne(src, fmt.Sprintf("%s-figure-%d", slug, n), "")
		if errors.Is(err, opsapi.ErrMediaNotConfigured) {
			hostErr = err
			return tag
		}
		if err != nil {
			r.logger.Warn("blog: body image not copied to the website; it stays where it was",
				zap.String("namespace", cms.Namespace()), zap.String("src", src), zap.Error(err))
			return tag
		}
		return strings.Replace(tag, m[1]+src+m[3], m[1]+u+m[3], 1)
	})
	if hostErr != nil {
		return "", "", fmt.Errorf("blog: copy images to %s: %w", cms.Namespace(), hostErr)
	}
	return html, featured, nil
}

// copyable reports whether HostImages may fetch src: an image in this ring's
// store, or a mermaid.ink diagram. Anything else is not ours to fetch.
func (r *Runner) copyable(src string) bool {
	if _, ok := r.storeKey(src); ok {
		return true
	}
	u, err := url.Parse(src)
	return err == nil && u.Scheme == "https" && strings.EqualFold(u.Hostname(), mermaidInkHost)
}

// storeKey is src's key in this ring's image store, when it is one of ours.
func (r *Runner) storeKey(src string) (string, bool) {
	s := strings.TrimSpace(src)
	if base := r.PublicBaseURL(); base != "" {
		s = strings.TrimPrefix(s, base)
	}
	key, ok := strings.CutPrefix(s, imagestore.URLPrefix)
	return key, ok && key != ""
}

// fetchImage reads an image to copy: from the store for our own (no round trip
// through the edge, which may sit behind a login on some rings), over HTTPS
// for a mermaid.ink diagram.
func (r *Runner) fetchImage(ctx context.Context, src string) ([]byte, string, error) {
	if key, ok := r.storeKey(src); ok && r.covers != nil {
		data, err := r.covers.Get(ctx, key)
		if err != nil {
			return nil, "", fmt.Errorf("read %s from the image store: %w", key, err)
		}
		return data, mime.TypeByExtension(path.Ext(key)), nil
	}
	abs := publicImageURL(r.PublicBaseURL(), src)
	if abs == "" || !strings.HasPrefix(abs, "https://") {
		return nil, "", fmt.Errorf("image %q has no https address to fetch", src)
	}
	ctx, cancel := context.WithTimeout(ctx, imageFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, abs, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("fetch %s: %w", abs, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("fetch %s: status %d", abs, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, opsapi.MaxMediaBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("fetch %s: %w", abs, err)
	}
	if len(data) > opsapi.MaxMediaBytes {
		return nil, "", fmt.Errorf("fetch %s: larger than %d bytes", abs, opsapi.MaxMediaBytes)
	}
	ctype := resp.Header.Get("Content-Type")
	if i := strings.IndexByte(ctype, ';'); i >= 0 {
		ctype = ctype[:i]
	}
	return data, strings.TrimSpace(ctype), nil
}

// extFor picks a file extension for an uploaded image's name. opsapi decides
// the real type from the bytes; the name is what its media library shows.
func extFor(contentType, src string) string {
	switch strings.ToLower(contentType) {
	case "image/svg+xml":
		return ".svg"
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	}
	if u, err := url.Parse(src); err == nil {
		if ext := strings.ToLower(path.Ext(u.Path)); len(ext) > 1 && len(ext) <= 5 {
			return ext
		}
	}
	return ""
}
