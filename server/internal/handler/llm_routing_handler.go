package handler

import (
	"context"
	"net/http"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/orgroute"
)

// LLMRoutingHandler exposes an org's model-routing settings:
//
//	GET /api/v1/llm-routing  — settings + the effective chain
//	PUT /api/v1/llm-routing  — set enabled and/or the ordered chain
type LLMRoutingHandler struct {
	repo interface {
		Get(ctx context.Context, orgID uuid.UUID) (*model.OrgLLMRouting, error)
		Upsert(ctx context.Context, r *model.OrgLLMRouting) error
	}
	providers interface {
		ListActiveWithSecrets(ctx context.Context, orgID uuid.UUID) ([]model.LLMProviderConfig, error)
	}
	resolver *orgroute.Resolver
	validate *validator.Validate
}

// NewLLMRoutingHandler builds the handler. resolver may be nil (no cache to
// invalidate, no effective chain reported).
func NewLLMRoutingHandler(
	repo interface {
		Get(ctx context.Context, orgID uuid.UUID) (*model.OrgLLMRouting, error)
		Upsert(ctx context.Context, r *model.OrgLLMRouting) error
	},
	providers interface {
		ListActiveWithSecrets(ctx context.Context, orgID uuid.UUID) ([]model.LLMProviderConfig, error)
	},
	resolver *orgroute.Resolver,
) *LLMRoutingHandler {
	return &LLMRoutingHandler{repo: repo, providers: providers, resolver: resolver, validate: validator.New()}
}

// llmRoutingResponse is the GET/PUT payload.
type llmRoutingResponse struct {
	// Source is "org" when this org's calls go through its own chain, "env"
	// when they use the server's env providers.
	Source    string               `json:"source"`
	Enabled   bool                 `json:"enabled"`
	Chain     []model.LLMRouteStep `json:"chain"`
	Effective []orgroute.Step      `json:"effective"`
}

func (h *LLMRoutingHandler) respond(w http.ResponseWriter, r *http.Request, orgID uuid.UUID, status int) {
	routing, err := h.repo.Get(r.Context(), orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to load routing")
		return
	}
	out := llmRoutingResponse{Source: "env", Enabled: true, Chain: []model.LLMRouteStep{}, Effective: []orgroute.Step{}}
	if routing != nil {
		out.Enabled = routing.Enabled
		out.Chain = routing.Chain
	}
	if h.resolver != nil {
		h.resolver.Invalidate(orgID)
		steps, err := h.resolver.Chain(r.Context(), orgID)
		if err == nil && len(steps) > 0 {
			out.Source = "org"
			out.Effective = steps
		}
	}
	RespondJSON(w, status, out)
}

// Get returns the org's routing settings and effective chain.
func (h *LLMRoutingHandler) Get(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}
	h.respond(w, r, orgID, http.StatusOK)
}

// Put replaces the org's routing settings. Every chain hop must name an
// active provider config of this org.
func (h *LLMRoutingHandler) Put(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id")
		return
	}
	var req model.PutOrgLLMRoutingRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation: "+err.Error())
		return
	}

	existing, err := h.repo.Get(r.Context(), orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to load routing")
		return
	}
	next := &model.OrgLLMRouting{OrgID: orgID, Enabled: true, Chain: []model.LLMRouteStep{}}
	if existing != nil {
		next.Enabled, next.Chain = existing.Enabled, existing.Chain
	}
	if req.Enabled != nil {
		next.Enabled = *req.Enabled
	}
	if req.Chain != nil {
		active, err := h.providers.ListActiveWithSecrets(r.Context(), orgID)
		if err != nil {
			RespondError(w, http.StatusInternalServerError, "failed to load provider configs")
			return
		}
		mine := make(map[uuid.UUID]bool, len(active))
		for _, p := range active {
			mine[p.ID] = true
		}
		seen := map[uuid.UUID]bool{}
		for _, s := range req.Chain {
			if !mine[s.ProviderConfigID] {
				// Another org's config is indistinguishable from a missing one.
				RespondError(w, http.StatusBadRequest, "chain: provider config "+s.ProviderConfigID.String()+" not found or inactive")
				return
			}
			if seen[s.ProviderConfigID] && s.Model == "" {
				RespondError(w, http.StatusBadRequest, "chain: provider config "+s.ProviderConfigID.String()+" repeated without a different model")
				return
			}
			seen[s.ProviderConfigID] = true
		}
		next.Chain = req.Chain
	}
	if uid, err := uuid.Parse(middleware.GetUserID(r.Context())); err == nil {
		next.UpdatedBy = &uid
	}
	if err := h.repo.Upsert(r.Context(), next); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to save routing")
		return
	}
	h.respond(w, r, orgID, http.StatusOK)
}
