package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/engine"
	"github.com/jobshout/server/internal/executor"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// slowRunner stands in for an engine that does not measure itself, as the
// LangChain and LangGraph runners do not.
type slowRunner struct {
	took   time.Duration
	result executor.Result
}

func (r slowRunner) Run(context.Context, uuid.UUID, *model.Agent, string, []string) executor.Result {
	time.Sleep(r.took)
	return r.result
}

// latencyExecRepo returns the row as the database holds it just after
// PersistResult — before the background usage write has filled latency_ms.
type latencyExecRepo struct {
	repository.ExecutionRepository
	row *model.AgentExecution
}

func (r *latencyExecRepo) PersistResult(context.Context, uuid.UUID, executor.Result) error {
	return nil
}

func (r *latencyExecRepo) GetByID(context.Context, uuid.UUID) (*model.AgentExecution, error) {
	cp := *r.row
	return &cp, nil
}

type statusAgentRepo struct{ repository.AgentRepository }

func (statusAgentRepo) UpdateStatus(context.Context, uuid.UUID, string) error { return nil }

func finishWith(t *testing.T, runner engine.Runner, row *model.AgentExecution) *model.AgentExecution {
	t.Helper()
	s := &executionService{
		agentRepo:    statusAgentRepo{},
		execRepo:     &latencyExecRepo{row: row},
		engineRouter: engine.NewRouter(runner, runner, runner, zap.NewNop()),
		logger:       zap.NewNop(),
	}
	got, err := s.finish(context.Background(), &startedExec{
		exec:       &model.AgentExecution{ID: row.ID},
		agent:      &model.Agent{},
		engineType: model.EngineGoNative,
	})
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	return got
}

// The task run copies latency_ms from what finish returns. It used to read 0
// because the only writer of that column runs in the background.
func TestFinishReportsMeasuredDurationForUnmeasuredEngines(t *testing.T) {
	row := &model.AgentExecution{ID: uuid.New(), Status: model.ExecutionStatusCompleted}
	got := finishWith(t, slowRunner{took: 30 * time.Millisecond}, row)
	if got.LatencyMs < 30 {
		t.Errorf("LatencyMs = %d, want at least the 30ms the run took", got.LatencyMs)
	}
}

func TestFinishKeepsTheEnginesOwnDuration(t *testing.T) {
	row := &model.AgentExecution{ID: uuid.New(), Status: model.ExecutionStatusCompleted}
	got := finishWith(t, slowRunner{result: executor.Result{LatencyMs: 4200, InputTokens: 10, OutputTokens: 5}}, row)
	if got.LatencyMs != 4200 || got.InputTokens != 10 || got.OutputTokens != 5 {
		t.Errorf("got latency %d tokens %d/%d", got.LatencyMs, got.InputTokens, got.OutputTokens)
	}
}

// A value already persisted wins over the in-memory one.
func TestFinishDoesNotOverwritePersistedDuration(t *testing.T) {
	row := &model.AgentExecution{ID: uuid.New(), Status: model.ExecutionStatusCompleted, LatencyMs: 999}
	got := finishWith(t, slowRunner{result: executor.Result{LatencyMs: 4200}}, row)
	if got.LatencyMs != 999 {
		t.Errorf("LatencyMs = %d, want the persisted 999", got.LatencyMs)
	}
}

func TestWithAgentModel(t *testing.T) {
	str := func(s string) *string { return &s }
	gemini := &model.Agent{ModelProvider: str("gemini"), ModelName: str("gemini-2.5-pro")}
	cases := []struct {
		name      string
		brief     model.CourseBrief
		agent     *model.Agent
		wantProv  string
		wantModel string
	}{
		{"agent fills both", model.CourseBrief{}, gemini, "gemini", "gemini-2.5-pro"},
		{"launch model kept", model.CourseBrief{Model: "gemini-flash-latest"}, gemini, "gemini", "gemini-flash-latest"},
		{"other provider drops agent model", model.CourseBrief{Provider: "openai"}, gemini, "openai", ""},
		{"auto agent", model.CourseBrief{}, &model.Agent{ModelProvider: str("auto")}, "", ""},
		{"no agent setting", model.CourseBrief{}, &model.Agent{}, "", ""},
		{"nil agent", model.CourseBrief{Provider: "gemini"}, nil, "gemini", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := withAgentModel(tc.brief, tc.agent)
			if got.Provider != tc.wantProv || got.Model != tc.wantModel {
				t.Errorf("got %q/%q, want %q/%q", got.Provider, got.Model, tc.wantProv, tc.wantModel)
			}
		})
	}
}
