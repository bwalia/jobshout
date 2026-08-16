package research

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
)

// Topic is a subject worth writing about, discovered rather than supplied.
//
// It is not the trending item it came from. "Cilium 1.18 released" is a
// headline about one event; "what Cilium's move to a kube-proxy-free datapath
// means for cluster operators" is something you can write eight hundred words
// on. The conversion between those two is the whole job of this file — a
// pipeline handed raw headlines writes news summaries, which is not what
// anybody wants from a technical blog.
type Topic struct {
	// Topic is the subject, phrased as the article's brief would phrase it.
	Topic string `json:"topic"`
	// Context is the angle and audience the discovery reasoned its way to, fed
	// to the writer exactly as a human-supplied context would be.
	Context string `json:"context"`
	// Rationale is why this is worth writing now, kept for the audit trail so
	// a scheduled run's choices can be understood after the fact.
	Rationale string `json:"rationale"`
	// Seeds are the trending URLs that prompted this topic. They are not
	// citations — research runs independently and finds its own sources — but
	// they show where the idea came from.
	Seeds []string `json:"seeds"`
}

// DiscoverRequest bounds a discovery sweep.
type DiscoverRequest struct {
	// Count is how many topics to return.
	Count int
	// Avoid lists subjects already written about recently. Discovery is run on
	// a schedule, and a story stays on the front page for days — without this a
	// daily job writes the same article all week.
	Avoid []string
	// Model optionally overrides the LLM.
	Model string
}

// discoverCandidates is how many trending items are put in front of the model.
// Generous because the filtering happens there: the sweep is one round of cheap
// HTTP, and a wider net gives the selection something to choose between.
const discoverCandidates = 40

// Discover finds subjects currently worth writing about.
//
// The shape mirrors the research loop deliberately — gather broadly, then let
// the model judge — because the judgement is the same kind: which of these
// things is actually about the domain we care about, and which merely shares
// its vocabulary.
func (a *Agent) Discover(ctx context.Context, req DiscoverRequest, progress ProgressFunc) ([]Topic, error) {
	if a.llm == nil {
		return nil, fmt.Errorf("research: llm client is nil")
	}
	if a.sources == nil {
		return nil, fmt.Errorf("research: source client is nil")
	}
	count := req.Count
	if count <= 0 {
		count = 1
	}

	progress.report(PhaseDiscovering, "Looking at what is trending")

	items, err := a.sources.Trending(ctx, discoverCandidates)
	if err != nil {
		return nil, fmt.Errorf("research: discover: %w", err)
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("research: discover: nothing is trending right now")
	}

	// Drop anything whose headline obviously restates something already
	// written. This is a cheap pre-filter on top of the model's own judgement,
	// not a replacement for it: exact-ish title overlap is easy to catch here
	// and wastes a slot in the prompt if left in.
	items = dropSeenTitles(items, req.Avoid)
	if len(items) == 0 {
		return nil, fmt.Errorf("research: discover: everything trending has been written about recently")
	}

	progress.report(PhaseDiscovering,
		fmt.Sprintf("Choosing %d topic(s) from %d trending item(s)", count, len(items)))

	topics, err := a.chooseTopics(ctx, req, items, count)
	if err != nil {
		return nil, err
	}
	if len(topics) == 0 {
		return nil, fmt.Errorf("research: discover: found nothing worth writing about that has not been covered")
	}

	a.logger.Info("research: discovered topics",
		zap.Int("candidates", len(items)),
		zap.Int("chosen", len(topics)),
		zap.Int("avoided", len(req.Avoid)),
	)
	return topics, nil
}

