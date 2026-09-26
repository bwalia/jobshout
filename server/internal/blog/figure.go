package blog

import (
	"fmt"
	"strings"
)

// illustrationKind is the type of informational figure a fence asks for.
//
// Body images used to be decorative editorial scenes with text banned — image
// models of that generation rendered lettering as gibberish, so the prompt
// asked for a mood instead of a claim. The pictures that came back could sit
// in any article. These kinds exist so a figure has a job: a path, a
// comparison, a system, a sequence of steps, or an annotated mechanism.
type illustrationKind string

const (
	kindFlow         illustrationKind = "flow"
	kindComparison   illustrationKind = "comparison"
	kindArchitecture illustrationKind = "architecture"
	kindProcess      illustrationKind = "process"
	kindConcept      illustrationKind = "concept"
)

// figurePromptStyle is the house look for in-body figures. Covers stay on the
// dark charcoal template; body figures need a light ground so labels read.
const figurePromptStyle = "clean editorial infographic, light off-white background, crisp geometric shapes, sharp high-contrast sans-serif labels, teal and coral accents, generous margins, high clarity"

// canonicalIllustrationKind maps a fence tag or alias onto a known kind.
// Unknown input comes back empty so the caller can infer from the body.
func canonicalIllustrationKind(raw string) illustrationKind {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "flow", "flowchart", "pipeline":
		return kindFlow
	case "comparison", "compare", "table", "matrix":
		return kindComparison
	case "architecture", "system", "components", "stack":
		return kindArchitecture
	case "process", "steps", "procedure", "workflow":
		return kindProcess
	case "concept", "annotated", "diagram":
		return kindConcept
	default:
		return ""
	}
}

// parseIllustration turns a fence's optional kind tag and body into a kind
// plus the facts to render. An untyped fence still works: the body is
// classified so a writer who omits the tag does not fall back to a scene.
func parseIllustration(kindRaw, body string) (illustrationKind, string) {
	desc := strings.TrimSpace(body)
	if kind := canonicalIllustrationKind(kindRaw); kind != "" {
		return kind, desc
	}
	return inferIllustrationKind(kindRaw + " " + desc), desc
}

// inferIllustrationKind guesses a figure type from heading and prose. The
// order is specific-to-general: a "vs" is a comparison even if the same
// sentence also says "then".
func inferIllustrationKind(text string) illustrationKind {
	t := " " + strings.ToLower(text) + " "
	switch {
	case containsAny(t,
		" vs ", " versus ", "compared", "comparison", "trade-off", "tradeoff",
		"pros and cons", "difference between", "rather than", "instead of",
	):
		return kindComparison
	case containsAny(t,
		"architecture", "control plane", "components", " the stack ",
		"service mesh", "microserv", "named component",
	):
		return kindArchitecture
	case containsAny(t,
		"flowchart", "request path", "data flow", "pipeline", "through the",
		"then sends", "then calls", "round trip", "handoff",
	):
		return kindFlow
	case containsAny(t,
		"step by step", "steps:", " first ", " then ", " finally ",
		"how to ", "procedure", "workflow", "lifecycle",
	):
		return kindProcess
	default:
		return kindConcept
	}
}

func containsAny(text string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(text, n) {
			return true
		}
	}
	return false
}

// formatIllustrationFence writes the fence the writer is taught and
// illustrateBody parses. The kind sits on the info line so a typed request
// survives a model that copies the example.
func formatIllustrationFence(kind illustrationKind, description string) string {
	if kind == "" {
		kind = kindConcept
	}
	return "```illustration " + string(kind) + "\n" + strings.TrimSpace(description) + "\n```"
}

// sectionFigure builds a typed figure spec from the section's own argument.
//
// The old version asked for "people or systems as one readable scene" and
// forbade labels. That produced animations. This one names a kind and keeps
// the section's own terms so the picture has something to letter.
func sectionFigure(heading, prose, title string) (illustrationKind, string) {
	h := strings.TrimSpace(heading)
	p := compactProse(prose)
	about := h
	if about == "" {
		about = strings.TrimSpace(title)
	}
	if about == "" {
		about = "this section"
	}
	kind := inferIllustrationKind(about + " " + p)
	return kind, figureSpec(kind, about, p)
}

// compactProse keeps the section's facts, not just its first sentence.
// Auto-insert used to feed the image model one sentence and get a generic
// picture back; a short paragraph still fits a prompt and gives it labels.
func compactProse(prose string) string {
	p := strings.Join(strings.Fields(strings.TrimSpace(citationMark.ReplaceAllString(prose, ""))), " ")
	if len(p) <= 400 {
		return p
	}
	cut := p[:400]
	if j := strings.LastIndex(cut, " "); j > 200 {
		cut = cut[:j]
	}
	return strings.TrimSpace(cut)
}

// sectionScene is the description half of sectionFigure. Tests and call
// sites that only need the body keep using this name.
func sectionScene(heading, prose, title string) string {
	_, desc := sectionFigure(heading, prose, title)
	return desc
}

