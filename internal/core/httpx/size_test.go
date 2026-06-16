package httpx

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestSizeRejectsOversize(t *testing.T) {
	const max = 8
	var readErr error
	h := RequestSize(max)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr = io.ReadAll(r.Body)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("x", max+1)))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if readErr == nil {
		t.Fatal("expected a read error for an oversize body")
	}
	if !strings.Contains(readErr.Error(), "request body too large") {
		t.Errorf("error = %v, want 'request body too large'", readErr)
	}
}

func TestRequestSizeAllowsUnderLimit(t *testing.T) {
	const (
		max  = 16
		body = "hello world"
	)
	var got []byte
	var readErr error
	h := RequestSize(max)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, readErr = io.ReadAll(r.Body)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if readErr != nil {
		t.Fatalf("unexpected error: %v", readErr)
	}
	if string(got) != body {
		t.Errorf("body = %q, want %q", got, body)
	}
}

func TestRequestSizeDisabledWhenZero(t *testing.T) {
	const body = "this body is definitely larger than eight bytes"
	var got []byte
	var readErr error
	h := RequestSize(0)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, readErr = io.ReadAll(r.Body)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	h.ServeHTTP(httptest.NewRecorder(), req)

	if readErr != nil {
		t.Fatalf("unexpected error: %v", readErr)
	}
	if string(got) != body {
		t.Errorf("body = %q, want the full body", got)
	}
}
