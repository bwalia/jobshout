package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
)

// fakeApprovalSvc is an in-memory service.ApprovalService holding approvals of
// several orgs, so the handler's org checks can be exercised without a DB.
type fakeApprovalSvc struct {
	byID    map[uuid.UUID]*model.Approval
	decided []uuid.UUID
}

func (f *fakeApprovalSvc) RequiresApproval(uuid.UUID, string) bool { return false }
func (f *fakeApprovalSvc) CreatePending(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, string, map[string]any, []byte) (uuid.UUID, error) {
	return uuid.Nil, nil
}
func (f *fakeApprovalSvc) List(_ context.Context, orgID uuid.UUID, _ string) ([]model.Approval, error) {
	out := []model.Approval{}
	for _, a := range f.byID {
		if a.OrgID == orgID {
			out = append(out, *a)
		}
	}
	return out, nil
}
func (f *fakeApprovalSvc) Get(_ context.Context, id uuid.UUID) (*model.Approval, error) {
	a, ok := f.byID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *a
	return &cp, nil
}
func (f *fakeApprovalSvc) Decide(_ context.Context, id, _ uuid.UUID, decision, _ string) (*model.Approval, error) {
	f.decided = append(f.decided, id)
	a := f.byID[id]
	a.Status = model.ApprovalStatusApproved
	if decision == "reject" {
		a.Status = model.ApprovalStatusRejected
	}
	cp := *a
	return &cp, nil
}

// asOrg returns req with the auth context of a user in orgID, plus the
// approvalID chi URL param when id is non-nil.
func asOrg(req *http.Request, orgID uuid.UUID, id *uuid.UUID) *http.Request {
	ctx := context.WithValue(req.Context(), middleware.ContextKeyOrgID, orgID.String())
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, uuid.NewString())
	if id != nil {
		rc := chi.NewRouteContext()
		rc.URLParams.Add("approvalID", id.String())
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rc)
	}
	return req.WithContext(ctx)
}

func TestApprovalHandler_OrgIsolation(t *testing.T) {
	orgA, orgB := uuid.New(), uuid.New()
	aID, bID := uuid.New(), uuid.New()
	svc := &fakeApprovalSvc{byID: map[uuid.UUID]*model.Approval{
		aID: {ID: aID, OrgID: orgA, Status: model.ApprovalStatusPending, ToolName: "send_email"},
		bID: {ID: bID, OrgID: orgB, Status: model.ApprovalStatusPending, ToolName: "send_email"},
	}}
	h := NewApprovalHandler(svc)

	// List is org-scoped: org A sees only its own approval.
	rec := httptest.NewRecorder()
	h.List(rec, asOrg(httptest.NewRequest(http.MethodGet, "/approvals", nil), orgA, nil))
	var list []model.Approval
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != aID {
		t.Fatalf("org A list = %+v, want only %s", list, aID)
	}

	// Get: own approval 200, other org's approval 404.
	rec = httptest.NewRecorder()
	h.Get(rec, asOrg(httptest.NewRequest(http.MethodGet, "/", nil), orgA, &aID))
	if rec.Code != http.StatusOK {
		t.Fatalf("get own = %d, want 200", rec.Code)
	}
	rec = httptest.NewRecorder()
	h.Get(rec, asOrg(httptest.NewRequest(http.MethodGet, "/", nil), orgA, &bID))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get other org = %d, want 404", rec.Code)
	}

	// Decide: other org's approval is 404 and never reaches the service.
	rec = httptest.NewRecorder()
	h.Decide(rec, asOrg(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"decision":"approve"}`)), orgA, &bID))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("decide other org = %d, want 404", rec.Code)
	}
	if len(svc.decided) != 0 {
		t.Fatalf("service Decide called for a foreign approval: %v", svc.decided)
	}
	if svc.byID[bID].Status != model.ApprovalStatusPending {
		t.Fatal("foreign approval was decided")
	}

	// Decide own: 200.
	rec = httptest.NewRecorder()
	h.Decide(rec, asOrg(httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"decision":"reject"}`)), orgA, &aID))
	if rec.Code != http.StatusOK {
		t.Fatalf("decide own = %d (%s), want 200", rec.Code, rec.Body.String())
	}
}
