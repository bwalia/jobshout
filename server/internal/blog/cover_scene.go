package blog

import (
	"encoding/json"
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
		var stat, eyebrow []string
		collectStrings(t["stat"], &stat)
		collectStrings(t["eyebrow"], &eyebrow)
		if num, ok := t["stat"].(float64); ok {
			stat = []string{strconv.FormatFloat(num, 'f', -1, 64)}
		}
		s.Stat = strings.Join(stat, " ")
		s.Eyebrow = strings.Join(eyebrow, " ")
	}
	return nil
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
	if s.Stat == "" && s.Layout == layoutBars {
		s.Stat = derived.Stat
	}
	if strings.TrimSpace(s.Eyebrow) == "" {
		s.Eyebrow = derived.Eyebrow
	}
	s.Eyebrow = strings.ToUpper(truncateWords(strings.TrimSpace(s.Eyebrow), 3, 26))
	return s
}

// percentStat finds a headline percentage such as "68%" in a title.
var percentStat = regexp.MustCompile(`[-+−]?\d{1,3}(?:\.\d+)?\s?%`)

// deriveCoverScene picks a layout and labels from article text when no plan
// supplied them. Heuristic, not an LLM call, for the same reason as
// deriveCoverBrief: redrawing old covers must not need a model round-trip.
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
		l = truncateWords(strings.TrimSpace(l), 4, maxCoverLabel)
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
