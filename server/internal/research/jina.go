package research

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// JinaBaseURL is the Reader endpoint. A URL appended to it comes back as clean
// markdown with the navigation, ads and script tags stripped.
const JinaBaseURL = "https://r.jina.ai/"

// jinaMaxBody caps the response read. Reader output for a long article is tens
// of kilobytes; a few megabytes means we asked for something that is not an
// article, and reading all of it only serves to blow up the next prompt.
const jinaMaxBody = 4 << 20 // 4 MB

// jinaTimeout is generous because Reader renders JavaScript pages server-side,
// which is slow for heavy sites. Below roughly 30s the slowest legitimate pages
// start failing.
const jinaTimeout = 45 * time.Second

// JinaFetcher retrieves pages through Jina Reader.
//
// Reader is used rather than fetching and parsing HTML in-process for two
// reasons. It does the extraction — boilerplate removal is a genuinely hard
// problem and a bad job at it poisons every citation built on the result. And
// it performs the request from its own network, which is what keeps an
// LLM-chosen URL from being a request forgery primitive against ours.
type JinaFetcher struct {
	baseURL string
	client  *http.Client
}

// NewJinaFetcher builds a Fetcher against the public Reader endpoint.
func NewJinaFetcher() *JinaFetcher {
	return &JinaFetcher{
		baseURL: JinaBaseURL,
		client:  &http.Client{Timeout: jinaTimeout},
	}
}

// jinaResponse is Reader's JSON envelope, requested via Accept:
// application/json. The default text response inlines the metadata as a
// "Title: …\n\nURL Source: …" preamble that has to be parsed back out; asking
// for JSON avoids guessing where the preamble ends and the article begins.
type jinaResponse struct {
	Code int `json:"code"`
	Data *struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		URL         string `json:"url"`
		Content     string `json:"content"`
		// HTTPStatus is the status the *target* returned, which is not the
		// status of our call to Reader. See Fetch for why this matters.
		HTTPStatus int `json:"httpStatus"`
	} `json:"data"`
	// ReadableMessage carries the failure reason on non-2xx envelopes, e.g. an
	// unresolvable domain.
	ReadableMessage string `json:"readableMessage"`
	Message         string `json:"message"`
}

// Fetch retrieves rawURL and returns its extracted body.
//
// The status handling is the load-bearing part. Reader answers 200 for a page
// that itself 404s, and hands back the site's not-found page as content — for
// kubernetes.io that is nearly 5KB of perfectly readable boilerplate. An agent
// checking whether a source supports a claim would be reading a real page that
// simply is not the cited one, and would sometimes conclude it says nothing
// contradictory. So a target status of 400 or above is a fetch failure here,
// not a document with an unfortunate title.
func (f *JinaFetcher) Fetch(ctx context.Context, rawURL string) (*Document, error) {
	clean, err := validateURL(rawURL)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.baseURL+clean, nil)
	if err != nil {
		return nil, fmt.Errorf("research: build reader request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	// Reader streams by default for some content types; this asks for the
	// whole document in one response.
	req.Header.Set("X-Return-Format", "markdown")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("research: fetch %q: %w", clean, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, jinaMaxBody))
	if err != nil {
		return nil, fmt.Errorf("research: read %q: %w", clean, err)
	}

	var parsed jinaResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("research: decode reader response for %q: %w", clean, err)
	}

	// A non-2xx envelope means Reader could not retrieve the page at all —
	// an unresolvable domain, a malformed URL, a rate limit. Its message is
	// specific and worth surfacing verbatim, since it is the difference
	// between "this citation is dead" and "we are being throttled".
	if resp.StatusCode < 200 || resp.StatusCode >= 300 || parsed.Data == nil {
		return nil, fmt.Errorf("research: fetch %q: %s", clean, jinaError(&parsed, resp.StatusCode))
	}
	if parsed.Data.HTTPStatus >= 400 {
		return nil, fmt.Errorf("research: fetch %q: target returned HTTP %d", clean, parsed.Data.HTTPStatus)
	}

	text := strings.TrimSpace(parsed.Data.Content)
	if text == "" {
		return nil, fmt.Errorf("research: fetch %q: page has no extractable text", clean)
	}

	// Prefer the URL Reader reports, which is post-redirect: citing the
	// resolved page is more useful than citing the shortener that led to it.
	finalURL := clean
	if parsed.Data.URL != "" {
		finalURL = parsed.Data.URL
	}

	return &Document{
		Source: Source{
			URL:     finalURL,
			Title:   strings.TrimSpace(parsed.Data.Title),
			Site:    siteOf(finalURL),
			Excerpt: truncate(strings.TrimSpace(parsed.Data.Description), 300),
		},
		Text:      text,
		FetchedAt: time.Now(),
	}, nil
}

// jinaError picks the most specific message available for a failed envelope.
func jinaError(r *jinaResponse, statusCode int) string {
	switch {
	case r.ReadableMessage != "":
		return r.ReadableMessage
	case r.Message != "":
		return r.Message
	default:
		return fmt.Sprintf("reader returned HTTP %d", statusCode)
	}
}
