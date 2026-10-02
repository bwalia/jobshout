package repository

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/database"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/llmbench"
	"github.com/jobshout/server/internal/model"
)

// TestLLMBenchmarkAgainstPostgres replays every migration twice and runs the
// benchmark writes and reads against a real database. It needs a throwaway
// Postgres with pgvector: LLMBENCH_PG_DSN=postgres://… go test -run Postgres.
func TestLLMBenchmarkAgainstPostgres(t *testing.T) {
	dsn := os.Getenv("LLMBENCH_PG_DSN")
	if dsn == "" {
		t.Skip("LLMBENCH_PG_DSN not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	for range 2 {
		if err := database.RunMigrations(ctx, pool, "../../migrations", zap.NewNop()); err != nil {
			t.Fatalf("migrations: %v", err)
		}
	}
	if err := database.EnsureUsagePartitions(ctx, pool, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	org, other := uuid.New(), uuid.New()
	run := uuid.NewString()
	ms := func(n int) *int { return &n }
	now := time.Now().UTC()
	recs := []llmbench.Record{}
	for i, d := range []int{100, 200, 300, 400, 1000} {
		recs = append(recs, llmbench.Record{
			OrgID: &org, RunKind: "course", RunID: run, Stage: "write", Attempt: 1,
			Provider: "gemini", Model: "gemini-flash-latest", TokensIn: ms(10), TokensOut: ms(5),
			LatencyMs: d, Status: llmbench.StatusSuccess,
			StartedAt: now.Add(time.Duration(i) * time.Second), CompletedAt: now.Add(time.Duration(i)*time.Second + time.Duration(d)*time.Millisecond),
		})
	}
	// Unknown tokens, a retry and a failure.
	recs = append(recs, llmbench.Record{OrgID: &org, RunKind: "course", RunID: run, Stage: "quiz", Attempt: 2,
		Provider: "gemini", Model: "gemini-flash-latest", LatencyMs: 50, Retries: 1, Status: llmbench.StatusFailed,
		Error: "boom", StartedAt: now.Add(10 * time.Second), CompletedAt: now.Add(10 * time.Second)})
	// Another org's call must never show up.
	recs = append(recs, llmbench.Record{OrgID: &other, RunKind: "course", RunID: run, Provider: "gemini",
		Model: "gemini-flash-latest", LatencyMs: 99999, Status: llmbench.StatusSuccess, StartedAt: now, CompletedAt: now})
	// An org-less call is still stored.
	recs = append(recs, llmbench.Record{RunKind: "intent", Provider: "ollama", Model: "m", LatencyMs: 1,
		Status: llmbench.StatusSuccess, StartedAt: now, CompletedAt: now})

	repo := NewLLMBenchmarkRepository(pool)
	if err := repo.InsertLLMCalls(ctx, recs); err != nil {
		t.Fatalf("insert: %v", err)
	}

	stats, err := repo.ModelStats(ctx, model.LLMBenchmarkFilter{OrgID: org})
	if err != nil {
		t.Fatal(err)
	}
	if len(stats) != 1 {
		t.Fatalf("stats = %+v", stats)
	}
	s := stats[0]
	if s.Calls != 6 || s.Runs != 1 || s.Failures != 1 || s.Successes != 5 || s.Retries != 1 || s.RetriedCalls != 1 {
		t.Errorf("counts = %+v", s)
	}
	if s.MinDurationMs != 50 || s.MaxDurationMs != 1000 || s.P50DurationMs != 250 {
		t.Errorf("durations min=%d max=%d p50=%v", s.MinDurationMs, s.MaxDurationMs, s.P50DurationMs)
	}
	if s.InputTokens == nil || *s.InputTokens != 50 || s.TotalTokens == nil || *s.TotalTokens != 75 || s.CallsWithUsage != 5 {
		t.Errorf("tokens in=%v total=%v with=%d", s.InputTokens, s.TotalTokens, s.CallsWithUsage)
	}

	byStage, err := repo.ModelStats(ctx, model.LLMBenchmarkFilter{OrgID: org, ByStage: true})
	if err != nil || len(byStage) != 2 {
		t.Fatalf("by stage = %+v, %v", byStage, err)
	}

	runs, err := repo.ListRuns(ctx, model.LLMBenchmarkFilter{OrgID: org}, model.PaginationParams{})
	if err != nil {
		t.Fatal(err)
	}
	if runs.Total != 1 || runs.Data[0].CallCount != 6 || runs.Data[0].RunDurationMs != nil {
		t.Errorf("runs = %+v", runs)
	}

	detail, err := repo.GetRun(ctx, org, "course", run)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Calls) != 6 || detail.Calls[0].DurationMs != 100 || detail.Calls[5].InputTokens != nil || detail.Calls[5].TotalTokens != nil {
		t.Errorf("detail calls = %+v", detail.Calls)
	}
	if _, err := repo.GetRun(ctx, uuid.New(), "course", run); err != ErrLLMRunNotFound {
		t.Errorf("other org GetRun err = %v", err)
	}

	calls, err := repo.ListCalls(ctx, model.LLMBenchmarkFilter{OrgID: org, Stage: "quiz"}, model.PaginationParams{})
	if err != nil || calls.Total != 1 || calls.Data[0].Status != "failed" {
		t.Errorf("calls = %+v, %v", calls, err)
	}
	// The failed call reported nothing, so everything optional is NULL.
	if q := calls.Data[0]; q.TotalTokens != nil || q.ReasoningTokens != nil || q.APIDurationMs != nil ||
		q.ProviderRequestID != nil || q.TaskRunID != nil || q.ExecutionID != nil || q.ModelReported {
		t.Errorf("unreported fields not NULL: %+v", q)
	}

	// An executor call from a Task Manager run carries every link and what the
	// provider reported, and a reported model longer than the old 100 chars.
	task, taskRun, exec := uuid.New(), uuid.New(), uuid.New()
	longModel := "gemini-2.5-flash-preview-" + strings.Repeat("x", 110)
	execRun := exec.String()
	if err := repo.InsertLLMCalls(ctx, []llmbench.Record{{
		OrgID: &org, TaskID: &task, TaskRunID: &taskRun, ExecutionID: &exec,
		RunKind: "executor", RunID: execRun, Attempt: 1, Provider: "gemini",
		Model: longModel, ModelReported: true, RequestedModel: "gemini-flash-latest",
		TokensIn: ms(12), TokensOut: ms(0), TokensTotal: ms(12), TokensReasoning: ms(0),
		LatencyMs: 900, APIDurationMs: ms(700), ProviderRequestID: "resp-123",
		APIAttempts: 2, Attempts: []llm.APIAttempt{
			{StartedAt: now, DurationMs: 300, HTTPStatus: 503, Error: "gemini: overloaded"},
			{StartedAt: now.Add(time.Second), DurationMs: 400, HTTPStatus: 200, RequestID: "hdr-2"},
		},
		Status: llmbench.StatusSuccess, StartedAt: now, CompletedAt: now,
	}}); err != nil {
		t.Fatalf("insert linked call: %v", err)
	}
	linked, err := repo.ListCalls(ctx, model.LLMBenchmarkFilter{OrgID: org, RunID: execRun}, model.PaginationParams{})
	if err != nil || linked.Total != 1 {
		t.Fatalf("linked = %+v, %v", linked, err)
	}
	l := linked.Data[0]
	if l.TaskID == nil || *l.TaskID != task || l.TaskRunID == nil || *l.TaskRunID != taskRun ||
		l.ExecutionID == nil || *l.ExecutionID != exec {
		t.Errorf("links = task %v task_run %v execution %v", l.TaskID, l.TaskRunID, l.ExecutionID)
	}
	if l.Model != longModel || !l.ModelReported || l.RequestedModel != "gemini-flash-latest" {
		t.Errorf("model = %q reported=%v requested=%q", l.Model, l.ModelReported, l.RequestedModel)
	}
	// A reported 0 stays 0; it is not turned into NULL.
	if l.OutputTokens == nil || *l.OutputTokens != 0 || l.TotalTokens == nil || *l.TotalTokens != 12 ||
		l.ReasoningTokens == nil || *l.ReasoningTokens != 0 {
		t.Errorf("tokens out=%v total=%v reasoning=%v", l.OutputTokens, l.TotalTokens, l.ReasoningTokens)
	}
	if l.DurationMs != 900 || l.APIDurationMs == nil || *l.APIDurationMs != 700 ||
		l.ProviderRequestID == nil || *l.ProviderRequestID != "resp-123" {
		t.Errorf("duration=%d api=%v request_id=%v", l.DurationMs, l.APIDurationMs, l.ProviderRequestID)
	}
	if l.APIAttemptCount == nil || *l.APIAttemptCount != 2 || len(l.APIAttempts) != 2 ||
		l.APIAttempts[0].HTTPStatus != 503 || l.APIAttempts[1].RequestID != "hdr-2" {
		t.Errorf("attempts = %v %+v", l.APIAttemptCount, l.APIAttempts)
	}

	// An execution resumed after an approval pause records calls without its
	// task link; the insert recovers it from the task run that owns the
	// execution. FKs are skipped so the test needs no org/agent/task rows.
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ownerRun, ownerTask, resumed := uuid.New(), uuid.New(), uuid.New()
	if _, err := conn.Exec(ctx, `SET session_replication_role = replica`); err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `INSERT INTO task_runs (id, task_id, agent_id, org_id, execution_id, prompt)
		VALUES ($1, $2, $3, $4, $5, 'p')`, ownerRun, ownerTask, uuid.New(), org, resumed); err != nil {
		t.Fatal(err)
	}
	_, _ = conn.Exec(ctx, `SET session_replication_role = origin`)
	conn.Release()
	if err := repo.InsertLLMCalls(ctx, []llmbench.Record{{
		OrgID: &org, ExecutionID: &resumed, RunKind: "executor", RunID: resumed.String(), Attempt: 1,
		Provider: "ollama", Model: "qwen3:8b", RequestedModel: "qwen3:8b", LatencyMs: 5,
		Status: llmbench.StatusFailed, Error: "boom", StartedAt: now, CompletedAt: now,
	}}); err != nil {
		t.Fatal(err)
	}
	res, err := repo.ListCalls(ctx, model.LLMBenchmarkFilter{OrgID: org, ExecutionID: &resumed}, model.PaginationParams{})
	if err != nil || res.Total != 1 {
		t.Fatalf("resumed = %+v, %v", res, err)
	}
	if rc := res.Data[0]; rc.TaskRunID == nil || *rc.TaskRunID != ownerRun || rc.TaskID == nil || *rc.TaskID != ownerTask ||
		rc.APIAttemptCount != nil || len(rc.APIAttempts) != 0 {
		t.Errorf("resumed links = task_run %v task %v attempts %v", rc.TaskRunID, rc.TaskID, rc.APIAttemptCount)
	}
	// Another org's task run is never borrowed.
	if err := repo.InsertLLMCalls(ctx, []llmbench.Record{{
		OrgID: &other, ExecutionID: &resumed, RunKind: "executor", RunID: "x", Provider: "ollama",
		Status: llmbench.StatusSuccess, StartedAt: now, CompletedAt: now,
	}}); err != nil {
		t.Fatal(err)
	}
	if res, _ := repo.ListCalls(ctx, model.LLMBenchmarkFilter{OrgID: other, ExecutionID: &resumed}, model.PaginationParams{}); res == nil ||
		res.Total != 1 || res.Data[0].TaskRunID != nil {
		t.Errorf("cross-org link = %+v", res)
	}
}
