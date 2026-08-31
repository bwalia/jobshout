package chatsvc

import (
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

func TestAgentCardMetaCopiesRunIdsFromEntities(t *testing.T) {
	agentID := uuid.New()
	execID := uuid.New()
	runID := uuid.New()
	resp := model.ChatResponse{
		Message: "done",
		Entities: []model.EntityRef{
			{Kind: model.EntityExecution, ID: execID.String(), Label: "Research Agent", Href: "/agents/" + agentID.String()},
			{Kind: model.EntityWorkflowRun, ID: runID.String(), Label: "Release"},
			{Kind: model.EntityAgent, ID: agentID.String(), Label: "Research Agent"},
		},
	}
	meta := agentCardMeta(resp)
	if meta["execution_id"] != execID.String() {
		t.Fatalf("execution_id = %v", meta["execution_id"])
	}
	if meta["workflow_run_id"] != runID.String() {
		t.Fatalf("workflow_run_id = %v", meta["workflow_run_id"])
	}
	if meta["agent_id"] != agentID.String() {
		t.Fatalf("agent_id = %v", meta["agent_id"])
	}
	if meta["agent_name"] != "Research Agent" {
		t.Fatalf("agent_name = %v", meta["agent_name"])
	}
}

func TestAgentIDFromHref(t *testing.T) {
	id := uuid.New().String()
	if got := agentIDFromHref("/agents/" + id); got != id {
		t.Fatalf("got %q", got)
	}
	if got := agentIDFromHref("/agents/" + id + "/knowledge"); got != id {
		t.Fatalf("path suffix: got %q", got)
	}
	if got := agentIDFromHref("/workflows/" + id); got != "" {
		t.Fatalf("wrong kind leaked: %q", got)
	}
}
