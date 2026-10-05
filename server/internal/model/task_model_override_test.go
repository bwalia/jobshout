package model

import "testing"

func TestWithModelOverrideKeepsOtherMetadata(t *testing.T) {
	meta := map[string]any{"run_id": "r1", "launch_values": map[string]any{"topic": "x"}}
	got := WithModelOverride(meta, TaskModelOverride{Provider: " gemini ", Model: "gemini-2.5-pro"})
	if got["run_id"] != "r1" || got["launch_values"] == nil {
		t.Fatalf("other keys lost: %v", got)
	}
	o := ModelOverrideFrom(got)
	if o == nil || o.Provider != "gemini" || o.Model != "gemini-2.5-pro" {
		t.Fatalf("override = %+v", o)
	}
	if _, ok := meta[TaskMetaModelOverride]; ok {
		t.Error("input map was mutated")
	}

	cleared := WithModelOverride(got, TaskModelOverride{})
	if ModelOverrideFrom(cleared) != nil || cleared["run_id"] != "r1" {
		t.Errorf("clear = %v", cleared)
	}
}

func TestModelOverrideFromToleratesBadValues(t *testing.T) {
	for _, meta := range []map[string]any{
		nil,
		{},
		{TaskMetaModelOverride: "gemini"},
		{TaskMetaModelOverride: map[string]any{"provider": 3}},
		{TaskMetaModelOverride: map[string]any{"provider": "", "model": "  "}},
	} {
		if o := ModelOverrideFrom(meta); o != nil {
			t.Errorf("ModelOverrideFrom(%v) = %+v, want nil", meta, o)
		}
	}
	// Model alone is valid: the agent's provider default applies.
	if o := ModelOverrideFrom(map[string]any{TaskMetaModelOverride: map[string]any{"model": "llama3"}}); o == nil || o.Model != "llama3" {
		t.Errorf("model-only override = %+v", o)
	}
}
