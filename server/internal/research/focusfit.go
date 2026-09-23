package research

import "strings"

// focusGeneric are words that appear in focus areas without saying what the
// area is about. "Observability for AI Agents" is about observability; a topic
// that only shares "AI" and "agents" with it is about something else, and the
// model marked exactly such a topic in focus on int ("Securing AI Agents with
// Boundary" for an observability/SRE brief).
var focusGeneric = map[string]struct{}{
	"ai": {}, "ml": {}, "genai": {}, "generative": {}, "llm": {}, "llms": {},
	"agent": {}, "agents": {}, "agentic": {}, "model": {}, "models": {},
	"aws": {}, "azure": {}, "gcp": {}, "google": {}, "microsoft": {}, "amazon": {}, "cloud": {},
	"platform": {}, "platforms": {}, "software": {}, "engineering": {}, "system": {}, "systems": {},
	"tool": {}, "tools": {}, "data": {},
}

// focusAliases lets an acronym focus area match its spelled-out form, which
// is how an article's context usually says it.
var focusAliases = map[string][]string{
	"sre": {"site reliability"},
	"k8s": {"kubernetes"},
}

// focusTerms is what a topic must mention to count as inside one focus area:
// the area's distinctive phrase, its spaceless form ("x ray" / "xray"), any
// distinctive word of four letters or more, and known aliases. Nil means the
// area is all generic words and cannot be checked this way.
func focusTerms(area string) []string {
	var kept []string
	for _, w := range strings.Fields(normaliseForCompare(area)) {
		if _, stop := stopWords[w]; stop {
			continue
		}
		if _, generic := focusGeneric[w]; generic {
			continue
		}
		kept = append(kept, w)
	}
	if len(kept) == 0 {
		return nil
	}
	phrase := strings.Join(kept, " ")
	terms := []string{phrase}
	if len(kept) > 1 {
		terms = append(terms, strings.Join(kept, ""))
		for _, w := range kept {
			// Short fragments ("x", "ray") only count as part of the phrase:
			// "Ray" alone is a different framework.
			if len(w) >= 4 {
				terms = append(terms, w)
			}
		}
	}
	for _, w := range kept {
		terms = append(terms, focusAliases[w]...)
	}
	return terms
}

// mentionsFocus reports whether text names at least one focus area by its
// distinctive terms. checkable is false when no area has any, in which case
// the caller has nothing to judge with.
func mentionsFocus(text string, focus []string) (mentions, checkable bool) {
	padded := " " + strings.Join(strings.Fields(normaliseForCompare(text)), " ") + " "
	for _, area := range focus {
		terms := focusTerms(area)
		if terms == nil {
			continue
		}
		checkable = true
		for _, t := range terms {
			if strings.Contains(padded, " "+t+" ") {
				return true, true
			}
		}
	}
	return false, checkable
}

// confirmFocus checks the model's in_focus flag against the topic's own words.
//
// The flag is a claim the prompt asks the model to make honestly, and it is
// generous: vocabulary shared with a focus area is enough for it to say yes.
// A topic keeps the flag only when its topic, context or rationale names a
// focus area by a distinctive term. Demoting only changes priority —
// selectByFocus still tops up with off-focus topics when nothing else is on
// subject — so the effect is that an on-subject topic wins the slot and an
// adjacent one is reported as off target instead of passing as in focus.
func confirmFocus(topics []Topic, focus []string) []Topic {
	if len(focus) == 0 {
		return topics
	}
	for i := range topics {
		if !topics[i].InFocus {
			continue
		}
		text := topics[i].Topic + " " + topics[i].Context + " " + topics[i].Rationale
		if mentions, checkable := mentionsFocus(text, focus); checkable && !mentions {
			topics[i].InFocus = false
		}
	}
	return topics
}
