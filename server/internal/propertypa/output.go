package propertypa

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// OutputContract is the value of "contract" in every PA run's output block.
const OutputContract = "property-pa.output.v1"

// OutputFenceTag is the info string of the fenced block the agent ends with.
const OutputFenceTag = "pa_output"

// Source is a record the answer relies on.
type Source struct {
	Type  string `json:"type"`
	ID    string `json:"id"`
	Label string `json:"label,omitempty"`
}

// Action is something the run created in OpsAPI.
type Action struct {
	Kind    string `json:"kind"`
	ID      string `json:"id"`
	Summary string `json:"summary,omitempty"`
}

// Output is the structured part of a PA run's output.
type Output struct {
	Contract   string   `json:"contract"`
	Mode       string   `json:"mode,omitempty"`
	Answer     string   `json:"answer"`
	Sources    []Source `json:"sources"`
	Unknowns   []string `json:"unknowns"`
	Inferences []string `json:"inferences"`
	Actions    []Action `json:"actions"`
	NeedsHuman bool     `json:"needs_human"`
}

// SourceTypes are the record types a source may name.
var SourceTypes = []string{
	"person", "organisation", "property", "lead", "opportunity", "deal", "interaction",
	"task", "event", "offer", "knowledge", "handoff", "briefing",
}

// ActionKinds are the things a run may report having created.
var ActionKinds = []string{
	"interaction", "proposed_update", "task", "handoff", "knowledge_proposal", "draft", "approval_requested",
}

// Modes are the PA modes.
var Modes = []string{"today", "prepare", "capture", "coach", "ask", "handoff", "manager_today", "teach"}

// ErrNoOutputBlock means the text has no parseable contract block.
var ErrNoOutputBlock = errors.New("propertypa: no " + OutputFenceTag + " block in the output")

var fenceRe = regexp.MustCompile("(?s)```[ \\t]*([A-Za-z0-9_\\-]*)[^\\n]*\\n(.*?)\\n?```")

// ParseOutput extracts and validates the contract block from a run's output.
// It takes the last fenced block tagged pa_output, falling back to the last
// ```json block whose "contract" is OutputContract (models sometimes tag it
// json). The prose before the block is returned as the display text.
func ParseOutput(text string) (*Output, string, error) {
	matches := fenceRe.FindAllStringSubmatchIndex(text, -1)
	pick := -1
	var out Output
	for i := len(matches) - 1; i >= 0; i-- {
		m := matches[i]
		tag := strings.ToLower(text[m[2]:m[3]])
		body := text[m[4]:m[5]]
		if tag != OutputFenceTag && tag != "json" && tag != "" {
			continue
		}
		var cand Output
		if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &cand); err != nil {
			if tag == OutputFenceTag {
				return nil, "", fmt.Errorf("propertypa: %s block is not valid JSON: %w", OutputFenceTag, err)
			}
			continue
		}
		if tag != OutputFenceTag && cand.Contract != OutputContract {
			continue
		}
		out, pick = cand, i
		break
	}
	if pick < 0 {
		return nil, strings.TrimSpace(text), ErrNoOutputBlock
	}
	prose := strings.TrimSpace(text[:matches[pick][0]])
	out.normalise()
	if err := out.Validate(); err != nil {
		return &out, prose, err
	}
	return &out, prose, nil
}

func (o *Output) normalise() {
	if o.Sources == nil {
		o.Sources = []Source{}
	}
	if o.Unknowns == nil {
		o.Unknowns = []string{}
	}
	if o.Inferences == nil {
		o.Inferences = []string{}
	}
	if o.Actions == nil {
		o.Actions = []Action{}
	}
	for i := range o.Sources {
		o.Sources[i].Type = strings.ToLower(strings.TrimSpace(o.Sources[i].Type))
		o.Sources[i].ID = strings.TrimSpace(o.Sources[i].ID)
	}
	for i := range o.Actions {
		o.Actions[i].Kind = strings.ToLower(strings.TrimSpace(o.Actions[i].Kind))
		o.Actions[i].ID = strings.TrimSpace(o.Actions[i].ID)
	}
	o.Mode = strings.ToLower(strings.TrimSpace(o.Mode))
}

// Validate checks the contract. It does not check that the ids exist; OpsAPI
// does that (grounded-or-silent validation lives with the records).
func (o *Output) Validate() error {
	var problems []string
	if o.Contract != OutputContract {
		problems = append(problems, fmt.Sprintf("contract is %q, want %q", o.Contract, OutputContract))
	}
	if strings.TrimSpace(o.Answer) == "" {
		problems = append(problems, "answer is empty")
	}
	if o.Mode != "" && !contains(Modes, o.Mode) {
		problems = append(problems, fmt.Sprintf("unknown mode %q", o.Mode))
	}
	for i, s := range o.Sources {
		if !contains(SourceTypes, s.Type) {
			problems = append(problems, fmt.Sprintf("sources[%d]: unknown type %q", i, s.Type))
		}
		if s.ID == "" {
			problems = append(problems, fmt.Sprintf("sources[%d]: id is empty", i))
		}
	}
	for i, a := range o.Actions {
		if !contains(ActionKinds, a.Kind) {
			problems = append(problems, fmt.Sprintf("actions[%d]: unknown kind %q", i, a.Kind))
		}
		if a.ID == "" {
			problems = append(problems, fmt.Sprintf("actions[%d]: id is empty", i))
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("propertypa: invalid output: %s", strings.Join(problems, "; "))
	}
	return nil
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
