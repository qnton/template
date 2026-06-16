// Package httpx is the security core: a hand-rolled middleware chain (no external
// router or middleware library) plus the request-scoped values that templates and
// handlers read (CSP nonce, real client IP, CSRF token).
//
// Middlewares are plain net/http decorators. Chain applies them outermost-first,
// so Chain(h, A, B) runs A → B → h on the way in. The recommended ordering is
// assembled in internal/core/app.
package httpx

import (
	"context"
	"net/http"
)

// Middleware decorates an http.Handler.
type Middleware func(http.Handler) http.Handler

// Chain wraps h with the given middlewares. The first middleware is outermost
// (runs first on the request, last on the response).
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// contextKey is unexported so only this package can set request-scoped values.
type contextKey int

const (
	nonceKey contextKey = iota
	realIPKey
	csrfTokenKey
	requestIDKey
)

// Nonce returns the per-request CSP nonce, or "" if the security-headers
// middleware did not run. Templates thread this into the import-map script tag.
func Nonce(ctx context.Context) string { return stringValue(ctx, nonceKey) }

// ClientIP returns the trusted client IP resolved by the RealIP middleware,
// falling back to "" if unset.
func ClientIP(ctx context.Context) string { return stringValue(ctx, realIPKey) }

// CSRFToken returns the masked, per-request CSRF token to embed in a page
// (meta tag / hidden form field). It is safe to expose; see csrf.go.
func CSRFToken(ctx context.Context) string { return stringValue(ctx, csrfTokenKey) }

func stringValue(ctx context.Context, k contextKey) string {
	if v, ok := ctx.Value(k).(string); ok {
		return v
	}
	return ""
}
