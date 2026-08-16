package research

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GitHubAPIBase is the public REST API.
const GitHubAPIBase = "https://api.github.com"

// githubRateLimit is what an unauthenticated caller gets per hour. It is not
// enforced here — GitHub does that — but it explains why a token is worth
// setting: research reads several GitHub sources per article, and a busy
// schedule will exhaust 60 requests well before the hour is up.
const githubRateLimit = 60

// GitHubFetcher retrieves GitHub content through GitHub's own API.
//
// It exists because Jina Reader refuses github.com anonymously — it answers
// "AbuseAlleviationError: Anonymous access to domain github.com blocked" — and
// GitHub is where a large share of infrastructure and AI sources live. A live
// research run lost two of its three reads to exactly that. Routing those URLs
// to the API instead recovers them, and returns better text than scraping the
// HTML would: a README or a release note without the page furniture around it.
//
// The API is keyless, at 60 requests an hour. A token raises that to 5000 and
// is optional — the same "free by default, better with a key" arrangement the
// rest of this package uses.
type GitHubFetcher struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewGitHubFetcher builds a fetcher. token may be empty.
func NewGitHubFetcher(token string) *GitHubFetcher {
	return &GitHubFetcher{
		baseURL: GitHubAPIBase,
		token:   strings.TrimSpace(token),
		client:  &http.Client{Timeout: 20 * time.Second},
	}
}

// Handles reports whether this fetcher can retrieve rawURL.
//
// It is deliberately narrow: it claims only the URL shapes it can genuinely
// turn into an API call. A shape it does not recognise is left unclaimed rather
// than fetched as something adjacent — citing a repository's README for a URL
// that pointed at a wiki page would attribute a claim to text that never
// contained it.
func (f *GitHubFetcher) Handles(rawURL string) bool {
	_, ok := parseGitHubURL(rawURL)
	return ok
}

// githubTarget is a github.com URL resolved to the API call that serves it.
type githubTarget struct {
	owner string
	repo  string
	// kind is one of "repo", "release", "issue", "file".
	kind string
	// ref is the release tag, issue number, or "branch/path" for a file.
	ref string
}

// parseGitHubURL maps a github.com URL onto an API target.
func parseGitHubURL(rawURL string) (githubTarget, bool) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return githubTarget{}, false
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	if host != "github.com" {
		return githubTarget{}, false
	}

	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return githubTarget{}, false // an org page, not a repo
	}
	t := githubTarget{owner: parts[0], repo: strings.TrimSuffix(parts[1], ".git")}

	if len(parts) == 2 {
		t.kind = "repo"
		return t, true
	}

	switch parts[2] {
	case "releases":
		// /releases/tag/v1.16.0 — a release list has no single body to read.
		if len(parts) >= 5 && parts[3] == "tag" {
			t.kind, t.ref = "release", strings.Join(parts[4:], "/")
			return t, true
		}
		if len(parts) == 4 && parts[3] == "latest" {
			t.kind, t.ref = "release", "latest"
			return t, true
		}
		return githubTarget{}, false
	case "issues", "pull":
		// Pull requests are issues as far as the issues API is concerned, and
		// its response carries the description body either way.
		if len(parts) >= 4 && parts[3] != "" {
			t.kind, t.ref = "issue", parts[3]
			return t, true
		}
		return githubTarget{}, false
	case "blob":
		// /blob/main/docs/thing.md
		if len(parts) >= 5 {
			t.kind, t.ref = "file", strings.Join(parts[3:], "/")
			return t, true
		}
		return githubTarget{}, false
	default:
		// Wikis, actions, projects, discussions and the rest have no clean API
		// equivalent worth the mapping. Leave them unclaimed.
		return githubTarget{}, false
	}
}

// Fetch retrieves the target and returns it as a Document.
func (f *GitHubFetcher) Fetch(ctx context.Context, rawURL string) (*Document, error) {
	clean, err := validateURL(rawURL)
	if err != nil {
		return nil, err
	}
	t, ok := parseGitHubURL(clean)
	if !ok {
		return nil, fmt.Errorf("research: github: %q is not a supported GitHub URL", clean)
	}

	switch t.kind {
	case "repo":
		return f.fetchRepo(ctx, t, clean)
	case "release":
		return f.fetchRelease(ctx, t, clean)
	case "issue":
		return f.fetchIssue(ctx, t, clean)
	case "file":
		return f.fetchFile(ctx, t, clean)
	default:
		return nil, fmt.Errorf("research: github: unsupported target %q", t.kind)
	}
}

// fetchRepo returns the repository description followed by its README, which
// together are what a reader visiting the repo would actually take in.
func (f *GitHubFetcher) fetchRepo(ctx context.Context, t githubTarget, sourceURL string) (*Document, error) {
	var meta struct {
		FullName    string `json:"full_name"`
		Description string `json:"description"`
		Stars       int    `json:"stargazers_count"`
		PushedAt    string `json:"pushed_at"`
		Homepage    string `json:"homepage"`
	}
	if err := f.getJSON(ctx, fmt.Sprintf("/repos/%s/%s", t.owner, t.repo), &meta); err != nil {
		return nil, err
	}

	// A repo with no README is still worth citing for its description, so a
	// missing one degrades the document rather than failing the fetch.
	readme, err := f.getRaw(ctx, fmt.Sprintf("/repos/%s/%s/readme", t.owner, t.repo))
	if err != nil {
		readme = ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", meta.FullName)
	if meta.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", meta.Description)
	}
	fmt.Fprintf(&b, "%d stars. Last pushed %s.\n\n", meta.Stars, meta.PushedAt)
	b.WriteString(readme)

	text := strings.TrimSpace(b.String())
	if text == "" {
		return nil, fmt.Errorf("research: github: %s has no readable content", sourceURL)
	}

	return &Document{
		Source: Source{
			URL:         sourceURL,
			Title:       firstNonEmpty(meta.FullName, t.owner+"/"+t.repo),
			Site:        "github.com",
			Excerpt:     truncate(meta.Description, 300),
			PublishedAt: parseGitHubTime(meta.PushedAt),
		},
		Text:      text,
		FetchedAt: time.Now(),
	}, nil
}

