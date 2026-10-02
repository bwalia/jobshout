package repository

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/database"
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
		Provider: "gemini", Model: "gemini-flash-latest", LatencyMs: 50, Retries: 1, Status: llmbench.StatusError,
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
	if err != nil || calls.Total != 1 || calls.Data[0].Status != "error" {
		t.Errorf("calls = %+v, %v", calls, err)
	}
}
