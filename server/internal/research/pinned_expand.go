package research

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// pinnedExpandLimit caps how many linked pages a pinned-mode run may read on
// top of the pinned pages themselves.
const pinnedExpandLimit = 4

var (
	// moneyRe is a currency symbol followed by a digit — the cheapest reliable
	// signal that a page states an amount rather than merely discussing money.
	moneyRe = regexp.MustCompile(`[$£€]\s?\d`)
	// mdLinkRe captures markdown links as Jina Reader renders them.
	mdLinkRe = regexp.MustCompile(`\[([^\]]{0,200})\]\((https?://[^)\s]+)\)`)
)

// priceLinkWords are the anchor-text / URL-path signals that a link leads to
// where a site actually states its amounts (buy flow, pricing page, press
// release with launch prices).
var priceLinkWords = []string{
	"buy", "shop", "store", "price", "pricing", "order", "purchase",
	"checkout", "newsroom", "plans",
}

// priceQuestionWords mark an inbound question as being about money.
var priceQuestionWords = []string{
	"price", "pricing", "cost", "how much", "$", "£", "€",
}

// expandPinnedForPrices follows a few of the pinned pages' own links when the
// inbound question asks about money and none of the pinned pages state an
// amount — the marketing-page-vs-buy-flow gap of issue #105. It only ever
// fetches links on the same host as a pinned page, never searches the open
// web, and says so in Warnings when it still comes back without an amount, so
// the drafter's "not listed" reply has a recorded reason.
func (a *Agent) expandPinnedForPrices(ctx context.Context, req Request, docs []Document, brief *Brief, tried map[string]struct{}, progress ProgressFunc) []Document {
	if len(docs) == 0 || !questionAsksPrice(req) || docsMentionMoney(docs) {
		return docs
	}
	extra := priceLinkCandidates(docs, pinnedExpandLimit, tried)
	if len(extra) == 0 {
		brief.Warnings = append(brief.Warnings,
			"the question asks about price, but the pinned pages state no amounts and have no same-site pricing links to follow")
		return docs
	}
	progress.report(PhaseReading, fmt.Sprintf(
		"Pinned page(s) state no prices; reading %d same-site pricing link(s)", len(extra)))
	docs = append(docs, a.read(ctx, extra, brief, len(extra), tried)...)
	if !docsMentionMoney(docs) {
		brief.Warnings = append(brief.Warnings,
			"no prices were found on the pinned pages or their same-site pricing links")
	}
	return docs
}

func questionAsksPrice(req Request) bool {
	haystack := strings.ToLower(req.Topic + "\n" + req.Context)
	for _, w := range priceQuestionWords {
		if strings.Contains(haystack, w) {
			return true
		}
	}
	return false
}

func docsMentionMoney(docs []Document) bool {
	for _, d := range docs {
		if moneyRe.MatchString(d.Text) {
			return true
		}
	}
	return false
}

// priceLinkCandidates pulls same-host links with a pricing signal out of the
// fetched pages' markdown, in page order, deduplicated and capped.
func priceLinkCandidates(docs []Document, limit int, tried map[string]struct{}) []Source {
	hosts := make(map[string]struct{}, len(docs))
	for _, d := range docs {
		if h := bareHost(d.URL); h != "" {
			hosts[h] = struct{}{}
		}
	}
	seen := make(map[string]struct{})
	var out []Source
	for _, d := range docs {
		for _, m := range mdLinkRe.FindAllStringSubmatch(d.Text, -1) {
			anchor := strings.ToLower(m[1])
			u, err := validateURL(m[2])
			if err != nil {
				continue
			}
			if _, same := hosts[bareHost(u)]; !same {
				continue
			}
			if !hasPriceSignal(anchor, u) {
				continue
			}
			key := canonicalURL(u)
			if key == "" {
				key = u
			}
			if _, dup := seen[key]; dup {
				continue
			}
			if _, was := tried[u]; was {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, Source{URL: u, Site: siteOf(u)})
			if len(out) >= limit {
				return out
			}
		}
	}
	return out
}

func hasPriceSignal(anchor, rawURL string) bool {
	path := ""
	if u, err := url.Parse(rawURL); err == nil {
		path = strings.ToLower(u.Path)
	}
	for _, w := range priceLinkWords {
		if strings.Contains(anchor, w) || strings.Contains(path, w) {
			return true
		}
	}
	return false
}

func bareHost(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Host), "www.")
}
