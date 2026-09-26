package handler

import (
	"net/http"

	"github.com/jobshout/server/internal/service"
)

// SimproPaymentsHandler exposes Simpro AR + F-Gas façades.
type SimproPaymentsHandler struct {
	svc service.SimproPaymentsService
}

func NewSimproPaymentsHandler(svc service.SimproPaymentsService) *SimproPaymentsHandler {
	return &SimproPaymentsHandler{svc: svc}
}

func (h *SimproPaymentsHandler) ensure(w http.ResponseWriter) bool {
	if h.svc == nil || !h.svc.Enabled() {
		RespondError(w, http.StatusServiceUnavailable, "simpro payments agent not configured")
		return false
	}
	return true
}

func (h *SimproPaymentsHandler) Status(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	RespondJSON(w, http.StatusOK, h.svc.Status(r.Context()))
}

func (h *SimproPaymentsHandler) Summary(w http.ResponseWriter, r *http.Request) {
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

func (h *SimproPaymentsHandler) ListInvoices(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	items, mode, err := h.svc.ListInvoices(r.Context())
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"mode": mode, "count": len(items), "invoices": items})
}

func (h *SimproPaymentsHandler) ListPayments(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	items, mode, err := h.svc.ListPayments(r.Context())
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"mode": mode, "count": len(items), "payments": items})
}

func (h *SimproPaymentsHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	items, mode, err := h.svc.ListJobs(r.Context())
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"mode": mode, "count": len(items), "jobs": items})
}

func (h *SimproPaymentsHandler) Aging(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	body, err := h.svc.AgingReport(r.Context())
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, body)
}

func (h *SimproPaymentsHandler) ReconcilePreview(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	body, err := h.svc.ReconcilePreview(r.Context())
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, body)
}

func (h *SimproPaymentsHandler) MonthEnd(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	body, err := h.svc.MonthEndChecklist(r.Context())
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, body)
}

func (h *SimproPaymentsHandler) FGas(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	body, err := h.svc.FGasReport(r.Context())
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, body)
}

func (h *SimproPaymentsHandler) ListFGas(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	items, mode, err := h.svc.ListFGas(r.Context())
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"mode": mode, "count": len(items), "events": items})
}
