package model

import (
	"context"
	"testing"
)

func TestRunContext(t *testing.T) {
	var nilRC *RunContext
	if nilRC.Normalize() != nil || (&RunContext{Role: "  "}).Normalize() != nil {
		t.Fatal("blank context should normalize to nil")
	}
	if nilRC.MergeInto(nil) != nil {
		t.Fatal("nil context must leave inputs untouched")
	}
	rc := (&RunContext{Role: " manager ", WorkspaceRef: "ws"}).Normalize()
	in := rc.MergeInto(map[string]any{"role": "spoofed", "listing": "L1"})
	if in["role"] != "manager" || in["workspace_ref"] != "ws" || in["listing"] != "L1" {
		t.Fatalf("inputs = %v", in)
	}
	if _, ok := in["user_ref"]; ok {
		t.Fatal("empty user_ref added")
	}
	if RunContextFrom(WithRunContext(context.Background(), rc)) != rc || RunContextFrom(context.Background()) != nil {
		t.Fatal("context round trip")
	}
}
