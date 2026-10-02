package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/llmbench"
	"github.com/jobshout/server/internal/model"
)

// ErrLLMRunNotFound is returned when an org has no recorded calls for a run.
var ErrLLMRunNotFound = errors.New("llm benchmark run not found")

// maxRunCalls bounds the calls returned for one run.
const maxRunCalls = 2000

// LLMBenchmarkRepository writes and reads per-call LLM benchmark rows in
// usage_records. Every read is scoped to one org.
type LLMBenchmarkRepository interface {
	InsertLLMCalls(ctx context.Context, recs []llmbench.Record) error
	ModelStats(ctx context.Context, f model.LLMBenchmarkFilter) ([]model.LLMModelStat, error)
	ListRuns(ctx context.Context, f model.LLMBenchmarkFilter, p model.PaginationParams) (*model.PaginatedResponse[model.LLMRunSummary], error)
	GetRun(ctx context.Context, orgID uuid.UUID, runKind, runID string) (*model.LLMRunDetail, error)
	ListCalls(ctx context.Context, f model.LLMBenchmarkFilter, p model.PaginationParams) (*model.PaginatedResponse[model.LLMCall], error)
}

type llmBenchmarkRepository struct {
	pool *pgxpool.Pool
}

// NewLLMBenchmarkRepository creates an LLMBenchmarkRepository.
func NewLLMBenchmarkRepository(pool *pgxpool.Pool) LLMBenchmarkRepository {
	return &llmBenchmarkRepository{pool: pool}
}

// benchmarkRows restricts reads to rows written by the benchmark recorder.
const benchmarkRows = `status <> ''`

// insertLLMCallSQL writes one call. An executor call that reached the
// recorder without its task link (an execution resumed after an approval
// pause starts from a fresh context) takes task_run_id and task_id from the
// task run that owns its execution, within the same org.
const insertLLMCallSQL = `
	INSERT INTO usage_records (org_id, agent_id, task_id, task_run_id, execution_id,
	    provider, model, model_reported, requested_model,
	    tokens_in, tokens_out, tokens_total, tokens_reasoning,
	    latency_ms, api_duration_ms, api_attempts, retries, is_error,
	    run_kind, run_id, stage, attempt, status, error, provider_request_id,
	    metadata, started_at, completed_at, created_at)
	VALUES ($1,$2,
	    COALESCE($3, (SELECT tr.task_id FROM task_runs tr WHERE tr.execution_id = $5 AND tr.org_id = $1 LIMIT 1)),
	    COALESCE($4, (SELECT tr.id FROM task_runs tr WHERE tr.execution_id = $5 AND tr.org_id = $1 LIMIT 1)),
	    $5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,
	    NULLIF($25, ''),$26,$27,$28,$27)`

// apiAttempts is NULL when the client did not report its HTTP exchanges,
// rather than a misleading 0.
func apiAttempts(rec llmbench.Record) *int {
	if rec.APIAttempts == 0 {
		return nil
	}
	n := rec.APIAttempts
	return &n
}

