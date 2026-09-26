package blog

import (
	"encoding/json"
	"testing"
)

func TestWritePlanCoverObjectsShapes(t *testing.T) {
	cases := map[string]string{
		`"lighthouse, hull"`:                                 "lighthouse, hull",
		`["lighthouse", " hull ", ""]`:                       "lighthouse, hull",
		`{"primary": "lighthouse", "secondary": "hull"}`:     "lighthouse, hull",
		`[{"name": "lighthouse"}, {"name": "hull", "n": 2}]`: "lighthouse, hull",
		`null`: "",
	}
	for in, want := range cases {
		var p writePlan
		if err := json.Unmarshal([]byte(`{"title":"t","cover_objects":`+in+`}`), &p); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if string(p.CoverObjects) != want {
			t.Errorf("%s: got %q, want %q", in, p.CoverObjects, want)
		}
	}
}
