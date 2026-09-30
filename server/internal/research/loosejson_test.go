package research

import (
	"encoding/json"
	"reflect"
	"testing"
)

// The shapes llama3.1:8b actually returns for "seeds" and "in_focus" must not
// fail discovery.
func TestDiscoverReplyToleratesLooseShapes(t *testing.T) {
	var parsed struct {
		Topics []struct {
			Seeds   looseInts `json:"seeds"`
			InFocus looseBool `json:"in_focus"`
		} `json:"topics"`
	}
	raw := `{"topics":[
		{"seeds":[0,4],"in_focus":true},
		{"seeds":["1","x"," 2 "],"in_focus":"true"},
		{"seeds":"3","in_focus":"no"},
		{"seeds":null,"in_focus":null}
	]}`
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	want := []struct {
		seeds []int
		focus bool
	}{{[]int{0, 4}, true}, {[]int{1, 2}, true}, {[]int{3}, false}, {nil, false}}
	for i, w := range want {
		got := parsed.Topics[i]
		if !reflect.DeepEqual([]int(got.Seeds), w.seeds) || bool(got.InFocus) != w.focus {
			t.Errorf("topic %d = %v/%v, want %v/%v", i, got.Seeds, got.InFocus, w.seeds, w.focus)
		}
	}
}
