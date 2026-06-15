package httpx

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
)

// permissionsPolicy disables powerful features the app does not use.
const permissionsPolicy = "accelerometer=(), autoplay=(), camera=(), display-capture=(), " +
	"encrypted-media=(), fullscreen=(self), geolocation=(), gyroscope=(), magnetometer=(), " +
	"microphone=(), midi=(), payment=(), usb=()"

// SecurityHeaders sets a strict, nonce-based Content-Security-Policy and the
// standard hardening headers on every response. A fresh 128-bit nonce is
// generated per request and stored in the context so the layout can thread it
// into the (only) inline script — the import map. There is no 'unsafe-inline'.
//
// hsts controls whether Strict-Transport-Security is emitted; enable it in
// production (TLS terminated here or at the edge). When TLS terminates at
// Cloudflare you may instead set HSTS at the edge.
func SecurityHeaders(hsts bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nonce := newNonce()

			h := w.Header()
			h.Set("Content-Security-Policy", csp(nonce))
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "no-referrer")
			h.Set("Permissions-Policy", permissionsPolicy)
			h.Set("Cross-Origin-Opener-Policy", "same-origin")
			if hsts {
				h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains")
			}

			ctx := context.WithValue(r.Context(), nonceKey, nonce)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// csp builds the policy. Notes:
//   - script-src is 'self' + the per-request nonce (covers the inline import map);
//     htmx.min.js and core.mjs load as external 'self' scripts.
//   - style-src is 'self' (Tailwind compiles to an external stylesheet); avoid
//     inline style="" attributes, which would require 'unsafe-inline'.
//   - worker-src 'self' supports module workers; it also falls back to default-src.
func csp(nonce string) string {
	return strings.Join([]string{
		"default-src 'self'",
		"base-uri 'none'",
		"object-src 'none'",
		"frame-ancestors 'none'",
		"form-action 'self'",
		"img-src 'self' data:",
		"font-src 'self'",
		"connect-src 'self'",
		"style-src 'self'",
		"worker-src 'self'",
		"script-src 'self' 'nonce-" + nonce + "'",
	}, "; ")
}

func newNonce() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand failure is catastrophic; better to fail loudly than to
		// emit an empty nonce that would let any inline script execute.
		panic("httpx: crypto/rand failed: " + err.Error())
	}
	return base64.StdEncoding.EncodeToString(b)
}
