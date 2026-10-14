package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

// A sprint takes only its own org's agents and jobs; adding one twice is not
// an error.
func TestSprintRepoPG_MembersStayInOrg(t *testing.T) {
	pool := platformPG(t)
	ctx := context.Background()
	orgA, agentA := seedOrgAgent(t, pool)
	orgB, agentB := seedOrgAgent(t, pool)
	repo := NewSprintRepository(pool)

	sp := &model.Sprint{ID: uuid.New(), OrgID: orgA, Name: "s1"}
	if err := repo.Create(ctx, sp); err != nil {
		t.Fatal(err)
	}
	seedJob := func(org, agent uuid.UUID) uuid.UUID {
		id := uuid.New()
		if _, err := pool.Exec(ctx, `INSERT INTO multi_agent_jobs (id, org_id, task_prompt, planner_id, executor_id, reviewer_id)
			VALUES ($1, $2, 'p', $3, $3, $3)`, id, org, agent); err != nil {
			t.Fatalf("seed job: %v", err)
		}
		return id
	}
	jobA, jobB := seedJob(orgA, agentA), seedJob(orgB, agentB)

	for i := 0; i < 2; i++ {
		if err := repo.AddAgent(ctx, sp.ID, agentA, "planner"); err != nil {
			t.Fatalf("add own agent (pass %d): %v", i, err)
		}
		if err := repo.AddJob(ctx, sp.ID, jobA, 0); err != nil {
			t.Fatalf("add own job (pass %d): %v", i, err)
		}
	}
	if err := repo.AddAgent(ctx, sp.ID, agentB, "planner"); !errors.Is(err, ErrSprintMemberNotFound) {
		t.Fatalf("add foreign agent = %v, want ErrSprintMemberNotFound", err)
	}
	if err := repo.AddJob(ctx, sp.ID, jobB, 0); !errors.Is(err, ErrSprintMemberNotFound) {
		t.Fatalf("add foreign job = %v, want ErrSprintMemberNotFound", err)
	}

	agents, _ := repo.ListAgents(ctx, sp.ID)
	jobs, _ := repo.ListJobs(ctx, sp.ID)
	if len(agents) != 1 || len(jobs) != 1 {
		t.Fatalf("sprint has %d agents, %d jobs; want 1 and 1", len(agents), len(jobs))
	}
}
