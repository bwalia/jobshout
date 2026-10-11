package repository

import (
	"context"
	"errors"
	"testing"
)

// One org must never list, read, update or delete another org's knowledge —
// not by guessing a file id, and not by pairing its own agent with a foreign
// file id.
func TestKnowledgeFileStorePG_OrgIsolation(t *testing.T) {
	pool := platformPG(t)
	ctx := context.Background()
	store := NewKnowledgeFileStore(pool)

	orgA, agentA := seedOrgAgent(t, pool)
	orgB, agentB := seedOrgAgent(t, pool)

	fileB, err := store.Create(ctx, agentB.String(), "b.md", "secret of B")
	if err != nil {
		t.Fatal(err)
	}

	if ok, _ := store.AgentInOrg(ctx, orgA.String(), agentB.String()); ok {
		t.Fatal("org A sees org B's agent")
	}
	if ok, _ := store.AgentInOrg(ctx, orgB.String(), agentB.String()); !ok {
		t.Fatal("org B does not see its own agent")
	}
	list, err := store.ListByAgent(ctx, orgA.String(), agentB.String())
	if err != nil || len(list) != 0 {
		t.Fatalf("org A listed B's files: %v %v", list, err)
	}
	for _, agent := range []string{agentA.String(), agentB.String()} {
		if _, err := store.Get(ctx, orgA.String(), agent, fileB); !errors.Is(err, ErrKnowledgeNotFound) {
			t.Fatalf("get via agent %s: err = %v, want not found", agent, err)
		}
		if err := store.UpdateContent(ctx, orgA.String(), agent, fileB, "pwned"); !errors.Is(err, ErrKnowledgeNotFound) {
			t.Fatalf("update via agent %s: err = %v, want not found", agent, err)
		}
		if err := store.Delete(ctx, orgA.String(), agent, fileB); !errors.Is(err, ErrKnowledgeNotFound) {
			t.Fatalf("delete via agent %s: err = %v, want not found", agent, err)
		}
	}

	// Org B still has its file, untouched.
	f, err := store.Get(ctx, orgB.String(), agentB.String(), fileB)
	if err != nil || f.Content != "secret of B" {
		t.Fatalf("org B file = %+v, %v", f, err)
	}
	if err := store.UpdateContent(ctx, orgB.String(), agentB.String(), fileB, "v2"); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete(ctx, orgB.String(), agentB.String(), fileB); err != nil {
		t.Fatal(err)
	}
}
