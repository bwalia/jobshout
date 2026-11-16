package blog

import (
	"encoding/json"
	"testing"
)

// A model's hint field decodes from any shape it is given.
func TestFlexibleStringShapes(t *testing.T) {
	cases := map[string]string{
		`"lighthouse, hull"`:                                 "lighthouse, hull",
		`["lighthouse", " hull ", ""]`:                       "lighthouse, hull",
		`{"primary": "lighthouse", "secondary": "hull"}`:     "lighthouse, hull",
		`[{"name": "lighthouse"}, {"name": "hull", "n": 2}]`: "lighthouse, hull",
		`null`: "",
	}
	for in, want := range cases {
		var got flexibleString
		if err := json.Unmarshal([]byte(in), &got); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if string(got) != want {
			t.Errorf("%s: got %q, want %q", in, got, want)
		}
	}
}

// A planned figure whose content comes back as a list still decodes, so the
// run goes on instead of failing at the plan step.
func TestWritePlan_FigureContentAsList(t *testing.T) {
	var plan writePlan
	in := `{"figures": [{"kind": "process", "section": "Rollout", "content": ["new pod starts", "readiness probe passes", "old pod drains"]}]}`
	if err := json.Unmarshal([]byte(in), &plan); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(plan.Figures) != 1 || plan.Figures[0].Content != "new pod starts, readiness probe passes, old pod drains" || plan.Figures[0].Section != "Rollout" {
		t.Fatalf("got %+v", plan.Figures)
	}
}
