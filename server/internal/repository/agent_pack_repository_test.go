package repository

import (
	"testing"

	"github.com/jobshout/server/internal/agentpack"
)

func TestOverlayEmptySkillsAndKnowledgeKeepDest(t *testing.T) {
	empty := &agentpack.Package{Agent: agentpack.Body{Name: "Mail", Role: "Mail"}}
	if overlayReplaceSkills(empty) || overlayReplaceKnowledge(empty) {
		t.Fatal("empty package must not wipe dest skills/knowledge")
	}
	filled := &agentpack.Package{
		Skills:    []agentpack.Skill{{Slug: "draft"}},
		Knowledge: []agentpack.File{{Filename: "notes.txt", Content: "hi"}},
	}
	if !overlayReplaceSkills(filled) || !overlayReplaceKnowledge(filled) {
		t.Fatal("non-empty package must replace dest skills/knowledge")
	}
}
