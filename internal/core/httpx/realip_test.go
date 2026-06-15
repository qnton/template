package httpx

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{
		netip.MustParsePrefix("10.0.0.0/8"), // pretend "proxy" range
		netip.MustParsePrefix("192.168.0.0/16"),
	}

	tests := []struct {
		name    string
		remote  string
		headers map[string]string
		want    string
	}{
		{
			name:   "untrusted peer: headers ignored",
			remote: "203.0.113.5:1234",
			headers: map[string]string{
				"X-Forwarded-For":  "1.1.1.1",
				"CF-Connecting-IP": "2.2.2.2",
			},
			want: "203.0.113.5",
		},
		{
			name:    "trusted peer: CF-Connecting-IP wins",
			remote:  "10.1.2.3:443",
			headers: map[string]string{"CF-Connecting-IP": "198.51.100.7", "X-Forwarded-For": "9.9.9.9"},
			want:    "198.51.100.7",
		},
		{
			name:    "trusted peer: rightmost untrusted XFF entry",
			remote:  "10.0.0.1:443",
			headers: map[string]string{"X-Forwarded-For": "198.51.100.7, 192.168.1.1, 10.0.0.9"},
			want:    "198.51.100.7",
		},
		{
			name:    "trusted peer, no headers: falls back to peer",
			remote:  "10.0.0.1:443",
			headers: nil,
			want:    "10.0.0.1",
		},
		{
			name:    "spoofed CF header from untrusted peer is ignored",
			remote:  "203.0.113.9:5",
			headers: map[string]string{"CF-Connecting-IP": "10.0.0.1"},
			want:    "203.0.113.9",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tc.remote
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			if got := resolveClientIP(req, trusted); got != tc.want {
				t.Errorf("clientIP = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestClientIPNoTrustedProxies(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1:443"
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	if got := resolveClientIP(req, nil); got != "10.0.0.1" {
		t.Errorf("with no trusted proxies, headers must be ignored; got %q", got)
	}
}
