package httpx

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecoverReturns500(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("boom")
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	if got := rec.Header().Get("Connection"); got != "close" {
		t.Errorf("Connection = %q, want close", got)
	}
	if body := rec.Body.String(); strings.Contains(body, "boom") || strings.Contains(body, "goroutine") {
		t.Errorf("client body leaks internals: %q", body)
	}
	logs := buf.String()
	for _, want := range []string{"panic recovered", "boom", "stack"} {
		if !strings.Contains(logs, want) {
			t.Errorf("log missing %q; got %s", want, logs)
		}
	}
}

func TestRecoverRePanicsAbortHandler(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		r := recover()
		if r != http.ErrAbortHandler {
			t.Fatalf("recovered = %v, want http.ErrAbortHandler (must propagate)", r)
		}
		if strings.Contains(buf.String(), "panic recovered") {
			t.Errorf("ErrAbortHandler must not be logged as a recovered panic")
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	t.Fatal("expected ErrAbortHandler to propagate")
}

func TestRecoverNoPanicPassthrough(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := Recover(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("passthrough broken: code=%d body=%q", rec.Code, rec.Body.String())
	}
	if buf.Len() != 0 {
		t.Errorf("no log expected on success; got %s", buf.String())
	}
}