func figureSpec(kind illustrationKind, about, prose string) string {
	facts := prose
	if facts == "" {
		facts = about
	}
	switch kind {
	case kindComparison:
		return "Comparison of " + about + ". Facts: " + facts +
			". Use those terms as column headers and row labels. Short readable text in every cell."
	case kindFlow:
		return "Labeled flowchart of " + about + ": " + facts +
			". Name each box with a real step or component from those facts. Arrows show the path."
	case kindArchitecture:
		return "Labeled system diagram of " + about + ": " + facts +
			". Each box is a named component. Lines show how they connect."
	case kindProcess:
		return "Numbered process for " + about + ": " + facts +
			". Each step has a short verb-first label taken from those facts."
	default:
		return "Annotated diagram of " + about + ": " + facts +
			". Label every important part with the terms from those facts so a reader can learn the mechanism."
	}
}

// figureForSection prefers a figure the planner already specified for this
// heading, and otherwise builds one from the section prose.
func figureForSection(plan *writePlan, heading, prose, title string) (illustrationKind, string) {
	if kind, content, ok := plannedFigure(plan, heading); ok {
		return kind, content
	}
	return sectionFigure(heading, prose, title)
}

func plannedFigure(plan *writePlan, heading string) (illustrationKind, string, bool) {
	if plan == nil || len(plan.Figures) == 0 {
		return "", "", false
	}
	h := strings.ToLower(strings.TrimSpace(heading))
	if h == "" {
		return "", "", false
	}
	for _, f := range plan.Figures {
		content := strings.TrimSpace(f.Content)
		if content == "" {
			continue
		}
		sec := strings.ToLower(strings.TrimSpace(f.Section))
		if sec == "" {
			continue
		}
		if sec == h || strings.Contains(sec, h) || strings.Contains(h, sec) {
			kind := canonicalIllustrationKind(f.Kind)
			if kind == "" {
				kind = inferIllustrationKind(f.Kind + " " + content)
			}
			return kind, content, true
		}
	}
	return "", "", false
}

// inlineSize is the pixel box for a kind. Comparison, flow and architecture
// want width for columns and arrows; process and concept keep the body 3:2.
func inlineSize(kind illustrationKind) (width, height int) {
	switch kind {
	case kindFlow, kindComparison, kindArchitecture:
		return 1280, 720
	default:
		return inlineWidth, inlineHeight
	}
}

func kindNoun(kind illustrationKind) string {
	switch kind {
	case kindFlow:
		return "flowchart"
	case kindComparison:
		return "comparison table"
	case kindArchitecture:
		return "architecture diagram"
	case kindProcess:
		return "process diagram"
	default:
		return "annotated concept diagram"
	}
}

// figureWorthInserting reports whether a section has enough of its own
// terms to be worth drawing. Scoring the wrapped spec is the wrong input:
// "Label every important part…" adds words that are not in the article.
// Writer-planned figures skip this bar — the planner already named the facts.
func figureWorthInserting(kind illustrationKind, heading, prose string, planned bool) bool {
	if planned {
		return strings.TrimSpace(heading+prose) != ""
	}
	facts := compactProse(heading + " " + prose)
	if facts == "" {
		return false
	}
	if looksDecorative(facts) && kind == kindConcept {
		return false
	}
	n := len(contentWords(facts))
	if kind == kindConcept {
		return n >= 6
	}
	return n >= 3
}

// contentWords are the section's own terms, stopwords removed.
func contentWords(text string) []string {
	var out []string
	for _, w := range strings.Fields(compactProse(text)) {
		w = strings.Trim(w, ".()[]\"'`:,;")
		if len(w) < 3 || labelStop[strings.ToLower(w)] {
			continue
		}
		out = append(out, w)
	}
	return out
}

// looksDecorative is the old scene language: people acting, offices, mood.
// Conservative on purpose — "an API gateway routing requests" is not a scene.
func looksDecorative(desc string) bool {
	t := strings.ToLower(desc)
	return containsAny(t,
		"server room", "stock photo", "at a desk", "handing a", "handing an",
		"smiling", "cinematic", "animation", "photographed", "modern office",
		"people working", "readable scene", "mascot", "metaphorical",
	)
}

// labelStop is filler that must not become an on-image label.
var labelStop = map[string]bool{
	"the": true, "a": true, "an": true, "of": true, "and": true, "or": true,
	"to": true, "for": true, "in": true, "on": true, "at": true, "is": true,
	"are": true, "as": true, "with": true, "from": true, "that": true,
	"this": true, "its": true, "it": true, "by": true, "be": true,
	"facts": true, "use": true, "those": true, "terms": true, "each": true,
	"every": true, "short": true, "labeled": true, "diagram": true,
	"flowchart": true, "comparison": true, "process": true, "annotated": true,
	"about": true, "system": true, "named": true, "can": true, "so": true,
	"component": true, "lines": true, "show": true, "connect": true,
	"box": true, "they": true, "how": true, "label": true, "important": true,
	"part": true, "reader": true, "learn": true, "mechanism": true,
	"number": true, "numbered": true, "verb-first": true, "taken": true,
	"column": true, "headers": true, "row": true, "labels": true,
	"readable": true, "text": true, "cell": true, "arrows": true, "path": true,
	"real": true, "step": true, "steps": true,
}

