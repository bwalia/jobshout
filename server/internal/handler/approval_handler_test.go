package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// stubApprovalSvc decides like the real service but never resumes anything.
type stubApprovalSvc struct {
	service.ApprovalService
	row       *model.Approval
	decideErr error
	decided   int
}

func (s *stubApprovalSvc) Get(context.Context, uuid.UUID) (*model.Approval, error) {
	if s.row == nil {
		return nil, service.ErrApprovalNotFound
	}
	cp := *s.row
	return &cp, nil
}

func (s *stubApprovalSvc) Decide(_ context.Context, _, _ uuid.UUID, decision, _ string) (*model.Approval, error) {
	if s.decideErr != nil {
		return nil, s.decideErr
	}
	s.decided++
	cp := *s.row
	cp.Status = model.ApprovalStatusApproved
	if decision == "reject" {
		cp.Status = model.ApprovalStatusRejected
	}
	return &cp, nil
}

func decideRequest(orgID, approvalID uuid.UUID, body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/approvals/"+approvalID.String()+"/decide", strings.NewReader(body))
	ctx := context.WithValue(req.Context(), middleware.ContextKeyOrgID, orgID.String())
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, uuid.NewString())
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("approvalID", approvalID.String())
	return req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
}

func TestApprovalDecideHandler(t *testing.T) {
	org := uuid.New()
	mine := &model.Approval{ID: uuid.New(), OrgID: org, Status: model.ApprovalStatusPending, RequestedAt: time.Now()}
	theirs := &model.Approval{ID: uuid.New(), OrgID: uuid.New(), Status: model.ApprovalStatusPending}

	cases := []struct {
		name       string
		row        *model.Approval
		decideErr  error
		body       string
		wantCode   int
		wantBody   string
		wantDecide bool
	}{
		{"approve is accepted, not awaited", mine, nil, `{"decision":"approve"}`, http.StatusAccepted, `"status":"approved"`, true},
		{"reject is accepted", mine, nil, `{"decision":"reject","reason":"no"}`, http.StatusAccepted, `"status":"rejected"`, true},
		{"double decide conflicts", mine, service.ErrApprovalAlreadyDecided, `{"decision":"approve"}`, http.StatusConflict, "already decided", false},
		{"unknown approval", nil, nil, `{"decision":"approve"}`, http.StatusNotFound, "approval not found", false},
		{"another org's approval is not found", theirs, nil, `{"decision":"approve"}`, http.StatusNotFound, "approval not found", false},
		{"bad decision", mine, nil, `{"decision":"maybe"}`, http.StatusBadRequest, "validation", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &stubApprovalSvc{row: tc.row, decideErr: tc.decideErr}
			h := NewApprovalHandler(svc)
			id := uuid.New()
			if tc.row != nil {
				id = tc.row.ID
			}
			rec := httptest.NewRecorder()
			h.Decide(rec, decideRequest(org, id, tc.body))
			if rec.Code != tc.wantCode {
				t.Fatalf("code %d, want %d (%s)", rec.Code, tc.wantCode, rec.Body.String())
			}
			if !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("body %s should contain %s", rec.Body.String(), tc.wantBody)
			}
			if (svc.decided == 1) != tc.wantDecide {
				t.Fatalf("decided %d times", svc.decided)
			}
		})
	}
}

func TestApprovalGetHandler_OrgScoped(t *testing.T) {
	org := uuid.New()
	theirs := &model.Approval{ID: uuid.New(), OrgID: uuid.New(), Status: model.ApprovalStatusPending}
	h := NewApprovalHandler(&stubApprovalSvc{row: theirs})
	rec := httptest.NewRecorder()
	h.Get(rec, decideRequest(org, theirs.ID, ""))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code %d", rec.Code)
	}
}
