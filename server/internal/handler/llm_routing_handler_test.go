package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/orgroute"
)

type memRouting struct{ rows map[uuid.UUID]*model.OrgLLMRouting }

func (m *memRouting) Get(_ context.Context, orgID uuid.UUID) (*model.OrgLLMRouting, error) {
	return m.rows[orgID], nil
}
func (m *memRouting) Upsert(_ context.Context, r *model.OrgLLMRouting) error {
	m.rows[r.OrgID] = r
	return nil
}

type memProviders struct{ byOrg map[uuid.UUID][]model.LLMProviderConfig }

func (m *memProviders) ListActiveWithSecrets(_ context.Context, orgID uuid.UUID) ([]model.LLMProviderConfig, error) {
	return m.byOrg[orgID], nil
}

type nopLLM struct{}

func (nopLLM) Generate(context.Context, llm.GenerateRequest) (*llm.GenerateResponse, error) {
	return &llm.GenerateResponse{}, nil
}
func (nopLLM) ProviderName() string { return "x" }

func TestLLMRoutingHandler_PutValidatesChainAgainstOrg(t *testing.T) {
	orgA, orgB := uuid.New(), uuid.New()
	pa := model.LLMProviderConfig{ID: uuid.New(), OrgID: orgA, Name: "a", ProviderType: "openai", DefaultModel: "m", IsActive: true}
	pb := model.LLMProviderConfig{ID: uuid.New(), OrgID: orgB, Name: "b", ProviderType: "openai", DefaultModel: "m", IsActive: true}
	providers := &memProviders{byOrg: map[uuid.UUID][]model.LLMProviderConfig{orgA: {pa}, orgB: {pb}}}
	routing := &memRouting{rows: map[uuid.UUID]*model.OrgLLMRouting{}}
	res := orgroute.NewResolver(orgroute.Stores{Providers: providers, Routing: routing}, orgroute.Options{
		Build: func(model.LLMProviderConfig) (llm.Client, error) { return nopLLM{}, nil },
	})
	h := NewLLMRoutingHandler(routing, providers, res)

	put := func(org uuid.UUID, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/", strings.NewReader(body))
		req = req.WithContext(context.WithValue(req.Context(), middleware.ContextKeyOrgID, org.String()))
		rec := httptest.NewRecorder()
		h.Put(rec, req)
		return rec
	}

	// Org A cannot put org B's config in its chain.
	if rec := put(orgA, `{"chain":[{"provider_config_id":"`+pb.ID.String()+`"}]}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("foreign config in chain = %d, want 400", rec.Code)
	}
	if routing.rows[orgA] != nil {
		t.Fatal("rejected chain was saved")
	}

	rec := put(orgA, `{"chain":[{"provider_config_id":"`+pa.ID.String()+`","model":"gpt-x"}]}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("own chain = %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		Source    string          `json:"source"`
		Effective []orgroute.Step `json:"effective"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Source != "org" || len(out.Effective) != 1 || out.Effective[0].Model != "gpt-x" {
		t.Fatalf("response = %s", rec.Body.String())
	}

	// Disabling keeps the chain but routes to env.
	rec = put(orgA, `{"enabled":false}`)
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.Source != "env" || len(routing.rows[orgA].Chain) != 1 {
		t.Fatalf("disabled response = %s", rec.Body.String())
	}
}
