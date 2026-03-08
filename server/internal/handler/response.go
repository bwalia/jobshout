package handler

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse is the standard error payload returned by all endpoints.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// RespondJSON writes a JSON response with the given status code and payload.
func RespondJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if payload != nil {
		json.NewEncoder(w).Encode(payload)
	}
}

// RespondError writes a JSON error response.
func RespondError(w http.ResponseWriter, statusCode int, errMsg string) {
	RespondJSON(w, statusCode, ErrorResponse{Error: errMsg})
}

// DecodeJSON decodes a JSON request body into the given target struct.
// Returns false and writes an error response if decoding fails.
func DecodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		RespondError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return false
	}
	return true
}
