package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

type CareerHandler struct {
	svc service.CareerService
}

func NewCareerHandler(svc service.CareerService) *CareerHandler {
	return &CareerHandler{svc: svc}
}

func (h *CareerHandler) ids(w http.ResponseWriter, r *http.Request) (orgID, userID uuid.UUID, ok bool) {
	var err error
	orgID, err = uuid.Parse(middleware.GetOrgID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid org_id in token")
		return
	}
	userID, err = uuid.Parse(middleware.GetUserID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid user_id in token")
		return
	}
	ok = true
	return
}

func (h *CareerHandler) writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, service.ErrCareerNotFound):
		RespondError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrCareerBadStatus):
		RespondError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrCareerMissingInput), errors.Is(err, service.ErrCareerEmptyBlacklist):
		RespondError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrCareerBadUpload):
		msg := strings.TrimPrefix(err.Error(), service.ErrCareerBadUpload.Error())
		msg = strings.TrimPrefix(msg, ": ")
		msg = strings.TrimPrefix(msg, "career: ")
		if msg == "" {
			msg = "could not read that file"
		}
		RespondError(w, http.StatusBadRequest, msg)
	default:
		RespondError(w, http.StatusInternalServerError, err.Error())
	}
}

func (h *CareerHandler) GetProfile(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	p, err := h.svc.GetOrCreateProfile(r.Context(), orgID, userID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, p)
}

func (h *CareerHandler) PatchProfile(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	var req model.UpdateCareerProfileRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	p, err := h.svc.UpdateProfile(r.Context(), orgID, userID, req)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, p)
}

