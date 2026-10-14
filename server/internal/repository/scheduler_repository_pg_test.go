package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

// Every field the edit form sends is stored, and a task can only be pointed
// at its own org's agent or workflow.
func TestSchedulerRepoPG_UpdateSavesFields(t *testing.T) {
	pool := platformPG(t)
	ctx := context.Background()
	orgA, agentA := seedOrgAgent(t, pool)
	orgB, agentB := seedOrgAgent(t, pool)
	seedWorkflow := func(org uuid.UUID) string {
		id := uuid.New()
		if _, err := pool.Exec(ctx, `INSERT INTO workflows (id, org_id, name) VALUES ($1, $2, 'w')`, id, org); err != nil {
			t.Fatalf("seed workflow: %v", err)
		}
		return id.String()
	}
	wfA, wfB := seedWorkflow(orgA), seedWorkflow(orgB)

	repo := NewSchedulerRepository(pool)
	task := &model.ScheduledTask{ID: uuid.New(), OrgID: orgA, Name: "t", TaskType: "workflow",
		ScheduleType: "once", Status: "active", Priority: "medium", MaxRetries: 3, Tags: []string{"blog"}}
	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatal(err)
	}

	runAt, retry, retries := "2030-01-02T09:00:00Z", true, 5
	got, err := repo.UpdateTask(ctx, task.ID, model.UpdateScheduledTaskRequest{
		WorkflowID: &wfA, RunAt: &runAt, RetryOnFailure: &retry, MaxRetries: &retries,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC)
	if got.WorkflowID == nil || got.WorkflowID.String() != wfA || got.RunAt == nil || !got.RunAt.Equal(want) ||
		!got.RetryOnFailure || got.MaxRetries != 5 {
		t.Fatalf("update not saved: %+v", got)
	}
	// Nil tags leave the stored ones; a list replaces them.
	if len(got.Tags) != 1 || got.Tags[0] != "blog" {
		t.Fatalf("tags = %v, want unchanged", got.Tags)
	}
	got, err = repo.UpdateTask(ctx, task.ID, model.UpdateScheduledTaskRequest{Tags: []string{"a", "b"}})
	if err != nil || len(got.Tags) != 2 {
		t.Fatalf("tags = %v, %v", got.Tags, err)
	}

	agentAStr, agentBStr := agentA.String(), agentB.String()
	if _, err := repo.UpdateTask(ctx, task.ID, model.UpdateScheduledTaskRequest{AgentID: &agentAStr}); err != nil {
		t.Fatalf("own agent: %v", err)
	}
	for name, req := range map[string]model.UpdateScheduledTaskRequest{
		"agent":    {AgentID: &agentBStr},
		"workflow": {WorkflowID: &wfB},
	} {
		if _, err := repo.UpdateTask(ctx, task.ID, req); !errors.Is(err, ErrScheduleTargetNotFound) {
			t.Errorf("foreign %s = %v, want ErrScheduleTargetNotFound", name, err)
		}
	}
	empty := ""
	got, err = repo.UpdateTask(ctx, task.ID, model.UpdateScheduledTaskRequest{WorkflowID: &empty})
	if err != nil || got.WorkflowID != nil || got.AgentID == nil || got.AgentID.String() != agentAStr {
		t.Fatalf("clear workflow = %+v, %v", got, err)
	}

	// Create holds to the same rule, for every agent role and the workflow.
	wfBID := uuid.MustParse(wfB)
	for name, mut := range map[string]func(*model.ScheduledTask){
		"agent":    func(x *model.ScheduledTask) { x.AgentID = &agentB },
		"reviewer": func(x *model.ScheduledTask) { x.AgentID = &agentA; x.ReviewerID = &agentB },
		"workflow": func(x *model.ScheduledTask) { x.WorkflowID = &wfBID },
	} {
		x := &model.ScheduledTask{ID: uuid.New(), OrgID: orgA, Name: name, TaskType: "agent",
			ScheduleType: "once", Status: "active", Priority: "medium", Tags: []string{}}
		mut(x)
		if err := repo.CreateTask(ctx, x); !errors.Is(err, ErrScheduleTargetNotFound) {
			t.Errorf("create with foreign %s = %v, want ErrScheduleTargetNotFound", name, err)
		}
	}
	own := &model.ScheduledTask{ID: uuid.New(), OrgID: orgA, Name: "own", TaskType: "agent", AgentID: &agentA,
		ScheduleType: "once", Status: "active", Priority: "medium", Tags: []string{}}
	if err := repo.CreateTask(ctx, own); err != nil {
		t.Fatalf("create with own agent: %v", err)
	}
}
