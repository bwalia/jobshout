package research

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseGitHubURL(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantOK   bool
		wantKind string
		wantRef  string
	}{
		{"repo root", "https://github.com/cilium/cilium", true, "repo", ""},
		{"repo trailing slash", "https://github.com/cilium/cilium/", true, "repo", ""},
		{"repo .git suffix", "https://github.com/cilium/cilium.git", true, "repo", ""},
		{"release by tag", "https://github.com/cilium/cilium/releases/tag/v1.16.0", true, "release", "v1.16.0"},
		{"latest release", "https://github.com/cilium/cilium/releases/latest", true, "release", "latest"},
		{"issue", "https://github.com/kubernetes/kubernetes/issues/12345", true, "issue", "12345"},
		// A pull request is an issue as far as the issues API is concerned, and
		// its body is the description either way.
		{"pull request", "https://github.com/kubernetes/kubernetes/pull/999", true, "issue", "999"},
		{"file blob", "https://github.com/cilium/cilium/blob/main/docs/intro.md", true, "file", "main/docs/intro.md"},

		// Unclaimed: no clean API equivalent, so the router falls through
		// rather than citing something adjacent to what was asked for.
		{"wiki", "https://github.com/cilium/cilium/wiki/Home", false, "", ""},
		{"actions", "https://github.com/cilium/cilium/actions", false, "", ""},
		{"release list", "https://github.com/cilium/cilium/releases", false, "", ""},
		{"org page", "https://github.com/cilium", false, "", ""},
		{"not github", "https://kubernetes.io/blog/", false, "", ""},
		{"gist", "https://gist.github.com/someone/abc", false, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseGitHubURL(tt.in)
			if ok != tt.wantOK {
				t.Fatalf("parseGitHubURL(%q) ok = %v, want %v", tt.in, ok, tt.wantOK)
			}
			if !tt.wantOK {
				return
			}
			if got.kind != tt.wantKind {
				t.Errorf("kind = %q, want %q", got.kind, tt.wantKind)
			}
			if got.ref != tt.wantRef {
				t.Errorf("ref = %q, want %q", got.ref, tt.wantRef)
			}
		})
	}
}

// newGitHubStub serves canned API responses keyed by request path.
func newGitHubStub(t *testing.T, routes map[string]string) *GitHubFetcher {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for path, body := range routes {
			if r.URL.Path == path {
				_, _ = w.Write([]byte(body))
				return
			}
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	return &GitHubFetcher{baseURL: srv.URL, client: srv.Client()}
}

func TestGitHubFetch_Repo(t *testing.T) {
	f := newGitHubStub(t, map[string]string{
		"/repos/cilium/cilium":        `{"full_name":"cilium/cilium","description":"eBPF-based Networking","stargazers_count":24934,"pushed_at":"2026-08-15T12:31:27Z"}`,
		"/repos/cilium/cilium/readme": "# Cilium\n\nCilium is a networking layer built on eBPF.",
	})

	doc, err := f.Fetch(context.Background(), "https://github.com/cilium/cilium")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if doc.Title != "cilium/cilium" {
		t.Errorf("title = %q", doc.Title)
	}
	if !strings.Contains(doc.Text, "built on eBPF") {
		t.Errorf("README body missing from text: %q", doc.Text)
	}
	if !strings.Contains(doc.Text, "eBPF-based Networking") {
		t.Errorf("repo description missing from text: %q", doc.Text)
	}
	if doc.Site != "github.com" {
		t.Errorf("site = %q", doc.Site)
	}
	if doc.PublishedAt == nil {
		t.Error("pushed_at was not parsed")
	}
}

// A repository with no README is still citable for its description, so a
// missing README degrades the document rather than failing the fetch.
func TestGitHubFetch_RepoWithoutReadme(t *testing.T) {
	f := newGitHubStub(t, map[string]string{
		"/repos/o/r": `{"full_name":"o/r","description":"A tool that does a thing","stargazers_count":5}`,
	})

	doc, err := f.Fetch(context.Background(), "https://github.com/o/r")
	if err != nil {
		t.Fatalf("Fetch failed on a repo with no README: %v", err)
	}
	if !strings.Contains(doc.Text, "does a thing") {
		t.Errorf("description missing: %q", doc.Text)
	}
}

func TestGitHubFetch_Release(t *testing.T) {
	f := newGitHubStub(t, map[string]string{
		"/repos/cilium/cilium/releases/tags/v1.16.0": `{"name":"1.16.0","tag_name":"v1.16.0","body":"Gateway API support is now stable.","published_at":"2024-07-24T15:53:07Z"}`,
	})

	doc, err := f.Fetch(context.Background(), "https://github.com/cilium/cilium/releases/tag/v1.16.0")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(doc.Text, "Gateway API support is now stable") {
		t.Errorf("release notes missing: %q", doc.Text)
	}
	if !strings.Contains(doc.Title, "1.16.0") {
		t.Errorf("title = %q, want the version in it", doc.Title)
	}
}

func TestGitHubFetch_Issue(t *testing.T) {
	f := newGitHubStub(t, map[string]string{
		"/repos/k/k/issues/42": `{"title":"Proposal: new scheduler","body":"We should change how pods are placed.","state":"open","created_at":"2026-01-02T00:00:00Z"}`,
	})

	doc, err := f.Fetch(context.Background(), "https://github.com/k/k/issues/42")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(doc.Text, "how pods are placed") {
		t.Errorf("issue body missing: %q", doc.Text)
	}
}

// An issue with an empty body has nothing to cite. Returning it as a document
// would let a claim be "verified" against a title alone.
func TestGitHubFetch_EmptyBodyIsAnError(t *testing.T) {
	f := newGitHubStub(t, map[string]string{
		"/repos/k/k/issues/1": `{"title":"Old issue","body":"","state":"closed"}`,
	})

	if _, err := f.Fetch(context.Background(), "https://github.com/k/k/issues/1"); err == nil {
		t.Fatal("Fetch accepted an issue with no description")
	}
}

func TestGitHubFetch_File(t *testing.T) {
	f := newGitHubStub(t, map[string]string{
		"/repos/o/r/contents/docs/intro.md": "# Intro\n\nThe contents of the file.",
	})

	doc, err := f.Fetch(context.Background(), "https://github.com/o/r/blob/main/docs/intro.md")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if !strings.Contains(doc.Text, "contents of the file") {
		t.Errorf("file body missing: %q", doc.Text)
	}
}

func TestGitHubFetch_UnsupportedURLIsAnError(t *testing.T) {
	f := newGitHubStub(t, nil)
	if _, err := f.Fetch(context.Background(), "https://github.com/o/r/wiki/Home"); err == nil {
		t.Fatal("Fetch accepted a URL shape it cannot serve")
	}
}

// Rate limiting is reported distinctly from other 403s because the fix differs:
// "wait, or set a token" rather than "this content is private".
func TestGitHubFetch_RateLimitSaysHowToFixIt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	f := &GitHubFetcher{baseURL: srv.URL, client: srv.Client()}

	_, err := f.Fetch(context.Background(), "https://github.com/o/r")
	if err == nil {
		t.Fatal("Fetch succeeded against a rate-limited API")
	}
	if !strings.Contains(err.Error(), "GITHUB_TOKEN") {
		t.Errorf("error %q does not say how to raise the limit", err)
	}
}

