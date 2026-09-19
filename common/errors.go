package common

import (
	"encoding/json"
	"net/http"
)

// ApiError matches the OpenAI error format so client SDKs can parse it.
type ApiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code,omitempty"`
}

type ErrorBody struct {
	Error ApiError `json:"error"`
}

// WriteError writes an OpenAI-format error response.
func WriteError(w http.ResponseWriter, status int, message, typ, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(ErrorBody{Error: ApiError{
		Message: message,
		Type:    typ,
		Code:    code,
	}})
}

// WriteJSON writes an arbitrary JSON response.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
