package seo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAnalyze_ScoresBasicPage(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html><html><head>
<title>DevOps Jobs UK — Hire Engineers</title>
<meta name="description" content="Find devops and site reliability roles across the UK. Post jobs and hire engineers faster.">
<link rel="canonical" href="https://example.test/">
<meta property="og:title" content="DevOps Jobs UK">
</head><body><h1>DevOps Jobs UK</h1><p>Hire engineers for devops and kubernetes teams.</p></body></html>`))
	})
	mux.HandleFunc("/robots.txt", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("User-agent: *\nAllow: /\nSitemap: https://example.test/sitemap.xml\n"))
	})
	mux.HandleFunc("/sitemap.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9"><url><loc>https://example.test/</loc></url></urlset>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	bundle, err := Analyze(context.Background(), AnalyzeOptions{
		URL:      srv.URL,
		Keywords: []string{"devops", "jobs"},
	})
	if err != nil {
		t.Fatalf("Analyze: %v", err)
	}
	if bundle.Title == "" {
		t.Fatal("expected title")
	}
	if !bundle.SitemapFound {
		t.Fatal("expected sitemap found")
	}
	if bundle.Score.Overall < 50 {
		t.Fatalf("expected decent score, got %d", bundle.Score.Overall)
	}
	if len(bundle.KeywordsFound) == 0 {
		t.Fatal("expected keywords found")
	}
	if !strings.Contains(bundle.Title, "DevOps") {
		t.Fatalf("unexpected title %q", bundle.Title)
	}
}

func TestInferSiteStructure_NextPublic(t *testing.T) {
	st := InferSiteStructure([]string{
		"README.md",
		"public/index.html",
		"public/sitemap.xml",
		"content/about.md",
		"next.config.js",
	}, "Static marketing site")
	if st.PublicRoot != "public" {
		t.Fatalf("public root: %q", st.PublicRoot)
	}
	if st.ContentRoot != "content" {
		t.Fatalf("content root: %q", st.ContentRoot)
	}
	if st.TemplateHint != "nextjs" {
		t.Fatalf("template: %q", st.TemplateHint)
	}
	if !st.HasSitemap {
		t.Fatal("expected sitemap")
	}
}

func TestParseKeywords(t *testing.T) {
	got := ParseKeywords("devops, sre; hiring")
	if len(got) != 3 {
		t.Fatalf("got %#v", got)
	}
}
