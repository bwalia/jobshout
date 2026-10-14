package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// fakeSprintSvc stores sprints by id and records every write. AddAgent and
// AddJob refuse IDs outside `members`, the way the repository refuses an agent
// or job from another org.
type fakeSprintSvc struct {
	byID    map[uuid.UUID]*model.Sprint
	members map[uuid.UUID]bool
	writes  []string
}

func (f *fakeSprintSvc) Create(context.Context, uuid.UUID, *uuid.UUID, model.CreateSprintRequest) (*model.Sprint, error) {
	return nil, nil
}
func (f *fakeSprintSvc) Get(_ context.Context, id uuid.UUID) (*model.Sprint, error) {
	sp, ok := f.byID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	return sp, nil
}
func (f *fakeSprintSvc) GetDetail(_ context.Context, id uuid.UUID) (*model.SprintDetail, error) {
	return &model.SprintDetail{Sprint: *f.byID[id]}, nil
}
func (f *fakeSprintSvc) List(context.Context, uuid.UUID) ([]model.Sprint, error) { return nil, nil }
func (f *fakeSprintSvc) Update(_ context.Context, id uuid.UUID, _ model.UpdateSprintRequest) (*model.Sprint, error) {
	f.writes = append(f.writes, "update")
	return f.byID[id], nil
}
func (f *fakeSprintSvc) Delete(context.Context, uuid.UUID) error {
	f.writes = append(f.writes, "delete")
	return nil
}
func (f *fakeSprintSvc) AddJob(_ context.Context, _ uuid.UUID, req model.AddSprintJobRequest) error {
	if !f.members[req.JobID] {
		return repository.ErrSprintMemberNotFound
	}
	f.writes = append(f.writes, "add-job")
	return nil
}
func (f *fakeSprintSvc) RemoveJob(context.Context, uuid.UUID, uuid.UUID) error {
	f.writes = append(f.writes, "remove-job")
	return nil
}
func (f *fakeSprintSvc) AddAgent(_ context.Context, _ uuid.UUID, req model.AddSprintAgentRequest) error {
	if !f.members[req.AgentID] {
		return repository.ErrSprintMemberNotFound
	}
	f.writes = append(f.writes, "add-agent")
	return nil
}
func (f *fakeSprintSvc) RemoveAgent(context.Context, uuid.UUID, uuid.UUID, string) error {
	f.writes = append(f.writes, "remove-agent")
	return nil
}

func sprintReq(body string, orgID uuid.UUID, params map[string]string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rc := chi.NewRouteContext()
	for k, v := range params {
		rc.URLParams.Add(k, v)
	}
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rc)
	ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID.String())
	return req.WithContext(ctx)
}

func TestSprintHandler_OrgIsolation(t *testing.T) {
	orgA, orgB := uuid.New(), uuid.New()
	spB, agentB, jobB := uuid.New(), uuid.New(), uuid.New()
	svc := &fakeSprintSvc{
		byID:    map[uuid.UUID]*model.Sprint{spB: {ID: spB, OrgID: orgB}},
		members: map[uuid.UUID]bool{agentB: true, jobB: true},
	}
	h := NewSprintHandler(svc)
	params := map[string]string{"sprintID": spB.String(), "agentID": agentB.String(), "jobID": jobB.String()}
	body := `{"agent_id":"` + agentB.String() + `","job_id":"` + jobB.String() + `","status":"active"}`

	routes := map[string]http.HandlerFunc{
		"get": h.Get, "update": h.Update, "delete": h.Delete,
		"add-job": h.AddJob, "remove-job": h.RemoveJob,
		"add-agent": h.AddAgent, "remove-agent": h.RemoveAgent,
	}
	for name, fn := range routes {
		rec := httptest.NewRecorder()
		fn(rec, sprintReq(body, orgA, params))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s on another org's sprint = %d, want 404", name, rec.Code)
		}
	}
	if len(svc.writes) != 0 {
		t.Fatalf("another org's sprint was changed: %v", svc.writes)
	}

	// The owning org still gets through.
	rec := httptest.NewRecorder()
	h.Get(rec, sprintReq("", orgB, params))
	if rec.Code != http.StatusOK {
		t.Fatalf("own get = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.AddAgent(rec, sprintReq(body, orgB, params))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("own add-agent = %d %s", rec.Code, rec.Body.String())
	}
}

func TestSprintHandler_RefusesForeignMembers(t *testing.T) {
	org, sp := uuid.New(), uuid.New()
	svc := &fakeSprintSvc{
		byID:    map[uuid.UUID]*model.Sprint{sp: {ID: sp, OrgID: org}},
		members: map[uuid.UUID]bool{},
	}
	h := NewSprintHandler(svc)
	foreign := uuid.New().String()
	params := map[string]string{"sprintID": sp.String()}

	rec := httptest.NewRecorder()
	h.AddAgent(rec, sprintReq(`{"agent_id":"`+foreign+`","role_label":"planner"}`, org, params))
	if rec.Code != http.StatusNotFound {
		t.Errorf("add foreign agent = %d, want 404", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.AddJob(rec, sprintReq(`{"job_id":"`+foreign+`"}`, org, params))
	if rec.Code != http.StatusNotFound {
		t.Errorf("add foreign job = %d, want 404", rec.Code)
	}
}
