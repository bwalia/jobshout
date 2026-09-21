package workflow

import "testing"

func TestParseLaunchValues(t *testing.T) {
	vals := ParseLaunchValues("mode: plan\npath: prod/ssh\nhosts: a,b", map[string]any{
		"dry_run": "true",
	})
	if vals["mode"] != "plan" || vals["path"] != "prod/ssh" || vals["hosts"] != "a,b" {
		t.Fatalf("%#v", vals)
	}
	if vals["dry_run"] != "true" {
		t.Fatalf("global merge %#v", vals)
	}
}
