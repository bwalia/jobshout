package repository

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

func TestTaskRunContextsPG(t *testing.T) {
	pool := platformPG(t)
	ctx := context.Background()
	orgID, agentID := seedOrgAgent(t, pool)
	projectID, taskID := uuid.New(), uuid.New()
	if _, err := pool.Exec(ctx, `INSERT INTO projects (id, org_id, name) VALUES ($1, $2, 'p')`, projectID, orgID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO tasks (id, project_id, title) VALUES ($1, $2, 't')`, taskID, projectID); err != nil {
		t.Fatal(err)
	}
	repo := &TaskRunContexts{TaskRunRepository: NewTaskRunRepository(pool), Pool: pool}

	rc := &model.RunContext{Role: " seller_agent ", UserRef: "u-1", WorkspaceRef: "ws-9"}
	run := &model.TaskRun{ID: uuid.New(), TaskID: taskID, AgentID: agentID, OrgID: orgID,
		Status: model.TaskRunStatusQueued, Prompt: "p"}
	if err := repo.Create(model.WithRunContext(ctx, rc), run); err != nil {
		t.Fatal(err)
	}
	if run.Context == nil || run.Context.Role != "seller_agent" {
		t.Fatalf("in-memory run context = %+v", run.Context)
	}
	plain := &model.TaskRun{ID: uuid.New(), TaskID: taskID, AgentID: agentID, OrgID: orgID,
		Status: model.TaskRunStatusQueued, Prompt: "p"}
	if err := repo.Create(ctx, plain); err != nil {
		t.Fatal(err)
	}

	// Updates (the run lifecycle) keep the stored context.
	run.Status, run.Context = model.TaskRunStatusCompleted, nil
	if err := repo.Update(ctx, run); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetByID(ctx, run.ID)
	if err != nil || got.Context == nil || *got.Context != (model.RunContext{Role: "seller_agent", UserRef: "u-1", WorkspaceRef: "ws-9"}) {
		t.Fatalf("GetByID context = %+v, %v", got.Context, err)
	}
	page, err := repo.ListByTask(ctx, taskID, model.PaginationParams{Page: 1, PerPage: 10})
	if err != nil || len(page.Data) != 2 {
		t.Fatalf("list = %+v, %v", page, err)
	}
	for _, r := range page.Data {
		if (r.ID == run.ID) != (r.Context != nil) {
			t.Fatalf("run %s context = %+v", r.ID, r.Context)
		}
	}
}
