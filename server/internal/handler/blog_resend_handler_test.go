package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// resendBlogSvc answers ResendArticle with a fixed result.
type resendBlogSvc struct {
	service.BlogService
	err       error
	gotOrg    uuid.UUID
	gotArticl uuid.UUID
}

func (s *resendBlogSvc) ResendArticle(_ context.Context, orgID, articleID uuid.UUID) (*model.BlogArticle, error) {
	s.gotOrg, s.gotArticl = orgID, articleID
	if s.err != nil {
		return nil, s.err
	}
	uuidStr := "post-1"
	return &model.BlogArticle{ID: articleID, OrgID: orgID, PostUUID: &uuidStr,
		LastResend: &model.ArticleResend{Result: model.ResendResultUpdated, PostUUID: uuidStr}}, nil
}

func resendRequest(t *testing.T, orgID, articleID string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/blogs/articles/"+articleID+"/resend", nil)
	ctx := context.WithValue(req.Context(), middleware.ContextKeyOrgID, orgID)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("articleID", articleID)
	return req.WithContext(context.WithValue(ctx, chi.RouteCtxKey, rctx))
}

func TestBlogHandler_ResendArticle_StatusCodes(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"ok", nil, http.StatusOK},
		{"not found", service.ErrArticleNotFound, http.StatusNotFound},
		{"other org", service.ErrArticleOtherOrg, http.StatusForbidden},
		{"in flight", service.ErrResendInProgress, http.StatusConflict},
		{"not configured", service.ErrResendNotConfigured, http.StatusServiceUnavailable},
		{"no content", service.ErrResendNoContent, http.StatusBadRequest},
		{"cms refused", errors.New("blog_svc: resend to OpsAPI: opsapi: cms update forbidden"), http.StatusBadGateway},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := &resendBlogSvc{err: tc.err}
			h := NewBlogHandler(svc)
			org, art := uuid.New(), uuid.New()
			w := httptest.NewRecorder()
			h.ResendArticle(w, resendRequest(t, org.String(), art.String()))
			if w.Code != tc.want {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tc.want, w.Body.String())
			}
			if svc.gotOrg != org || svc.gotArticl != art {
				t.Fatalf("service got org=%s article=%s", svc.gotOrg, svc.gotArticl)
			}
		})
	}
}

func TestBlogHandler_ResendArticle_BadInput(t *testing.T) {
	h := NewBlogHandler(&resendBlogSvc{})
	w := httptest.NewRecorder()
	h.ResendArticle(w, resendRequest(t, uuid.NewString(), "not-a-uuid"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad article id: status = %d", w.Code)
	}
	w = httptest.NewRecorder()
	h.ResendArticle(w, resendRequest(t, "", uuid.NewString()))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("no org: status = %d", w.Code)
	}
}
