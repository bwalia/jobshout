package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jobshout/server/internal/service"
	"github.com/jobshout/server/internal/wslproxymcp"
)

// ABTestHandler exposes AB Testing agent HTTP endpoints.
type ABTestHandler struct {
	svc service.ABTestService
}

func NewABTestHandler(svc service.ABTestService) *ABTestHandler {
	return &ABTestHandler{svc: svc}
}

func (h *ABTestHandler) ensure(w http.ResponseWriter) bool {
	if h.svc == nil || !h.svc.Enabled() {
		RespondError(w, http.StatusServiceUnavailable, "ab testing agent not configured")
		return false
	}
	return true
}

func (h *ABTestHandler) Status(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	RespondJSON(w, http.StatusOK, h.svc.Status(r.Context()))
}

func (h *ABTestHandler) ListExperiments(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	items, mode, err := h.svc.ListExperiments(r.Context())
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"mode": mode, "count": len(items), "experiments": items})
}

func (h *ABTestHandler) GetExperiment(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	id := chi.URLParam(r, "id")
	ex, err := h.svc.GetExperiment(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusNotFound, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, ex)
}

func (h *ABTestHandler) SetWeights(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Backends []wslproxymcp.BackendWeight `json:"backends"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		RespondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(body.Backends) == 0 {
		RespondError(w, http.StatusBadRequest, "backends required")
		return
	}
	out, err := h.svc.SetWeights(r.Context(), id, body.Backends)
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *ABTestHandler) Promote(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		Label string `json:"label"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	if body.Label == "" {
		body.Label = "v2"
	}
	out, err := h.svc.Promote(r.Context(), id, body.Label)
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *ABTestHandler) Rollback(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	id := chi.URLParam(r, "id")
	out, err := h.svc.Rollback(r.Context(), id)
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *ABTestHandler) Observe(w http.ResponseWriter, r *http.Request) {
	if !h.ensure(w) {
		return
	}
	id := chi.URLParam(r, "id")
	n := 40
	if q := r.URL.Query().Get("n"); q != "" {
		if v, err := strconv.Atoi(q); err == nil {
			n = v
		}
	}
	out, err := h.svc.Observe(r.Context(), id, n)
	if err != nil {
		RespondError(w, http.StatusBadGateway, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, out)
}
