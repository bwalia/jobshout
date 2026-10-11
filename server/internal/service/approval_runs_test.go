package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/executor"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// --- execution side: how a paused or resumed outcome is stored ---

// outcomeExecRepo stores PersistResult like the real repository would and
// serves the row back.
type outcomeExecRepo struct {
	repository.ExecutionRepository
	row       model.AgentExecution
	persisted []executor.Result
}

func (r *outcomeExecRepo) PersistResult(_ context.Context, _ uuid.UUID, res executor.Result) error {
	r.persisted = append(r.persisted, res)
	switch {
	case res.Status == executor.StatusAwaitingApproval && res.Err == nil:
		r.row.Status, r.row.Output, r.row.CompletedAt = model.ExecutionStatusAwaitingApproval, nil, nil
	case res.Err != nil:
		msg := res.Err.Error()
		r.row.Status, r.row.ErrorMessage = model.ExecutionStatusFailed, &msg
	default:
		now := time.Now()
		r.row.Status, r.row.Output, r.row.CompletedAt = model.ExecutionStatusCompleted, &res.FinalAnswer, &now
	}
	r.row.TotalTokens, r.row.Iterations = res.TotalTokens, res.Iterations
	return nil
}

func (r *outcomeExecRepo) GetByID(context.Context, uuid.UUID) (*model.AgentExecution, error) {
	cp := r.row
	return &cp, nil
}

// usageGov prices usage synchronously, writing the cost like RecordUsage does.
type usageGov struct {
	GovernanceService
	repo     *outcomeExecRepo
	recorded []*model.AgentExecution
}

func (g *usageGov) RecordUsage(_ context.Context, e *model.AgentExecution) error {
	e.CostUSD = float64(e.TotalTokens) / 1000
	g.recorded = append(g.recorded, e)
	g.repo.row.CostUSD, g.repo.row.LatencyMs = e.CostUSD, e.LatencyMs
	g.repo.row.InputTokens, g.repo.row.OutputTokens = e.InputTokens, e.OutputTokens
	return nil
}

func TestExecutionOutcome_PausedAndResumed(t *testing.T) {
	execID, agentID, orgID := uuid.New(), uuid.New(), uuid.New()
	nextApproval := uuid.New()
	cases := []struct {
		name        string
		result      executor.Result
		wantStatus  string
		wantUsage   bool
		wantCost    float64
		wantPending *uuid.UUID
	}{
		{"paused: awaiting approval, no usage yet",
			executor.Result{Status: executor.StatusAwaitingApproval, ApprovalID: nextApproval, TotalTokens: 100, LatencyMs: 50},
			model.ExecutionStatusAwaitingApproval, false, 0, &nextApproval},
		{"resumed to completion: cumulative usage and cost",
			executor.Result{FinalAnswer: "sent", TotalTokens: 2000, InputTokens: 1500, OutputTokens: 500, LatencyMs: 1234, PriorToolCalls: 1},
			model.ExecutionStatusCompleted, true, 2, nil},
		{"resumed to failure: usage still recorded",
			executor.Result{Err: errors.New("tool exploded"), TotalTokens: 1000, LatencyMs: 10},
			model.ExecutionStatusFailed, true, 1, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &outcomeExecRepo{row: model.AgentExecution{
				ID: execID, AgentID: agentID, OrgID: orgID, Status: model.ExecutionStatusRunning,
			}}
			gov := &usageGov{repo: repo}
			s := &executionService{agentRepo: statusAgentRepo{}, execRepo: repo, govSvc: gov, logger: zap.NewNop()}

			got, err := s.CompleteResumed(context.Background(), execID, tc.result)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.wantStatus {
				t.Fatalf("status %q, want %q", got.Status, tc.wantStatus)
			}
			if (len(gov.recorded) == 1) != tc.wantUsage {
				t.Fatalf("usage recorded %d times, want usage=%v", len(gov.recorded), tc.wantUsage)
			}
			if tc.wantUsage {
				u := gov.recorded[0]
				if u.AgentID != agentID || u.OrgID != orgID || u.TotalTokens != tc.result.TotalTokens {
					t.Fatalf("usage carries wrong identity/metering: %+v", u)
				}
			}
			if got.CostUSD != tc.wantCost {
				t.Fatalf("cost %v, want %v (must be on the returned execution, for the task run)", got.CostUSD, tc.wantCost)
			}
			if got.LatencyMs != tc.result.LatencyMs || got.TotalTokens != tc.result.TotalTokens {
				t.Fatalf("metering: latency=%d tokens=%d", got.LatencyMs, got.TotalTokens)
			}
			if (got.PendingApprovalID == nil) != (tc.wantPending == nil) ||
				(tc.wantPending != nil && *got.PendingApprovalID != *tc.wantPending) {
				t.Fatalf("pending approval id %v, want %v", got.PendingApprovalID, tc.wantPending)
			}
			if len(repo.persisted) != 1 || repo.persisted[0].PriorToolCalls != tc.result.PriorToolCalls {
				t.Fatal("the result (with PriorToolCalls) must reach PersistResult unchanged")
			}
		})
	}
}

