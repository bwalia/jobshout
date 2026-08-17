package blog

import (
	"strings"
	"testing"
)

func TestRenderHTML(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		want     []string
		notWant  []string
	}{
		{
			name:     "headings and emphasis",
			markdown: "# Title\n\n## Section\n\nSome **bold** and *italic* text.",
			want:     []string{"<h2>Section</h2>", "<strong>bold</strong>", "<em>italic</em>"},
			// The leading H1 is the post title, which the CMS renders itself.
			notWant: []string{"<h1>"},
		},
		{
			name:     "fenced code block keeps its language",
			markdown: "# T\n\n```go\nfmt.Println(\"hi\")\n```",
			want:     []string{"<pre>", "<code class=\"language-go\">", "fmt.Println"},
		},
		{
			name:     "lists",
			markdown: "# T\n\n- one\n- two\n",
			want:     []string{"<ul>", "<li>one</li>", "<li>two</li>"},
		},
		{
			name:     "GFM table",
			markdown: "# T\n\n| a | b |\n|---|---|\n| 1 | 2 |",
			want:     []string{"<table>", "<th>a</th>", "<td>1</td>"},
			notWant:  []string{"|---|"},
		},
		{
			name:     "links",
			markdown: "# T\n\n[docs](https://example.com)",
			want:     []string{`<a href="https://example.com">docs</a>`},
		},
		{
			// The prompt asks for pure markdown; HTML in the output is the model
			// off-script, and this body goes onto a public page.
			name:     "raw HTML is escaped, not passed through",
			markdown: "# T\n\n<script>alert(1)</script>",
			notWant:  []string{"<script>"},
		},
		{
			// Models soft-wrap paragraphs. Those newlines must not become <br>
			// or the article renders with breaks mid-sentence in the CMS.
			name:     "a soft-wrapped paragraph reflows into one paragraph",
			markdown: "# T\n\nA sentence that the model wrapped\nacross two source lines.",
			want:     []string{"<p>A sentence that the model wrapped\nacross two source lines.</p>"},
			notWant:  []string{"<br>"},
		},
		{
			name:     "an H1 further down the body survives",
			markdown: "# Title\n\nIntro.\n\n# Later heading\n\nMore.",
			want:     []string{"<h1>Later heading</h1>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := renderHTML(tt.markdown)
			if err != nil {
				t.Fatalf("renderHTML: %v", err)
			}
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("output missing %q:\n%s", w, got)
				}
			}
			for _, n := range tt.notWant {
				if strings.Contains(got, n) {
					t.Errorf("output should not contain %q:\n%s", n, got)
				}
			}
		})
	}
}

func TestArticleTitle(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		topic    string
		want     string
	}{
		{"H1 wins", "# Debugging Kubernetes\n\nBody", "k8s debugging", "Debugging Kubernetes"},
		{"topic when there is no H1", "Just prose.", "k8s debugging", "k8s debugging"},
		{"topic when the H1 is empty", "#\n\nBody", "k8s debugging", "k8s debugging"},
		{"H2 is not a title", "## Section\n\nBody", "fallback", "fallback"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := articleTitle(tt.markdown, tt.topic); got != tt.want {
				t.Errorf("articleTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

// The whole article being one H1 line leaves nothing to render. It must not
// take the stripping logic out of bounds.
func TestStripLeadingH1_OnlyHeading(t *testing.T) {
	if got := stripLeadingH1("# Just a title"); got != "" {
		t.Errorf("stripLeadingH1() = %q, want empty", got)
	}
}

func TestArticleExcerpt(t *testing.T) {
	t.Run("strips markup and collapses whitespace", func(t *testing.T) {
		got := articleExcerpt("<h2>Heading</h2>\n<p>First <strong>real</strong> sentence.</p>")
		want := "Heading First real sentence."
		if got != want {
			t.Errorf("articleExcerpt() = %q, want %q", got, want)
		}
	})

	t.Run("resolves entities so the excerpt reads as text", func(t *testing.T) {
		got := articleExcerpt("<p>Ops &amp; monitoring</p>")
		if strings.Contains(got, "&amp;") {
			t.Errorf("entity left unresolved: %q", got)
		}
	})

	t.Run("trims on a word boundary", func(t *testing.T) {
		body := "<p>" + strings.Repeat("alpha ", 60) + "</p>"
		got := articleExcerpt(body)
		if len([]rune(got)) > excerptLimit+1 { // +1 for the ellipsis
			t.Errorf("excerpt is %d runes, want at most %d", len([]rune(got)), excerptLimit+1)
		}
		if !strings.HasSuffix(got, "…") {
			t.Errorf("a trimmed excerpt should be marked as such: %q", got)
		}
		if strings.Contains(got, "alph…") {
			t.Errorf("excerpt cut mid-word: %q", got)
		}
	})

	t.Run("empty body", func(t *testing.T) {
		if got := articleExcerpt(""); got != "" {
			t.Errorf("articleExcerpt(\"\") = %q, want empty", got)
		}
	})
}

// goldmark has no idea what mermaid is and renders the fence as a code block,
// which would publish the diagram's source as literal text. The div is what
// mermaid.js scans for.
func TestRenderHTML_MermaidBecomesADiv(t *testing.T) {
	md := "# Title\n\nBefore.\n\n```mermaid\nsequenceDiagram\n    A->>B: hi\n```\n\nAfter."

	got, err := renderHTML(md)
	if err != nil {
		t.Fatalf("renderHTML: %v", err)
	}
	if !strings.Contains(got, `<div class="mermaid">`) {
		t.Errorf("mermaid fence was not converted:\n%s", got)
	}
	if strings.Contains(got, `class="language-mermaid"`) {
		t.Errorf("the code block survived:\n%s", got)
	}
	if !strings.Contains(got, "sequenceDiagram") {
		t.Errorf("diagram source was lost:\n%s", got)
	}
}

// Only mermaid is rewritten. An article's shell and Go snippets must keep
// rendering as code.
func TestRenderHTML_OtherCodeBlocksAreUntouched(t *testing.T) {
	md := "# T\n\n```go\nfunc main() {}\n```\n\n```bash\nkubectl get pods\n```"

	got, err := renderHTML(md)
	if err != nil {
		t.Fatalf("renderHTML: %v", err)
	}
	for _, want := range []string{`class="language-go"`, `class="language-bash"`} {
		if !strings.Contains(got, want) {
			t.Errorf("expected %s to survive:\n%s", want, got)
		}
	}
	if strings.Contains(got, `class="mermaid"`) {
		t.Error("a non-mermaid block was converted")
	}
}

// A label containing a < must not become markup. goldmark escapes on the way
// in and mermaid reads text content, which the browser unescapes.
func TestRenderHTML_MermaidStaysEscaped(t *testing.T) {
	md := "# T\n\n```mermaid\nflowchart LR\n    A[\"a < b\"] --> B\n```"

	got, err := renderHTML(md)
	if err != nil {
		t.Fatalf("renderHTML: %v", err)
	}
	if strings.Contains(got, `A["a < b"]`) {
		t.Errorf("raw < survived into the markup:\n%s", got)
	}
	if !strings.Contains(got, "&lt;") {
		t.Errorf("expected the < to stay escaped:\n%s", got)
	}
}
