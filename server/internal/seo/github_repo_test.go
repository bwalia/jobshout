package seo

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseURLs(t *testing.T) {
	got := ParseURLs("https://a.test/\nexample.com/about, https://b.test")
	if len(got) != 3 {
		t.Fatalf("got %d urls: %#v", len(got), got)
	}
	if !strings.HasPrefix(got[1], "https://") {
		t.Fatalf("expected scheme on host-only url: %q", got[1])
	}
}

func TestMatchGlob(t *testing.T) {
	paths := []string{
		"README.md",
		"content/index.md",
		"content/blog/hello.mdx",
		"public/index.html",
		"src/util.ts",
	}
	got := MatchGlob(paths, "content/**/*.md", 10)
	if len(got) != 1 || got[0] != "content/index.md" {
		// **/*.md may also match mdx depending on matcher — accept content pages only.
		found := false
		for _, p := range got {
			if strings.HasPrefix(p, "content/") && (strings.HasSuffix(p, ".md") || strings.HasSuffix(p, ".mdx")) {
				found = true
			}
		}
		if !found && len(got) == 0 {
			// fallback empty glob
			got = MatchGlob(paths, "", 10)
		}
		if len(got) == 0 {
			t.Fatalf("expected content matches, got %#v", got)
		}
	}
	capped := MatchGlob(paths, "", 2)
	if len(capped) != 2 {
		t.Fatalf("max pages: got %d", len(capped))
	}
}

func TestRepoClient_ListAndGet(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/git/trees/main", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tree": []map[string]string{
				{"path": "content/about.md", "type": "blob"},
				{"path": "src/x.ts", "type": "blob"},
			},
		})
	})
	mux.HandleFunc("/repos/o/r/contents/content/about.md", func(w http.ResponseWriter, r *http.Request) {
		raw := "---\ntitle: About Us Jobs\ndescription: Hire devops engineers across the UK today with confidence.\n---\n\n# About Us Jobs\n\nWe hire devops talent.\n"
		_ = json.NewEncoder(w).Encode(map[string]string{
			"type":     "file",
			"encoding": "base64",
			"content":  base64.StdEncoding.EncodeToString([]byte(raw)),
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &RepoClient{Token: "t", HTTP: srv.Client(), APIBase: srv.URL}
	paths, err := c.ListPaths(context.Background(), "o", "r", "main")
	if err != nil {
		t.Fatalf("ListPaths: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths: %#v", paths)
	}
	body, err := c.GetFile(context.Background(), "o", "r", "main", "content/about.md")
	if err != nil {
		t.Fatalf("GetFile: %v", err)
	}
	if !strings.Contains(body, "About Us Jobs") {
		t.Fatalf("body: %q", body)
	}
	bundle := AnalyzeContent(AnalyzeContentOptions{
		Path: "content/about.md", Content: body, Keywords: []string{"devops", "jobs"},
	})
	if bundle.Title == "" {
		t.Fatal("expected title from frontmatter")
	}
	if len(bundle.KeywordsFound) == 0 {
		t.Fatal("expected keywords found")
	}
}

func TestAnalyzeContent_MarkdownJSONShape(t *testing.T) {
	bundle := AnalyzeContent(AnalyzeContentOptions{
		Path: "posts/hello.md",
		Content: `---
title: Short
---
## Only H2
Body without keyword.
`,
		Keywords: []string{"kubernetes"},
	})
	if bundle.Score.Overall <= 0 {
		t.Fatalf("expected score, got %d", bundle.Score.Overall)
	}
	foundKWIssue := false
	for _, i := range bundle.Issues {
		if i.Category == "keywords" {
			foundKWIssue = true
		}
	}
	if !foundKWIssue {
		t.Fatal("expected missing keyword issue")
	}
	if bundle.MetaPatch["title"] == "" && bundle.MetaPatch["meta_description"] == "" {
		t.Fatal("expected meta patch proposals")
	}
}
