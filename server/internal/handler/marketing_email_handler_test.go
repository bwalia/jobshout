package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/marketingemail"
	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

type fakeEmailRuns struct{ run model.MarketingEmailRun }

func (f *fakeEmailRuns) List(_ context.Context, orgID uuid.UUID, _ int) ([]model.MarketingEmailRun, error) {
	if orgID != f.run.OrgID {
		return []model.MarketingEmailRun{}, nil
	}
	return []model.MarketingEmailRun{f.run}, nil
}

func (f *fakeEmailRuns) Get(_ context.Context, orgID, id uuid.UUID) (*model.MarketingEmailRun, error) {
	if orgID != f.run.OrgID || id != f.run.ID {
		return nil, repository.ErrMarketingEmailRunNotFound
	}
	r := f.run
	return &r, nil
}

func (f *fakeEmailRuns) ReGenerate(ctx context.Context, orgID, id uuid.UUID) (*model.MarketingEmailRun, error) {
	run, err := f.Get(ctx, orgID, id)
	if err == nil && run.Status == model.MarketingEmailStatusWriting {
		return run, marketingemail.ErrBusy
	}
	return run, err
}

func (f *fakeEmailRuns) SaveTemplate(ctx context.Context, orgID, id uuid.UUID) (*model.MarketingEmailRun, error) {
	return f.Get(ctx, orgID, id)
}

func (f *fakeEmailRuns) Unpublish(ctx context.Context, orgID, id uuid.UUID) (*model.MarketingEmailRun, error) {
	run, err := f.Get(ctx, orgID, id)
	if err == nil {
		run.Status = model.MarketingEmailStatusArchived
	}
	return run, err
}

func emailRouter(h *MarketingEmailHandler, orgID uuid.UUID) http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			ctx := context.WithValue(req.Context(), middleware.ContextKeyOrgID, orgID.String())
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	})
	r.Get("/runs", h.ListRuns)
	r.Get("/runs/{runID}", h.GetRun)
	r.Post("/runs/{runID}/regenerate", h.Regenerate)
	r.Post("/runs/{runID}/unpublish", h.Unpublish)
	r.Post("/runs/{runID}/template", h.SaveTemplate)
	return r
}

func TestMarketingEmailRuns_ScopedAndListOmitsHTML(t *testing.T) {
	org := uuid.New()
	runs := &fakeEmailRuns{run: model.MarketingEmailRun{ID: uuid.New(), OrgID: org, Status: model.MarketingEmailStatusPublished, ContentHTML: "<html>big</html>", TemplateHTML: "<html>merge</html>"}}
	h := NewMarketingEmailHandler(runs)

	rec := httptest.NewRecorder()
	emailRouter(h, org).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/runs", nil))
	var list struct {
		Data []model.MarketingEmailRun `json:"data"`
	}
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &list) != nil || len(list.Data) != 1 || list.Data[0].ContentHTML != "" || list.Data[0].TemplateHTML != "" {
		t.Fatalf("list: %d %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	emailRouter(h, org).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/runs/"+runs.run.ID.String(), nil))
	var got model.MarketingEmailRun
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.ContentHTML == "" {
		t.Fatalf("get: %d %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	emailRouter(h, uuid.New()).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/runs/"+runs.run.ID.String(), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("another org read the run: %d", rec.Code)
	}
}

func TestMarketingEmailRuns_UnpublishAndBusy(t *testing.T) {
	org := uuid.New()
	runs := &fakeEmailRuns{run: model.MarketingEmailRun{ID: uuid.New(), OrgID: org, Status: model.MarketingEmailStatusPublished}}
	h := NewMarketingEmailHandler(runs)

	rec := httptest.NewRecorder()
	emailRouter(h, org).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/runs/"+runs.run.ID.String()+"/unpublish", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("unpublish: %d %s", rec.Code, rec.Body)
	}

	rec = httptest.NewRecorder()
	emailRouter(h, org).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/runs/"+runs.run.ID.String()+"/template", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("save template: %d %s", rec.Code, rec.Body)
	}

	runs.run.Status = model.MarketingEmailStatusWriting
	rec = httptest.NewRecorder()
	emailRouter(h, org).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/runs/"+runs.run.ID.String()+"/regenerate", nil))
	if rec.Code != http.StatusConflict {
		t.Fatalf("regenerate while writing: %d", rec.Code)
	}
}