// TestExecutionFinish_PausedRunIsAwaitingApproval: a fresh run that pauses comes
// back from Execute as awaiting_approval with the approval id — never completed.
func TestExecutionFinish_PausedRunIsAwaitingApproval(t *testing.T) {
	approvalID := uuid.New()
	row := &model.AgentExecution{ID: uuid.New(), Status: model.ExecutionStatusAwaitingApproval}
	got := finishWith(t, slowRunner{result: executor.Result{
		Status: executor.StatusAwaitingApproval, ApprovalID: approvalID, TotalTokens: 12,
	}}, row)
	if got.Status != model.ExecutionStatusAwaitingApproval || got.Output != nil {
		t.Fatalf("paused run returned as %q (output %v)", got.Status, got.Output)
	}
	if got.PendingApprovalID == nil || *got.PendingApprovalID != approvalID {
		t.Fatalf("pending approval id %v, want %s", got.PendingApprovalID, approvalID)
	}
}

// --- task-run side: mirroring the execution ---

type memRuns struct {
	repository.TaskRunRepository
	byExec  map[uuid.UUID]*model.TaskRun
	updates []model.TaskRun
}

func (m *memRuns) GetByExecutionID(_ context.Context, id uuid.UUID) (*model.TaskRun, error) {
	r, ok := m.byExec[id]
	if !ok {
		return nil, nil
	}
	cp := *r
	return &cp, nil
}

func (m *memRuns) Update(_ context.Context, r *model.TaskRun) error {
	m.updates = append(m.updates, *r)
	return nil
}

func (m *memRuns) CountActiveByTask(context.Context, uuid.UUID) (int, error) { return 0, nil }

type boardTasks struct {
	repository.TaskRepository
	moves []string
}

func (b *boardTasks) TransitionStatus(_ context.Context, _ uuid.UUID, status string, _ *uuid.UUID) error {
	b.moves = append(b.moves, status)
	return nil
}

func TestApplyExecution(t *testing.T) {
	approval := uuid.New()
	done := time.Now().Add(-time.Minute)
	out := "final"
	errMsg := "boom"
	cases := []struct {
		name          string
		exec          model.AgentExecution
		wantStatus    string
		wantPending   bool
		wantCompleted bool
	}{
		{"paused", model.AgentExecution{Status: model.ExecutionStatusAwaitingApproval, PendingApprovalID: &approval},
			model.TaskRunStatusAwaitingApproval, true, false},
		{"resuming", model.AgentExecution{Status: model.ExecutionStatusRunning},
			model.TaskRunStatusRunning, false, false},
		{"completed", model.AgentExecution{Status: model.ExecutionStatusCompleted, Output: &out, CompletedAt: &done},
			model.TaskRunStatusCompleted, false, true},
		{"failed", model.AgentExecution{Status: model.ExecutionStatusFailed, ErrorMessage: &errMsg, CompletedAt: &done},
			model.TaskRunStatusFailed, false, true},
		{"cancelled while waiting", model.AgentExecution{Status: model.ExecutionStatusCancelled, CompletedAt: &done},
			model.TaskRunStatusFailed, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stale := uuid.New()
			run := &model.TaskRun{Status: model.TaskRunStatusAwaitingApproval, PendingApprovalID: &stale}
			tc.exec.ID = uuid.New()
			tc.exec.TotalTokens, tc.exec.CostUSD, tc.exec.LatencyMs = 77, 0.5, 900
			applyExecution(run, &tc.exec)
			if run.Status != tc.wantStatus {
				t.Fatalf("status %q, want %q", run.Status, tc.wantStatus)
			}
			if (run.PendingApprovalID != nil) != tc.wantPending {
				t.Fatalf("pending approval id %v", run.PendingApprovalID)
			}
			if tc.wantPending && *run.PendingApprovalID != approval {
				t.Fatal("wrong pending approval id")
			}
			if (run.CompletedAt != nil) != tc.wantCompleted {
				t.Fatalf("completed_at %v", run.CompletedAt)
			}
			if run.TotalTokens != 77 || run.CostUSD != 0.5 || run.LatencyMs != 900 || *run.ExecutionID != tc.exec.ID {
				t.Fatalf("metering not mirrored: %+v", run)
			}
		})
	}
}

