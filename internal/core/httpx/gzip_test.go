package httpx

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveGzip(t *testing.T, acceptEncoding string, handler http.HandlerFunc) *httptest.ResponseRecorder {
	t.Helper()
	h := Gzip()(handler)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if acceptEncoding != "" {
		req.Header.Set("Accept-Encoding", acceptEncoding)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestGzipCompression(t *testing.T) {
	const body = "<!DOCTYPE html><html><head><title>x</title></head><body>hello hello hello</body></html>"
	tests := []struct {
		name        string
		contentType string
		accept      string
		preEncoded  string // sets Content-Encoding before write
		wantEnc     string
		wantGzipped bool
	}{
		{"html gzipped", "text/html; charset=utf-8", "gzip", "", "gzip", true},
		{"json gzipped", "application/json", "gzip", "", "gzip", true},
		{"css gzipped with q-list", "text/css", "gzip, br", "", "gzip", true},
		{"png not compressed", "image/png", "gzip", "", "", false},
		{"no accept-encoding", "text/html", "", "", "", false},
		{"identity only", "text/html", "identity", "", "", false},
		{"already encoded passthrough", "text/html", "gzip", "br", "br", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveGzip(t, tc.accept, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				if tc.preEncoded != "" {
					w.Header().Set("Content-Encoding", tc.preEncoded)
				}
				_, _ = io.WriteString(w, body)
			})

			if got := rec.Header().Get("Content-Encoding"); got != tc.wantEnc {
				t.Errorf("Content-Encoding = %q, want %q", got, tc.wantEnc)
			}

			wantVary := strings.Contains(tc.accept, "gzip")
			gotVary := strings.Contains(rec.Header().Get("Vary"), "Accept-Encoding")
			if gotVary != wantVary {
				t.Errorf("Vary contains Accept-Encoding = %v, want %v", gotVary, wantVary)
			}

			if tc.wantGzipped {
				zr, err := gzip.NewReader(rec.Body)
				if err != nil {
					t.Fatalf("gzip.NewReader: %v", err)
				}
				out, err := io.ReadAll(zr)
				if err != nil {
					t.Fatalf("gunzip: %v", err)
				}
				if string(out) != body {
					t.Errorf("decompressed = %q, want original", string(out))
				}
			} else if rec.Body.String() != body {
				t.Errorf("body altered when not compressing: %q", rec.Body.String())
			}
		})
	}
}

func TestGzipSniffsContentType(t *testing.T) {
	rec := serveGzip(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
		// No Content-Type set; an HTML body should be sniffed as text/html, which
		// is compressible, so the response is gzipped.
		_, _ = io.WriteString(w, "<!DOCTYPE html><html><body>"+strings.Repeat("x", 100)+"</body></html>")
	})
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("sniffed Content-Type = %q, want text/html prefix", ct)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Errorf("Content-Encoding = %q, want gzip", got)
	}
}

func TestGzipFlush(t *testing.T) {
	rec := serveGzip(t, "gzip", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, "<p>part one</p>")
		f, ok := w.(http.Flusher)
		if !ok {
			t.Errorf("gzip writer does not implement http.Flusher")
			return
		}
		f.Flush()
		_, _ = io.WriteString(w, "<p>part two</p>")
	})
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("gunzip: %v", err)
	}
	if want := "<p>part one</p><p>part two</p>"; string(out) != want {
		t.Errorf("got %q, want %q", string(out), want)
	}
}

func TestBodyAllowedForStatus(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{100, false}, {101, false}, {199, false},
		{200, true}, {204, false}, {304, false},
		{404, true}, {500, true},
	}
	for _, tc := range tests {
		if got := bodyAllowedForStatus(tc.code); got != tc.want {
			t.Errorf("bodyAllowedForStatus(%d) = %v, want %v", tc.code, got, tc.want)
		}
	}
}

func TestIsCompressible(t *testing.T) {
	tests := []struct {
		ct   string
		want bool
	}{
		{"text/html", true},
		{"text/html; charset=utf-8", true},
		{"application/json", true},
		{"application/javascript", true},
		{"image/svg+xml", true},
		{"application/wasm", true},
		{"text/css ; x=y", true},
		{"image/png", false},
		{"application/octet-stream", false},
		{"", false},
	}
	for _, tc := range tests {
		if got := isCompressible(tc.ct); got != tc.want {
			t.Errorf("isCompressible(%q) = %v, want %v", tc.ct, got, tc.want)
		}
	}
}
