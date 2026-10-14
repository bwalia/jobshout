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
	"github.com/jobshout/server/internal/service"
)

type stubWorkflowSvc struct {
	service.WorkflowService
	wf      *model.Workflow
	deleted bool
}

func (s *stubWorkflowSvc) GetByID(context.Context, uuid.UUID) (*model.Workflow, error) {
	return s.wf, nil
}

func (s *stubWorkflowSvc) Delete(context.Context, uuid.UUID) error {
	s.deleted = true
	return nil
}

type stubMultiAgentSvc struct {
	service.MultiAgentService
	job *model.MultiAgentJob
}

func (s *stubMultiAgentSvc) GetJob(context.Context, uuid.UUID) (*model.MultiAgentJob, error) {
	return s.job, nil
}

func serveAs(org uuid.UUID, method, path, pattern string, h http.HandlerFunc) int {
	r := chi.NewRouter()
	r.MethodFunc(method, pattern, h)
	req := httptest.NewRequest(method, path, nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyOrgID, org.String()))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	return rr.Code
}

// Another org's workflow or multi-agent job reads as not found, and is not
// deleted.
func TestCrossOrg_WorkflowAndJobAreNotFound(t *testing.T) {
	owner, other := uuid.New(), uuid.New()

	wf := &stubWorkflowSvc{wf: &model.Workflow{ID: uuid.New(), OrgID: owner}}
	wh := NewWorkflowHandler(wf)
	path := "/workflows/" + wf.wf.ID.String()
	if c := serveAs(other, http.MethodGet, path, "/workflows/{workflowID}", wh.GetByID); c != http.StatusNotFound {
		t.Errorf("other org GET workflow = %d, want 404", c)
	}
	if c := serveAs(other, http.MethodDelete, path, "/workflows/{workflowID}", wh.Delete); c != http.StatusNotFound || wf.deleted {
		t.Errorf("other org DELETE workflow = %d, deleted=%v", c, wf.deleted)
	}
	if c := serveAs(owner, http.MethodGet, path, "/workflows/{workflowID}", wh.GetByID); c != http.StatusOK {
		t.Errorf("owner GET workflow = %d, want 200", c)
	}

	ma := &stubMultiAgentSvc{job: &model.MultiAgentJob{ID: uuid.New(), OrgID: owner}}
	mh := NewMultiAgentHandler(ma)
	jpath := "/multi-agent/jobs/" + ma.job.ID.String()
	if c := serveAs(other, http.MethodGet, jpath, "/multi-agent/jobs/{jobID}", mh.GetJob); c != http.StatusNotFound {
		t.Errorf("other org GET job = %d, want 404", c)
	}
	if c := serveAs(owner, http.MethodGet, jpath, "/multi-agent/jobs/{jobID}", mh.GetJob); c != http.StatusOK {
		t.Errorf("owner GET job = %d, want 200", c)
	}
}