func TestSyncFromExecution(t *testing.T) {
	execID := uuid.New()
	runs := &memRuns{byExec: map[uuid.UUID]*model.TaskRun{
		execID: {ID: uuid.New(), TaskID: uuid.New(), Status: model.TaskRunStatusAwaitingApproval},
	}}
	board := &boardTasks{}
	s := &taskRunService{runRepo: runs, taskRepo: board, logger: zap.NewNop()}
	out := "sent"

	if err := s.SyncFromExecution(context.Background(), &model.AgentExecution{
		ID: execID, Status: model.ExecutionStatusCompleted, Output: &out, TotalTokens: 5, CostUSD: 0.01,
	}); err != nil {
		t.Fatal(err)
	}
	if len(runs.updates) != 1 || runs.updates[0].Status != model.TaskRunStatusCompleted || *runs.updates[0].Output != "sent" {
		t.Fatalf("run not finalized from the resumed execution: %+v", runs.updates)
	}
	if len(board.moves) != 1 || board.moves[0] != "done" {
		t.Fatalf("board card should move to done, got %v", board.moves)
	}

	// An execution not launched as a task run is ignored.
	if err := s.SyncFromExecution(context.Background(), &model.AgentExecution{ID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	if len(runs.updates) != 1 {
		t.Fatal("no task run must be written for an execution without one")
	}
}

// pausingExecSvc returns a paused execution from Execute.
type pausingExecSvc struct {
	ExecutionService
	exec *model.AgentExecution
}

func (p *pausingExecSvc) Execute(context.Context, uuid.UUID, uuid.UUID, model.ExecuteAgentRequest) (*model.AgentExecution, error) {
	return p.exec, nil
}

// TestTaskRunExecute_PausedRun: GET /task-runs/{id} for a run that paused shows
// awaiting_approval with the approval id — not completed with an empty output.
func TestTaskRunExecute_PausedRun(t *testing.T) {
	approval := uuid.New()
	exec := &model.AgentExecution{ID: uuid.New(), Status: model.ExecutionStatusAwaitingApproval,
		PendingApprovalID: &approval, TotalTokens: 40}
	runs := &memRuns{}
	board := &boardTasks{}
	s := &taskRunService{runRepo: runs, taskRepo: board, execSvc: &pausingExecSvc{exec: exec}, logger: zap.NewNop()}

	run := &model.TaskRun{ID: uuid.New(), TaskID: uuid.New(), OrgID: uuid.New(), Status: model.TaskRunStatusQueued}
	s.execute(context.Background(), run, uuid.New())

	last := runs.updates[len(runs.updates)-1]
	if last.Status != model.TaskRunStatusAwaitingApproval {
		t.Fatalf("paused run stored as %q", last.Status)
	}
	if last.PendingApprovalID == nil || *last.PendingApprovalID != approval {
		t.Fatalf("pending approval id %v", last.PendingApprovalID)
	}
	if last.CompletedAt != nil || last.Output != nil {
		t.Fatal("a paused run has no completed_at and no output")
	}
	for _, m := range board.moves {
		if m == "done" {
			t.Fatal("a paused run must not move the card to done")
		}
	}
}
