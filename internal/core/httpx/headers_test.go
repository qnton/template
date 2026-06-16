package httpx

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// serveSecurity runs SecurityHeaders(hsts) over a handler that captures the
// per-request nonce, returning the recorder and the captured nonce.
func serveSecurity(t *testing.T, hsts bool) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var gotNonce string
	h := SecurityHeaders(hsts)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotNonce = Nonce(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	return rec, gotNonce
}

func TestSecurityHeadersCSP(t *testing.T) {
	rec, nonce := serveSecurity(t, false)
	if nonce == "" {
		t.Fatal("nonce not set in context")
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Fatal("Content-Security-Policy header missing")
	}
	if !strings.Contains(csp, "script-src 'self' 'nonce-"+nonce+"'") {
		t.Errorf("CSP missing nonce'd script-src; got %q", csp)
	}
	for _, want := range []string{
		"default-src 'self'", "base-uri 'none'", "object-src 'none'",
		"frame-ancestors 'none'", "form-action 'self'", "style-src 'self'",
	} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP missing directive %q; got %q", want, csp)
		}
	}
	for _, bad := range []string{"unsafe-inline", "unsafe-eval"} {
		if strings.Contains(csp, bad) {
			t.Errorf("CSP must not contain %q; got %q", bad, csp)
		}
	}
}

func TestSecurityHeadersStaticHeaders(t *testing.T) {
	rec, _ := serveSecurity(t, false)
	tests := []struct{ header, want string }{
		{"X-Content-Type-Options", "nosniff"},
		{"X-Frame-Options", "DENY"},
		{"Referrer-Policy", "no-referrer"},
		{"Permissions-Policy", permissionsPolicy},
		{"Cross-Origin-Opener-Policy", "same-origin"},
	}
	for _, tc := range tests {
		if got := rec.Header().Get(tc.header); got != tc.want {
			t.Errorf("%s = %q, want %q", tc.header, got, tc.want)
		}
	}
	// COEP is deliberately NOT set: require-corp would break legitimate
	// cross-origin resource loads. Assert its absence so any future addition is a
	// conscious decision rather than an accident.
	if got := rec.Header().Get("Cross-Origin-Embedder-Policy"); got != "" {
		t.Errorf("Cross-Origin-Embedder-Policy = %q, want unset", got)
	}
}

func TestSecurityHeadersHSTS(t *testing.T) {
	tests := []struct {
		name string
		hsts bool
		want string
	}{
		{"production sets HSTS", true, "max-age=63072000; includeSubDomains"},
		{"non-production omits HSTS", false, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec, _ := serveSecurity(t, tc.hsts)
			if got := rec.Header().Get("Strict-Transport-Security"); got != tc.want {
				t.Errorf("HSTS = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNoncePerRequestUnique(t *testing.T) {
	_, n1 := serveSecurity(t, false)
	_, n2 := serveSecurity(t, false)
	if n1 == n2 {
		t.Errorf("nonce reused across requests: %q", n1)
	}
	for _, n := range []string{n1, n2} {
		b, err := base64.StdEncoding.DecodeString(n)
		if err != nil {
			t.Errorf("nonce %q is not valid base64: %v", n, err)
		}
		if len(b) != 16 {
			t.Errorf("nonce decodes to %d bytes, want 16", len(b))
		}
	}
}
