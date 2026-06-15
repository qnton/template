package httpx

import (
	"log/slog"
	"net/http"
	"runtime/debug"
)

// Recover catches panics from downstream handlers, logs the error and stack
// trace, and returns a generic 500 — the stack is NEVER written to the client.
func Recover(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					// http.ErrAbortHandler is the documented way to abort a
					// response; propagate it instead of logging a "panic".
					if rec == http.ErrAbortHandler {
						panic(rec)
					}
					logger.ErrorContext(r.Context(), "panic recovered",
						slog.Any("error", rec),
						slog.String("method", r.Method),
						slog.String("path", r.URL.Path),
						slog.String("stack", string(debug.Stack())),
					)
					// Connection may be unusable mid-write, but attempt a clean 500.
					w.Header().Set("Connection", "close")
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
