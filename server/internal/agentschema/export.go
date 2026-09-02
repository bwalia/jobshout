package agentschema

import "github.com/jobshout/server/internal/model"

// Builtins is every builtin this package has an interview for, in the order
// the Task Manager lists them. The generic fallback is not included.
var Builtins = []string{
	model.BuiltinArticleWriter,
	model.BuiltinResearcher,
	model.BuiltinPentester,
	model.BuiltinPRReviewer,
	model.BuiltinMail,
	model.BuiltinImages,
	model.BuiltinCareerOps,
}

// WireField is one field as the API exposes it. It carries the properties
// that decide behaviour — key, order, required, default — not presentation.
type WireField struct {
	Key       string `json:"key"`
	Label     string `json:"label"`
	Question  string `json:"question,omitempty"`
	Required  bool   `json:"required"`
	MinLength int    `json:"min_length,omitempty"`
	Default   string `json:"default,omitempty"`
	Options   []struct {
		Label string `json:"label"`
		Value string `json:"value"`
	} `json:"options,omitempty"`
}

// WireSchema is one builtin's interview as the API exposes it.
type WireSchema struct {
	Builtin        string      `json:"builtin"`
	SpecialistTool string      `json:"specialist_tool,omitempty"`
	Fields         []WireField `json:"fields"`
}

// All returns every builtin's schema, for GET /api/v1/agent-schemas.
func All() []WireSchema {
	out := make([]WireSchema, 0, len(Builtins))
	for _, b := range Builtins {
		out = append(out, wire(ForBuiltin(b)))
	}
	return out
}

func wire(s Schema) WireSchema {
	ws := WireSchema{
		Builtin:        s.Builtin,
		SpecialistTool: s.SpecialistTool,
		Fields:         make([]WireField, 0, len(s.Fields)),
	}
	for _, f := range s.Fields {
		wf := WireField{
			Key:       f.Key,
			Label:     f.Label,
			Question:  f.Question,
			Required:  f.Required,
			MinLength: f.MinLength,
			Default:   f.Default,
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
