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
