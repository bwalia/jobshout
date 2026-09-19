package securityreport

import "testing"

func TestDiffDetectFix(t *testing.T) {
	prev := map[string]FindingRef{
		"a": {Key: "a", Title: "A", Severity: "high"},
		"b": {Key: "b", Title: "B", Severity: "low"},
	}
	curr := map[string]FindingRef{
		"b": {Key: "b", Title: "B", Severity: "low"},
		"c": {Key: "c", Title: "C", Severity: "medium"},
	}
	ev := Diff(prev, curr)
	got := map[string]string{}
	for _, e := range ev {
		got[e.FindingKey] = e.Event
	}
	if got["a"] != EventFixed || got["b"] != EventStillOpen || got["c"] != EventDetected {
		t.Fatalf("got %#v", got)
	}
}