// concreteLabels pulls the article's own terms out of a figure spec so the
// image prompt can demand those exact words. Without this the model invents
// labels, which is how a figure about reconciliation becomes a generic loop.
func concreteLabels(text string) []string {
	s := compactProse(text)
	for _, sep := range []string{"→", "->", " vs ", " versus ", ":", ";", ",", "/", " — ", " – ", "."} {
		s = strings.ReplaceAll(s, sep, " | ")
	}
	var out []string
	seen := map[string]bool{}
	var buf []string
	flush := func() {
		if len(buf) == 0 {
			return
		}
		if len(buf) > 4 {
			buf = buf[:4]
		}
		label := strings.Join(buf, " ")
		key := strings.ToLower(label)
		if !seen[key] {
			seen[key] = true
			out = append(out, label)
		}
		buf = nil
	}
	for _, w := range strings.Fields(s) {
		if w == "|" {
			flush()
			continue
		}
		w = strings.Trim(w, ".()[]\"'`")
		if len(w) < 3 || labelStop[strings.ToLower(w)] {
			flush()
			continue
		}
		buf = append(buf, w)
	}
	flush()
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

func kindLayoutInstruction(kind illustrationKind) string {
	switch kind {
	case kindFlow:
		return "Left-to-right or top-down labeled flowchart. Each box is a named step or component. Arrows show the only paths that exist. A decision has branches that go to different places."
	case kindComparison:
		return "A two- or three-column comparison table. The header row names the options. Each later row is one criterion with a short value in every cell. Align the columns. This is a readable table, not a collage."
	case kindArchitecture:
		return "A labeled system diagram. Each box is a named component. Lines show how they connect. Group related parts. No unnamed blobs."
	case kindProcess:
		return "Numbered steps in order, top-to-bottom or left-to-right. Each step has a short verb-first label from the content. Show the sequence, not a scene of people doing the work."
	default:
		return "An annotated diagram of the mechanism. Call out each important part with a label from the content. The picture explains how it works, not a mood."
	}
}

// inlineImagePrompt is what the image model sees. The fence body is the
// facts; this wrapper forbids the decorative scenes the old prompt produced
// and requires the labels those scenes lacked.
func inlineImagePrompt(kind illustrationKind, description string) string {
	if kind == "" {
		kind = inferIllustrationKind(description)
	}
	description = strings.TrimSpace(description)
	labelBlock := description
	if labels := concreteLabels(description); len(labels) > 0 {
		labelBlock = strings.Join(labels, "\n- ")
		labelBlock = "- " + labelBlock
	}
	return fmt.Sprintf(
		"Create an informational %s for a technical article. "+
			"A reader who only sees this image should learn the article's point.\n\n"+
			"Required labels — render each, spelled exactly, large enough to read:\n%s\n\n"+
			"Context:\n%s\n\n"+
			"Layout: %s\n\n"+
			"Style: %s. "+
			"Do not invent brand names, product logos, extra claims, or extra labels. "+
			"No decorative characters, mascots, animations, metaphorical scenes, "+
			"stock offices, server rooms, or abstract art. "+
			"No watermarks, no UI chrome, no extra title beyond what the content names.",
		kindNoun(kind), labelBlock, description, kindLayoutInstruction(kind), figurePromptStyle,
	)
}

// salvageIllustration rewrites a decorative fence body into a figure spec
// when it still names enough terms. A pure scene with nothing to letter is
// dropped — drawing it is how we used to ship animations.
func salvageIllustration(kind illustrationKind, desc string) (illustrationKind, string, bool) {
	desc = strings.TrimSpace(desc)
	if desc == "" {
		return "", "", false
	}
	if !looksDecorative(desc) {
		if kind == "" {
			kind = inferIllustrationKind(desc)
		}
		return kind, desc, true
	}
	if len(contentWords(desc)) < 6 {
		return "", "", false
	}
	if kind == "" {
		kind = inferIllustrationKind(desc)
	}
	return kind, figureSpec(kind, firstSentence(desc), desc), true
}

// imageRendersLabels reports whether a finished image came from a model that
// can letter a table or flowchart. Workstation diffusion (mflux / z-image)
// cannot; putting its output in the article is the original bug.
func imageRendersLabels(provider, model string) bool {
	p := strings.ToLower(strings.TrimSpace(provider))
	m := strings.ToLower(strings.TrimSpace(model))
	if p == "mflux" {
		return false
	}
	return !strings.Contains(m, "z-image") && !strings.Contains(m, "qwen-image") && !strings.Contains(m, "flux")
}
