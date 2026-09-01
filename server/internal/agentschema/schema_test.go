package agentschema

import (
	"testing"

	"github.com/jobshout/server/internal/model"
)

func TestForBuiltin_ResearcherTopicFirst(t *testing.T) {
	s := ForBuiltin(model.BuiltinResearcher)
	if s.SpecialistTool != "research_run" {
		t.Fatalf("tool = %q", s.SpecialistTool)
	}
	slot, q, opts := s.NextMissing(map[string]string{"name": "Research Agent"})
	if slot != "topic" {
		t.Fatalf("slot = %q; want topic", slot)
	}
	if q == "" {
		t.Fatal("expected a topic question")
	}
	if len(opts) != 0 {
		t.Fatal("topic is free text; no chips")
	}
}

func TestForBuiltin_PentesterTarget(t *testing.T) {
	s := ForBuiltin(model.BuiltinPentester)
	slot, _, _ := s.NextMissing(nil)
	if slot != "target" {
		t.Fatalf("slot = %q; want target", slot)
	}
	filled := s.ApplyDefaults(map[string]string{"target": "https://int.example.com"})
	slot, _, _ = s.NextMissing(filled)
	if slot != "" {
		t.Fatalf("scan_mode should default; still missing %q", slot)
	}
	if filled["scan_mode"] != "quick" {
		t.Fatalf("scan_mode default = %q", filled["scan_mode"])
	}
}

func TestForBuiltin_PRReviewerSequential(t *testing.T) {
	s := ForBuiltin(model.BuiltinPRReviewer)
	slot, _, _ := s.NextMissing(map[string]string{"repo": "acme/api"})
	if slot != "pr_number" {
		t.Fatalf("slot = %q; want pr_number", slot)
	}
}

func TestForBuiltin_CareerOps(t *testing.T) {
	s := ForBuiltin(model.BuiltinCareerOps)
	if s.SpecialistTool != "career_evaluate" {
		t.Fatalf("tool = %q", s.SpecialistTool)
	}
}

func TestForBuiltin_GenericPrompt(t *testing.T) {
	s := ForBuiltin("")
	slot, _, _ := s.NextMissing(map[string]string{"name": "Custom"})
	if slot != "prompt" {
		t.Fatalf("slot = %q; want prompt", slot)
	}
}

func TestIsThinPrompt(t *testing.T) {
	cases := []struct {
		prompt, name string
		thin         bool
	}{
		{"run the research agent", "Research Agent", true},
		{"Run the Research Agent", "Research Agent", true},
		{"please run the research agent", "Research Agent", true},
		{"run research agent", "Research Agent", true},
		{"Kubernetes cost optimisation", "Research Agent", false},
		{"run the research agent on kubernetes costs", "Research Agent", false},
		{"", "X", true},
		{"fix the login timeout", "Custom", false},
	}
	for _, c := range cases {
		if got := IsThinPrompt(c.prompt, c.name); got != c.thin {
			t.Errorf("IsThinPrompt(%q, %q) = %v; want %v", c.prompt, c.name, got, c.thin)
		}
	}
}
