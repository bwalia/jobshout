package blog

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// coverLayout is the kind of diagram a cover draws. Each is a picture of an
// idea an article can be about — a path, a hub, a loop, a falling cost, two
// opposing effects, a failure spreading, a pruned search, a hidden mass, a
// policy flowing down, a metric crossing its line — so the cover says what
// the article argues instead of repeating its title on a gradient.
type coverLayout string

const (
	layoutFlow      coverLayout = "flow"
	layoutHub       coverLayout = "hub"
	layoutLoop      coverLayout = "loop"
	layoutBars      coverLayout = "bars"
	layoutVersus    coverLayout = "versus"
	layoutRipple    coverLayout = "ripple"
	layoutTree      coverLayout = "tree"
	layoutIceberg   coverLayout = "iceberg"
	layoutHierarchy coverLayout = "hierarchy"
	layoutThreshold coverLayout = "threshold"
)

// coverLayouts is every layout, in the order the plan prompt lists them.
var coverLayouts = []coverLayout{
	layoutFlow, layoutHub, layoutLoop, layoutBars, layoutVersus,
	layoutRipple, layoutTree, layoutIceberg, layoutHierarchy, layoutThreshold,
}

// coverLayoutGuide is the plan prompt's menu: what each layout shows and what
// its labels become, so the model picks by meaning rather than by name.
const coverLayoutGuide = `- flow: 3-5 steps left to right (labels = the steps, last one highlighted)
- hub: one core connecting 4-6 things (labels = core first, then the things)
- loop: 3-4 stages that repeat (labels = the stages; optional 5th = the boundary, e.g. "guardrails")
- bars: a number that falls or rises in steps (labels = the steps, optionally "caching 76"; stat = the headline, e.g. "-68%")
- versus: one change with a good and a bad effect (labels = the good effect, the bad effect)
- ripple: a failure spreading through dependent parts (labels = up to 3 legend states)
- tree: many candidate paths, weak ones pruned (labels = what the survivors become, then a caption)
- iceberg: a visible effect over hidden ones (labels = the visible one first, then 3-5 hidden)
- hierarchy: one policy or standard applied to 3 units (labels = the policy, then the 3 units)
- threshold: a metric drifting past its limit and raising an alert (labels = the distribution, the score, the alert, the record)`

// coverScene is what a diagram cover draws: a layout, the short labels the
// article supplies for it, an optional headline number, and a 1-3 word topic
// tag for the corner.
type coverScene struct {
	Layout  coverLayout `json:"layout"`
	Labels  []string    `json:"labels"`
	Stat    string      `json:"stat,omitempty"`
	Eyebrow string      `json:"eyebrow,omitempty"`
	// Picture is the real-world scene the art under the diagram shows: the
	// article's subject as things in a place. Without it the art was drawn
	// from the labels, and a property-tax cover came out as a server room.
	Picture string `json:"picture,omitempty"`
	// Headline, Trend, TrendFrom and TrendTo are the headline card's: the
	// article's point set large after the stat, and the numbers its chart
	// draws with what the first and last are.
	Headline  string    `json:"headline,omitempty"`
	Trend     []float64 `json:"trend,omitempty"`
	TrendFrom string    `json:"trend_from,omitempty"`
	TrendTo   string    `json:"trend_to,omitempty"`
	// Dek is one sentence saying what the article explains, set under the
	// headline in place of the title. Notes say what each label means here,
	// keyed by the label in lower case: grounding and cleaning drop and
	// reorder labels, so a list beside them would drift.
	Dek   string            `json:"dek,omitempty"`
	Notes map[string]string `json:"notes,omitempty"`
	// Facts are the figures a reader should take away, set along a card's
	// foot. Each figure must appear in the article as written.
	Facts []coverFact `json:"facts,omitempty"`
}

// coverFact is one figure the article states and what it is.
type coverFact struct {
	Figure string `json:"figure"`
	Label  string `json:"label"`
}

// maxCoverFacts is how many facts a card's foot holds.
const maxCoverFacts = 3