// fetchRelease returns a release's notes, which are frequently the most
// citable statement of what changed in a version.
func (f *GitHubFetcher) fetchRelease(ctx context.Context, t githubTarget, sourceURL string) (*Document, error) {
	path := fmt.Sprintf("/repos/%s/%s/releases/tags/%s", t.owner, t.repo, t.ref)
	if t.ref == "latest" {
		path = fmt.Sprintf("/repos/%s/%s/releases/latest", t.owner, t.repo)
	}

	var rel struct {
		Name        string `json:"name"`
		TagName     string `json:"tag_name"`
		Body        string `json:"body"`
		PublishedAt string `json:"published_at"`
	}
	if err := f.getJSON(ctx, path, &rel); err != nil {
		return nil, err
	}

	body := strings.TrimSpace(rel.Body)
	if body == "" {
		return nil, fmt.Errorf("research: github: release %s has no notes", sourceURL)
	}
	title := fmt.Sprintf("%s/%s %s", t.owner, t.repo, firstNonEmpty(rel.Name, rel.TagName))

	return &Document{
		Source: Source{
			URL:         sourceURL,
			Title:       title,
			Site:        "github.com",
			Excerpt:     truncate(body, 300),
			PublishedAt: parseGitHubTime(rel.PublishedAt),
		},
		Text:      "# " + title + "\n\n" + body,
		FetchedAt: time.Now(),
	}, nil
}

// fetchIssue returns an issue or pull request description.
func (f *GitHubFetcher) fetchIssue(ctx context.Context, t githubTarget, sourceURL string) (*Document, error) {
	var issue struct {
		Title     string `json:"title"`
		Body      string `json:"body"`
		CreatedAt string `json:"created_at"`
		State     string `json:"state"`
	}
	if err := f.getJSON(ctx, fmt.Sprintf("/repos/%s/%s/issues/%s", t.owner, t.repo, t.ref), &issue); err != nil {
		return nil, err
	}

	body := strings.TrimSpace(issue.Body)
	if body == "" {
		return nil, fmt.Errorf("research: github: %s has no description", sourceURL)
	}

	return &Document{
		Source: Source{
			URL:         sourceURL,
			Title:       fmt.Sprintf("%s/%s: %s", t.owner, t.repo, issue.Title),
			Site:        "github.com",
			Excerpt:     truncate(body, 300),
			PublishedAt: parseGitHubTime(issue.CreatedAt),
		},
		Text:      fmt.Sprintf("# %s\n\n(%s)\n\n%s", issue.Title, issue.State, body),
		FetchedAt: time.Now(),
	}, nil
}

// fetchFile returns a single file's contents, for URLs that point at
// documentation inside a repository.
func (f *GitHubFetcher) fetchFile(ctx context.Context, t githubTarget, sourceURL string) (*Document, error) {
	// ref is "branch/path/to/file"; the contents API wants them separated.
	ref, path, ok := strings.Cut(t.ref, "/")
	if !ok || path == "" {
		return nil, fmt.Errorf("research: github: %q does not name a file", sourceURL)
	}

	text, err := f.getRaw(ctx, fmt.Sprintf("/repos/%s/%s/contents/%s?ref=%s",
		t.owner, t.repo, path, url.QueryEscape(ref)))
	if err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, fmt.Errorf("research: github: %s is empty", sourceURL)
	}

	return &Document{
		Source: Source{
			URL:   sourceURL,
			Title: fmt.Sprintf("%s/%s: %s", t.owner, t.repo, path),
			Site:  "github.com",
		},
		Text:      text,
		FetchedAt: time.Now(),
	}, nil
}

// getJSON performs an API call and decodes the response.
func (f *GitHubFetcher) getJSON(ctx context.Context, path string, out any) error {
	body, err := f.get(ctx, path, "application/vnd.github+json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(body), out); err != nil {
		return fmt.Errorf("research: github: decode %s: %w", path, err)
	}
	return nil
}

// getRaw performs an API call asking for the raw file body rather than the
// JSON envelope, which avoids base64-decoding the contents ourselves.
func (f *GitHubFetcher) getRaw(ctx context.Context, path string) (string, error) {
	return f.get(ctx, path, "application/vnd.github.raw")
}

func (f *GitHubFetcher) get(ctx context.Context, path, accept string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.baseURL+path, nil)
	if err != nil {
		return "", fmt.Errorf("research: github: build request: %w", err)
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "JobShout-ResearchAgent/1.0")
	if f.token != "" {
		req.Header.Set("Authorization", "Bearer "+f.token)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("research: github: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, jinaMaxBody))
	if err != nil {
		return "", fmt.Errorf("research: github: read body: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusNotFound:
		return "", fmt.Errorf("research: github: %s not found", path)
	case resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0":
		// Distinguished from a plain 403 because the fix is different: this one
		// is "wait, or set GITHUB_TOKEN", not "this content is private".
		return "", fmt.Errorf(
			"research: github: rate limit exhausted (%d/hour unauthenticated); set GITHUB_TOKEN to raise it",
			githubRateLimit)
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return "", fmt.Errorf("research: github: HTTP %d for %s", resp.StatusCode, path)
	}
	return string(body), nil
}

// parseGitHubTime parses the RFC3339 timestamps the API returns.
func parseGitHubTime(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return nil
	}
	return &t
}
