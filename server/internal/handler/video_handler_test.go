package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

type fakeVideoRuns struct {
	run       model.VideoRun
	cancelled bool
}

func (f *fakeVideoRuns) List(_ context.Context, orgID uuid.UUID, _ int) ([]model.VideoRun, error) {
	if orgID != f.run.OrgID {
		return []model.VideoRun{}, nil
	}
	return []model.VideoRun{f.run}, nil
}

func (f *fakeVideoRuns) Get(_ context.Context, orgID, id uuid.UUID) (*model.VideoRun, error) {
	if orgID != f.run.OrgID || id != f.run.ID {
		return nil, repository.ErrVideoRunNotFound
	}
	r := f.run
	return &r, nil
}

func (f *fakeVideoRuns) Cancel(ctx context.Context, orgID, id uuid.UUID) (*model.VideoRun, error) {
	run, err := f.Get(ctx, orgID, id)
	if err == nil {
		f.cancelled = true
		run.Status = model.VideoRunCancelled
	}
	return run, err
}

func videoRouter(h *VideoHandler, orgID uuid.UUID) http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), middleware.ContextKeyOrgID, orgID.String())
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Get("/runs", h.ListRuns)
	r.Get("/runs/{runID}", h.GetRun)
	r.Post("/runs/{runID}/cancel", h.CancelRun)
	return r
}

func TestVideoRunsAreScopedToTheCallersOrg(t *testing.T) {
	org := uuid.New()
	runs := &fakeVideoRuns{run: model.VideoRun{ID: uuid.New(), OrgID: org, Status: model.VideoRunRendering, Stage: "motion"}}
	h := NewVideoHandler(runs)

	rec := httptest.NewRecorder()
	videoRouter(h, org).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/runs", nil))
	var list struct {
		Data []model.VideoRun `json:"data"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &list) != nil || len(list.Data) != 1 || list.Data[0].Stage != "motion" {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	videoRouter(h, uuid.New()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/runs/"+runs.run.ID.String(), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("another org read the run: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	videoRouter(h, org).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/runs/not-a-uuid", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad id: %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	videoRouter(h, org).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/runs/"+runs.run.ID.String()+"/cancel", nil))
	if rec.Code != http.StatusOK || !runs.cancelled {
		t.Fatalf("cancel: %d %s", rec.Code, rec.Body)
	}
}