func TestGitHubFetch_SendsTokenWhenSet(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"full_name":"o/r","description":"d"}`))
	}))
	t.Cleanup(srv.Close)
	f := &GitHubFetcher{baseURL: srv.URL, token: "secret", client: srv.Client()}

	if _, err := f.Fetch(context.Background(), "https://github.com/o/r"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if gotAuth != "Bearer secret" {
		t.Errorf("Authorization = %q, want the bearer token", gotAuth)
	}
}

func TestGitHubFetch_NoTokenSendsNoAuthHeader(t *testing.T) {
	var hadAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadAuth = r.Header["Authorization"]
		_, _ = w.Write([]byte(`{"full_name":"o/r","description":"d"}`))
	}))
	t.Cleanup(srv.Close)
	f := &GitHubFetcher{baseURL: srv.URL, client: srv.Client()}

	if _, err := f.Fetch(context.Background(), "https://github.com/o/r"); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if hadAuth {
		t.Error("sent an Authorization header with no token configured")
	}
}

// The router is the point of the GitHub work: Jina blocks github.com, so those
// URLs must never reach it.
func TestRoutingFetcher_SendsGitHubToTheAPIAndTheRestToFallback(t *testing.T) {
	gh := newGitHubStub(t, map[string]string{
		"/repos/o/r": `{"full_name":"o/r","description":"from the github api"}`,
	})
	fallback := stubFetcher{doc: &Document{Source: Source{URL: "x"}, Text: "from the fallback"}}
	r := NewRoutingFetcher(gh, nil, fallback)

	ghDoc, err := r.Fetch(context.Background(), "https://github.com/o/r")
	if err != nil {
		t.Fatalf("github route: %v", err)
	}
	if !strings.Contains(ghDoc.Text, "from the github api") {
		t.Errorf("github URL did not go to the API: %q", ghDoc.Text)
	}

	otherDoc, err := r.Fetch(context.Background(), "https://kubernetes.io/blog/")
	if err != nil {
		t.Fatalf("fallback route: %v", err)
	}
	if otherDoc.Text != "from the fallback" {
		t.Errorf("non-github URL did not go to the fallback: %q", otherDoc.Text)
	}
}

// A GitHub failure is not retried through the fallback: the fallback is Jina,
// which blocks GitHub anyway, so it would only replace a precise error with a
// vague one after a slow round trip.
func TestRoutingFetcher_DoesNotRetryGitHubThroughFallback(t *testing.T) {
	gh := newGitHubStub(t, nil) // every path 404s
	fallback := stubFetcher{doc: &Document{Text: "should not be reached"}}
	r := NewRoutingFetcher(gh, nil, fallback)

	doc, err := r.Fetch(context.Background(), "https://github.com/o/r")
	if err == nil {
		t.Fatalf("expected the GitHub error to surface, got %q", doc.Text)
	}
	if strings.Contains(err.Error(), "should not be reached") {
		t.Error("the fallback was used for a GitHub URL")
	}
}

// An unclaimed GitHub path (a wiki) still routes to the fallback, since the
// GitHub fetcher never claimed it.
func TestRoutingFetcher_UnclaimedGitHubPathUsesFallback(t *testing.T) {
	gh := newGitHubStub(t, nil)
	fallback := stubFetcher{doc: &Document{Text: "fallback handled it"}}
	r := NewRoutingFetcher(gh, nil, fallback)

	doc, err := r.Fetch(context.Background(), "https://github.com/o/r/wiki/Home")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if doc.Text != "fallback handled it" {
		t.Errorf("got %q, want the fallback's document", doc.Text)
	}
}
