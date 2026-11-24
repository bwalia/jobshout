package handler

import (
	"context"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

type stubBlogRunSvc struct {
	service.BlogService
	run *model.BlogRun
}

func (s *stubBlogRunSvc) GetByID(context.Context, uuid.UUID) (*model.BlogRun, error) {
	return s.run, nil
}

// API keys with the runs scope may read a blog run, so the handler itself must
// keep it in its org: another org's run reads as not found.
func TestBlogGetRun_OrgScoped(t *testing.T) {
	owner, other := uuid.New(), uuid.New()
	svc := &stubBlogRunSvc{run: &model.BlogRun{ID: uuid.New(), OrgID: owner, Status: model.BlogRunStatusRunning}}
	h := NewBlogHandler(svc)
	path := "/blogs/runs/" + svc.run.ID.String()
	if c := serveAs(other, http.MethodGet, path, "/blogs/runs/{runID}", h.GetRun); c != http.StatusNotFound {
		t.Errorf("other org GET blog run = %d, want 404", c)
	}
	if c := serveAs(owner, http.MethodGet, path, "/blogs/runs/{runID}", h.GetRun); c != http.StatusOK {
		t.Errorf("owner GET blog run = %d, want 200", c)
	}
	if c := serveAs(owner, http.MethodGet, "/blogs/runs/not-a-uuid", "/blogs/runs/{runID}", h.GetRun); c != http.StatusBadRequest {
		t.Errorf("bad id = %d, want 400", c)
	}
}
