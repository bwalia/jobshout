package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// LLMBenchmarkStore is what the benchmark endpoints read.
type LLMBenchmarkStore interface {
	ModelStats(ctx context.Context, f model.LLMBenchmarkFilter) ([]model.LLMModelStat, error)
	ListRuns(ctx context.Context, f model.LLMBenchmarkFilter, p model.PaginationParams) (*model.PaginatedResponse[model.LLMRunSummary], error)
	GetRun(ctx context.Context, orgID uuid.UUID, runKind, runID string) (*model.LLMRunDetail, error)
	ListCalls(ctx context.Context, f model.LLMBenchmarkFilter, p model.PaginationParams) (*model.PaginatedResponse[model.LLMCall], error)
}

// LLMBenchmarkHandler serves the read-only LLM benchmark API. Every query is
// scoped to the caller's org.
type LLMBenchmarkHandler struct {
	store LLMBenchmarkStore
}

// NewLLMBenchmarkHandler creates an LLMBenchmarkHandler.
func NewLLMBenchmarkHandler(store LLMBenchmarkStore) *LLMBenchmarkHandler {
	return &LLMBenchmarkHandler{store: store}
}

// Models handles GET /benchmarks/models — per provider + exact model (and per
// stage with group_by=stage) call statistics over a time range.
func (h *LLMBenchmarkHandler) Models(w http.ResponseWriter, r *http.Request) {
	f, ok := benchmarkFilter(w, r)
	if !ok {
		return
	}
	f.ByStage = r.URL.Query().Get("group_by") == "stage"
	stats, err := h.store.ModelStats(r.Context(), f)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to query benchmark")
		return
	}
	RespondJSON(w, http.StatusOK, stats)
}

// Runs handles GET /benchmarks/runs — calls grouped by run.
func (h *LLMBenchmarkHandler) Runs(w http.ResponseWriter, r *http.Request) {
	f, ok := benchmarkFilter(w, r)
	if !ok {
		return
	}
	res, err := h.store.ListRuns(r.Context(), f, pageParams(r))
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to query benchmark runs")
		return
	}
	RespondJSON(w, http.StatusOK, res)
}

// Run handles GET /benchmarks/runs/{kind}/{runID} — one run and every call.
func (h *LLMBenchmarkHandler) Run(w http.ResponseWriter, r *http.Request) {
	orgID, ok := benchmarkOrg(w, r)
	if !ok {
		return
	}
	kind, runID := chi.URLParam(r, "kind"), chi.URLParam(r, "runID")
	if kind == "" || runID == "" || len(runID) > 100 {
		RespondError(w, http.StatusBadRequest, "invalid run")
		return
	}
	run, err := h.store.GetRun(r.Context(), orgID, kind, runID)
	if errors.Is(err, repository.ErrLLMRunNotFound) {
		RespondError(w, http.StatusNotFound, "no LLM calls recorded for this run")
		return
	}
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to query benchmark run")
		return
	}
	RespondJSON(w, http.StatusOK, run)
}

// Calls handles GET /benchmarks/calls — raw per-call rows, paginated.
func (h *LLMBenchmarkHandler) Calls(w http.ResponseWriter, r *http.Request) {
	f, ok := benchmarkFilter(w, r)
	if !ok {
		return
	}
	res, err := h.store.ListCalls(r.Context(), f, pageParams(r))
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to query benchmark calls")
		return
	}
	RespondJSON(w, http.StatusOK, res)
}

func benchmarkOrg(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil || orgID == uuid.Nil {
		RespondError(w, http.StatusUnauthorized, "organisation required")
		return uuid.Nil, false
	}
	return orgID, true
}

// benchmarkFilter reads the shared query params. from/to are RFC3339; the
// default range is the last 30 days.
func benchmarkFilter(w http.ResponseWriter, r *http.Request) (model.LLMBenchmarkFilter, bool) {
	orgID, ok := benchmarkOrg(w, r)
	if !ok {
		return model.LLMBenchmarkFilter{}, false
	}
	q := r.URL.Query()
	f := model.LLMBenchmarkFilter{
		OrgID:    orgID,
		RunKind:  q.Get("run_kind"),
		RunID:    q.Get("run_id"),
		Provider: q.Get("provider"),
		Model:    q.Get("model"),
		Stage:    q.Get("stage"),
	}
	var err error
	if v := q.Get("from"); v != "" {
		if f.From, err = time.Parse(time.RFC3339, v); err != nil {
			RespondError(w, http.StatusBadRequest, "invalid from: use RFC3339")
			return f, false
		}
	}
	if v := q.Get("to"); v != "" {
		if f.To, err = time.Parse(time.RFC3339, v); err != nil {
			RespondError(w, http.StatusBadRequest, "invalid to: use RFC3339")
			return f, false
		}
	}
	for _, p := range []struct {
		name string
		dst  **uuid.UUID
	}{{"task_id", &f.TaskID}, {"task_run_id", &f.TaskRunID}, {"execution_id", &f.ExecutionID}} {
		v := q.Get(p.name)
		if v == "" {
			continue
		}
		id, perr := uuid.Parse(v)
		if perr != nil {
			RespondError(w, http.StatusBadRequest, "invalid "+p.name)
			return f, false
		}
		*p.dst = &id
	}
	// A query for one run, task, task run or execution spans all time.
	if f.From.IsZero() && f.RunID == "" && f.TaskID == nil && f.TaskRunID == nil && f.ExecutionID == nil {
		f.From = time.Now().AddDate(0, 0, -30)
	}
	return f, true
}

func pageParams(r *http.Request) model.PaginationParams {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	per, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	p := model.PaginationParams{Page: page, PerPage: per}
	p.Normalize()
	return p
}
