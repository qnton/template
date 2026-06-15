package example

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/example/app/internal/core/assets"
	"github.com/example/app/internal/feature/example/store"
)

// fakeStore is an in-memory store.Querier for fast, DB-free handler tests.
type fakeStore struct {
	items     []store.Item
	createErr error
	listErr   error
}

var _ store.Querier = (*fakeStore)(nil)

func (f *fakeStore) ListItems(_ context.Context, limit int32) ([]store.Item, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	if int(limit) < len(f.items) {
		return f.items[:limit], nil
	}
	return f.items, nil
}

func (f *fakeStore) CreateItem(_ context.Context, title string) (store.Item, error) {
	if f.createErr != nil {
		return store.Item{}, f.createErr
	}
	it := store.Item{ID: int64(len(f.items) + 1), Title: title, CreatedAt: time.Now()}
	f.items = append([]store.Item{it}, f.items...) // newest first
	return it, nil
}

func (f *fakeStore) DeleteItem(_ context.Context, id int64) error {
	out := f.items[:0]
	for _, it := range f.items {
		if it.ID != id {
			out = append(out, it)
		}
	}
	f.items = out
	return nil
}

func (f *fakeStore) CountItems(_ context.Context) (int64, error) { return int64(len(f.items)), nil }

func testAssets(tb testing.TB) *assets.Manager {
	tb.Helper()
	m, err := assets.NewManager(fstest.MapFS{
		"css/app.css":                 {Data: []byte("/*css*/")},
		"js/htmx.min.js":              {Data: []byte("/*htmx*/")},
		"js/core.mjs":                 {Data: []byte("/*core*/")},
		"js/islands/theme-toggle.mjs": {Data: []byte("/*toggle*/")},
	})
	if err != nil {
		tb.Fatalf("assets: %v", err)
	}
	return m
}

func newTestModule(tb testing.TB, f *fakeStore) (*Module, *http.ServeMux) {
	tb.Helper()
	m := &Module{
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		assets: testAssets(tb),
		q:      f,
	}
	mux := http.NewServeMux()
	m.Routes(mux)
	return m, mux
}

func TestIndexRendersItems(t *testing.T) {
	_, mux := newTestModule(t, &fakeStore{items: []store.Item{
		{ID: 1, Title: "hello world", CreatedAt: time.Now()},
	}})

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Example feature", "hello world", "importmap"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q", want)
		}
	}
}

func TestCreateAddsItem(t *testing.T) {
	f := &fakeStore{}
	_, mux := newTestModule(t, f)

	form := url.Values{"title": {"a new item"}}
	req := httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if len(f.items) != 1 || f.items[0].Title != "a new item" {
		t.Fatalf("item not created: %+v", f.items)
	}
	if !strings.Contains(rec.Body.String(), "a new item") {
		t.Error("response panel missing the new item")
	}
}

func TestCreateRejectsBlankTitle(t *testing.T) {
	f := &fakeStore{}
	_, mux := newTestModule(t, f)

	form := url.Values{"title": {"   "}}
	req := httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", rec.Code)
	}
	if len(f.items) != 0 {
		t.Fatalf("blank title should not create an item: %+v", f.items)
	}
	if !strings.Contains(rec.Body.String(), "is required") {
		t.Error("expected validation message in response")
	}
}

func TestDeleteRemovesItem(t *testing.T) {
	f := &fakeStore{items: []store.Item{
		{ID: 1, Title: "a", CreatedAt: time.Now()},
		{ID: 2, Title: "b", CreatedAt: time.Now()},
	}}
	_, mux := newTestModule(t, f)

	req := httptest.NewRequest(http.MethodDelete, "/items/1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if len(f.items) != 1 || f.items[0].ID != 2 {
		t.Fatalf("item 1 not deleted: %+v", f.items)
	}
}

// BenchmarkIndex measures the hottest path: list query (faked) + full-page render.
func BenchmarkIndex(b *testing.B) {
	items := make([]store.Item, 100)
	now := time.Now()
	for i := range items {
		items[i] = store.Item{ID: int64(i + 1), Title: "benchmark item", CreatedAt: now}
	}
	_, mux := newTestModule(b, &fakeStore{items: items})
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status %d", rec.Code)
		}
	}
}
