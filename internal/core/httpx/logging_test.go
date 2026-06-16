package httpx

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStatusRecorder(t *testing.T) {
	tests := []struct {
		name       string
		writeCode  int // 0 = don't call WriteHeader
		secondCode int // 0 = no second WriteHeader
		writes     []string
		wantStatus int
		wantBytes  int64
	}{
		{"explicit 201", 201, 0, []string{"abc"}, 201, 3},
		{"default 200 without WriteHeader", 0, 0, []string{"hello"}, 200, 5},
		{"first WriteHeader wins", 404, 500, nil, 404, 0},
		{"multi-write accumulates", 200, 0, []string{"ab", "cde"}, 200, 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			under := httptest.NewRecorder()
			rec := &statusRecorder{ResponseWriter: under, status: http.StatusOK}
			if tc.writeCode != 0 {
				rec.WriteHeader(tc.writeCode)
			}
			if tc.secondCode != 0 {
				rec.WriteHeader(tc.secondCode)
			}
			for _, w := range tc.writes {
				if _, err := rec.Write([]byte(w)); err != nil {
					t.Fatalf("write: %v", err)
				}
			}
			if rec.status != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.status, tc.wantStatus)
			}
			if rec.bytes != tc.wantBytes {
				t.Errorf("bytes = %d, want %d", rec.bytes, tc.wantBytes)
			}
		})
	}
}

type nonFlusher struct{ http.ResponseWriter }

func TestStatusRecorderFlush(t *testing.T) {
	// Underlying recorder implements Flusher: Flush passes through, no panic.
	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder(), status: http.StatusOK}
	rec.Flush()

	// Non-flushing writer: Flush must be a safe no-op.
	nf := &statusRecorder{ResponseWriter: nonFlusher{httptest.NewRecorder()}, status: http.StatusOK}
	nf.Flush()
}

func TestRequestLoggerLogsLine(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := RequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("data"))
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/things", nil))

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("log line is not JSON: %v (%s)", err, buf.String())
	}
	if entry["msg"] != "request" {
		t.Errorf("msg = %v, want request", entry["msg"])
	}
	if entry["method"] != "POST" {
		t.Errorf("method = %v, want POST", entry["method"])
	}
	if entry["path"] != "/things" {
		t.Errorf("path = %v, want /things", entry["path"])
	}
	if status, _ := entry["status"].(float64); status != 201 {
		t.Errorf("status = %v, want 201", entry["status"])
	}
	if b, _ := entry["bytes"].(float64); b != 4 {
		t.Errorf("bytes = %v, want 4", entry["bytes"])
	}
	if _, ok := entry["duration"]; !ok {
		t.Error("missing duration attribute")
	}
}