// readFact takes a fact as the object asked for, under the key names a model
// tends to use, or as "figure: label" text.
func readFact(v any) (coverFact, bool) {
	var f coverFact
	switch t := v.(type) {
	case map[string]any:
		for _, k := range []string{"figure", "value", "number", "stat"} {
			switch x := t[k].(type) {
			case string:
				f.Figure = x
			case float64:
				f.Figure = strconv.FormatFloat(x, 'f', -1, 64)
			}
			if f.Figure != "" {
				break
			}
		}
		for _, k := range []string{"label", "caption", "text", "what"} {
			if x, ok := t[k].(string); ok && x != "" {
				f.Label = x
				break
			}
		}
	case string:
		for _, sep := range []string{": ", " — ", " - "} {
			if i := strings.Index(t, sep); i > 0 {
				f.Figure, f.Label = t[:i], t[i+len(sep):]
				break
			}
		}
	}
	f.Figure, f.Label = strings.TrimSpace(f.Figure), strings.TrimSpace(f.Label)
	return f, f.Figure != "" && f.Label != ""
}

// groundFacts keeps the facts whose figure the article states, not the stat
// again, at most maxCoverFacts.
func groundFacts(facts []coverFact, markdown, stat string) []coverFact {
	text := strings.ToLower(markdown)
	seen := map[string]bool{strings.ToLower(strings.TrimSpace(stat)): true}
	var out []coverFact
	for _, f := range facts {
		f.Figure = truncateRunes(strings.TrimSpace(f.Figure), 14)
		f.Label = truncateWords(strings.TrimSpace(f.Label), 6, 40)
		k := strings.ToLower(f.Figure)
		if f.Figure == "" || f.Label == "" || seen[k] || !strings.Contains(text, k) {
			continue
		}
		seen[k] = true
		if out = append(out, f); len(out) == maxCoverFacts {
			break
		}
	}
	return out
}

// note is what label means in the article, or "".
func (s coverScene) note(label string) string {
	return s.Notes[strings.ToLower(strings.TrimSpace(label))]
}

// UnmarshalJSON accepts the shapes a model gives a scene: the object asked
// for, a bare layout name ("flow"), labels as one comma-joined string, or
// labels as objects. A cover hint must never fail the plan.
func (s *coverScene) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*s = coverScene{}
	switch t := v.(type) {
	case string:
		s.Layout = canonicalCoverLayout(t)
	case map[string]any:
		var layout []string
		collectStrings(t["layout"], &layout)
		if len(layout) > 0 {
			s.Layout = canonicalCoverLayout(layout[0])
		}
		var labels []string
		collectStrings(t["labels"], &labels)
		for _, l := range labels {
			for _, part := range strings.Split(l, ",") {
				if part = strings.TrimSpace(part); part != "" {
					s.Labels = append(s.Labels, part)
				}
			}
		}
		var stat, eyebrow, picture []string
		collectStrings(t["stat"], &stat)
		collectStrings(t["eyebrow"], &eyebrow)
		collectStrings(t["picture"], &picture)
		s.Picture = strings.Join(picture, " ")
		if num, ok := t["stat"].(float64); ok {
			stat = []string{strconv.FormatFloat(num, 'f', -1, 64)}
		}
		s.Stat = strings.Join(stat, " ")
		s.Eyebrow = strings.Join(eyebrow, " ")
		var headline, from, to []string
		collectStrings(t["headline"], &headline)
		collectStrings(t["trend_from"], &from)
		collectStrings(t["trend_to"], &to)
		s.Headline, s.TrendFrom, s.TrendTo = strings.Join(headline, " "), strings.Join(from, " "), strings.Join(to, " ")
		var dek []string
		collectStrings(t["dek"], &dek)
		s.Dek = strings.Join(dek, " ")
		// Notes arrive as a list beside the labels, or as the map stored here.
		switch n := t["notes"].(type) {
		case []any:
			var notes []string
			collectStrings(n, &notes)
			if len(notes) == len(s.Labels) {
				for i, l := range s.Labels {
					s.setNote(l, notes[i])
				}
			}
		case map[string]any:
			for l, v := range n {
				if note, ok := v.(string); ok {
					s.setNote(l, note)
				}
			}
		}
		facts, _ := t["facts"].([]any)
		for _, v := range facts {
			if f, ok := readFact(v); ok {
				s.Facts = append(s.Facts, f)
			}
		}
		points, _ := t["trend"].([]any)
		for _, v := range points {
			if n, ok := trendNumber(v); ok {
				s.Trend = append(s.Trend, n)
			}
		}
	}
	return nil
}

func (s *coverScene) setNote(label, note string) {
	if note = truncateWords(strings.TrimSpace(note), 10, 64); note == "" {
		return
	}
	if s.Notes == nil {
		s.Notes = map[string]string{}
	}
	s.Notes[strings.ToLower(strings.TrimSpace(label))] = note
}

