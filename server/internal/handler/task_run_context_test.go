package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

type captureRuns struct {
	service.TaskRunService
	req model.CreateTaskRunRequest
	ctx context.Context
}

func (c *captureRuns) CreateRun(ctx context.Context, taskID uuid.UUID, req model.CreateTaskRunRequest, orgID uuid.UUID, _ *uuid.UUID) (*model.TaskRun, error) {
	c.req, c.ctx = req, ctx
	return &model.TaskRun{ID: uuid.New(), TaskID: taskID, OrgID: orgID, Context: req.Context}, nil
}

func TestCreateRun_RoleContextAndSkills(t *testing.T) {
	svc := &captureRuns{}
	h := NewTaskRunHandler(svc)
	body := `{"prompt":"Reply to the buyer","skill_slugs":["Seller-Voice"],
		"context":{"role":"seller_agent","user_ref":"opsapi-user-7","workspace_ref":"property-42"},
		"inputs":{"listing":"L1"}}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rc := chi.NewRouteContext()
	rc.URLParams.Add("taskID", uuid.NewString())
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rc)
	ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, uuid.NewString())
	rec := httptest.NewRecorder()
	h.CreateRun(rec, req.WithContext(ctx))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("code = %d %s", rec.Code, rec.Body.String())
	}
	if svc.req.Inputs["role"] != "seller_agent" || svc.req.Inputs["workspace_ref"] != "property-42" || svc.req.Inputs["listing"] != "L1" {
		t.Fatalf("inputs (rendered into the prompt) = %v", svc.req.Inputs)
	}
	if got := model.RunContextFrom(svc.ctx); got == nil || got.UserRef != "opsapi-user-7" {
		t.Fatalf("run context not carried to run creation: %+v", got)
	}
	if len(svc.req.SkillSlugs) != 1 || svc.req.SkillSlugs[0] != "Seller-Voice" {
		t.Fatalf("skill slugs = %v", svc.req.SkillSlugs)
	}
	if !strings.Contains(rec.Body.String(), `"role":"seller_agent"`) {
		t.Fatalf("response lacks context: %s", rec.Body.String())
	}
}
