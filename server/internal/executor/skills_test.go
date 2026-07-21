package executor

import (
	"strings"
	"testing"

	"github.com/jobshout/server/internal/model"
)

func strPtr(s string) *string { return &s }

func TestApplySkills_ToolSkillsWidenAllowList(t *testing.T) {
	skills := []model.Skill{
		{Kind: "tool", ConfigJSON: map[string]any{"tool": "http_request"}},
		{Kind: "tool", ConfigJSON: map[string]any{"tools": []any{"shell_command", "http_request"}}},
	}

	tools, patch := applySkills([]string{"existing_tool"}, skills)

	if patch != "" {
		t.Fatalf("expected no prompt patch for tool-only skills, got %q", patch)
	}
	want := []string{"existing_tool", "http_request", "shell_command"}
	if len(tools) != len(want) {
		t.Fatalf("expected %v, got %v", want, tools)
	}
	for i := range want {
		if tools[i] != want[i] {
			t.Fatalf("tool[%d] = %q, want %q (full: %v)", i, tools[i], want[i], tools)
		}
	}
}

func TestApplySkills_DedupesToolNames(t *testing.T) {
	skills := []model.Skill{
		{Kind: "tool", ConfigJSON: map[string]any{"tool": "http_request"}},
	}
	// http_request is already permitted; it must not be added twice.
	tools, _ := applySkills([]string{"http_request"}, skills)
	if len(tools) != 1 {
		t.Fatalf("expected deduped single tool, got %v", tools)
	}
}

func TestApplySkills_PromptSkillsBuildPatch(t *testing.T) {
	skills := []model.Skill{
		{Kind: "prompt", ConfigJSON: map[string]any{"prompt": "Be concise."}},
		{Kind: "prompt", Description: strPtr("Cite your sources."), ConfigJSON: map[string]any{}},
	}

	tools, patch := applySkills([]string{"http_request"}, skills)

	if len(tools) != 1 || tools[0] != "http_request" {
		t.Fatalf("prompt skills must not change tools, got %v", tools)
	}
	if !strings.Contains(patch, "## Enabled Skills") {
		t.Fatalf("expected Enabled Skills heading, got %q", patch)
	}
	if !strings.Contains(patch, "Be concise.") || !strings.Contains(patch, "Cite your sources.") {
		t.Fatalf("patch missing skill fragments: %q", patch)
	}
}

func TestApplySkills_PromptPrefersConfigOverDescription(t *testing.T) {
	skills := []model.Skill{
		{Kind: "prompt", Description: strPtr("fallback"), ConfigJSON: map[string]any{"prompt": "explicit"}},
	}
	_, patch := applySkills(nil, skills)
	if !strings.Contains(patch, "explicit") || strings.Contains(patch, "fallback") {
		t.Fatalf("expected config prompt to win over description, got %q", patch)
	}
}

func TestApplySkills_BundleKindIsIgnored(t *testing.T) {
	skills := []model.Skill{
		{Kind: "bundle", ConfigJSON: map[string]any{"tool": "http_request", "prompt": "x"}},
	}
	tools, patch := applySkills([]string{"base"}, skills)
	if len(tools) != 1 || tools[0] != "base" {
		t.Fatalf("bundle must not add tools yet, got %v", tools)
	}
	if patch != "" {
		t.Fatalf("bundle must not add prompt yet, got %q", patch)
	}
}

func TestApplySkills_NoSkillsIsNoOp(t *testing.T) {
	tools, patch := applySkills([]string{"a", "b"}, nil)
	if len(tools) != 2 || patch != "" {
		t.Fatalf("expected unchanged tools and empty patch, got %v / %q", tools, patch)
	}
}
