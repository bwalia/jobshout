package repository

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/executor"
	"github.com/jobshout/server/internal/model"
)

// approvalPG runs the approvals, task-run and pause-status migrations in a
// throwaway schema of a real Postgres, with legacy rows in place before
// migration 062 first runs, then replays everything as a boot would:
//
//	APPROVALS_TEST_DATABASE_URL=postgres://postgres:pw@localhost:5432/postgres go test ./internal/repository -run ApprovalPG
func approvalPG(t *testing.T) (*pgxpool.Pool, map[string]uuid.UUID) {
	t.Helper()
	dsn := os.Getenv("APPROVALS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("APPROVALS_TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "approval_test_" + uuid.NewString()[:8]
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
		pool.Close()
	})
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%v\n%s", err, sql[:min(200, len(sql))])
		}
	}
	migrate := func(name string) {
		t.Helper()
		b, err := os.ReadFile("../../migrations/" + name)
		if err != nil {
			t.Fatal(err)
		}
		exec(string(b))
	}

	// Just enough of the tables migrations 018 and 027 refer to.
	exec(`CREATE SCHEMA ` + schema)
	exec(`CREATE TABLE organizations (id UUID PRIMARY KEY DEFAULT gen_random_uuid());
		CREATE TABLE users (id UUID PRIMARY KEY DEFAULT gen_random_uuid());
		CREATE TABLE tasks (id UUID PRIMARY KEY DEFAULT gen_random_uuid());
		CREATE TABLE agents (id UUID PRIMARY KEY DEFAULT gen_random_uuid());
		CREATE TABLE agent_executions (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(), agent_id UUID, org_id UUID,
			workflow_run_id UUID, step_id UUID, input_prompt TEXT NOT NULL DEFAULT '', output TEXT,
			status VARCHAR(50) NOT NULL DEFAULT 'pending', error_message TEXT,
			total_tokens INT NOT NULL DEFAULT 0, input_tokens INT NOT NULL DEFAULT 0,
			output_tokens INT NOT NULL DEFAULT 0, latency_ms INT NOT NULL DEFAULT 0,
			cost_usd DOUBLE PRECISION NOT NULL DEFAULT 0, model_name TEXT, model_provider TEXT,
			iterations INT NOT NULL DEFAULT 0, engine_type TEXT NOT NULL DEFAULT 'go_native',
			started_at TIMESTAMPTZ, completed_at TIMESTAMPTZ, created_at TIMESTAMPTZ NOT NULL DEFAULT NOW());
		CREATE TABLE execution_tool_calls (
			id UUID PRIMARY KEY, execution_id UUID, tool_name TEXT, input JSONB, output TEXT,
			error_message TEXT, duration_ms INT, called_at TIMESTAMPTZ);`)
	migrate("000018_approvals.up.sql")
	migrate("000027_task_runs.up.sql")

	ids := map[string]uuid.UUID{}
	for _, k := range []string{"org", "user", "task", "agent", "legacyExec", "legacyRun", "legacyApproval", "decidedApproval", "decidedExec"} {
		ids[k] = uuid.New()
	}
	exec(`INSERT INTO organizations (id) VALUES ($1)`, ids["org"])
	exec(`INSERT INTO users (id) VALUES ($1)`, ids["user"])
	exec(`INSERT INTO tasks (id) VALUES ($1)`, ids["task"])
	exec(`INSERT INTO agents (id) VALUES ($1)`, ids["agent"])

	// Legacy state the old code left behind: a paused run saved as completed
	// (with its pending approval), and an approval decided and resumed in the
	// request.
	exec(`INSERT INTO agent_executions (id, agent_id, org_id, status, completed_at) VALUES
		($1, $3, $4, 'completed', NOW()), ($2, $3, $4, 'completed', NOW())`,
		ids["legacyExec"], ids["decidedExec"], ids["agent"], ids["org"])
	exec(`INSERT INTO task_runs (id, task_id, agent_id, org_id, execution_id, status, prompt, completed_at)
		VALUES ($1, $2, $3, $4, $5, 'completed', 'p', NOW())`,
		ids["legacyRun"], ids["task"], ids["agent"], ids["org"], ids["legacyExec"])
	exec(`INSERT INTO approvals (id, org_id, execution_id, agent_id, tool_name, status) VALUES
		($1, $2, $3, $4, 'send', 'pending')`,
		ids["legacyApproval"], ids["org"], ids["legacyExec"], ids["agent"])
	exec(`INSERT INTO approvals (id, org_id, execution_id, agent_id, tool_name, status, decided_at) VALUES
		($1, $2, $3, $4, 'send', 'approved', NOW())`,
		ids["decidedApproval"], ids["org"], ids["decidedExec"], ids["agent"])

	migrate("000062_approval_pause_status.up.sql")
	// Replays on every boot must be harmless.
	for i := 0; i < 2; i++ {
		migrate("000018_approvals.up.sql")
		migrate("000027_task_runs.up.sql")
		migrate("000062_approval_pause_status.up.sql")
	}
	return pool, ids
}

