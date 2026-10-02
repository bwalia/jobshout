package repository

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
)

func TestBenchmarkQueriesAreOrgScoped(t *testing.T) {
	org := uuid.New()
	f := model.LLMBenchmarkFilter{OrgID: org, RunKind: "course", Provider: "gemini",
		From: time.Unix(0, 0), To: time.Unix(100, 0)}

	q, args := buildModelStatsQuery(f)
	rq, cq, rargs := buildRunsQuery(f, 20, 0)
	lq, largs := buildCallsQuery(f, 20, 0, false)
	for name, c := range map[string]struct {
		q    string
		args []any
	}{"stats": {q, args}, "runs": {rq, rargs}, "count": {cq, rargs}, "calls": {lq, largs}} {
		if !strings.Contains(c.q, "org_id = $1") {
			t.Errorf("%s: missing org scope:\n%s", name, c.q)
		}
		if c.args[0] != org {
			t.Errorf("%s: $1 = %v, want org", name, c.args[0])
		}
		if !strings.Contains(c.q, "status <> ''") {
			t.Errorf("%s: must read benchmark rows only", name)
		}
	}
	// Run-table joins repeat the org check.
	for _, join := range []string{"a.org_id = $1", "cr.org_id = $1", "br.org_id = $1"} {
		if !strings.Contains(rq, join) {
			t.Errorf("runs query missing %q", join)
		}
	}
}

func TestBuildModelStatsQueryArgsAndGrouping(t *testing.T) {
	org := uuid.New()
	from := time.Unix(10, 0)
	q, args := buildModelStatsQuery(model.LLMBenchmarkFilter{OrgID: org, From: from, Model: "gemini-flash-latest"})
	if len(args) != 3 || args[1] != from || args[2] != "gemini-flash-latest" {
		t.Errorf("args = %v", args)
	}
	if !strings.Contains(q, "created_at >= $2") || !strings.Contains(q, "model = $3") {
		t.Errorf("placeholders wrong:\n%s", q)
	}
	if strings.Contains(q, "GROUP BY provider, model, stage") {
		t.Error("stage grouping must be opt-in")
	}
	for _, agg := range []string{"percentile_cont(0.5)", "percentile_cont(0.95)", "MIN(latency_ms)", "MAX(latency_ms)", "AVG(latency_ms)"} {
		if !strings.Contains(q, agg) {
			t.Errorf("missing %s", agg)
		}
	}
	q, _ = buildModelStatsQuery(model.LLMBenchmarkFilter{OrgID: org, ByStage: true})
	if !strings.Contains(q, "GROUP BY provider, model, stage") {
		t.Errorf("by-stage grouping missing:\n%s", q)
	}
}

func TestBuildCallsQueryPaging(t *testing.T) {
	q, args := buildCallsQuery(model.LLMBenchmarkFilter{OrgID: uuid.New(), RunID: "r1"}, 50, 100, true)
	if !strings.Contains(q, "run_id = $2") || !strings.Contains(q, "LIMIT $3 OFFSET $4") {
		t.Errorf("query:\n%s", q)
	}
	if args[2] != 50 || args[3] != 100 {
		t.Errorf("args = %v", args)
	}
	if !strings.Contains(q, "ORDER BY started_at ASC") {
		t.Error("a run's calls are listed in order")
	}
	if strings.Contains(strings.ToLower(q), "prompt") || strings.Contains(strings.ToLower(q), "content") {
		t.Error("calls query must not select payload columns")
	}
}

func TestInsertLLMCallSQLPlaceholdersMatchColumns(t *testing.T) {
	cols := insertLLMCallSQL[strings.Index(insertLLMCallSQL, "(")+1 : strings.Index(insertLLMCallSQL, ")")]
	nCols := len(strings.Split(cols, ","))
	vals := insertLLMCallSQL[strings.Index(insertLLMCallSQL, "VALUES"):]
	// created_at reuses started_at's placeholder, so one fewer distinct arg.
	maxN := 0
	for _, m := range regexp.MustCompile(`\$(\d+)`).FindAllStringSubmatch(vals, -1) {
		n, _ := strconv.Atoi(m[1])
		maxN = max(maxN, n)
	}
	if nCols != 29 || maxN != 28 {
		t.Errorf("columns = %d, distinct args = %d; want 29 and 28", nCols, maxN)
	}
	for _, c := range []string{"task_run_id", "execution_id", "model_reported", "requested_model",
		"tokens_total", "tokens_reasoning", "api_duration_ms", "api_attempts", "metadata", "provider_request_id"} {
		if !strings.Contains(cols, c) {
			t.Errorf("insert misses %s", c)
		}
	}
}

func TestFilterWhereLinks(t *testing.T) {
	task, run, exec := uuid.New(), uuid.New(), uuid.New()
	where, args := filterWhere(model.LLMBenchmarkFilter{OrgID: uuid.New(), TaskID: &task, TaskRunID: &run, ExecutionID: &exec})
	for _, c := range []string{"task_id = $2", "task_run_id = $3", "execution_id = $4"} {
		if !strings.Contains(where, c) {
			t.Errorf("where %q misses %q", where, c)
		}
	}
	if len(args) != 4 || args[1] != task || args[2] != run || args[3] != exec {
		t.Errorf("args = %v", args)
	}
}
