package httpx

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// RealIP resolves the client IP and stores it in the context (read via the RealIP
// accessor). Client-IP headers are spoofable, so they are trusted ONLY when the
// DIRECT peer (r.RemoteAddr) is inside one of the trusted proxy CIDRs — e.g.
// Cloudflare's ranges. From any untrusted source the headers are ignored and the
// direct peer address is used. With an empty trusted list, headers are never used.
func RealIP(trusted []netip.Prefix) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), realIPKey, resolveClientIP(r, trusted))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func resolveClientIP(r *http.Request, trusted []netip.Prefix) string {
	peer := peerAddr(r.RemoteAddr)
	if !peer.IsValid() {
		return r.RemoteAddr
	}
	if !ipTrusted(peer, trusted) {
		return peer.String() // untrusted direct peer → never trust headers
	}

	// Trusted proxy in front. Cloudflare sets CF-Connecting-IP to the true client.
	if cf := strings.TrimSpace(r.Header.Get("CF-Connecting-IP")); cf != "" {
		if a, err := netip.ParseAddr(cf); err == nil {
			return a.Unmap().String()
		}
	}

	// Otherwise walk X-Forwarded-For right-to-left and return the first address
	// that is not itself a trusted proxy — that is the real client.
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			a, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
			if err != nil {
				continue
			}
			a = a.Unmap()
			if ipTrusted(a, trusted) {
				continue
			}
			return a.String()
		}
	}
	return peer.String()
}

func peerAddr(remoteAddr string) netip.Addr {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr // may already be a bare IP
	}
	a, err := netip.ParseAddr(strings.TrimSpace(host))
	if err != nil {
		return netip.Addr{}
	}
	return a.Unmap()
}

func ipTrusted(a netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
