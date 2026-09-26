package research

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// The issue #105 shape: a marketing page that states no prices but links to
// the same-host buy flow and newsroom, where the prices actually live.
const marketingText = `The most powerful Mac ever. Mac Studio delivers groundbreaking
performance in a compact form built around M5 Max and M5 Ultra.

[Buy Mac Studio](https://www.apple.com/shop/buy-mac/mac-studio)
[Read the announcement](https://www.apple.com/newsroom/2026/03/new-mac-studio/)
[Compare all models](https://www.apple.com/mac/compare/)
[Independent price tracker](https://prices.example.com/mac-studio)

Learn more about its performance, connectivity and design.`

const buyPageText = `Configure your new Mac Studio. The M5 Max model starts at $2,499.00 with free delivery. The M5 Ultra model starts at $5,499.00 with free delivery. All orders ship within two days.`

const newsroomText = `Apple today announced the new Mac Studio, its most powerful desktop yet, featuring the M5 Max and M5 Ultra chips with breakthrough performance.`

func pinnedPriceBackend() *fixedBackend {
	return &fixedBackend{
		docs: map[string]*Document{
			"https://www.apple.com/mac-studio/": {
				Source: Source{URL: "https://www.apple.com/mac-studio/", Title: "Mac Studio", Site: "apple.com"},
				Text:   marketingText,
			},
			"https://www.apple.com/shop/buy-mac/mac-studio": {
				Source: Source{URL: "https://www.apple.com/shop/buy-mac/mac-studio", Title: "Buy Mac Studio", Site: "apple.com"},
				Text:   buyPageText,
			},
			"https://www.apple.com/newsroom/2026/03/new-mac-studio/": {
				Source: Source{URL: "https://www.apple.com/newsroom/2026/03/new-mac-studio/", Title: "New Mac Studio", Site: "apple.com"},
				Text:   newsroomText,
			},
		},
	}
}

func fetchedSet(backend *fixedBackend) map[string]bool {
	out := make(map[string]bool)
	for _, u := range backend.fetchURLs() {
		out[u] = true
	}
	return out
}

// A price question against a pinned page with no amounts must follow the
// page's own same-host buy/pricing/newsroom links and quote only what those
// fetched pages state — never an off-host link, never open-web search.
func TestResearch_PinnedPriceQuestionFollowsSameHostPricingLinks(t *testing.T) {
	backend := pinnedPriceBackend()
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "URL:\nhttps://www.apple.com/mac-studio/", content: `{"findings": []}`},
		{trigger: "shop/buy-mac", content: `{"findings": [
			{"claim": "The Mac Studio M5 Max model starts at $2,499.", "quote": "The M5 Max model starts at $2,499.00 with free delivery."},
			{"claim": "The Mac Studio M5 Ultra model starts at $5,499.", "quote": "The M5 Ultra model starts at $5,499.00 with free delivery."}]}`},
		{trigger: "newsroom", content: `{"findings": []}`},
		{trigger: "fact-checking citations", content: `{"verdicts": [{"index": 0, "supported": true}, {"index": 1, "supported": true}]}`},
		{trigger: "Summarise the current state", content: "The M5 Max starts at $2,499 and the M5 Ultra at $5,499 per the Apple buy page."},
	}}
	agent := newTestAgent(t, backend, model)

	brief, err := agent.Research(context.Background(), pinnedMailRequest(
		"", "[js-test] What is the price of the Mac Studio?",
		"Hi, what is the price of the Mac Studio?",
		[]string{"https://www.apple.com/mac-studio/"},
	), nil)
	if err != nil {
		t.Fatalf("Research returned error: %v", err)
	}
	if backend.searchCalls() != 0 {
		t.Fatalf("Search called %d times; pinned mode must never search the open web", backend.searchCalls())
	}
	fetched := fetchedSet(backend)
	if !fetched["https://www.apple.com/shop/buy-mac/mac-studio"] {
		t.Errorf("buy page not fetched; got %v", backend.fetchURLs())
	}
	if !fetched["https://www.apple.com/newsroom/2026/03/new-mac-studio/"] {
		t.Errorf("newsroom page not fetched; got %v", backend.fetchURLs())
	}
	if fetched["https://prices.example.com/mac-studio"] {
		t.Error("followed an off-host link; expansion must stay on the pinned page's host")
	}
	if fetched["https://www.apple.com/mac/compare/"] {
		t.Error("followed a link with no pricing signal in its anchor or path")
	}
	var sawPrice bool
	for _, f := range brief.Findings {
		if strings.Contains(f.Claim, "$2,499") {
			sawPrice = true
			if f.SourceURL != "https://www.apple.com/shop/buy-mac/mac-studio" {
				t.Errorf("price claim cites %s, want the buy page", f.SourceURL)
			}
		}
	}
	if !sawPrice {
		t.Fatalf("no finding carries the buy-page price; findings: %+v", brief.Findings)
	}
	if !brief.IsUsable() {
		t.Error("brief reports itself unusable despite verified price findings")
	}
}

