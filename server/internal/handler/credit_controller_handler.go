package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/jobshout/server/internal/service"
)

// CreditControllerHandler exposes the AP credit-controller façade.
type CreditControllerHandler struct {
	svc service.CreditControllerService
}

func NewCreditControllerHandler(svc service.CreditControllerService) *CreditControllerHandler {
	return &CreditControllerHandler{svc: svc}
}

func (h *CreditControllerHandler) ensure(w http.ResponseWriter) bool {
	if h.svc == nil || !h.svc.Enabled() {
		RespondError(w, http.StatusServiceUnavailable, "credit controller runtime not configured (AIVC_BASE_URL)")
		return false
	}
	return true
}

func (h *CreditControllerHandler) Status(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	body, err := h.svc.Ping(r.Context())
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, body)
}

func (h *CreditControllerHandler) ListInvoices(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	body, err := h.svc.ListInvoices(r.Context())
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, body)
}

func (h *CreditControllerHandler) GetInvoice(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	id := chi.URLParam(r, "invoiceID")
	body, err := h.svc.GetInvoice(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, body)
}

func (h *CreditControllerHandler) Summary(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	body, err := h.svc.Summary(r.Context())
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, body)
}

func (h *CreditControllerHandler) Generate(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	var req struct {
		Cadence string `json:"cadence"`
		Count   int    `json:"count"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Count == 0 {
		req.Count = 10
	}
	body, err := h.svc.Generate(r.Context(), req.Cadence, req.Count)
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, body)
}

func (h *CreditControllerHandler) Triage(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	var req struct {
		InvoiceID string `json:"invoice_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.InvoiceID) == "" {
		RespondError(w, http.StatusBadRequest, "invoice_id is required")
		return
	}
	body, err := h.svc.Triage(r.Context(), req.InvoiceID)
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, body)
}

func (h *CreditControllerHandler) TriageBatch(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	var req struct {
		InvoiceIDs []string `json:"invoice_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.InvoiceIDs) == 0 {
		RespondError(w, http.StatusBadRequest, "invoice_ids is required")
		return
	}
	body, err := h.svc.TriageBatch(r.Context(), req.InvoiceIDs)
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, body)
}

func (h *CreditControllerHandler) Queue(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	body, err := h.svc.Queue(r.Context())
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, body)
}

func (h *CreditControllerHandler) Approve(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	var req struct {
		RunID    string `json:"run_id"`
		Approved bool   `json:"approved"`
		Approver string `json:"approver"`
		Note     string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.RunID) == "" {
		RespondError(w, http.StatusBadRequest, "run_id is required")
		return
	}
	body, err := h.svc.Approve(r.Context(), req.RunID, req.Approved, req.Approver, req.Note)
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, body)
}

// ParseCount is a tiny helper for query forms (kept for future use).
func ParseCount(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return fallback
	}
	return n
}
