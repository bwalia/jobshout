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
	"github.com/jobshout/server/internal/service"
)

// stubTaskRunSvc is enough of TaskRunService for ListOrgRuns.
type stubTaskRunSvc struct {
	service.TaskRunService
	orgID   uuid.UUID
	status  string
	agentID *uuid.UUID
	page    *model.PaginatedResponse[model.TaskRun]
	err     error
}

func (s *stubTaskRunSvc) ListOrgRuns(_ context.Context, orgID uuid.UUID, p model.PaginationParams, status string, agentID *uuid.UUID) (*model.PaginatedResponse[model.TaskRun], error) {
	s.orgID = orgID
	s.status = status
	s.agentID = agentID
	if s.err != nil {
		return nil, s.err
	}
	if s.page != nil {
		return s.page, nil
	}
	return &model.PaginatedResponse[model.TaskRun]{
		Data:    []model.TaskRun{},
		Total:   0,
		Page:    p.Page,
		PerPage: p.PerPage,
	}, nil
}

func TestListOrgRuns_ParsesFilters(t *testing.T) {
	orgID := uuid.New()
	agentID := uuid.New()
	stub := &stubTaskRunSvc{}
	h := NewTaskRunHandler(stub)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/task-runs?status=failed&agent_id="+agentID.String()+"&per_page=25", nil)
	ctx := context.WithValue(req.Context(), middleware.ContextKeyOrgID, orgID.String())
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Get("/api/v1/task-runs", h.ListOrgRuns)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if stub.orgID != orgID {
		t.Fatalf("orgID = %v, want %v", stub.orgID, orgID)
	}
	if stub.status != "failed" {
		t.Fatalf("status = %q", stub.status)
	}
	if stub.agentID == nil || *stub.agentID != agentID {
		t.Fatalf("agentID = %v, want %v", stub.agentID, agentID)
	}
	var page model.PaginatedResponse[model.TaskRun]
	if err := json.Unmarshal(rr.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode: %v", err)
	}
}

func TestListOrgRuns_BadAgentID(t *testing.T) {
	orgID := uuid.New()
	h := NewTaskRunHandler(&stubTaskRunSvc{})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/task-runs?agent_id=not-a-uuid", nil)
	ctx := context.WithValue(req.Context(), middleware.ContextKeyOrgID, orgID.String())
	req = req.WithContext(ctx)

	rr := httptest.NewRecorder()
	r := chi.NewRouter()
	r.Get("/api/v1/task-runs", h.ListOrgRuns)
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}
