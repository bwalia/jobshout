package abtest

import (
	"context"
	"testing"
)

func TestDemoListAndObserve(t *testing.T) {
	c := NewClient(nil)
	if !c.Enabled() {
		t.Fatal("demo client should be enabled")
	}
	if c.LiveConfigured() {
		t.Fatal("nil mcp should not be live")
	}
	list, mode, err := c.ListExperiments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if mode != "demo" || len(list) != 1 {
		t.Fatalf("mode=%s len=%d", mode, len(list))
	}
	out, err := c.Observe(context.Background(), "abtesting", 10)
	if err != nil {
		t.Fatal(err)
	}
	if out["mode"] != "demo" {
		t.Fatalf("observe mode=%v", out["mode"])
	}
	counts, _ := out["counts"].(map[string]int)
	if counts["v1"]+counts["v2"] != 10 {
		t.Fatalf("counts=%v", counts)
	}
}