// chooseTopics asks the model to turn trending items into writable subjects.
func (a *Agent) chooseTopics(ctx context.Context, req DiscoverRequest, items []TrendingItem, count int) ([]Topic, error) {
	var b strings.Builder
	for i, it := range items {
		published := "unknown date"
		if it.PublishedAt != nil {
			published = it.PublishedAt.Format("2006-01-02")
		}
		fmt.Fprintf(&b, "\n[%d] %s\n    %s | %s | %s", i, it.Title, it.Site, published, it.Channel)
		if it.Score > 0 {
			fmt.Fprintf(&b, " | %d points", it.Score)
		}
		if it.Excerpt != "" {
			fmt.Fprintf(&b, "\n    %s", truncate(it.Excerpt, 160))
		}
		b.WriteString("\n")
	}

	avoid := "(nothing yet)"
	if len(req.Avoid) > 0 {
		avoid = "- " + strings.Join(req.Avoid, "\n- ")
	}

	prompt := fmt.Sprintf(`You are choosing what a technical blog should write about this week. The blog
covers software engineering, AI and infrastructure, for a developer audience.

WHAT IS TRENDING RIGHT NOW:
%s

ALREADY WRITTEN ABOUT RECENTLY — do not propose these again, or close variants:
%s

Choose the %d best subjects to write about.

Turn each into a TOPIC, not a headline. A trending item is one event; a topic is
something an engineer can read 1000 words about and come away more capable.
"Cilium 1.18 released" is an event. "What a kube-proxy-free datapath changes for
cluster operators" is a topic.

Choose subjects that:
- Are genuinely about software, AI or infrastructure. Trending lists carry
  politics, business and general news — ignore all of it.
- Have enough substance for a technical article, not just an announcement.
- A working engineer would actually benefit from understanding.

Avoid:
- Pure funding, acquisition and company-drama stories.
- Anything you cannot imagine a code example or a concrete trade-off in.
- Subjects too close to the already-written list above.

For each, also write the CONTEXT: who it is for and what angle to take, in the
form you would brief a writer. And a one-line RATIONALE for why it is worth
writing now.

Reference the candidate numbers you drew on in "seeds".

Respond with JSON only, in exactly this shape:
{"topics": [{"topic": "...", "context": "...", "rationale": "...", "seeds": [0, 4]}]}`,
		b.String(), avoid, count)

	resp, err := a.generate(ctx, req.Model, prompt)
	if err != nil {
		return nil, fmt.Errorf("research: discover: %w", err)
	}

	var parsed struct {
		Topics []struct {
			Topic     string `json:"topic"`
			Context   string `json:"context"`
			Rationale string `json:"rationale"`
			Seeds     []int  `json:"seeds"`
		} `json:"topics"`
	}
	if err := llm.DecodeJSON(resp, &parsed); err != nil {
		return nil, fmt.Errorf("research: discover: parse response: %w", err)
	}

	out := make([]Topic, 0, len(parsed.Topics))
	for _, t := range parsed.Topics {
		topic := strings.TrimSpace(t.Topic)
		if topic == "" {
			continue
		}
		// A proposal that matches something already written is dropped here as
		// well as in the prompt: the instruction is a request, this is the
		// guarantee, and a scheduled job repeating itself is the failure that
		// makes the whole feature untrustworthy.
		if matchesAny(topic, req.Avoid) {
			a.logger.Info("research: discarded a topic already covered", zap.String("topic", topic))
			continue
		}

		seeds := make([]string, 0, len(t.Seeds))
		for _, idx := range t.Seeds {
			if idx >= 0 && idx < len(items) {
				seeds = append(seeds, items[idx].URL)
			}
		}

		out = append(out, Topic{
			Topic:     topic,
			Context:   strings.TrimSpace(t.Context),
			Rationale: strings.TrimSpace(t.Rationale),
			Seeds:     seeds,
		})
		if len(out) >= count {
			break
		}
	}
	return out, nil
}

// dropSeenTitles removes trending items whose headline restates something
// already written about.
func dropSeenTitles(items []TrendingItem, avoid []string) []TrendingItem {
	if len(avoid) == 0 {
		return items
	}
	out := make([]TrendingItem, 0, len(items))
	for _, it := range items {
		if !matchesAny(it.Title, avoid) {
			out = append(out, it)
		}
	}
	return out
}

// topicOverlapThreshold is the fraction of significant words two subjects must
// share to count as the same story.
//
// Set moderately rather than high: the cost of skipping a borderline duplicate
// is one article not written, while the cost of missing one is a schedule that
// republishes the same piece for as long as it trends.
const topicOverlapThreshold = 0.6

// matchesAny reports whether s covers the same ground as any of the candidates.
//
// This is word overlap, not similarity in any deeper sense — it catches
// "Kubernetes Gateway API goes GA" against "Gateway API reaches GA in
// Kubernetes", which is the case that actually recurs, and does not pretend to
// catch a genuinely different angle on the same technology.
func matchesAny(s string, candidates []string) bool {
	target := significantWords(s)
	if len(target) == 0 {
		return false
	}
	for _, c := range candidates {
		other := significantWords(c)
		if len(other) == 0 {
			continue
		}
		shared := 0
		for w := range target {
			if _, ok := other[w]; ok {
				shared++
			}
		}
		// Measured against the shorter of the two, so a long descriptive topic
		// still matches the short headline it duplicates.
		shorter := len(target)
		if len(other) < shorter {
			shorter = len(other)
		}
		if float64(shared)/float64(shorter) >= topicOverlapThreshold {
			return true
		}
	}
	return false
}

// stopWords are too common to carry meaning in a technical headline, and would
// otherwise make any two sentences look related.
var stopWords = map[string]struct{}{
	"a": {}, "an": {}, "the": {}, "and": {}, "or": {}, "but": {}, "for": {},
	"to": {}, "of": {}, "in": {}, "on": {}, "at": {}, "by": {}, "with": {},
	"from": {}, "is": {}, "are": {}, "was": {}, "were": {}, "be": {}, "been": {},
	"it": {}, "its": {}, "this": {}, "that": {}, "these": {}, "those": {},
	"how": {}, "what": {}, "why": {}, "when": {}, "your": {}, "you": {},
	"new": {}, "using": {}, "use": {}, "guide": {}, "introduction": {},
}

// significantWords reduces a phrase to its meaningful lowercase words.
func significantWords(s string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, w := range strings.Fields(normaliseForCompare(s)) {
		if len(w) < 3 {
			continue
		}
		if _, skip := stopWords[w]; skip {
			continue
		}
		out[w] = struct{}{}
	}
	return out
}