// trendNumber reads a trend point given as a number or as text like "$1,200"
// or "31%".
func trendNumber(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case string:
		f, err := strconv.ParseFloat(strings.Trim(strings.ReplaceAll(n, ",", ""), " $£€%"), 64)
		return f, err == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
	}
	return 0, false
}

// canonicalCoverLayout maps a layout name or a near miss onto a known layout.
// Unknown input is empty, and the caller derives one from the article.
func canonicalCoverLayout(raw string) coverLayout {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "flow", "pipeline", "process", "steps":
		return layoutFlow
	case "hub", "hub-and-spoke", "spokes", "star":
		return layoutHub
	case "loop", "cycle", "feedback":
		return layoutLoop
	case "bars", "bar", "chart", "staircase":
		return layoutBars
	case "versus", "vs", "comparison", "double-edged", "tradeoff", "trade-off":
		return layoutVersus
	case "ripple", "blast", "blast-radius", "rings":
		return layoutRipple
	case "tree", "branches", "pruning", "search":
		return layoutTree
	case "iceberg", "hidden":
		return layoutIceberg
	case "hierarchy", "policy", "org", "tree-down":
		return layoutHierarchy
	case "threshold", "drift", "monitor", "alert":
		return layoutThreshold
	}
	return ""
}

// coverSceneFor completes an article's scene: the plan's when it has one,
// otherwise one derived from the written piece, so regenerating a cover for
// an old article still draws its subject.
func coverSceneFor(a *GeneratedArticle) coverScene {
	s := a.CoverScene
	derived := deriveCoverScene(a.Title, a.Topic, a.CoverAngle, a.Excerpt, a.Markdown)
	if s.Layout == "" {
		s.Layout = derived.Layout
	}
	s.Labels = cleanCoverLabels(s.Labels)
	if len(s.Labels) == 0 && s.Layout == derived.Layout {
		s.Labels = derived.Labels
	}
	// A plan scene too thin for its layout would be padded with the layout's
	// own wording ("teams", "data"); a hub of what the article names, or its
	// derived scene, is better.
	if len(s.Labels) > 0 && len(s.Labels) < layoutMinLabels[s.Layout] {
		if hub, ok := termsHub(a.Title, a.Markdown); ok {
			s.Layout, s.Labels = hub.Layout, hub.Labels
		} else {
			s.Layout, s.Labels = derived.Layout, derived.Labels
		}
	}
	if s.Stat == "" && s.Layout == layoutBars {
		s.Stat = derived.Stat
	}
	// A flow draws five steps, and its last is the one it highlights: keep the
	// destination ("proxy to pod") rather than the sixth step's absence. ERNIE
	// also letters five steps far more reliably than six.
	if s.Layout == layoutFlow && len(s.Labels) > 5 {
		s.Labels = append(s.Labels[:4:4], s.Labels[len(s.Labels)-1])
	}
	if strings.TrimSpace(s.Eyebrow) == "" {
		s.Eyebrow = derived.Eyebrow
	}
	s.Eyebrow = strings.ToUpper(truncateWords(strings.TrimSpace(s.Eyebrow), 3, 26))
	s.Picture = wordlessPicture(truncateRunes(strings.TrimSpace(s.Picture), maxCoverPicture))
	s.Stat = truncateRunes(strings.TrimSpace(s.Stat), 16)
	s.Headline = truncateWords(strings.TrimSpace(s.Headline), 8, 48)
	s.TrendFrom = truncateWords(strings.TrimSpace(s.TrendFrom), 5, 30)
	s.TrendTo = truncateWords(strings.TrimSpace(s.TrendTo), 5, 30)
	s.Dek = truncateWords(strings.TrimSpace(s.Dek), 24, 150)
	s.Facts = groundFacts(s.Facts, a.Markdown, s.Stat)
	if len(s.Trend) < minTrendPoints || len(s.Trend) > maxTrendPoints {
		s.Trend = nil
	}
	return s
}

// writingObject is a thing an image model paints text on. Prod's tax cover
// was "a payment slip and a calculator, with a calendar showing tax quarter
// dates": ERNIE lettered them and the picture was thrown away.
var writingObject = regexp.MustCompile(`(?i)\b(paper|document|form|slip|receipt|invoice|letter|book|notebook|calendar|clock|calculator|money|cash|banknote|coin|sign|signage|poster|whiteboard|screen|monitor|laptop|computer|phone|tablet|chart|graph|newspaper|label|ticket|passport|ledger|date)s?\b`)

