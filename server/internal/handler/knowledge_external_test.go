package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
)

func externalReq(method, body string, orgID uuid.UUID, agentID, key string) *http.Request {
	req := httptest.NewRequest(method, "/", strings.NewReader(body))
	rc := chi.NewRouteContext()
	rc.URLParams.Add("agentID", agentID)
	rc.URLParams.Add("key", key)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rc)
	ctx = context.WithValue(ctx, middleware.ContextKeyOrgID, orgID.String())
	return req.WithContext(ctx)
}

func TestExternalKnowledgeVersioning(t *testing.T) {
	org, otherOrg := uuid.New(), uuid.New()
	agent := uuid.NewString()
	store := &fakeKnowledgeStore{
		agentOrg: map[string]string{agent: org.String()},
		files: map[string]model.KnowledgeFile{
			"keep": {ID: "keep", AgentID: agent, Filename: "notes.md", Content: "unrelated"},
		},
	}
	h := NewKnowledgeHandlerWithStore(store, nil, zap.NewNop())

	put := func(o uuid.UUID, key, body string) (int, map[string]any) {
		w := httptest.NewRecorder()
		h.PutExternal(w, externalReq(http.MethodPut, body, o, agent, key))
		var out map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	versions := func(key string) []string {
		var out []string
		for _, f := range store.files {
			if k, _, ok := parseExternalFilename(f.Filename); ok && k == key {
				out = append(out, f.Filename)
			}
		}
		return out
	}

	code, out := put(org, "kb-17", `{"version":1,"title":"Valuations","content":"A valuation is not the seller's minimum.","scope":"company","source":"opsapi"}`)
	if code != http.StatusCreated || out["status"] != "created" {
		t.Fatalf("v1: %d %v", code, out)
	}
	f := store.files[out["file_id"].(string)]
	if f.Filename != "ext/kb-17@v1.md" || !strings.HasPrefix(f.Content, "[knowledge id=kb-17 version=1 scope=company source=opsapi]\n# Valuations") {
		t.Fatalf("stored file = %+v", f)
	}

	code, out = put(org, "kb-17", `{"version":1,"content":"refreshed"}`)
	if code != http.StatusOK || out["status"] != "updated" || len(versions("kb-17")) != 1 {
		t.Fatalf("same version: %d %v %v", code, out, versions("kb-17"))
	}

	code, out = put(org, "kb-17", `{"version":3,"content":"v3 text"}`)
	if code != http.StatusCreated || out["status"] != "replaced" || out["removed_versions"].(float64) != 1 {
		t.Fatalf("v3: %d %v", code, out)
	}
	if v := versions("kb-17"); len(v) != 1 || v[0] != "ext/kb-17@v3.md" {
		t.Fatalf("old version not retired: %v", v)
	}

	code, out = put(org, "kb-17", `{"version":2,"content":"late delivery"}`)
	if code != http.StatusConflict || out["current_version"].(float64) != 3 {
		t.Fatalf("stale version accepted: %d %v", code, out)
	}

	if code, _ := put(otherOrg, "kb-17", `{"version":9,"content":"x"}`); code != http.StatusNotFound {
		t.Fatalf("other org reached the agent: %d", code)
	}
	for _, bad := range []string{"../x", "", "a/b", strings.Repeat("k", 101)} {
		if code, _ := put(org, bad, `{"version":1,"content":"x"}`); code != http.StatusBadRequest {
			t.Errorf("key %q: %d", bad, code)
		}
	}
	if code, _ := put(org, "kb-1", `{"version":0,"content":"x"}`); code != http.StatusBadRequest {
		t.Errorf("version 0 accepted: %d", code)
	}

	w := httptest.NewRecorder()
	h.DeleteExternal(w, externalReq(http.MethodDelete, "", org, agent, "kb-17"))
	if w.Code != http.StatusNoContent || len(versions("kb-17")) != 0 {
		t.Fatalf("delete: %d %v", w.Code, versions("kb-17"))
	}
	if _, ok := store.files["keep"]; !ok {
		t.Fatal("an unrelated knowledge file was removed")
	}
}

func TestParseExternalFilename(t *testing.T) {
	k, v, ok := parseExternalFilename(ExternalKnowledgeFilename("rule.a-1", 12))
	if !ok || k != "rule.a-1" || v != 12 {
		t.Fatalf("round trip: %q %d %v", k, v, ok)
	}
	for _, name := range []string{"notes.md", "ext/x.md", "ext/@v1.md", "ext/x@v0.md", "ext/x@vZ.md", "ext/x@v1.txt"} {
		if _, _, ok := parseExternalFilename(name); ok {
			t.Errorf("%q parsed as external", name)
		}
	}
}
