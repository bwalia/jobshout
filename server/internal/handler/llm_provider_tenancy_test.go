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
)

type fakeProviderRepo struct {
	byID    map[uuid.UUID]*model.LLMProviderConfig
	updated []uuid.UUID
	deleted []uuid.UUID
}

func (f *fakeProviderRepo) Create(_ context.Context, p *model.LLMProviderConfig) error {
	f.byID[p.ID] = p
	return nil
}
func (f *fakeProviderRepo) GetByID(_ context.Context, id uuid.UUID) (*model.LLMProviderConfig, error) {
	p, ok := f.byID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *p
	return &cp, nil
}
func (f *fakeProviderRepo) List(context.Context, uuid.UUID) ([]model.LLMProviderConfig, error) {
	return nil, nil
}
func (f *fakeProviderRepo) Update(_ context.Context, id uuid.UUID, _ model.UpdateLLMProviderRequest) (*model.LLMProviderConfig, error) {
	f.updated = append(f.updated, id)
	cp := *f.byID[id]
	return &cp, nil
}
func (f *fakeProviderRepo) Delete(_ context.Context, id uuid.UUID) error {
	f.deleted = append(f.deleted, id)
	return nil
}
func (f *fakeProviderRepo) ClearDefault(context.Context, uuid.UUID) error { return nil }

func providerReq(method, body string, orgID, id uuid.UUID) *http.Request {
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	rc := chi.NewRouteContext()
	rc.URLParams.Add("providerID", id.String())
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rc)
	ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID.String())
	return req.WithContext(ctx)
}

func TestLLMProviderHandler_OrgIsolation(t *testing.T) {
	orgA, orgB := uuid.New(), uuid.New()
	pB := uuid.New()
	repo := &fakeProviderRepo{byID: map[uuid.UUID]*model.LLMProviderConfig{
		pB: {ID: pB, OrgID: orgB, APIKey: strings.Repeat("k", 20)},
	}}
	h := NewLLMProviderHandler(repo, nil, false)

	for name, fn := range map[string]http.HandlerFunc{"get": h.GetByID, "update": h.Update, "delete": h.Delete} {
		rec := httptest.NewRecorder()
		fn(rec, providerReq(http.MethodPut, `{"name":"x"}`, orgA, pB))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s foreign provider = %d, want 404", name, rec.Code)
		}
	}
	if len(repo.updated)+len(repo.deleted) != 0 {
		t.Fatal("foreign provider was modified")
	}
	rec := httptest.NewRecorder()
	h.GetByID(rec, providerReq(http.MethodGet, "", orgB, pB))
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), strings.Repeat("k", 20)) {
		t.Fatalf("own get = %d %s", rec.Code, rec.Body.String())
	}
}