// clauseBreak is where a scene sentence can be cut and still read.
var clauseBreak = regexp.MustCompile(`,\s*|;\s*|\s+(?:with|holding|showing|on|beside|next to|while)\s+`)

// wordlessPicture cuts a scene before the clause that names a writing object,
// so the art prompt never asks for one; "" when the first clause does.
func wordlessPicture(p string) string {
	loc := writingObject.FindStringIndex(p)
	if loc == nil {
		return p
	}
	cut := -1
	for _, b := range clauseBreak.FindAllStringIndex(p[:loc[0]], -1) {
		cut = b[0]
	}
	if cut <= 0 {
		return ""
	}
	return strings.TrimSpace(p[:cut])
}

// maxCoverPicture bounds the scene sentence, so the art prompt stays a prompt.
const maxCoverPicture = 300

// coverPictureAsk is what every plan prompt asks for in "picture". The example
// is from an unrelated field, so a copied one cannot pass for the article.
const coverPictureAsk = `a "picture" of one sentence: a real scene showing the article's subject as physical objects in a place a reader would recognise (for an article on bakery deliveries: "a delivery van loading wooden trays of bread outside a small bakery at dawn"). Name only things with no writing on them: no paper, documents, forms, slips, receipts, books, calendars, clocks, calculators, money, signs, screens, phones or charts, and none of the article's own terms. The image model paints those as text, and a picture with text in it is thrown away`

// percentStat finds a headline percentage such as "68%" in a title.
var percentStat = regexp.MustCompile(`[-+−]?\d{1,3}(?:\.\d+)?\s?%`)

// deriveCoverScene picks a layout and labels from article text when no plan
// supplied them. Heuristic, not an LLM call: redrawing old covers must not
// need a model round-trip.
func deriveCoverScene(title, topic, angle, excerpt, markdown string) coverScene {
	head := strings.ToLower(strings.Join([]string{title, topic}, " "))
	blob := strings.ToLower(strings.Join([]string{title, topic, angle, excerpt}, " "))
	s := coverScene{Eyebrow: topicEyebrow(head, topic, blob)}
	hints := []struct {
		layout  coverLayout
		needles []string
	}{
		{layoutBars, []string{"cost", "cheaper", "saving", "reduc", "cutting", "faster"}},
		{layoutThreshold, []string{"drift", "degradation", "anomal", "threshold", "monitoring"}},
		{layoutRipple, []string{"blast radius", "root cause", "incident", "outage", "cascad"}},
		{layoutTree, []string{"prun", "pass@", "reasoning path", "beam", "candidate"}},
		{layoutIceberg, []string{"hidden", "unseen", "beneath", "invisible", "iceberg"}},
		{layoutVersus, []string{"double-edged", " vs ", "versus", "trade-off", "but don't", "but do not"}},
		{layoutHierarchy, []string{"policy", "governance", "resilience", "compliance", "organization"}},
		{layoutHub, []string{"mcp", "protocol", "gateway", "integration", "platform"}},
		{layoutLoop, []string{"agentic", "agent", "autonomous", "feedback", "loop"}},
		{layoutFlow, []string{"pipeline", "workflow", "execution", "migration", "diffusion"}},
	}
	// The title and topic decide first: they name the subject, where an
	// excerpt mentions everything in passing.
	for _, text := range []string{head, blob} {
		for _, h := range hints {
			if containsAny(" "+text+" ", h.needles...) {
				s.Layout = h.layout
				break
			}
		}
		if s.Layout != "" {
			break
		}
	}
	if s.Layout == "" {
		// Nothing in the title says which layout: a hub of what the article
		// names says more about it than a flow of its headings.
		if hub, ok := termsHub(title, markdown); ok {
			hub.Eyebrow = s.Eyebrow
			return hub
		}
		s.Layout = layoutFlow
	}
	if s.Layout == layoutBars {
		if m := percentStat.FindString(title + " " + excerpt); m != "" {
			s.Stat = strings.ReplaceAll(m, " ", "")
			if !strings.HasPrefix(s.Stat, "+") && !strings.HasPrefix(s.Stat, "-") && !strings.HasPrefix(s.Stat, "−") {
				s.Stat = "−" + s.Stat
			}
		}
	}
	// Headings make good labels only where a layout's labels are a list of
	// parts. Where each label has a role — the good and bad edge, the alert,
	// the visible tip — a heading reads as nonsense in that slot, and the
	// layout's own wording is better.
	switch s.Layout {
	case layoutFlow, layoutHub, layoutBars, layoutHierarchy:
		s.Labels = headingLabels(markdown)
	case layoutIceberg:
		s.Labels = append([]string{""}, headingLabels(markdown)...)
	}
	return s
}

