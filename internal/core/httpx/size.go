package httpx

import "net/http"

// RequestSize caps the request body using http.MaxBytesReader, guarding against
// oversized-body memory exhaustion. Reads beyond max fail and the handler (or
// form parsing) sees an error; MaxBytesReader also writes a 413 when the limit
// is hit during a read it controls.
func RequestSize(max int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if max > 0 {
				r.Body = http.MaxBytesReader(w, r.Body, max)
			}
			next.ServeHTTP(w, r)
		})
	}
}
