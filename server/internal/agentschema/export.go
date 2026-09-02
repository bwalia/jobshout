package agentschema

import "github.com/jobshout/server/internal/model"

// WireField is one field as the API exposes it — behaviour and presentation.
type WireField struct {
	Key         string `json:"key"`
	Label       string `json:"label"`
	Question    string `json:"question,omitempty"`
	Type        string `json:"type,omitempty"`
	Required    bool   `json:"required"`
	MinLength   int    `json:"min_length,omitempty"`
	Min         int    `json:"min,omitempty"`
	Default     string `json:"default,omitempty"`
	Placeholder string `json:"placeholder,omitempty"`
	Help        string `json:"help,omitempty"`
	Group       string `json:"group,omitempty"`
	Options     []struct {
		Label string `json:"label"`
		Value string `json:"value"`
	} `json:"options,omitempty"`
}

// WireSchema is one builtin's interview as GET /api/v1/agent-schemas exposes it.
//
// All specialists are wired this way: schema on the module, this API, web consumes
// it. A new agent does not need a TypeScript SCHEMAS map — register it.
type WireSchema struct {
	Builtin        string         `json:"builtin"`
	SpecialistTool string         `json:"specialist_tool,omitempty"`
	Hint           string         `json:"hint,omitempty"`
	Label          string         `json:"label,omitempty"`
	Icon           string         `json:"icon,omitempty"`
	TabSlug        string         `json:"tab_slug,omitempty"`
	StayOnTab      bool           `json:"stay_on_tab,omitempty"`
	Prefill        string         `json:"prefill,omitempty"`
	Fields         []WireField    `json:"fields"`
	TitleRules     []TitleRule    `json:"title_rules,omitempty"`
	DescRules      []DescRule     `json:"desc_rules,omitempty"`
	RequireAny     []RequireGroup `json:"require_any,omitempty"`
}

// All returns every registered specialist's schema.
func All() []WireSchema {
	out := make([]WireSchema, 0, len(Builtins()))
	for _, b := range Builtins() {
		out = append(out, Wire(ForBuiltin(b)))
	}
	return out
}

// Wire converts a Schema to the API shape. Label/icon/tab are filled by the handler
// from the module registry when present; this copies schema-owned fields.
func Wire(s Schema) WireSchema {
	ws := WireSchema{
		Builtin:        s.Builtin,
		SpecialistTool: s.SpecialistTool,
		Hint:           s.Hint,
		Prefill:        s.Prefill,
		Fields:         make([]WireField, 0, len(s.Fields)),
		TitleRules:     s.TitleRules,
		DescRules:      s.DescRules,
		RequireAny:     s.RequireAny,
	}
	for _, f := range s.Fields {
		wf := WireField{
			Key: f.Key, Label: f.Label, Question: f.Question, Type: f.Type,
			Required: f.Required, MinLength: f.MinLength, Min: f.Min,
			Default: f.Default, Placeholder: f.Placeholder, Help: f.Help, Group: f.Group,
		}
		for _, o := range f.Options {
			wf.Options = append(wf.Options, struct {
				Label string `json:"label"`
				Value string `json:"value"`
			}{Label: o.Label, Value: o.Value})
		}
		ws.Fields = append(ws.Fields, wf)
	}
	return ws
}

// Option is re-exported for callers building fields.
type Option = model.ClarifyOption
