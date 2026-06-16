package httpx

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

// FuzzResolveClientIP asserts client-IP resolution never panics on malformed
// RemoteAddr / X-Forwarded-For / CF-Connecting-IP values.
func FuzzResolveClientIP(f *testing.F) {
	seeds := []struct{ remote, xff, cf string }{
		{"203.0.113.5:1234", "1.1.1.1", "2.2.2.2"},
		{"10.0.0.1:443", "198.51.100.7, 192.168.1.1, 10.0.0.9", ""},
		{"", "", ""},
		{"[::1]:80", "::1", ""},
		{"garbage", ",,,", "  "},
		{"1.2.3.4", "  ,1.2.3.4", "not-an-ip"},
	}
	for _, s := range seeds {
		f.Add(s.remote, s.xff, s.cf)
	}

	trusted := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"),
		netip.MustParsePrefix("192.168.0.0/16"),
	}
	f.Fuzz(func(t *testing.T, remote, xff, cf string) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = remote
		req.Header.Set("X-Forwarded-For", xff)
		req.Header.Set("CF-Connecting-IP", cf)
		_ = resolveClientIP(req, trusted) // must never panic
	})
}