func (h *CareerHandler) UploadCV(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	// 5MB file plus multipart wrapping. Reject before buffering the rest.
	r.Body = http.MaxBytesReader(w, r.Body, (5<<20)+512<<10)
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			RespondError(w, http.StatusRequestEntityTooLarge, "file is larger than 5MB")
			return
		}
		RespondError(w, http.StatusBadRequest, "expected a multipart file field named file")
		return
	}
	f, hdr, err := r.FormFile("file")
	if err != nil {
		RespondError(w, http.StatusBadRequest, "file is required")
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (5<<20)+1))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "could not read file")
		return
	}
	if len(data) > 5<<20 {
		RespondError(w, http.StatusRequestEntityTooLarge, "file is larger than 5MB")
		return
	}
	ct := hdr.Header.Get("Content-Type")
	out, err := h.svc.UploadCV(r.Context(), orgID, userID, hdr.Filename, ct, data)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) Intake(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	var req struct {
		Document string `json:"document"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}
	out, err := h.svc.Intake(r.Context(), orgID, userID, req.Document)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) Evaluate(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	var req model.EvaluateCareerRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	out, err := h.svc.Evaluate(r.Context(), orgID, userID, req)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) ListEvaluations(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	page, per := pagePer(r)
	out, err := h.svc.ListEvaluations(r.Context(), orgID, userID, model.PaginationParams{Page: page, PerPage: per})
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) GetEvaluation(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid id")
		return
	}
	out, err := h.svc.GetEvaluation(r.Context(), orgID, userID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) ListPipeline(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	page, per := pagePer(r)
	out, err := h.svc.ListPipeline(r.Context(), orgID, userID, model.PaginationParams{Page: page, PerPage: per})
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) ListTracker(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	page, per := pagePer(r)
	out, err := h.svc.ListTracker(r.Context(), orgID, userID, r.URL.Query().Get("status"), model.PaginationParams{Page: page, PerPage: per})
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) SetStatus(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req model.SetCareerStatusRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	out, err := h.svc.SetStatus(r.Context(), orgID, userID, id, req.Status, req.Note)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) Scan(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	var req model.ScanCareerRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}
	out, err := h.svc.Scan(r.Context(), orgID, userID, req)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) ListPortals(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	out, err := h.svc.ListPortals(r.Context(), orgID, userID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) AddPortal(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	var req model.AddCareerPortalRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	out, err := h.svc.AddPortal(r.Context(), orgID, userID, req)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusCreated, out)
}

func (h *CareerHandler) ListBlacklist(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	out, err := h.svc.ListBlacklist(r.Context(), orgID, userID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) AddBlacklist(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	var req model.AddCareerBlacklistRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	out, err := h.svc.AddBlacklist(r.Context(), orgID, userID, req)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusCreated, out)
}

func (h *CareerHandler) Doctor(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Doctor(r.Context(), orgID, userID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) Patterns(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	out, err := h.svc.Patterns(r.Context(), orgID, userID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) CoverLetter(w http.ResponseWriter, r *http.Request) {
	h.runArtifact(w, r, h.svc.CoverLetter)
}

func (h *CareerHandler) TailorCV(w http.ResponseWriter, r *http.Request) {
	h.runArtifact(w, r, h.svc.TailorCV)
}

func (h *CareerHandler) EmailDraft(w http.ResponseWriter, r *http.Request) {
	h.runArtifact(w, r, h.svc.EmailDraft)
}

func (h *CareerHandler) ListArtifacts(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid id")
		return
	}
	out, err := h.svc.ListArtifacts(r.Context(), orgID, userID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) Followup(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid id")
		return
	}
	out, err := h.svc.Followup(r.Context(), orgID, userID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusCreated, out)
}

func (h *CareerHandler) ListFollowups(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	out, err := h.svc.ListFollowups(r.Context(), orgID, userID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) InterviewPrep(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid id")
		return
	}
	out, err := h.svc.InterviewPrep(r.Context(), orgID, userID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) OfferPrep(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid id")
		return
	}
	out, err := h.svc.OfferPrep(r.Context(), orgID, userID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) SalaryGap(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		Advertised string `json:"advertised"`
		Actual     string `json:"actual"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	out, err := h.svc.SalaryGap(r.Context(), orgID, userID, id, req.Advertised, req.Actual)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) ListStories(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	out, err := h.svc.ListStories(r.Context(), orgID, userID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) UpsertStory(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	var st model.CareerStory
	if !DecodeJSON(w, r, &st) {
		return
	}
	out, err := h.svc.UpsertStory(r.Context(), orgID, userID, st)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) ListContacts(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	out, err := h.svc.ListContacts(r.Context(), orgID, userID)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) AddContact(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	var req model.AddCareerContactRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	out, err := h.svc.AddContact(r.Context(), orgID, userID, req)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusCreated, out)
}

func (h *CareerHandler) Upskill(w http.ResponseWriter, r *http.Request) {
	h.Patterns(w, r)
}

func (h *CareerHandler) PreviewListing(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	var req struct {
		JobURL string `json:"job_url"`
	}
	if !DecodeJSON(w, r, &req) {
		return
	}
	out, err := h.svc.PreviewListing(r.Context(), orgID, userID, req.JobURL)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) BatchEvaluate(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	var req struct {
		Limit int      `json:"limit"`
		URLs  []string `json:"urls"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	out, err := h.svc.BatchEvaluate(r.Context(), orgID, userID, req.Limit, req.URLs)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) runArtifact(w http.ResponseWriter, r *http.Request, fn func(context.Context, uuid.UUID, uuid.UUID, uuid.UUID) (*model.CareerArtifact, error)) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid id")
		return
	}
	out, err := fn(r.Context(), orgID, userID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	RespondJSON(w, http.StatusOK, out)
}

func (h *CareerHandler) ArtifactPDF(w http.ResponseWriter, r *http.Request) {
	orgID, userID, ok := h.ids(w, r)
	if !ok {
		return
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid id")
		return
	}
	a, pdf, err := h.svc.ArtifactPDF(r.Context(), orgID, userID, id)
	if err != nil {
		h.writeErr(w, err)
		return
	}
	name := "tailored-cv.pdf"
	if a != nil && a.PDFFilename != "" {
		name = a.PDFFilename
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(pdf)
}

func pagePer(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	per, _ := strconv.Atoi(r.URL.Query().Get("per_page"))
	return page, per
}