// callMetadata is the usage_records.metadata of a recorded call: the HTTP
// attempts actually sent to the provider.
func callMetadata(rec llmbench.Record) string {
	if len(rec.Attempts) == 0 {
		return "{}"
	}
	b, err := json.Marshal(map[string]any{"api_attempts": rec.Attempts})
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (r *llmBenchmarkRepository) InsertLLMCalls(ctx context.Context, recs []llmbench.Record) error {
	if len(recs) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	for _, rec := range recs {
		batch.Queue(insertLLMCallSQL,
			rec.OrgID, rec.AgentID, rec.TaskID, rec.TaskRunID, rec.ExecutionID,
			rec.Provider, rec.Model, rec.ModelReported, rec.RequestedModel,
			rec.TokensIn, rec.TokensOut, rec.TokensTotal, rec.TokensReasoning,
			rec.LatencyMs, rec.APIDurationMs, apiAttempts(rec), rec.Retries, rec.Status != llmbench.StatusSuccess,
			rec.RunKind, rec.RunID, rec.Stage, rec.Attempt, rec.Status, rec.Error, rec.ProviderRequestID,
			callMetadata(rec), rec.StartedAt, rec.CompletedAt,
		)
	}
	if err := r.pool.SendBatch(ctx, batch).Close(); err != nil {
		return fmt.Errorf("llm_benchmark_repo: insert calls: %w", err)
	}
	return nil
}

// filterWhere builds the shared WHERE clause. org_id is always $1.
func filterWhere(f model.LLMBenchmarkFilter) (string, []any) {
	conds := []string{"org_id = $1", benchmarkRows}
	args := []any{f.OrgID}
	add := func(cond string, v any) {
		args = append(args, v)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if !f.From.IsZero() {
		add("created_at >= $%d", f.From)
	}
	if !f.To.IsZero() {
		add("created_at < $%d", f.To)
	}
	if f.RunKind != "" {
		add("run_kind = $%d", f.RunKind)
	}
	if f.RunID != "" {
		add("run_id = $%d", f.RunID)
	}
	if f.Provider != "" {
		add("provider = $%d", f.Provider)
	}
	if f.Model != "" {
		add("model = $%d", f.Model)
	}
	if f.Stage != "" {
		add("stage = $%d", f.Stage)
	}
	if f.TaskID != nil {
		add("task_id = $%d", *f.TaskID)
	}
	if f.TaskRunID != nil {
		add("task_run_id = $%d", *f.TaskRunID)
	}
	if f.ExecutionID != nil {
		add("execution_id = $%d", *f.ExecutionID)
	}
	return strings.Join(conds, " AND "), args
}

// buildModelStatsQuery aggregates raw calls per provider + exact model (and
// stage). Every statistic is computed here from the raw rows.
func buildModelStatsQuery(f model.LLMBenchmarkFilter) (string, []any) {
	where, args := filterWhere(f)
	group := "provider, model"
	stageCol := "NULL::text"
	if f.ByStage {
		group += ", stage"
		stageCol = "stage"
	}
	q := `
		SELECT provider, model, ` + stageCol + `,
		       COUNT(*),
		       COUNT(DISTINCT NULLIF(run_id, '')),
		       COUNT(*) FILTER (WHERE status = 'success'),
		       COUNT(*) FILTER (WHERE status = 'failed'),
		       COUNT(*) FILTER (WHERE status = 'cancelled'),
		       AVG(latency_ms)::float8,
		       MIN(latency_ms),
		       MAX(latency_ms),
		       percentile_cont(0.5)  WITHIN GROUP (ORDER BY latency_ms)::float8,
		       percentile_cont(0.95) WITHIN GROUP (ORDER BY latency_ms)::float8,
		       SUM(tokens_in)::bigint,
		       SUM(tokens_out)::bigint,
		       SUM(tokens_in + tokens_out)::bigint,
		       COUNT(*) FILTER (WHERE tokens_in IS NOT NULL OR tokens_out IS NOT NULL),
		       AVG(tokens_in)::float8,
		       AVG(tokens_out)::float8,
		       COALESCE(SUM(retries), 0)::int,
		       COUNT(*) FILTER (WHERE retries > 0)
		FROM usage_records
		WHERE ` + where + `
		GROUP BY ` + group + `
		ORDER BY ` + group
	return q, args
}

func (r *llmBenchmarkRepository) ModelStats(ctx context.Context, f model.LLMBenchmarkFilter) ([]model.LLMModelStat, error) {
	q, args := buildModelStatsQuery(f)
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("llm_benchmark_repo: model stats: %w", err)
	}
	defer rows.Close()
	out := []model.LLMModelStat{}
	for rows.Next() {
		var s model.LLMModelStat
		if err := rows.Scan(&s.Provider, &s.Model, &s.Stage,
			&s.Calls, &s.Runs, &s.Successes, &s.Failures, &s.Cancelled,
			&s.AvgDurationMs, &s.MinDurationMs, &s.MaxDurationMs, &s.P50DurationMs, &s.P95DurationMs,
			&s.InputTokens, &s.OutputTokens, &s.TotalTokens, &s.CallsWithUsage,
			&s.AvgInputTokens, &s.AvgOutputTokens, &s.Retries, &s.RetriedCalls,
		); err != nil {
			return nil, fmt.Errorf("llm_benchmark_repo: scan model stat: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// buildRunsQuery groups calls by run and joins the run's own table for its
// overall duration, which is kept separate from the per-call durations. Each
// join repeats the org check so a run ID can never surface another org's run.
func buildRunsQuery(f model.LLMBenchmarkFilter, limit, offset int) (string, string, []any) {
	where, args := filterWhere(f)
	where += " AND run_id <> ''"
	grouped := `
		SELECT run_kind, run_id,
		       (array_agg(agent_id) FILTER (WHERE agent_id IS NOT NULL))[1] AS agent_id,
		       array_agg(DISTINCT provider || '/' || model) AS models,
		       COUNT(*) AS calls,
		       COUNT(*) FILTER (WHERE status = 'failed') AS failures,
		       COALESCE(SUM(retries), 0)::int AS retries,
		       COALESCE(SUM(latency_ms), 0)::bigint AS llm_ms,
		       SUM(tokens_in)::bigint AS tokens_in,
		       SUM(tokens_out)::bigint AS tokens_out,
		       MIN(started_at) AS first_at,
		       MAX(completed_at) AS last_at
		FROM usage_records
		WHERE ` + where + `
		GROUP BY run_kind, run_id`
	countQ := `SELECT COUNT(*) FROM (` + grouped + `) g`
	n := len(args)
	q := `
		WITH g AS (` + grouped + `)
		SELECT g.run_kind, g.run_id, g.agent_id, COALESCE(a.name, ''), g.models,
		       g.calls, g.failures, g.retries, g.llm_ms, g.tokens_in, g.tokens_out,
		       g.first_at, g.last_at,
		       COALESCE(cr.status, br.status, ''),
		       COALESCE(
		         (EXTRACT(EPOCH FROM (cr.completed_at - cr.started_at)) * 1000)::bigint,
		         (EXTRACT(EPOCH FROM (br.completed_at - br.started_at)) * 1000)::bigint)
		FROM g
		LEFT JOIN agents a       ON a.id = g.agent_id AND a.org_id = $1
		LEFT JOIN course_runs cr ON g.run_kind = 'course' AND cr.id::text = g.run_id AND cr.org_id = $1
		LEFT JOIN blog_runs br   ON g.run_kind = 'blog'   AND br.id::text = g.run_id AND br.org_id = $1
		ORDER BY g.first_at DESC NULLS LAST
		LIMIT $` + fmt.Sprint(n+1) + ` OFFSET $` + fmt.Sprint(n+2)
	return q, countQ, append(args, limit, offset)
}

func scanRunSummary(row pgx.Row) (model.LLMRunSummary, error) {
	var s model.LLMRunSummary
	err := row.Scan(&s.RunKind, &s.RunID, &s.AgentID, &s.AgentName, &s.Models,
		&s.CallCount, &s.Failures, &s.Retries, &s.LLMTimeMs, &s.InputTokens, &s.OutputTokens,
		&s.FirstCallAt, &s.LastCallAt, &s.RunStatus, &s.RunDurationMs)
	return s, err
}

func (r *llmBenchmarkRepository) ListRuns(ctx context.Context, f model.LLMBenchmarkFilter, p model.PaginationParams) (*model.PaginatedResponse[model.LLMRunSummary], error) {
	p.Normalize()
	q, countQ, args := buildRunsQuery(f, p.PerPage, p.Offset())
	var total int
	if err := r.pool.QueryRow(ctx, countQ, args[:len(args)-2]...).Scan(&total); err != nil {
		return nil, fmt.Errorf("llm_benchmark_repo: count runs: %w", err)
	}
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("llm_benchmark_repo: list runs: %w", err)
	}
	defer rows.Close()
	data := []model.LLMRunSummary{}
	for rows.Next() {
		s, err := scanRunSummary(rows)
		if err != nil {
			return nil, fmt.Errorf("llm_benchmark_repo: scan run: %w", err)
		}
		data = append(data, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return paginated(data, total, p), nil
}

func (r *llmBenchmarkRepository) GetRun(ctx context.Context, orgID uuid.UUID, runKind, runID string) (*model.LLMRunDetail, error) {
	f := model.LLMBenchmarkFilter{OrgID: orgID, RunKind: runKind, RunID: runID}
	q, _, args := buildRunsQuery(f, 1, 0)
	s, err := scanRunSummary(r.pool.QueryRow(ctx, q, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrLLMRunNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("llm_benchmark_repo: get run: %w", err)
	}
	cq, cargs := buildCallsQuery(f, maxRunCalls, 0, true)
	calls, err := r.queryCalls(ctx, cq, cargs)
	if err != nil {
		return nil, err
	}
	return &model.LLMRunDetail{LLMRunSummary: s, Calls: calls}, nil
}

const llmCallColumns = `id, agent_id, task_id, task_run_id, execution_id, run_kind, run_id, stage, attempt,
	provider, model, model_reported, requested_model, latency_ms, api_duration_ms,
	api_attempts, COALESCE(metadata->'api_attempts', '[]'::jsonb),
	tokens_in, tokens_out, tokens_total, tokens_reasoning, provider_request_id,
	retries, status, error, started_at, completed_at`

// buildCallsQuery lists raw calls; chronological within a run, newest first
// otherwise.
func buildCallsQuery(f model.LLMBenchmarkFilter, limit, offset int, chronological bool) (string, []any) {
	where, args := filterWhere(f)
	order := "created_at DESC, started_at DESC"
	if chronological {
		order = "started_at ASC, created_at ASC"
	}
	n := len(args)
	q := `SELECT ` + llmCallColumns + ` FROM usage_records WHERE ` + where +
		` ORDER BY ` + order +
		` LIMIT $` + fmt.Sprint(n+1) + ` OFFSET $` + fmt.Sprint(n+2)
	return q, append(args, limit, offset)
}

func (r *llmBenchmarkRepository) queryCalls(ctx context.Context, q string, args []any) ([]model.LLMCall, error) {
	rows, err := r.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("llm_benchmark_repo: list calls: %w", err)
	}
	defer rows.Close()
	out := []model.LLMCall{}
	for rows.Next() {
		var c model.LLMCall
		if err := rows.Scan(&c.ID, &c.AgentID, &c.TaskID, &c.TaskRunID, &c.ExecutionID,
			&c.RunKind, &c.RunID, &c.Stage, &c.Attempt,
			&c.Provider, &c.Model, &c.ModelReported, &c.RequestedModel, &c.DurationMs, &c.APIDurationMs,
			&c.APIAttemptCount, &c.APIAttempts,
			&c.InputTokens, &c.OutputTokens, &c.TotalTokens, &c.ReasoningTokens, &c.ProviderRequestID,
			&c.Retries, &c.Status, &c.Error, &c.StartedAt, &c.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("llm_benchmark_repo: scan call: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *llmBenchmarkRepository) ListCalls(ctx context.Context, f model.LLMBenchmarkFilter, p model.PaginationParams) (*model.PaginatedResponse[model.LLMCall], error) {
	p.Normalize()
	where, wargs := filterWhere(f)
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM usage_records WHERE `+where, wargs...).Scan(&total); err != nil {
		return nil, fmt.Errorf("llm_benchmark_repo: count calls: %w", err)
	}
	q, args := buildCallsQuery(f, p.PerPage, p.Offset(), f.RunID != "")
	calls, err := r.queryCalls(ctx, q, args)
	if err != nil {
		return nil, err
	}
	return paginated(calls, total, p), nil
}

func paginated[T any](data []T, total int, p model.PaginationParams) *model.PaginatedResponse[T] {
	pages := 0
	if p.PerPage > 0 {
		pages = (total + p.PerPage - 1) / p.PerPage
	}
	return &model.PaginatedResponse[T]{Data: data, Total: total, Page: p.Page, PerPage: p.PerPage, TotalPages: pages}
}