func TestApprovalPG_MigrationBackfillAndReplay(t *testing.T) {
	pool, ids := approvalPG(t)
	ctx := context.Background()

	var execStatus string
	var execCompleted *time.Time
	if err := pool.QueryRow(ctx, `SELECT status, completed_at FROM agent_executions WHERE id = $1`,
		ids["legacyExec"]).Scan(&execStatus, &execCompleted); err != nil {
		t.Fatal(err)
	}
	if execStatus != model.ExecutionStatusAwaitingApproval || execCompleted != nil {
		t.Fatalf("legacy paused execution: status=%q completed_at=%v", execStatus, execCompleted)
	}

	runs := NewTaskRunRepository(pool)
	run, err := runs.GetByExecutionID(ctx, ids["legacyExec"])
	if err != nil || run == nil {
		t.Fatalf("GetByExecutionID: %v %v", run, err)
	}
	if run.Status != model.TaskRunStatusAwaitingApproval || run.PendingApprovalID == nil || *run.PendingApprovalID != ids["legacyApproval"] {
		t.Fatalf("legacy task run not backfilled: %+v", run)
	}
	if none, err := runs.GetByExecutionID(ctx, uuid.New()); err != nil || none != nil {
		t.Fatalf("unknown execution must give (nil, nil), got %v %v", none, err)
	}

	approvals := NewApprovalRepository(pool)
	decided, err := approvals.FindByID(ctx, ids["decidedApproval"])
	if err != nil {
		t.Fatal(err)
	}
	if decided.ResumedAt == nil {
		t.Fatal("an approval decided before the migration must be marked resumed, or it would be resumed twice")
	}
	unresumed, err := approvals.ListUnresumed(ctx, time.Minute, 10)
	if err != nil || len(unresumed) != 0 {
		t.Fatalf("nothing should be waiting for a resume after the backfill: %v %v", unresumed, err)
	}

	// The widened constraint accepts the new status and still rejects junk.
	run.Status = model.TaskRunStatusRunning
	run.PendingApprovalID = nil
	if err := runs.Update(ctx, run); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE task_runs SET status = 'bogus' WHERE id = $1`, run.ID); err == nil {
		t.Fatal("status constraint must still reject unknown statuses")
	}
}

func TestApprovalPG_DecideClaimAndResumeLifecycle(t *testing.T) {
	pool, ids := approvalPG(t)
	ctx := context.Background()
	approvals := NewApprovalRepository(pool)
	execs := NewExecutionRepository(pool)

	id := ids["legacyApproval"]
	if _, err := approvals.FindByID(ctx, uuid.New()); !errors.Is(err, ErrApprovalNotFound) {
		t.Fatalf("FindByID unknown: want ErrApprovalNotFound, got %v", err)
	}
	if err := approvals.UpdateDecision(ctx, uuid.New(), model.ApprovalStatusApproved, "", ids["user"]); !errors.Is(err, ErrApprovalNotFound) {
		t.Fatalf("decide unknown: want ErrApprovalNotFound, got %v", err)
	}
	// A pending approval cannot be claimed for a resume.
	if ok, err := approvals.ClaimResume(ctx, id, time.Minute); err != nil || ok {
		t.Fatalf("claim before decision: ok=%v err=%v", ok, err)
	}
	if err := approvals.UpdateDecision(ctx, id, model.ApprovalStatusApproved, "fine", ids["user"]); err != nil {
		t.Fatal(err)
	}
	if err := approvals.UpdateDecision(ctx, id, model.ApprovalStatusRejected, "", ids["user"]); !errors.Is(err, ErrApprovalNotPending) {
		t.Fatalf("second decide: want ErrApprovalNotPending, got %v", err)
	}

	list, err := approvals.ListUnresumed(ctx, time.Minute, 10)
	if err != nil || len(list) != 1 || list[0].ID != id {
		t.Fatalf("decided approval should await a resume: %v %v", list, err)
	}
	if ok, err := approvals.ClaimResume(ctx, id, time.Minute); err != nil || !ok {
		t.Fatalf("first claim: ok=%v err=%v", ok, err)
	}
	if ok, _ := approvals.ClaimResume(ctx, id, time.Minute); ok {
		t.Fatal("a live claim must not be taken twice")
	}
	if list, _ := approvals.ListUnresumed(ctx, time.Minute, 10); len(list) != 0 {
		t.Fatal("a claimed approval must not be listed for recovery")
	}
	// A stale claim (its worker died) is recoverable.
	if list, _ := approvals.ListUnresumed(ctx, 0, 10); len(list) != 1 {
		t.Fatal("a stale claim must be listed for recovery")
	}
	if ok, _ := approvals.ClaimResume(ctx, id, 0); !ok {
		t.Fatal("a stale claim must be claimable")
	}

	// Execution: paused -> resuming -> completed, tool calls written once.
	execID := ids["legacyExec"]
	if ok, err := execs.MarkResuming(ctx, execID); err != nil || !ok {
		t.Fatalf("MarkResuming paused exec: ok=%v err=%v", ok, err)
	}
	prior := executor.ToolCallRecord{ToolName: "lookup", Output: "x"}
	paused := executor.Result{Status: executor.StatusAwaitingApproval, TotalTokens: 5, Iterations: 1,
		ToolCalls: []executor.ToolCallRecord{prior}}
	if err := execs.PersistResult(ctx, execID, paused); err != nil {
		t.Fatal(err)
	}
	got, err := execs.GetByID(ctx, execID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.ExecutionStatusAwaitingApproval || got.Output != nil || got.CompletedAt != nil || got.TotalTokens != 5 {
		t.Fatalf("paused execution persisted wrong: %+v", got)
	}
	done := executor.Result{FinalAnswer: "sent", TotalTokens: 9, Iterations: 2, PriorToolCalls: 1,
		ToolCalls: []executor.ToolCallRecord{prior, {ToolName: "send", Output: "ok"}}}
	if err := execs.PersistResult(ctx, execID, done); err != nil {
		t.Fatal(err)
	}
	got, _ = execs.GetByID(ctx, execID)
	if got.Status != model.ExecutionStatusCompleted || got.Output == nil || *got.Output != "sent" || got.TotalTokens != 9 {
		t.Fatalf("resumed execution persisted wrong: %+v", got)
	}
	if len(got.ToolCalls) != 2 {
		t.Fatalf("tool calls must be written once each across the pause, got %d", len(got.ToolCalls))
	}
	if ok, _ := execs.MarkResuming(ctx, execID); ok {
		t.Fatal("a finished execution must not be resumed")
	}

	if err := approvals.MarkResumed(ctx, id); err != nil {
		t.Fatal(err)
	}
	if ok, _ := approvals.ClaimResume(ctx, id, 0); ok {
		t.Fatal("a resumed approval must never be claimed again")
	}
}
