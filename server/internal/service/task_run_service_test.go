package service

import (
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

func strptr(s string) *string { return &s }

func TestResolveRunAgent(t *testing.T) {
	override := uuid.New()
	assigned := uuid.New()

	if got, err := resolveRunAgent(&override, &assigned); err != nil || got != override {
		t.Fatalf("override should win: got %v err %v", got, err)
	}
	if got, err := resolveRunAgent(nil, &assigned); err != nil || got != assigned {
		t.Fatalf("assigned should be used when no override: got %v err %v", got, err)
	}
	if _, err := resolveRunAgent(nil, nil); err == nil {
		t.Fatalf("expected error when no agent is available")
	}
	nilID := uuid.Nil
	if _, err := resolveRunAgent(&nilID, nil); err == nil {
		t.Fatalf("a nil UUID override must not count as an agent")
	}
}

func TestBuildRunPrompt_TitleAndDescription(t *testing.T) {
	task := &model.Task{Title: "Ship the thing", Description: strptr("with tests")}
	got := buildRunPrompt(task, model.CreateTaskRunRequest{})
	want := "Ship the thing\n\nwith tests"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBuildRunPrompt_OverrideAndInputs(t *testing.T) {
	task := &model.Task{Title: "ignored", Description: strptr("ignored too")}
	got := buildRunPrompt(task, model.CreateTaskRunRequest{
		Prompt: strptr("do exactly this"),
		Inputs: map[string]any{"env": "int", "budget": 5},
	})
	// Prompt override replaces the task text; inputs are appended in sorted order.
	want := "do exactly this\n\n## Inputs\n- budget: 5\n- env: int"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestBoardStatusForRun(t *testing.T) {
	cases := []struct {
		run  string
		want string
		move bool
	}{
		{model.TaskRunStatusQueued, "in_progress", true},
		{model.TaskRunStatusRunning, "in_progress", true},
		{model.TaskRunStatusCompleted, "done", true},
		{model.TaskRunStatusFailed, "", false},
	}
	for _, tc := range cases {
		got, ok := boardStatusForRun(tc.run)
		if ok != tc.move || got != tc.want {
			t.Fatalf("%s: got (%q, %v), want (%q, %v)", tc.run, got, ok, tc.want, tc.move)
		}
	}
}

func TestNormalizeSlugs(t *testing.T) {
	got := normalizeSlugs([]string{" Web-Search ", "web-search", "", "RAG", "rag"})
	want := []string{"web-search", "rag"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	if len(normalizeSlugs(nil)) != 0 {
		t.Fatalf("nil should normalize to empty, non-nil slice")
	}
}
