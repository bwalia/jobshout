package blog

import (
	"fmt"
	"regexp"
	"strings"
)

// A diagram the agent writes is a claim like any other, and until now it was
// the only claim nothing checked. A malformed diagram survived the whole
// pipeline and failed in the reader's browser, where MermaidDiagram falls back
// to printing the source — so the reader is shown a wall of diagram syntax
// where a picture was promised, and the CMS copy shows it too.
//
// That is worth catching here for the same reason citations are: it can be
// decided mechanically. A benchmark of two local models over three runs each
// produced valid Mermaid in every sequence and state diagram and invalid
// Mermaid in every flowchart, from a single cause — an unquoted "bpf()" inside
// a node label. Quoting node labels fixes that class outright, so the repair
// runs first and only what is still broken afterwards is dropped.
//
// This is not a Mermaid parser and does not pretend to be one. It repairs the
// failure we have measured and rejects diagrams that are obviously not going to
// render; anything subtler still reaches the browser, where the viewer's
// fallback handles it.

// mermaidFence matches a ```mermaid block and captures its body.
var mermaidFence = regexp.MustCompile("(?s)```mermaid[ \t]*\r?\n(.*?)```")

// flowchartNodeLabel matches a square-bracket node label in a flowchart, e.g.
// the `Loader calls bpf() syscall` in `C[Loader calls bpf() syscall]`.
// Newlines are excluded so a runaway match cannot swallow the rest of the
// diagram. Quotes are allowed through so a label containing one is seen and
// neutralised; already-quoted labels are recognised and skipped in the replace.
var flowchartNodeLabel = regexp.MustCompile(`([A-Za-z0-9_]+)\[([^\]\n]+)\]`)

// flowchartDecisionLabel is the same thing for rhombus `{...}` decision nodes.
var flowchartDecisionLabel = regexp.MustCompile(`([A-Za-z0-9_]+)\{([^}\n]+)\}`)

// supportedDiagramTypes are the diagram headers the writing prompt offers.
//
// classDiagram is absent on purpose and is rejected rather than merely
// undocumented: the prompt tells the model not to use it, and both benchmarked
// models produced something unrenderable whenever they tried it anyway.
var supportedDiagramTypes = []string{
	"sequenceDiagram",
	"stateDiagram-v2",
	"stateDiagram",
	"erDiagram",
	"flowchart",
	"graph",
}

// sanitiseDiagrams repairs the Mermaid blocks in markdown and removes the ones
// that cannot be repaired, returning the result and a note for each block it
// changed or dropped.
func sanitiseDiagrams(markdown string) (string, []string) {
	var notes []string

	out := mermaidFence.ReplaceAllStringFunc(markdown, func(fence string) string {
		m := mermaidFence.FindStringSubmatch(fence)
		if m == nil {
			return fence
		}
		body := m[1]

		repaired, changed := repairDiagram(body)
		if err := validateDiagram(repaired); err != nil {
			notes = append(notes, fmt.Sprintf("dropped a diagram: %v", err))
			return ""
		}
		if changed {
			notes = append(notes, "quoted node labels in a diagram so it would parse")
		}
		return "```mermaid\n" + repaired + "```"
	})

	// Dropping a fence leaves the blank lines that surrounded it back to back.
	return collapseBlankRuns(out), notes
}

// diagramType returns the lowercased first word of a diagram, which is its
// type, plus the first non-empty line it came from.
func diagramType(body string) (string, string) {
	for _, line := range strings.Split(body, "\n") {
		if t := strings.TrimSpace(line); t != "" {
			first, _, _ := strings.Cut(t, " ")
			return strings.ToLower(strings.TrimSpace(first)), t
		}
	}
	return "", ""
}

// repairDiagram quotes node labels so punctuation inside them cannot break the
// parser, reporting whether it changed anything.
//
// Quoting is applied to every label rather than only the ones that look
// dangerous, because `A["Load Balancer"]` is as valid as `A[Load Balancer]` and
// deciding which characters Mermaid tolerates in which position is exactly the
// judgement this code should not be making.
//
// It runs only on flowcharts. Other diagram types use square brackets to mean
// other things — `[*]` is a state diagram's start and end marker — and quoting
// those would break diagrams that were already correct.
func repairDiagram(body string) (string, bool) {
	kind, _ := diagramType(body)
	if kind != "flowchart" && kind != "graph" {
		return body, false
	}

	quote := func(re *regexp.Regexp, open, close string) func(string) string {
		return func(match string) string {
			m := re.FindStringSubmatch(match)
			if m == nil {
				return match
			}
			label := strings.TrimSpace(m[2])
			// Already quoted — leave it exactly as it is, so running this twice
			// cannot nest one set of quotes inside another.
			if len(label) >= 2 && strings.HasPrefix(label, `"`) && strings.HasSuffix(label, `"`) {
				return match
			}
			// A stray double quote inside the label would close the one being
			// added; a single quote reads the same and cannot.
			label = strings.ReplaceAll(label, `"`, `'`)
			return m[1] + open + `"` + label + `"` + close
		}
	}

	repaired := flowchartNodeLabel.ReplaceAllStringFunc(body, quote(flowchartNodeLabel, "[", "]"))
	repaired = flowchartDecisionLabel.ReplaceAllStringFunc(repaired, quote(flowchartDecisionLabel, "{", "}"))
	return repaired, repaired != body
}

// validateDiagram reports why a diagram will not render, or nil if it looks
// renderable.
func validateDiagram(body string) error {
	kind, header := diagramType(body)
	if kind == "" {
		return fmt.Errorf("it was empty")
	}
	if kind == "classdiagram" {
		return fmt.Errorf("it used classDiagram, which the writing prompt excludes")
	}

	supported := false
	for _, t := range supportedDiagramTypes {
		if kind == strings.ToLower(t) {
			supported = true
			break
		}
	}
	if !supported {
		return fmt.Errorf("%q is not a diagram type this pipeline supports", header)
	}

	// A header with nothing under it renders as an empty box.
	lines := 0
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) != "" {
			lines++
		}
	}
	if lines < 2 {
		return fmt.Errorf("%q had no content under it", header)
	}
	return nil
}

var blankRuns = regexp.MustCompile(`\n{3,}`)

// collapseBlankRuns closes the gap a removed diagram leaves behind.
func collapseBlankRuns(s string) string {
	return blankRuns.ReplaceAllString(s, "\n\n")
}
