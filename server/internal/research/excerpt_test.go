package research

import (
	"strings"
	"testing"
)

// navLines builds a block of the link-per-line markdown a reader emits for a
// site's navigation and table of contents.
func navLines(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString("*   [What is eBPF?](https://ebpf.io/what-is-ebpf/#what-is-ebpf)\n")
	}
	return b.String()
}

const proseParagraph = "eBPF is a revolutionary technology with origins in the Linux kernel that " +
	"can run sandboxed programs in a privileged context such as the operating system kernel. " +
	"It is used to safely and efficiently extend the capabilities of the kernel without " +
	"requiring to change kernel source code or load kernel modules.\n"

// The case that motivated the change: a page whose first several thousand
// characters are navigation. Taking the head hands the extractor a menu.
func TestProseExcerptSkipsLeadingNavigation(t *testing.T) {
	doc := navLines(120) + "\n" + strings.Repeat(proseParagraph, 6)

	got := proseExcerpt(doc, 2000)

	if strings.Contains(got, "](https://ebpf.io/what-is-ebpf/#what-is-ebpf)") {
		t.Errorf("excerpt still contains navigation links:\n%s", got)
	}
	if !strings.Contains(got, "sandboxed programs in a privileged context") {
		t.Errorf("excerpt missed the prose entirely:\n%s", got)
	}
}

// The excerpt has to be a contiguous slice of the document, because extraction
// quotes it and verification checks those quotes against the full text.
func TestProseExcerptIsContiguousInTheSource(t *testing.T) {
	doc := navLines(40) + strings.Repeat(proseParagraph, 4) + navLines(40)

	got := strings.TrimSpace(proseExcerpt(doc, 1500))

	if got == "" {
		t.Fatal("empty excerpt")
	}
	if !strings.Contains(doc, got) {
		t.Errorf("excerpt is not a substring of the document:\n%s", got)
	}
}

// A document that fits is passed through untouched — no window to choose.
func TestProseExcerptShortDocumentUnchanged(t *testing.T) {
	doc := proseParagraph
	if got := proseExcerpt(doc, 6000); got != doc {
		t.Errorf("short document was altered:\ngot  %q\nwant %q", got, doc)
	}
}

// With no prose anywhere there is nothing to prefer, so the old behaviour
// stands rather than returning nothing.
func TestProseExcerptFallsBackToHead(t *testing.T) {
	doc := navLines(200)

	got := proseExcerpt(doc, 500)

	if got == "" {
		t.Fatal("empty excerpt for an all-navigation document")
	}
	if !strings.HasPrefix(doc, strings.TrimSuffix(got, "…")) {
		t.Errorf("fallback did not return the head of the document:\n%s", got)
	}
}

// Ties go to the earlier window: a page's opening is more often its thesis.
func TestProseExcerptPrefersTheEarlierOfEqualWindows(t *testing.T) {
	block := strings.Repeat(proseParagraph, 3)
	doc := block + navLines(60) + block

	got := proseExcerpt(doc, len([]rune(block))+10)

	if idx := strings.Index(doc, strings.TrimSpace(got)); idx > len(block) {
		t.Errorf("chose the later window (offset %d, first block ends %d)", idx, len(block))
	}
}

func TestProseExcerptRespectsLimit(t *testing.T) {
	doc := strings.Repeat(proseParagraph, 40)

	got := proseExcerpt(doc, 800)

	// truncate appends an ellipsis, so allow one rune of slack.
	if n := len([]rune(got)); n > 801 {
		t.Errorf("excerpt is %d runes, limit was 800", n)
	}
}

func TestProseScoreRejectsLinkLists(t *testing.T) {
	cases := []struct {
		name string
		line string
		want bool // true when the line should count as prose
	}{
		{"nav link", "*   [Get Started](https://ebpf.io/get-started/)", false},
		{"heading", "# eBPF Documentation", false},
		{"image", "![Image 5](https://ebpf.io/static/logo-black.svg)", false},
		{"short label", "What is eBPF?", false},
		{"sentence", proseParagraph, true},
		{"bulleted prose", "* Programs are verified before they are allowed to run, which is what makes this safe.", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := proseScore(strings.TrimSuffix(tc.line, "\n")) > 0; got != tc.want {
				t.Errorf("proseScore(%q) counted as prose = %v, want %v", tc.line, got, tc.want)
			}
		})
	}
}
