package repository

import (
	"testing"

	"github.com/google/uuid"
)

func TestBuildOrgRunWhere(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()

	where, args := buildOrgRunWhere(orgID, TaskRunListFilter{})
	if where != "org_id = $1" || len(args) != 1 || args[0] != orgID {
		t.Fatalf("empty filter: where=%q args=%v", where, args)
	}

	where, args = buildOrgRunWhere(orgID, TaskRunListFilter{Status: "failed"})
	if where != "org_id = $1 AND status = $2" || len(args) != 2 || args[1] != "failed" {
		t.Fatalf("status filter: where=%q args=%v", where, args)
	}

	where, args = buildOrgRunWhere(orgID, TaskRunListFilter{AgentID: &agentID})
	if where != "org_id = $1 AND agent_id = $2" || len(args) != 2 || args[1] != agentID {
		t.Fatalf("agent filter: where=%q args=%v", where, args)
	}

	where, args = buildOrgRunWhere(orgID, TaskRunListFilter{Status: "completed", AgentID: &agentID})
	if where != "org_id = $1 AND status = $2 AND agent_id = $3" {
		t.Fatalf("both filters: where=%q", where)
	}
	if len(args) != 3 || args[1] != "completed" || args[2] != agentID {
		t.Fatalf("both filters args: %v", args)
	}
}
