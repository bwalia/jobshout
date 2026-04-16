package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// GovernanceHandler exposes budget and policy management endpoints.
type GovernanceHandler struct {
	svc      service.GovernanceService
	validate *validator.Validate
}

// NewGovernanceHandler creates a GovernanceHandler.
func NewGovernanceHandler(svc service.GovernanceService) *GovernanceHandler {
	return &GovernanceHandler{svc: svc, validate: validator.New()}
}

// ─── Budgets ────────────────────────────────────────────────────────────────

// ListBudgets handles GET /governance/budgets
func (h *GovernanceHandler) ListBudgets(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	budgets, err := h.svc.ListBudgets(r.Context(), orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list budgets")
		return
	}
	if budgets == nil {
		budgets = []model.OrgBudget{}
	}
	RespondJSON(w, http.StatusOK, budgets)
}

// UpsertBudget handles POST /governance/budgets
func (h *GovernanceHandler) UpsertBudget(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	var req model.CreateBudgetRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	budget, err := h.svc.UpsertBudget(r.Context(), orgID, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to upsert budget: "+err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, budget)
}

// DeleteBudget handles DELETE /governance/budgets/{budgetID}
func (h *GovernanceHandler) DeleteBudget(w http.ResponseWriter, r *http.Request) {
	budgetID, err := uuid.Parse(chi.URLParam(r, "budgetID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid budget ID")
		return
	}

	if err := h.svc.DeleteBudget(r.Context(), budgetID); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to delete budget")
		return
	}
	RespondJSON(w, http.StatusNoContent, nil)
}

// ListAlerts handles GET /governance/budgets/alerts
func (h *GovernanceHandler) ListAlerts(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	alerts, err := h.svc.ListAlerts(r.Context(), orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list alerts")
		return
	}
	if alerts == nil {
		alerts = []model.BudgetAlert{}
	}
	RespondJSON(w, http.StatusOK, alerts)
}

// ─── Policies ───────────────────────────────────────────────────────────────

// ListPolicies handles GET /governance/policies
func (h *GovernanceHandler) ListPolicies(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	policies, err := h.svc.ListPolicies(r.Context(), orgID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list policies")
		return
	}
	if policies == nil {
		policies = []model.AgentPolicy{}
	}
	RespondJSON(w, http.StatusOK, policies)
}

// UpsertPolicy handles POST /governance/policies
func (h *GovernanceHandler) UpsertPolicy(w http.ResponseWriter, r *http.Request) {
	orgID, err := uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}

	var req model.CreatePolicyRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	policy, err := h.svc.UpsertPolicy(r.Context(), orgID, req)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to upsert policy: "+err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, policy)
}

// DeletePolicy handles DELETE /governance/policies/{policyID}
func (h *GovernanceHandler) DeletePolicy(w http.ResponseWriter, r *http.Request) {
	policyID, err := uuid.Parse(chi.URLParam(r, "policyID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid policy ID")
		return
	}

	if err := h.svc.DeletePolicy(r.Context(), policyID); err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to delete policy")
		return
	}
	RespondJSON(w, http.StatusNoContent, nil)
}
