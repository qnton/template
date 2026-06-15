package httpx

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
)

const (
	csrfTokenLen   = 32
	csrfHeaderName = "X-CSRF-Token"
	csrfFieldName  = "csrf_token"
	csrfCookieDev  = "csrf_token"
	csrfCookieProd = "__Host-csrf_token" // __Host- requires Secure + Path=/ + no Domain
)

// CSRF implements the masked double-submit-cookie pattern — stateless CSRF
// protection with no external dependency, compatible with HTMX.
//
//   - A random 32-byte "real" token lives in an HttpOnly cookie, so XSS cannot
//     read it. It persists across requests.
//   - Each page render gets a fresh MASKED token (mask ‖ real⊕mask) via the
//     CSRFToken context value, embedded in a <meta> tag (and optionally a hidden
//     field). Masking changes the on-page value every response, which defeats
//     BREACH even with gzip enabled.
//   - On unsafe methods the submitted masked token (X-CSRF-Token header or
//     csrf_token form field) is unmasked and compared to the cookie's real token
//     with a constant-time comparison.
//
// HTMX sends the header automatically; see static/js/core.mjs, which reads the
// meta tag on htmx:configRequest (no inline script, so it is CSP-safe).
type CSRF struct {
	prod bool
}

// NewCSRF builds the helper. In production the cookie gains the __Host- prefix
// and the Secure flag; in development (plain HTTP) both are relaxed.
func NewCSRF(prod bool) *CSRF { return &CSRF{prod: prod} }

func (c *CSRF) cookieName() string {
	if c.prod {
		return csrfCookieProd
	}
	return csrfCookieDev
}

// Middleware issues/loads the token cookie, verifies unsafe requests, and exposes
// the masked token to downstream handlers and templates.
func (c *CSRF) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		real := c.loadOrIssue(w, r)

		if !safeMethod(r.Method) && !c.verify(r, real) {
			http.Error(w, "CSRF token invalid or missing", http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), csrfTokenKey, maskToken(real))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (c *CSRF) loadOrIssue(w http.ResponseWriter, r *http.Request) []byte {
	if ck, err := r.Cookie(c.cookieName()); err == nil {
		if raw, err := base64.RawURLEncoding.DecodeString(ck.Value); err == nil && len(raw) == csrfTokenLen {
			return raw
		}
	}
	real := randomBytes(csrfTokenLen)
	http.SetCookie(w, &http.Cookie{
		Name:     c.cookieName(),
		Value:    base64.RawURLEncoding.EncodeToString(real),
		Path:     "/",
		HttpOnly: true,
		Secure:   c.prod,
		SameSite: http.SameSiteLaxMode,
	})
	return real
}

func (c *CSRF) verify(r *http.Request, real []byte) bool {
	sent := r.Header.Get(csrfHeaderName)
	if sent == "" {
		// Fall back to a form field for classic (non-HTMX) posts. ParseForm
		// respects any MaxBytesReader set by the request-size middleware.
		if err := r.ParseForm(); err == nil {
			sent = r.PostForm.Get(csrfFieldName)
		}
	}
	unmasked := unmaskToken(sent)
	if unmasked == nil {
		return false
	}
	return subtle.ConstantTimeCompare(unmasked, real) == 1
}

// maskToken returns base64(mask ‖ real⊕mask) with a fresh random mask.
func maskToken(real []byte) string {
	m := randomBytes(csrfTokenLen)
	out := make([]byte, 2*csrfTokenLen)
	copy(out, m)
	for i := range csrfTokenLen {
		out[csrfTokenLen+i] = real[i] ^ m[i]
	}
	return base64.RawURLEncoding.EncodeToString(out)
}

// unmaskToken reverses maskToken, returning the real token or nil if malformed.
func unmaskToken(s string) []byte {
	if s == "" {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(raw) != 2*csrfTokenLen {
		return nil
	}
	m, xored := raw[:csrfTokenLen], raw[csrfTokenLen:]
	real := make([]byte, csrfTokenLen)
	for i := range csrfTokenLen {
		real[i] = xored[i] ^ m[i]
	}
	return real
}

func safeMethod(m string) bool {
	switch m {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("httpx: crypto/rand failed: " + err.Error())
	}
	return b
}
