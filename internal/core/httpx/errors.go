package httpx

import (
	"errors"
	"net/http"
)

// Sentinel errors let handlers signal an outcome without hand-coding a status at
// every call site. Wrap them with fmt.Errorf("...: %w", ErrNotFound) and map to a
// status with StatusFor. Optional: features may keep returning plain errors.
var (
	ErrNotFound     = errors.New("not found")
	ErrInvalidInput = errors.New("invalid input")
	ErrConflict     = errors.New("conflict")
	ErrUnauthorized = errors.New("unauthorized")
)

// StatusError carries an explicit HTTP status alongside an error, for cases that
// do not fit a sentinel. It unwraps to the underlying error.
type StatusError struct {
	Code int
	Err  error
}

func (e *StatusError) Error() string { return e.Err.Error() }
func (e *StatusError) Unwrap() error { return e.Err }

// StatusFor maps err to an HTTP status code, defaulting to 500. A *StatusError's
// code takes precedence; otherwise the sentinels above are matched via errors.Is.
func StatusFor(err error) int {
	var se *StatusError
	if errors.As(err, &se) {
		return se.Code
	}
	switch {
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrInvalidInput):
		return http.StatusBadRequest
	case errors.Is(err, ErrConflict):
		return http.StatusConflict
	case errors.Is(err, ErrUnauthorized):
		return http.StatusUnauthorized
	default:
		return http.StatusInternalServerError
	}
}
