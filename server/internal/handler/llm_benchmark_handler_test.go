package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

type fakeBenchStore struct {
	gotFilter model.LLMBenchmarkFilter
	gotOrg    uuid.UUID
	runErr    error
}

func (f *fakeBenchStore) ModelStats(_ context.Context, fl model.LLMBenchmarkFilter) ([]model.LLMModelStat, error) {
	f.gotFilter = fl
	return []model.LLMModelStat{{Provider: "gemini", Model: "gemini-flash-latest", Calls: 3}}, nil
}
func (f *fakeBenchStore) ListRuns(_ context.Context, fl model.LLMBenchmarkFilter, p model.PaginationParams) (*model.PaginatedResponse[model.LLMRunSummary], error) {
	f.gotFilter = fl
	return &model.PaginatedResponse[model.LLMRunSummary]{Data: []model.LLMRunSummary{}, Page: p.Page, PerPage: p.PerPage}, nil
}
func (f *fakeBenchStore) GetRun(_ context.Context, org uuid.UUID, _, _ string) (*model.LLMRunDetail, error) {
	f.gotOrg = org
	if f.runErr != nil {
		return nil, f.runErr
	}
	return &model.LLMRunDetail{}, nil
}
func (f *fakeBenchStore) ListCalls(_ context.Context, fl model.LLMBenchmarkFilter, _ model.PaginationParams) (*model.PaginatedResponse[model.LLMCall], error) {
	f.gotFilter = fl
	return &model.PaginatedResponse[model.LLMCall]{Data: []model.LLMCall{}}, nil
}

func benchRouter(store LLMBenchmarkStore) http.Handler {
	h := NewLLMBenchmarkHandler(store)
	r := chi.NewRouter()
	r.Get("/benchmarks/models", h.Models)
	r.Get("/benchmarks/runs", h.Runs)
	r.Get("/benchmarks/runs/{kind}/{runID}", h.Run)
	r.Get("/benchmarks/calls", h.Calls)
	return r
}

func benchReq(path, org string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if org != "" {
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyOrgID, org))
	}
	return req
}

func TestBenchmarkEndpointsRequireOrg(t *testing.T) {
	for _, p := range []string{"/benchmarks/models", "/benchmarks/runs", "/benchmarks/calls", "/benchmarks/runs/course/x"} {
		rec := httptest.NewRecorder()
		benchRouter(&fakeBenchStore{}).ServeHTTP(rec, benchReq(p, ""))
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("%s without org: %d, want 401", p, rec.Code)
		}
	}
}

func TestBenchmarkModelsScopesToCallerOrg(t *testing.T) {
	org := uuid.New()
	store := &fakeBenchStore{}
	rec := httptest.NewRecorder()
	benchRouter(store).ServeHTTP(rec, benchReq("/benchmarks/models?provider=gemini&group_by=stage&from=2026-10-01T00:00:00Z", org.String()))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if store.gotFilter.OrgID != org || store.gotFilter.Provider != "gemini" || !store.gotFilter.ByStage {
		t.Errorf("filter = %+v", store.gotFilter)
	}
}

func TestBenchmarkBadTimeIs400(t *testing.T) {
	rec := httptest.NewRecorder()
	benchRouter(&fakeBenchStore{}).ServeHTTP(rec, benchReq("/benchmarks/calls?from=yesterday", uuid.NewString()))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", rec.Code)
	}
}

func TestBenchmarkRunNotFoundIs404(t *testing.T) {
	org := uuid.New()
	store := &fakeBenchStore{runErr: repository.ErrLLMRunNotFound}
	rec := httptest.NewRecorder()
	benchRouter(store).ServeHTTP(rec, benchReq("/benchmarks/runs/course/"+uuid.NewString(), org.String()))
	if rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
	if store.gotOrg != org {
		t.Error("run lookup must use the caller's org")
	}
}

func TestBenchmarkCallsFilterByTaskRunAndExecution(t *testing.T) {
	org, task, taskRun, exec := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	store := &fakeBenchStore{}
	rec := httptest.NewRecorder()
	benchRouter(store).ServeHTTP(rec, benchReq("/benchmarks/calls?task_id="+task.String()+
		"&task_run_id="+taskRun.String()+"&execution_id="+exec.String(), org.String()))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	f := store.gotFilter
	if f.TaskID == nil || *f.TaskID != task || f.TaskRunID == nil || *f.TaskRunID != taskRun ||
		f.ExecutionID == nil || *f.ExecutionID != exec {
		t.Errorf("filter = %+v", f)
	}
	// A query for one task run spans all time rather than the last 30 days.
	if !f.From.IsZero() {
		t.Errorf("from = %v, want unbounded", f.From)
	}
}

func TestBenchmarkBadTaskRunIDIs400(t *testing.T) {
	rec := httptest.NewRecorder()
	benchRouter(&fakeBenchStore{}).ServeHTTP(rec, benchReq("/benchmarks/calls?task_run_id=nope", uuid.NewString()))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", rec.Code)
	}
}