// topicEyebrow is the corner tag: a known field when the title (head) or,
// failing that, the wider text names one; otherwise the topic's first words.
func topicEyebrow(head, topic, blob string) string {
	fields := []struct{ tag, needles string }{
		{"Observability", "observab|blast radius|tracing|telemetry"},
		{"Kubernetes", "kubernetes|cluster|gateway api|kube-"},
		{"AI inference", "inference"},
		{"Agentic AI", "agentic|agents"},
		{"Responsible AI", "drift|responsible ai|bias"},
		{"Reliability", "reliab|resilien|disaster"},
		{"DevOps", "devops|ci/cd|deploy"},
		{"Security", "security|vault|secret|zero trust"},
		{"Generative models", "diffusion|generative model"},
		{"AI engineering", "llm|generative ai|coding"},
	}
	for _, text := range []string{head, blob} {
		for _, f := range fields {
			for _, n := range strings.Split(f.needles, "|") {
				if strings.Contains(text, n) {
					return f.tag
				}
			}
		}
	}
	if t := strings.TrimSpace(topic); t != "" {
		return truncateWords(t, 3, 26)
	}
	return truncateWords(head, 3, 26)
}

// skipHeading marks section headings that are not part of the argument —
// references, conclusions, per-sector walk-throughs — and so make poor labels.
var skipHeading = regexp.MustCompile(`(?i)^(references|sources|conclusion|summary|introduction|overview|next steps|faq)|real-world|in (banking|education|healthtech|fintech|tax)|best practices|future|opportunities and challenges|lessons from`)

// headingLabels turns the article's section headings into short cover labels:
// "Caching: The Hidden Cost-Saver" becomes "Caching".
func headingLabels(markdown string) []string {
	var out []string
	for _, h := range collectH2s(markdown) {
		h = strings.TrimSpace(strings.Trim(h, "*_"))
		if skipHeading.MatchString(h) {
			continue
		}
		if i := strings.IndexAny(h, ":—–"); i > 0 {
			h = h[:i]
		}
		h = strings.TrimSpace(h)
		for _, lead := range []string{"What is ", "What Is ", "How ", "Why ", "The ", "Introducing ", "Understanding "} {
			h = strings.TrimPrefix(h, lead)
		}
		h = strings.TrimSuffix(strings.TrimSpace(h), "?")
		if h != "" {
			out = append(out, h)
		}
	}
	return cleanCoverLabels(out)
}

// maxCoverLabel is the longest label a layout box holds legibly at cover size.
const maxCoverLabel = 24

// labelNoise is markup that leaks into headings: emphasis, code ticks and
// TeX dollar signs ("Pass@$k$").
var labelNoise = strings.NewReplacer("$", "", "**", "", "`", "")

// cleanCoverLabels trims, shortens and de-duplicates labels and caps how many
// a cover gets: more than six is a table, not a picture. A leading empty
// label is kept — it means "use the layout's own wording for this slot".
func cleanCoverLabels(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range in {
		l = labelNoise.Replace(citationMark.ReplaceAllString(l, ""))
		// The box's width is the limit, not a word count: "You let a UK property"
		// fits, and cutting it at four words left "You let a UK".
		l = truncateWords(strings.TrimSpace(l), 6, maxCoverLabel)
		k := strings.ToLower(l)
		if l == "" && len(out) == 0 {
			out = append(out, "")
			continue
		}
		if l == "" || seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, l)
		if len(out) == 6 {
			break
		}
	}
	return out
}

// truncateWords keeps at most maxWords words and maxChars characters, cutting
// at a word boundary and never ending on a dangling "of" or "and".
func truncateWords(s string, maxWords, maxChars int) string {
	words := strings.Fields(s)
	if len(words) > maxWords {
		words = words[:maxWords]
	}
	for len(words) > 1 && len(strings.Join(words, " ")) > maxChars {
		words = words[:len(words)-1]
	}
	for len(words) > 1 {
		switch strings.ToLower(words[len(words)-1]) {
		case "of", "and", "the", "for", "in", "with", "to", "a", "an", "on", "by", "&",
			"how", "why", "what", "does", "is", "can", "it":
			words = words[:len(words)-1]
			continue
		}
		break
	}
	out := strings.Join(words, " ")
	if len(out) > maxChars {
		out = strings.TrimSpace(out[:maxChars])
	}
	return strings.TrimRight(out, ",;:-–— ")
}
