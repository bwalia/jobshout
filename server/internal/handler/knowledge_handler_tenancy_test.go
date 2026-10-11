package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// fakeKnowledgeStore mirrors the Postgres store's ownership rule: a file is
// visible only through its own agent, and only to that agent's org.
type fakeKnowledgeStore struct {
	agentOrg map[string]string              // agentID -> orgID
	files    map[string]model.KnowledgeFile // fileID -> file
}

func (f *fakeKnowledgeStore) owned(orgID, agentID, fileID string) bool {
	file, ok := f.files[fileID]
	return ok && file.AgentID == agentID && f.agentOrg[agentID] == orgID
}
func (f *fakeKnowledgeStore) AgentInOrg(_ context.Context, orgID, agentID string) (bool, error) {
	return f.agentOrg[agentID] == orgID, nil
}
func (f *fakeKnowledgeStore) ListByAgent(_ context.Context, orgID, agentID string) ([]model.KnowledgeFile, error) {
	out := []model.KnowledgeFile{}
	for id, file := range f.files {
		if f.owned(orgID, agentID, id) {
			out = append(out, file)
		}
	}
	return out, nil
}
func (f *fakeKnowledgeStore) Get(_ context.Context, orgID, agentID, fileID string) (*model.KnowledgeFile, error) {
	if !f.owned(orgID, agentID, fileID) {
		return nil, repository.ErrKnowledgeNotFound
	}
	file := f.files[fileID]
	return &file, nil
}
func (f *fakeKnowledgeStore) Create(_ context.Context, agentID, filename, content string) (string, error) {
	id := uuid.NewString()
	f.files[id] = model.KnowledgeFile{ID: id, AgentID: agentID, Filename: filename, Content: content}
	return id, nil
}
func (f *fakeKnowledgeStore) UpdateContent(_ context.Context, orgID, agentID, fileID, content string) error {
	if !f.owned(orgID, agentID, fileID) {
		return repository.ErrKnowledgeNotFound
	}
	file := f.files[fileID]
	file.Content = content
	f.files[fileID] = file
	return nil
}
func (f *fakeKnowledgeStore) Delete(_ context.Context, orgID, agentID, fileID string) error {
	if !f.owned(orgID, agentID, fileID) {
		return repository.ErrKnowledgeNotFound
	}
	delete(f.files, fileID)
	return nil
}

func knowledgeReq(method, body string, orgID uuid.UUID, agentID, fileID string) *http.Request {
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	rc := chi.NewRouteContext()
	rc.URLParams.Add("agentID", agentID)
	if fileID != "" {
		rc.URLParams.Add("fileID", fileID)
	}
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rc)
	ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID.String())
	return req.WithContext(ctx)
}

func TestKnowledgeHandler_OrgIsolation(t *testing.T) {
	orgA, orgB := uuid.New(), uuid.New()
	agentA, agentB := uuid.NewString(), uuid.NewString()
	fileB := uuid.NewString()
	store := &fakeKnowledgeStore{
		agentOrg: map[string]string{agentA: orgA.String(), agentB: orgB.String()},
		files:    map[string]model.KnowledgeFile{fileB: {ID: fileB, AgentID: agentB, Content: "B"}},
	}
	h := NewKnowledgeHandlerWithStore(store, nil, zap.NewNop())

	cases := []struct {
		name string
		fn   http.HandlerFunc
		req  *http.Request
		want int
	}{
		{"list foreign agent", h.ListByAgent, knowledgeReq(http.MethodGet, "", orgA, agentB, ""), http.StatusNotFound},
		{"create on foreign agent", h.CreateFile, knowledgeReq(http.MethodPost, `{"filename":"x","content":"y"}`, orgA, agentB, ""), http.StatusNotFound},
		{"get foreign file", h.GetFile, knowledgeReq(http.MethodGet, "", orgA, agentB, fileB), http.StatusNotFound},
		{"get foreign file via own agent", h.GetFile, knowledgeReq(http.MethodGet, "", orgA, agentA, fileB), http.StatusNotFound},
		{"update foreign file", h.UpdateFile, knowledgeReq(http.MethodPut, `{"content":"pwned"}`, orgA, agentB, fileB), http.StatusNotFound},
		{"delete foreign file", h.DeleteFile, knowledgeReq(http.MethodDelete, "", orgA, agentA, fileB), http.StatusNotFound},
		{"bad file id", h.GetFile, knowledgeReq(http.MethodGet, "", orgA, agentA, "nope"), http.StatusBadRequest},
		{"list own agent", h.ListByAgent, knowledgeReq(http.MethodGet, "", orgB, agentB, ""), http.StatusOK},
		{"get own file", h.GetFile, knowledgeReq(http.MethodGet, "", orgB, agentB, fileB), http.StatusOK},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		tc.fn(rec, tc.req)
		if rec.Code != tc.want {
			t.Errorf("%s: status = %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body.String())
		}
	}
	if store.files[fileB].Content != "B" {
		t.Fatal("foreign file was modified")
	}
	if len(store.files) != 1 {
		t.Fatalf("files = %d, want 1 (no foreign create/delete)", len(store.files))
	}
}
