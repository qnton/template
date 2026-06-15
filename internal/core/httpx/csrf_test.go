package httpx

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMaskUnmaskRoundTrip(t *testing.T) {
	real := randomBytes(csrfTokenLen)

	a := maskToken(real)
	b := maskToken(real)
	if a == b {
		t.Fatal("two masks of the same token should differ (BREACH mitigation)")
	}
	if got := unmaskToken(a); !bytes.Equal(got, real) {
		t.Fatalf("unmask(mask(real)) != real")
	}
	if unmaskToken("not-base64!!") != nil {
		t.Error("malformed token should unmask to nil")
	}
	if unmaskToken("") != nil {
		t.Error("empty token should unmask to nil")
	}
}

func TestCSRFFlow(t *testing.T) {
	c := NewCSRF(false)
	var issued string
	h := c.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		issued = CSRFToken(r.Context())
		w.WriteHeader(http.StatusOK)
	}))

	// A safe GET issues the cookie and exposes a masked token.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if issued == "" {
		t.Fatal("GET should expose a masked CSRF token")
	}
	cookie := findCookie(rec.Result().Cookies(), csrfCookieDev)
	if cookie == nil {
		t.Fatal("GET should set the CSRF cookie")
	}

	// POST without a token is rejected.
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(cookie)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST without token = %d, want 403", rec.Code)
	}

	// POST with the masked token in the header is accepted.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(cookie)
	req.Header.Set(csrfHeaderName, issued)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST with valid token = %d, want 200", rec.Code)
	}

	// A token that does not match the cookie is rejected.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/", nil)
	req.AddCookie(cookie)
	req.Header.Set(csrfHeaderName, maskToken(randomBytes(csrfTokenLen)))
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("POST with foreign token = %d, want 403", rec.Code)
	}
}

func findCookie(cookies []*http.Cookie, name string) *http.Cookie {
	for _, ck := range cookies {
		if ck.Name == name {
			return ck
		}
	}
	return nil
}
