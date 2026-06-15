package httpx

import (
	"compress/gzip"
	"io"
	"net/http"
	"strings"
	"sync"
)

var gzipPool = sync.Pool{New: func() any { return gzip.NewWriter(io.Discard) }}

// Gzip compresses textual responses using the standard library when the client
// advertises gzip support. This is OPTIONAL defense-in-depth: behind Cloudflare
// the edge already negotiates gzip/brotli. No brotli dependency is added — the
// stdlib has none and the edge covers it.
func Gzip() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Add("Vary", "Accept-Encoding")
			gw := &gzipResponseWriter{ResponseWriter: w}
			defer gw.close()
			next.ServeHTTP(gw, r)
		})
	}
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	wroteHeader bool
	compress    bool
}

func (g *gzipResponseWriter) Unwrap() http.ResponseWriter { return g.ResponseWriter }

func (g *gzipResponseWriter) WriteHeader(code int) {
	if g.wroteHeader {
		return
	}
	g.wroteHeader = true

	h := g.Header()
	if bodyAllowedForStatus(code) && h.Get("Content-Encoding") == "" && isCompressible(h.Get("Content-Type")) {
		h.Del("Content-Length") // length changes after compression
		h.Set("Content-Encoding", "gzip")
		g.gz = gzipPool.Get().(*gzip.Writer)
		g.gz.Reset(g.ResponseWriter)
		g.compress = true
	}
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	if !g.wroteHeader {
		if g.Header().Get("Content-Type") == "" {
			g.Header().Set("Content-Type", http.DetectContentType(b))
		}
		g.WriteHeader(http.StatusOK)
	}
	if g.compress {
		return g.gz.Write(b)
	}
	return g.ResponseWriter.Write(b)
}

func (g *gzipResponseWriter) Flush() {
	if g.compress && g.gz != nil {
		_ = g.gz.Flush()
	}
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (g *gzipResponseWriter) close() {
	if g.gz != nil {
		_ = g.gz.Close()
		gzipPool.Put(g.gz)
		g.gz = nil
	}
}

func isCompressible(contentType string) bool {
	if i := strings.IndexByte(contentType, ';'); i >= 0 {
		contentType = contentType[:i]
	}
	switch strings.TrimSpace(contentType) {
	case "text/html", "text/css", "text/plain", "text/xml",
		"application/javascript", "text/javascript", "application/json",
		"application/xml", "image/svg+xml", "application/wasm":
		return true
	default:
		return false
	}
}

func bodyAllowedForStatus(code int) bool {
	switch {
	case code >= 100 && code < 200:
		return false
	case code == http.StatusNoContent, code == http.StatusNotModified:
		return false
	default:
		return true
	}
}
