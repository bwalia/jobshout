package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

type fakeJWT struct{ claims *service.Claims }

func (f fakeJWT) GenerateAccessToken(uuid.UUID, string, *uuid.UUID, string) (string, error) {
	return "", nil
}
func (f fakeJWT) GenerateRefreshToken() (string, string, time.Time) { return "", "", time.Time{} }
func (f fakeJWT) ParseAccessToken(tok string) (*service.Claims, error) {
	if tok != "good.jwt.token" {
		return nil, errors.New("bad")
	}
	return f.claims, nil
}
func (f fakeJWT) HashToken(string) string { return "" }

type fakeKeys map[string]*model.APIKeyPrincipal

func (f fakeKeys) Authenticate(_ context.Context, raw string) (*model.APIKeyPrincipal, error) {
	if p, ok := f[raw]; ok {
		return p, nil
	}
	return nil, errors.New("invalid")
}

func TestRequireAuthOrAPIKey(t *testing.T) {
	org, user, keyID := uuid.New(), uuid.New(), uuid.New()
	runsKey := "jsk_00000000_runs"
	allKey := "jsk_00000000_all"
	keys := fakeKeys{
		runsKey: {KeyID: keyID, OrgID: org, UserID: user, Scopes: []string{model.ScopeRuns}},
		allKey:  {KeyID: keyID, OrgID: org, UserID: user, Scopes: model.AllAPIKeyScopes},
	}
	jwtUser := uuid.NewString()
	mw := RequireAuthOrAPIKey(fakeJWT{claims: &service.Claims{UserID: jwtUser, OrgID: org.String(), Role: "admin"}}, keys)

	var seen context.Context
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.Context()
		w.WriteHeader(http.StatusOK)
	}))
	call := func(method, path, token string) int {
		seen = nil
		req := httptest.NewRequest(method, path, nil)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	id := uuid.NewString()

	// JWT keeps working, on any route.
	if c := call("GET", "/api/v1/llm-routing", "good.jwt.token"); c != 200 || GetUserID(seen) != jwtUser || IsAPIKey(seen) {
		t.Fatalf("jwt: code=%d", c)
	}
	if c := call("GET", "/api/v1/agents", ""); c != 401 {
		t.Fatalf("no token = %d", c)
	}
	if c := call("GET", "/api/v1/agents", "jsk_unknown_key_value"); c != 401 {
		t.Fatalf("unknown key = %d", c)
	}

	// Allowed route with the right scope: acts as the key's org + creator.
	if c := call("POST", "/api/v1/tasks/"+id+"/run", runsKey); c != 200 {
		t.Fatalf("runs key on run = %d", c)
	}
	if GetOrgID(seen) != org.String() || GetUserID(seen) != user.String() || !IsAPIKey(seen) ||
		GetAPIKeyID(seen) != keyID.String() || seen.Value(ContextKeyRole) != RoleService {
		t.Fatal("api key principal not placed on the context")
	}
	// Right route, wrong scope.
	if c := call("GET", "/api/v1/approvals", runsKey); c != 403 {
		t.Fatalf("runs key on approvals = %d, want 403", c)
	}
	// Never-allowed routes, even with every scope.
	for _, p := range [][2]string{
		{"POST", "/api/v1/api-keys"},
		{"GET", "/api/v1/api-keys"},
		{"PUT", "/api/v1/llm-routing"},
		{"DELETE", "/api/v1/agents/" + id},
		{"PUT", "/api/v1/agents/" + id},
		{"GET", "/api/v1/tasks/" + id},
		{"GET", "/api/v1/webhook-endpoints"},
	} {
		if c := call(p[0], p[1], allKey); c != 403 {
			t.Errorf("%s %s with all scopes = %d, want 403", p[0], p[1], c)
		}
	}
}

func TestAPIKeyRouteScope_OpsAPIRoutes(t *testing.T) {
	id, id2 := uuid.NewString(), uuid.NewString()
	want := map[[2]string]string{
		{"POST", "/api/v1/tasks/launch"}:                                 model.ScopeRuns,
		{"POST", "/api/v1/tasks/" + id + "/run"}:                         model.ScopeRuns,
		{"GET", "/api/v1/tasks/" + id + "/runs"}:                         model.ScopeRuns,
		{"GET", "/api/v1/task-runs/" + id}:                               model.ScopeRuns,
		{"GET", "/api/v1/approvals"}:                                     model.ScopeApprovals,
		{"GET", "/api/v1/approvals/"}:                                    model.ScopeApprovals,
		{"GET", "/api/v1/approvals/" + id}:                               model.ScopeApprovals,
		{"POST", "/api/v1/approvals/" + id + "/decide"}:                  model.ScopeApprovals,
		{"GET", "/api/v1/agents"}:                                        model.ScopeAgentsRead,
		{"GET", "/api/v1/agents/" + id}:                                  model.ScopeAgentsRead,
		{"GET", "/api/v1/agents/" + id + "/knowledge"}:                   model.ScopeKnowledge,
		{"POST", "/api/v1/agents/" + id + "/knowledge"}:                  model.ScopeKnowledge,
		{"GET", "/api/v1/agents/" + id + "/knowledge/" + id2}:            model.ScopeKnowledge,
		{"PUT", "/api/v1/agents/" + id + "/knowledge/" + id2}:            model.ScopeKnowledge,
		{"DELETE", "/api/v1/agents/" + id + "/knowledge/" + id2}:         model.ScopeKnowledge,
		{"PUT", "/api/v1/agents/" + id + "/knowledge/external/kb-17"}:    model.ScopeKnowledge,
		{"DELETE", "/api/v1/agents/" + id + "/knowledge/external/kb-17"}: model.ScopeKnowledge,
		{"GET", "/api/v1/property-pa/agents"}:                            model.ScopeAgentsRead,
	}
	for k, scope := range want {
		got, ok := APIKeyRouteScope(k[0], k[1])
		if !ok || got != scope {
			t.Errorf("%s %s = %q,%v want %q", k[0], k[1], got, ok, scope)
		}
	}
	if _, ok := APIKeyRouteScope("GET", "/api/v1/tasks/launch/../../api-keys"); ok {
		t.Error("path traversal-looking path matched")
	}
	if _, ok := APIKeyRouteScope("POST", "/api/v1/property-pa/install"); ok {
		t.Error("an API key may not install agents")
	}
	if _, ok := APIKeyRouteScope("PUT", "/api/v1/agents/"+id+"/knowledge/external/../x"); ok {
		t.Error("dot-segment external key matched")
	}
	if _, ok := APIKeyRouteScope("POST", "/api/v1/approvals/not-a-uuid/decide"); ok {
		t.Error("non-uuid id matched")
	}
}
