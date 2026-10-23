// Package api provides the HTTP API building blocks: the error envelope
// used by all handlers and helpers for writing JSON responses.
package api

import (
	"encoding/json"
	"net/http"
)

// Error is the API error envelope payload. Detail, SQLState and DriverCode
// are omitted from JSON when empty.
type Error struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Detail     string `json:"detail,omitempty"`
	SQLState   string `json:"sqlState,omitempty"`
	DriverCode int    `json:"driverCode,omitempty"`
}

var _ error = (*Error)(nil)

// Error implements the error interface.
func (e *Error) Error() string {
	return e.Code + ": " + e.Message
}

// WriteJSON writes v as a JSON response with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteErr writes e wrapped in the error envelope: {"error": {...}}.
func WriteErr(w http.ResponseWriter, status int, e *Error) {
	WriteJSON(w, status, map[string]any{"error": e})
}

// ErrBadRequest returns a bad_request error with the given message.
func ErrBadRequest(msg string) *Error {
	return &Error{Code: "bad_request", Message: msg}
}

// ErrAuthFailed returns an auth_failed error with the given message.
func ErrAuthFailed(msg string) *Error {
	return &Error{Code: "auth_failed", Message: msg}
}

// ErrAuthRequired returns an auth_required error.
func ErrAuthRequired() *Error {
	return &Error{Code: "auth_required", Message: "authentication required"}
}

// ErrRateLimited returns a rate_limited error.
func ErrRateLimited() *Error {
	return &Error{Code: "rate_limited", Message: "too many requests"}
}

// ErrNotFound returns a not_found error.
func ErrNotFound() *Error {
	return &Error{Code: "not_found", Message: "not found"}
}

// ErrInternal returns an internal error; err, when non-nil, is exposed as detail.
func ErrInternal(err error) *Error {
	e := &Error{Code: "internal", Message: "internal error"}
	if err != nil {
		e.Detail = err.Error()
	}
	return e
}
