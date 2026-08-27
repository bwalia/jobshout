package handler

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/imagegen"
	"github.com/jobshout/server/internal/imagestore"
	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// ImageHandler serves the image generation API: generating one, listing what
// can generate it, browsing what has been generated, and serving the bytes.
type ImageHandler struct {
	images *service.ImageService
}

// NewImageHandler builds the handler over the image service.
func NewImageHandler(images *service.ImageService) *ImageHandler {
	return &ImageHandler{images: images}
}

// generateImageRequest is the POST body.
type generateImageRequest struct {
	Prompt   string `json:"prompt"`
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	Steps    int    `json:"steps,omitempty"`
	Seed     int64  `json:"seed,omitempty"`
}

// generateImageResponse is what comes back.
type generateImageResponse struct {
	// URL is empty when object storage is not configured. ImageBase64 is
	// populated in exactly that case, so the caller can still display what it
	// asked for — a picture that exists but has no home is still a picture.
	URL         string `json:"url,omitempty"`
	ImageBase64 string `json:"image_base64,omitempty"`

	Provider   string `json:"provider"`
	Model      string `json:"model"`
	Seed       int64  `json:"seed"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	DurationMS int    `json:"duration_ms"`
	ID         string `json:"id,omitempty"`
}

// Generate handles POST /api/v1/images/generate.
func (h *ImageHandler) Generate(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgFromRequest(w, r)
	if !ok {
		return
	}

	var req generateImageRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		RespondError(w, http.StatusBadRequest, "a prompt is required")
		return
	}

	// The person who asked is recorded where there is one. A request made by a
	// schedule has no user, and the column is nullable for that reason.
	var userID *uuid.UUID
	if raw := middleware.GetUserID(r.Context()); raw != "" {
		if parsed, err := uuid.Parse(raw); err == nil {
			userID = &parsed
		}
	}

	seed := req.Seed
	if seed == 0 {
		seed = -1
	}

	result, err := h.images.Generate(r.Context(), service.GenerateImageRequest{
		OrgID:    orgID,
		UserID:   userID,
		Prompt:   req.Prompt,
		Provider: req.Provider,
		Model:    req.Model,
		Width:    req.Width,
		Height:   req.Height,
		Steps:    req.Steps,
		Seed:     seed,
		Source:   model.ImageSourceManual,
	})
	if err != nil {
		RespondError(w, imageErrorStatus(err), err.Error())
		return
	}

	resp := generateImageResponse{
		URL:        result.URL,
		Provider:   result.Provider,
		Model:      result.Model,
		Seed:       result.Seed,
		Width:      result.Width,
		Height:     result.Height,
		DurationMS: result.DurationMS,
	}
	if result.RecordID != nil {
		resp.ID = result.RecordID.String()
	}
	if result.URL == "" {
		resp.ImageBase64 = base64.StdEncoding.EncodeToString(result.PNG)
	}

	RespondJSON(w, http.StatusOK, resp)
}

// imageModelsResponse is the payload behind the image model picker. It mirrors
// the LLM picker's shape so the two controls can share a component.
type imageModelsResponse struct {
	Enabled         bool                 `json:"enabled"`
	DefaultProvider string               `json:"default_provider"`
	Models          []imagegen.ModelInfo `json:"models"`
}

// ListModels handles GET /api/v1/images/models.
//
// It answers with enabled:false rather than an error when nothing is
// configured: "this platform cannot draw" is a state the UI should render, not
// a failure it should apologise for.
func (h *ImageHandler) ListModels(w http.ResponseWriter, r *http.Request) {
	if !h.images.Enabled() {
		RespondJSON(w, http.StatusOK, imageModelsResponse{Enabled: false, Models: []imagegen.ModelInfo{}})
		return
	}

	models := h.images.ListModels(r.Context())
	if models == nil {
		models = []imagegen.ModelInfo{}
	}
	RespondJSON(w, http.StatusOK, imageModelsResponse{
		Enabled:         true,
		DefaultProvider: h.images.DefaultProvider(),
		Models:          models,
	})
}

// List handles GET /api/v1/images — the org's generated images, newest first.
func (h *ImageHandler) List(w http.ResponseWriter, r *http.Request) {
	orgID, ok := h.orgFromRequest(w, r)
	if !ok {
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))

	images, err := h.images.Recent(r.Context(), orgID, limit)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, err.Error())
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"images": images})
}

// Serve handles GET /api/v1/images/file/* — the stored bytes.
//
// The route is registered without auth: object keys contain UUIDs (not
// sequential IDs), and CMS featured-image previews load this URL with a plain
// <img> that cannot attach a JWT.
func (h *ImageHandler) Serve(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimPrefix(r.URL.Path, imagestore.URLPrefix)
	if key == "" {
		RespondError(w, http.StatusBadRequest, "missing image key")
		return
	}

	data, err := h.images.Fetch(r.Context(), key)
	if err != nil {
		RespondError(w, http.StatusNotFound, "image not found")
		return
	}

	w.Header().Set("Content-Type", "image/png")
	// Generated images never change: the key contains a UUID, so a new image is
	// a new key. Caching hard is safe and keeps an article's cover off the
	// storage backend on every page view.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	_, _ = w.Write(data)
}

// orgFromRequest resolves the calling organization, answering the request with
// an error and reporting false when there is none.
func (h *ImageHandler) orgFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	raw := middleware.GetOrgID(r.Context())
	if raw == "" {
		RespondError(w, http.StatusUnauthorized, "missing organization context")
		return uuid.Nil, false
	}
	orgID, err := uuid.Parse(raw)
	if err != nil {
		RespondError(w, http.StatusUnauthorized, "invalid organization context")
		return uuid.Nil, false
	}
	return orgID, true
}

// imageErrorStatus maps a generation failure onto a status code that says what
// the caller should do about it.
//
// The distinction that matters is between "this server cannot do this" (fix
// configuration), "the generator is busy" (try again shortly) and "your request
// was wrong" (change it). Collapsing all three into 500 makes a busy GPU look
// like a bug.
func imageErrorStatus(err error) int {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "not configured"):
		return http.StatusServiceUnavailable
	case strings.Contains(msg, "busy"):
		return http.StatusServiceUnavailable
	case strings.Contains(msg, "a prompt is required"):
		return http.StatusBadRequest
	default:
		return http.StatusInternalServerError
	}
}
