package httpx

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/netip"
)

// RequestIDHeader is the header used to read an inbound request ID and to echo
// the resolved one back on the response.
const RequestIDHeader = "X-Request-Id"

// RequestID assigns every request a unique ID for log correlation, echoes it in
// the X-Request-Id response header, and stores it in the context (read via
// RequestIDOf). An inbound X-Request-Id is honored ONLY when the direct peer is a
// trusted proxy (the same trust model as RealIP) and the value is short and
// printable; otherwise a fresh random ID is minted. This stops an untrusted
// client from injecting forged or malicious IDs into logs and downstream headers.
//
// Install it as the OUTERMOST middleware so the ID is present for the panic
// recover log and the request log, and is emitted even when a handler panics.
func RequestID(trusted []netip.Prefix) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := ""
			if inbound := r.Header.Get(RequestIDHeader); inbound != "" {
				if peer := peerAddr(r.RemoteAddr); peer.IsValid() && ipTrusted(peer, trusted) {
					id = sanitizeRequestID(inbound)
				}
			}
			if id == "" {
				id = newRequestID()
			}
			w.Header().Set(RequestIDHeader, id)
			ctx := context.WithValue(r.Context(), requestIDKey, id)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequestIDOf returns the request ID set by the RequestID middleware, or "".
func RequestIDOf(ctx context.Context) string { return stringValue(ctx, requestIDKey) }

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("httpx: crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// sanitizeRequestID accepts an inbound ID only when it is short and made solely
// of printable, non-space ASCII; otherwise it returns "" so a fresh ID is minted.
// This blocks header/log injection (CR/LF, control bytes) via a forwarded value.
func sanitizeRequestID(s string) string {
	if len(s) == 0 || len(s) > 64 {
		return ""
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x21 || c > 0x7e {
			return ""
		}
	}
	return s
}

// requestIDLogHandler decorates a slog.Handler so every record carries the
// request ID from its context, correlating handler logs with the request log
// without each call site threading the ID explicitly.
type requestIDLogHandler struct{ slog.Handler }

func (h requestIDLogHandler) Handle(ctx context.Context, r slog.Record) error {
	if id := RequestIDOf(ctx); id != "" {
		r.AddAttrs(slog.String("request_id", id))
	}
	return h.Handler.Handle(ctx, r)
}

func (h requestIDLogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return requestIDLogHandler{h.Handler.WithAttrs(attrs)}
}

func (h requestIDLogHandler) WithGroup(name string) slog.Handler {
	return requestIDLogHandler{h.Handler.WithGroup(name)}
}

// WithRequestIDLogging wraps a logger so its records carry the context request
// ID. Apply it once at startup to the logger stored in Deps.
func WithRequestIDLogging(l *slog.Logger) *slog.Logger {
	return slog.New(requestIDLogHandler{l.Handler()})
}
