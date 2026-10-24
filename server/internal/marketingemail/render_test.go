package marketingemail

import (
	"regexp"
	"strings"
	"testing"

	"github.com/jobshout/server/internal/model"
)

func TestRender_Golden(t *testing.T) {
	d := goodDraft()
	d.Subject = `Fast "AI" <b>shop</b>`
	d.Sections[0].BodyMarkdown = "Hello <script>alert('x')</script> **bold** and *soft*\n\n" +
		"- one <img src=x onerror=alert(1)>\n- two\n\n" +
		"[ok](https://www.workstation.co.uk/shop) [bad](javascript:void) [http](http://x.example)"
	d.Sections[0].Items = []string{`item "quoted" <i>`}
	d.Sections = append([]model.MarketingEmailSection{
		{Type: "quote", Heading: "A customer", BodyMarkdown: "Great {{ .Secret }} kit"},
	}, d.Sections...)

	out, err := Render(d, "https://img.example/hero.png", Chrome{})
	if err != nil {
		t.Fatal(err)
	}

	for _, must := range []string{
		"<!doctype html>", `lang="en"`, `<meta charset="utf-8">`, `name="viewport"`,
		`name="color-scheme" content="light dark"`, "<title>Fast &#34;AI&#34; &lt;b&gt;shop&lt;/b&gt;</title>",
		`width="600"`, `role="presentation"`, "#0d0b21", "#e5165f",
		"<strong>bold</strong>", "<em>soft</em>",
		`href="https://www.workstation.co.uk/shop"`,
		"Manage preferences / unsubscribe", DefaultUnsubscribeURL, DefaultLogoURL,
		"&lt;script&gt;alert(&#39;x&#39;)&lt;/script&gt;",
		"&lt;img src=x onerror=alert(1)&gt;",
	} {
		if !strings.Contains(out, must) {
			t.Errorf("output missing %q", must)
		}
	}
	for _, mustNot := range []string{
		"<script", "onerror=alert(1)>", "javascript:", "http://x.example", "{{", "}}",
		"<iframe", "<form", `rel="stylesheet"`, "<b>shop", "<i>",
	} {
		if strings.Contains(out, mustNot) {
			t.Errorf("output contains %q", mustNot)
		}
	}
}

func TestRender_EveryImgHasAltSizeAndBlock(t *testing.T) {
	out, err := Render(goodDraft(), "https://img.example/hero.png", Chrome{})
	if err != nil {
		t.Fatal(err)
	}
	imgs := regexp.MustCompile(`<img [^>]*>`).FindAllString(out, -1)
	if len(imgs) != 2 {
		t.Fatalf("imgs = %d, want logo + hero", len(imgs))
	}
	for _, img := range imgs {
		for _, want := range []string{` alt="`, ` width="`, ` height="`, "display:block"} {
			if !strings.Contains(img, want) {
				t.Errorf("img %s missing %s", img, want)
			}
		}
		if strings.Contains(img, `alt=""`) {
			t.Errorf("img has empty alt: %s", img)
		}
	}
}

func TestRender_NoHeroWithoutHTTPSImage(t *testing.T) {
	for _, u := range []string{"", "http://x.example/a.png", "/api/images/a.png", "javascript:alert(1)"} {
		out, err := Render(goodDraft(), u, Chrome{})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Count(out, "<img ") != 1 {
			t.Errorf("hero %q: want only the logo image, got %d imgs", u, strings.Count(out, "<img "))
		}
	}
}

func TestRender_InlineStylesAnd600Table(t *testing.T) {
	out, err := Render(goodDraft(), "", Chrome{})
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`<table[^>]*class="container"[^>]*width="600"`).MatchString(out) {
		t.Error("no 600px container table")
	}
	// Only one <style> block, and only for media queries.
	if n := strings.Count(out, "<style>"); n != 1 {
		t.Errorf("style blocks = %d, want 1", n)
	}
	style := out[strings.Index(out, "<style>"):strings.Index(out, "</style>")]
	if !strings.Contains(style, "@media") {
		t.Error("style block has no media query")
	}
	if !strings.Contains(out, `style="margin:0 0 16px 0;font-family:Arial`) {
		t.Error("paragraphs are not inline-styled")
	}
}

func TestRender_ChromeFromEnv(t *testing.T) {
	t.Setenv(EnvLogoURL, "https://cdn.example/logo.png")
	t.Setenv(EnvUnsubscribeURL, "https://esp.example/unsub")
	t.Setenv(EnvCompanyAddress, "1 Test Street, London")
	out, err := Render(goodDraft(), "", ChromeFromEnv())
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"https://cdn.example/logo.png", "https://esp.example/unsub", "1 Test Street, London"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q", want)
		}
	}
	// A non-https env value is ignored in favour of the default.
	t.Setenv(EnvLogoURL, "http://insecure.example/logo.png")
	if got := ChromeFromEnv().LogoURL; got != DefaultLogoURL {
		t.Errorf("logo = %q", got)
	}
}

func TestMarkdownHTML_Allowlist(t *testing.T) {
	got := string(MarkdownHTML("# Title\n\nPara one\nstill one.\n\n1. a\n2. b\n\n- x"))
	for _, want := range []string{"<p ", "Title", "<ol ", "<li ", "<ul "} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "<h1") || strings.Contains(got, "# Title") {
		t.Errorf("heading markup leaked: %s", got)
	}
}

func TestBrowserURL(t *testing.T) {
	tests := []struct{ base, slug, override, want string }{
		{"https://int-opsapi.workstation.co.uk", "ai-shop", "", "https://int.workstation.co.uk/email/ai-shop"},
		{"https://opsapi.workstation.co.uk/", "ai-shop", "", "https://www.workstation.co.uk/email/ai-shop"},
		{"https://INT-OPSAPI.workstation.co.uk:443", "ai-shop", "", "https://int.workstation.co.uk/email/ai-shop"},
		{"https://opsapi.workstation.co.uk", "ai-shop", "https://staging.example.com/", "https://staging.example.com/email/ai-shop"},
		{"https://opsapi.workstation.co.uk", "ai-shop", "http://insecure.example", "https://www.workstation.co.uk/email/ai-shop"},
		{"https://other.example.com", "ai-shop", "", ""},
		{"https://opsapi.workstation.co.uk", "", "", ""},
	}
	for _, tt := range tests {
		if got := BrowserURL(tt.base, tt.slug, tt.override); got != tt.want {
			t.Errorf("BrowserURL(%q,%q,%q) = %q, want %q", tt.base, tt.slug, tt.override, got, tt.want)
		}
	}
}

func TestSlugify(t *testing.T) {
	if got := Slugify("  The AI Shop — is LIVE!! "); got != "the-ai-shop-is-live" {
		t.Errorf("got %q", got)
	}
	if got := Slugify(strings.Repeat("abc ", 40)); len(got) > maxSlug || strings.HasSuffix(got, "-") {
		t.Errorf("long slug = %q", got)
	}
}
