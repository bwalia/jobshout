package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/jobshout/server/internal/middleware"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/service"
)

// Logout handles POST /auth/logout. It is public: holding the refresh token
// is the proof, and a client whose access token has expired must still be
// able to sign out.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req model.LogoutRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}
	if err := h.authSvc.Logout(r.Context(), req.RefreshToken); err != nil {
		RespondError(w, http.StatusInternalServerError, "logout failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// LogoutAll handles POST /auth/logout-all: every session and device the
// caller has, this one included.
func (h *AuthHandler) LogoutAll(w http.ResponseWriter, r *http.Request) {
	userID, ok := authUserID(w, r)
	if !ok {
		return
	}
	if err := h.authSvc.LogoutAll(r.Context(), userID); err != nil {
		RespondError(w, http.StatusInternalServerError, "logout failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// AppleStatus handles GET /auth/apple/status.
func (h *AuthHandler) AppleStatus(w http.ResponseWriter, r *http.Request) {
	RespondJSON(w, http.StatusOK, map[string]bool{"enabled": h.authSvc.AppleEnabled()})
}

// AppleNonce handles POST /auth/apple/nonce.
func (h *AuthHandler) AppleNonce(w http.ResponseWriter, r *http.Request) {
	nonce, expires, err := h.authSvc.AppleNonce(r.Context())
	if err != nil {
		if errors.Is(err, service.ErrAppleAuthNotConfigured) {
			RespondError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		RespondError(w, http.StatusInternalServerError, "could not start sign in with apple")
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"nonce": nonce, "expires_at": expires})
}

// AppleSignIn handles POST /auth/apple.
func (h *AuthHandler) AppleSignIn(w http.ResponseWriter, r *http.Request) {
	var req model.AppleSignInRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}
	resp, err := h.authSvc.SignInWithApple(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrAppleAuthNotConfigured):
			RespondError(w, http.StatusServiceUnavailable, err.Error())
		case errors.Is(err, service.ErrInvalidAppleToken), errors.Is(err, service.ErrUserNotFound):
			RespondError(w, http.StatusUnauthorized, service.ErrInvalidAppleToken.Error())
		case errors.Is(err, service.ErrAppleEmailRequired):
			RespondError(w, http.StatusUnprocessableEntity, err.Error())
		default:
			RespondError(w, http.StatusInternalServerError, "sign in with apple failed")
		}
		return
	}
	RespondJSON(w, http.StatusOK, resp)
}

// ListDevices handles GET /devices.
func (h *AuthHandler) ListDevices(w http.ResponseWriter, r *http.Request) {
	userID, ok := authUserID(w, r)
	if !ok {
		return
	}
	devices, err := h.authSvc.ListDevices(r.Context(), userID)
	if err != nil {
		RespondError(w, http.StatusInternalServerError, "failed to list devices")
		return
	}
	RespondJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

// RevokeDevice handles DELETE /devices/{deviceID}.
func (h *AuthHandler) RevokeDevice(w http.ResponseWriter, r *http.Request) {
	userID, ok := authUserID(w, r)
	if !ok {
		return
	}
	deviceID, err := uuid.Parse(chi.URLParam(r, "deviceID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid device id")
		return
	}
	if err := h.authSvc.RevokeDevice(r.Context(), userID, deviceID); err != nil {
		if errors.Is(err, service.ErrDeviceNotFound) {
			RespondError(w, http.StatusNotFound, err.Error())
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to revoke device")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetDevicePush handles PUT /devices/{deviceID}/push.
func (h *AuthHandler) SetDevicePush(w http.ResponseWriter, r *http.Request) {
	userID, ok := authUserID(w, r)
	if !ok {
		return
	}
	deviceID, err := uuid.Parse(chi.URLParam(r, "deviceID"))
	if err != nil {
		RespondError(w, http.StatusBadRequest, "invalid device id")
		return
	}
	var req model.DevicePushRequest
	if !DecodeJSON(w, r, &req) {
		return
	}
	if err := h.validate.Struct(req); err != nil {
		RespondError(w, http.StatusBadRequest, "validation failed: "+err.Error())
		return
	}
	if err := h.authSvc.SetDevicePush(r.Context(), userID, deviceID, req); err != nil {
		if errors.Is(err, service.ErrDeviceNotFound) {
			RespondError(w, http.StatusNotFound, err.Error())
			return
		}
		RespondError(w, http.StatusInternalServerError, "failed to set push token")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func authUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	userID, err := uuid.Parse(middleware.GetUserID(r.Context()))
	if err != nil {
		RespondError(w, http.StatusUnauthorized, "invalid user ID in token")
		return uuid.Nil, false
	}
	return userID, true
}
