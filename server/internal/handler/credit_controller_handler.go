package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/jobshout/server/internal/creditcontroller"
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
		RespondError(w, http.StatusServiceUnavailable, "credit controller client not initialised")
		return false
	}
	return true
}

// respondUpstream answers with the status the client chose (upstream 4xx, or
// 404/409 for demo-mode refusals); anything else is a 502 from aivc-agents.
func respondUpstream(w http.ResponseWriter, err error) {
	var ccErr *creditcontroller.Error
	if errors.As(err, &ccErr) && ccErr.Status >= 400 && ccErr.Status < 500 {
		RespondError(w, ccErr.Status, ccErr.Msg)
		return
	}
	RespondError(w, http.StatusBadGateway, err.Error())
}

// Status reports demo/live mode and runtime health. It answers 200 even when
// aivc-agents is down so the tab can show why instead of failing to load.
func (h *CreditControllerHandler) Status(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	RespondJSON(w, http.StatusOK, h.svc.Status(r.Context()))
}

func (h *CreditControllerHandler) ListInvoices(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	body, err := h.svc.ListInvoices(r.Context())
	if err != nil {
		respondUpstream(w, err)
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
		respondUpstream(w, err)
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
		respondUpstream(w, err)
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
		respondUpstream(w, err)
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
		respondUpstream(w, err)
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
		respondUpstream(w, err)
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
		respondUpstream(w, err)
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
		respondUpstream(w, err)
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