// When the pinned page itself states an amount there is nothing to chase:
// only the pinned URL may be fetched.
func TestResearch_PinnedExpansionSkippedWhenPageStatesPrices(t *testing.T) {
	backend := pinnedPriceBackend()
	priced := strings.Replace(marketingText, "compact form", "compact form from $1,999", 1)
	backend.docs["https://www.apple.com/mac-studio/"].Text = priced
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "You extract facts from one knowledge page", content: `{"findings": [
			{"claim": "Mac Studio starts at $1,999.", "quote": "Mac Studio delivers groundbreaking performance in a compact form from $1,999"}]}`},
		{trigger: "fact-checking citations", content: `{"verdicts": [{"index": 0, "supported": true}]}`},
		{trigger: "Summarise the current state", content: "Mac Studio starts at $1,999 per the pinned page."},
	}}
	agent := newTestAgent(t, backend, model)

	_, err := agent.Research(context.Background(), pinnedMailRequest(
		"", "What is the price of the Mac Studio?", "How much is it?",
		[]string{"https://www.apple.com/mac-studio/"},
	), nil)
	if err != nil {
		t.Fatalf("Research returned error: %v", err)
	}
	if got := backend.fetchURLs(); len(got) != 1 {
		t.Fatalf("fetched %v, want the pinned URL only when it already states a price", got)
	}
}

// A question that is not about money must not trigger the price expansion.
func TestResearch_PinnedExpansionSkippedForNonPriceQuestion(t *testing.T) {
	backend := pinnedPriceBackend()
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "You extract facts from one knowledge page", content: `{"findings": []}`},
	}}
	agent := newTestAgent(t, backend, model)

	_, err := agent.Research(context.Background(), pinnedMailRequest(
		"", "Mac Studio colours", "Does the Mac Studio come in black?",
		[]string{"https://www.apple.com/mac-studio/"},
	), nil)
	if err != nil {
		t.Fatalf("Research returned error: %v", err)
	}
	if got := backend.fetchURLs(); len(got) != 1 {
		t.Fatalf("fetched %v, want the pinned URL only for a non-price question", got)
	}
}

// Expansion is capped, and coming back still empty-handed is said out loud so
// the drafter's honest "not listed" has a reason attached.
func TestResearch_PinnedExpansionCapsFetchesAndWarnsWhenStillNoPrices(t *testing.T) {
	var links strings.Builder
	links.WriteString("Our widget page.\n")
	backend := &fixedBackend{docs: map[string]*Document{}}
	for i := 1; i <= 6; i++ {
		u := fmt.Sprintf("https://shop.example.com/buy-widget-%d", i)
		fmt.Fprintf(&links, "[Buy option %d](%s)\n", i, u)
		backend.docs[u] = &Document{
			Source: Source{URL: u, Site: "shop.example.com"},
			Text:   "This configuration page lists no amounts at all.",
		}
	}
	backend.docs["https://shop.example.com/widget"] = &Document{
		Source: Source{URL: "https://shop.example.com/widget", Site: "shop.example.com"},
		Text:   links.String(),
	}
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "You extract facts from one knowledge page", content: `{"findings": []}`},
	}}
	agent := newTestAgent(t, backend, model)

	brief, err := agent.Research(context.Background(), pinnedMailRequest(
		"", "Widget cost", "What is the price of the widget?",
		[]string{"https://shop.example.com/widget"},
	), nil)
	if err != nil {
		t.Fatalf("Research returned error: %v", err)
	}
	if got := len(backend.fetchURLs()); got != 1+pinnedExpandLimit {
		t.Fatalf("fetched %d pages %v, want pinned + at most %d expansions",
			got, backend.fetchURLs(), pinnedExpandLimit)
	}
	if !warningsContain(brief.Warnings, "no prices") {
		t.Errorf("warnings %v should record that no prices were found anywhere", brief.Warnings)
	}
}

// A price question against a pinned page with no amounts and no pricing links
// records why research came back empty-handed.
func TestResearch_PinnedPriceQuestionWithNoLinksWarns(t *testing.T) {
	backend := &fixedBackend{docs: map[string]*Document{
		"https://shop.example.com/widget": {
			Source: Source{URL: "https://shop.example.com/widget", Site: "shop.example.com"},
			Text:   "A lovely widget with no links and no amounts mentioned anywhere.",
		},
	}}
	model := &scriptedLLM{responses: []scriptedResponse{
		{trigger: "You extract facts from one knowledge page", content: `{"findings": []}`},
	}}
	agent := newTestAgent(t, backend, model)

	brief, err := agent.Research(context.Background(), pinnedMailRequest(
		"", "Widget cost", "How much does the widget cost?",
		[]string{"https://shop.example.com/widget"},
	), nil)
	if err != nil {
		t.Fatalf("Research returned error: %v", err)
	}
	if got := backend.fetchURLs(); len(got) != 1 {
		t.Fatalf("fetched %v, want the pinned URL only when it links nowhere", got)
	}
	if !warningsContain(brief.Warnings, "no same-site pricing links") {
		t.Errorf("warnings %v should say the page had no pricing links to follow", brief.Warnings)
	}
}

func warningsContain(warnings []string, needle string) bool {
	for _, w := range warnings {
		if strings.Contains(w, needle) {
			return true
		}
	}
	return false
}
