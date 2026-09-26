package handler

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// AuthHandler handles authentication HTTP endpoints.
type AuthHandler struct {
	authSvc         service.AuthService
	validate        *validator.Validate
	frontendBaseURL string
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(authSvc service.AuthService, frontendBaseURL string) *AuthHandler {
	return &AuthHandler{
		authSvc:         authSvc,
		validate:        validator.New(),
		frontendBaseURL: strings.TrimRight(frontendBaseURL, "/"),
	}
}

// Register handles POST /auth/register
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req model.RegisterRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	resp, err := h.authSvc.Register(r.Context(), req)
	if err != nil {
		if errors.Is(err, service.ErrEmailAlreadyExists) {
			RespondError(w, http.StatusConflict, err.Error())
			return
		}
		RespondError(w, http.StatusInternalServerError, "registration failed")
		return
	}

	RespondJSON(w, http.StatusCreated, resp)
}

// Login handles POST /auth/login
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req model.LoginRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	resp, err := h.authSvc.Login(r.Context(), req)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCredentials) {
			RespondError(w, http.StatusUnauthorized, err.Error())
			return
		}
		RespondError(w, http.StatusInternalServerError, "login failed")
		return
	}

	RespondJSON(w, http.StatusOK, resp)
}

// Refresh handles POST /auth/refresh
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req model.RefreshRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}

	resp, err := h.authSvc.RefreshToken(r.Context(), req.RefreshToken)
	if err != nil {
		if errors.Is(err, service.ErrInvalidRefreshToken) || errors.Is(err, service.ErrRefreshTokenExpired) {
			RespondError(w, http.StatusUnauthorized, err.Error())
			return
		}
		RespondError(w, http.StatusInternalServerError, "token refresh failed")
		return
	}

	RespondJSON(w, http.StatusOK, resp)
}

// GetMe handles GET /auth/me
func (h *AuthHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	userIDStr := middleware.GetUserID(r.Context())
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		RespondError(w, http.StatusUnauthorized, "invalid user ID in token")
		return
	}

	user, err := h.authSvc.GetMe(r.Context(), userID)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			RespondError(w, http.StatusNotFound, err.Error())
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	RespondJSON(w, http.StatusOK, user)
}

// UpdateProfile handles PATCH /auth/me
func (h *AuthHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userIDStr := middleware.GetUserID(r.Context())
	userID, err := uuid.Parse(userIDStr)
	if err != nil {
		RespondError(w, http.StatusUnauthorized, "invalid user ID in token")
		return
	}

	var req model.UpdateProfileRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	user, err := h.authSvc.UpdateProfile(r.Context(), userID, req)
	if err != nil {
		if errors.Is(err, service.ErrUserNotFound) {
			RespondError(w, http.StatusNotFound, err.Error())
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to update profile")
		return
	}

	RespondJSON(w, http.StatusOK, user)
}

// GoogleStatus handles GET /auth/google/status
func (h *AuthHandler) GoogleStatus(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, http.StatusOK, map[string]bool{"enabled": h.authSvc.GoogleEnabled()})
}

// GoogleStart handles GET /auth/google/start — the browser is sent to Google.
func (h *AuthHandler) GoogleStart(w http.ResponseWriter, r *http.Request) {
	intent := r.URL.Query().Get("intent")
	orgName := r.URL.Query().Get("org_name")
	authURL, err := h.authSvc.StartGoogle(r.Context(), intent, orgName)
	if err != nil {
		h.redirectGoogleError(w, r, intent, googleErrorCode(err))
		return
	}
	http.Redirect(w, r, authURL, http.StatusFound)
}

// GoogleCallback handles GET /auth/google/callback — Google redirects here.
func (h *AuthHandler) GoogleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		code := "denied"
		if errParam != "access_denied" {
			code = "failed"
		}
		intent := h.authSvc.AbandonGoogle(r.Context(), state)
		h.redirectGoogleError(w, r, intent, code)
		return
	}
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		intent := h.authSvc.AbandonGoogle(r.Context(), state)
		h.redirectGoogleError(w, r, intent, "missing_code")
		return
	}
	ticket, gotIntent, err := h.authSvc.CompleteGoogle(r.Context(), state, code)
	if err != nil {
		h.redirectGoogleError(w, r, gotIntent, googleErrorCode(err))
		return
	}
	dest := h.frontendBaseURL + "/auth/google/callback?ticket=" + url.QueryEscape(ticket)
	http.Redirect(w, r, dest, http.StatusFound)
}

// GoogleComplete handles POST /auth/google/complete
func (h *AuthHandler) GoogleComplete(w http.ResponseWriter, r *http.Request) {
	var req model.GoogleCompleteRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}
	resp, err := h.authSvc.ExchangeGoogleTicket(r.Context(), req.Ticket)
	if err != nil {
		if errors.Is(err, service.ErrInvalidGoogleTicket) || errors.Is(err, service.ErrUserNotFound) {
			RespondError(w, http.StatusUnauthorized, err.Error())
			return
		}
		RespondError(w, http.StatusInternalServerError, "google sign-in failed")
		return
	}
	RespondJSON(w, http.StatusOK, resp)
}

func (h *AuthHandler) redirectGoogleError(w http.ResponseWriter, r *http.Request, intent, code string) {
	path := "/login"
	if intent == "signup" {
		path = "/signup"
	}
	dest := h.frontendBaseURL + path + "?error=" + url.QueryEscape(code)
	http.Redirect(w, r, dest, http.StatusFound)
}

func googleErrorCode(err error) string {
	switch {
	case errors.Is(err, service.ErrGoogleAuthNotConfigured):
		return "not_configured"
	case errors.Is(err, service.ErrInvalidGoogleState):
		return "invalid_state"
	case errors.Is(err, service.ErrGoogleEmailNotVerified):
		return "unverified_email"
	default:
		return "failed"
	}
}
