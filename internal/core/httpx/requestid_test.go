package httpx

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func serveRequestID(t *testing.T, trusted []netip.Prefix, remote, inbound string) (*httptest.ResponseRecorder, string) {
	t.Helper()
	var gotID string
	h := RequestID(trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID = RequestIDOf(r.Context())
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if remote != "" {
		req.RemoteAddr = remote
	}
	if inbound != "" {
		req.Header.Set(RequestIDHeader, inbound)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, gotID
}

func TestRequestIDGeneratesAndEchoes(t *testing.T) {
	rec, id := serveRequestID(t, nil, "203.0.113.5:1234", "")
	if id == "" {
		t.Fatal("no request ID stored in context")
	}
	if got := rec.Header().Get(RequestIDHeader); got != id {
		t.Errorf("X-Request-Id header = %q, want %q (the context value)", got, id)
	}
	if len(id) != 32 { // 16 random bytes, hex-encoded
		t.Errorf("generated id length = %d, want 32 hex chars", len(id))
	}
}

func TestRequestIDUniquePerRequest(t *testing.T) {
	_, a := serveRequestID(t, nil, "203.0.113.5:1234", "")
	_, b := serveRequestID(t, nil, "203.0.113.5:1234", "")
	if a == b {
		t.Errorf("request IDs should differ across requests: %q", a)
	}
}

func TestRequestIDHonorsTrustedInbound(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	_, id := serveRequestID(t, trusted, "10.1.2.3:443", "abc123")
	if id != "abc123" {
		t.Errorf("trusted inbound id = %q, want abc123", id)
	}
}

func TestRequestIDRejectsUntrustedInbound(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	_, id := serveRequestID(t, trusted, "203.0.113.9:5", "spoofed")
	if id == "spoofed" {
		t.Error("inbound id from an untrusted peer must be ignored")
	}
	if len(id) != 32 {
		t.Errorf("expected a freshly generated id; got %q", id)
	}
}

func TestSanitizeRequestID(t *testing.T) {
	tests := []struct{ in, want string }{
		{"abc123", "abc123"},
		{"", ""},
		{"has space", ""},
		{"line\nbreak", ""},
		{"tab\there", ""},
	}
	for _, tc := range tests {
		if got := sanitizeRequestID(tc.in); got != tc.want {
			t.Errorf("sanitizeRequestID(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	if got := sanitizeRequestID(strings.Repeat("a", 65)); got != "" {
		t.Errorf("over-length id should be rejected, got %q", got)
	}
}

func TestWithRequestIDLogging(t *testing.T) {
	var buf bytes.Buffer
	logger := WithRequestIDLogging(slog.New(slog.NewJSONHandler(&buf, nil)))

	ctx := context.WithValue(context.Background(), requestIDKey, "rid-123")
	logger.InfoContext(ctx, "hello")
	if !strings.Contains(buf.String(), `"request_id":"rid-123"`) {
		t.Errorf("log line missing request_id; got %s", buf.String())
	}

	buf.Reset()
	logger.InfoContext(context.Background(), "hello")
	if strings.Contains(buf.String(), "request_id") {
		t.Errorf("no request_id expected when absent from context; got %s", buf.String())
	}
}
