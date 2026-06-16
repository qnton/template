package main

// Generation templates (text/template). Go templates are formatted with
// go/format after rendering; .templ/.sql/.mjs/.yaml are written verbatim. Single
// braces ({ ... }) are templ/Go syntax and pass through untouched — only {{ }} is
// interpolated.

// ── Feature: non-DB ──────────────────────────────────────────────────────────

const featureHandlerTmpl = `// Package {{.Name}} is a scaffolded feature slice. It follows the strict feature
// anatomy that ` + "`make structure`" + ` enforces; edit freely.
package {{.Name}}

import (
	"log/slog"
	"net/http"

	"{{.Module}}/internal/core/app"
	"{{.Module}}/internal/core/assets"
	"{{.Module}}/internal/core/httpx"
)

// Module bundles the feature's dependencies and HTTP handlers.
type Module struct {
	log    *slog.Logger
	assets *assets.Manager
}

// New constructs the feature from the stable Core dependencies.
func New(deps app.Deps) *Module {
	return &Module{
		log:    deps.Logger,
		assets: deps.Assets,
	}
}

// Routes registers the feature's endpoints.
func (m *Module) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{{.Name}}", m.index)
}

func (m *Module) index(w http.ResponseWriter, r *http.Request) {
	if err := httpx.RenderHTML(w, r, http.StatusOK, Page(m.assets)); err != nil {
		m.log.ErrorContext(r.Context(), "render {{.Name}} index", slog.Any("error", err))
	}
}
`

const featureViewTmpl = `package {{.Name}}

import (
	"{{.Module}}/internal/core/assets"
	"{{.Module}}/internal/view/component"
	"{{.Module}}/internal/view/layout"
)

// Page is the full HTML document for the {{.Name}} feature.
templ Page(a *assets.Manager) {
	@layout.Base(a, "{{.Title}} · App") {
		@component.PageHeader("{{.Title}}", "A scaffolded feature — edit internal/feature/{{.Name}}/.")
		@component.Card("") {
			@component.CardBody("") {
				@component.EmptyState("Inbox", "Nothing here yet", "Add handlers and views in internal/feature/{{.Name}}/.")
			}
		}
	}
}
`

const featureTestTmpl = `package {{.Name}}

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"{{.Module}}/internal/core/assets"
)

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

func newTestModule(tb testing.TB) *Module {
	tb.Helper()
	return &Module{
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		assets: testAssets(tb),
	}
}

func TestIndexRenders(t *testing.T) {
	m := newTestModule(t)
	mux := http.NewServeMux()
	m.Routes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/{{.Name}}", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "{{.Title}}") {
		t.Errorf("body should contain %q", "{{.Title}}")
	}
}
`

// ── Feature: DB-backed (--db) ────────────────────────────────────────────────

const featureHandlerDBTmpl = `// Package {{.Name}} is a scaffolded DB-backed feature slice. It follows the strict
// feature anatomy that ` + "`make structure`" + ` enforces; edit freely.
package {{.Name}}

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"{{.Module}}/internal/core/app"
	"{{.Module}}/internal/core/assets"
	"{{.Module}}/internal/core/httpx"
	"{{.Module}}/internal/feature/{{.Name}}/store"
)

// Module bundles the feature's dependencies and HTTP handlers. It depends on the
// generated store.Querier INTERFACE so tests can inject a DB-free fake.
type Module struct {
	log    *slog.Logger
	assets *assets.Manager
	q      store.Querier
}

// New constructs the feature from the stable Core dependencies.
func New(deps app.Deps) *Module {
	return &Module{
		log:    deps.Logger,
		assets: deps.Assets,
		q:      store.New(deps.Pool),
	}
}

// Routes registers the feature's endpoints.
func (m *Module) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{{.Name}}", m.index)
}

func (m *Module) index(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	count, err := m.q.Count{{.Title}}(ctx)
	if err != nil {
		m.log.ErrorContext(ctx, "count {{.Name}}", slog.Any("error", err))
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if err := httpx.RenderHTML(w, r, http.StatusOK, Page(m.assets, count)); err != nil {
		m.log.ErrorContext(ctx, "render {{.Name}} index", slog.Any("error", err))
	}
}
`

const featureViewDBTmpl = `package {{.Name}}

import (
	"strconv"

	"{{.Module}}/internal/core/assets"
	"{{.Module}}/internal/view/component"
	"{{.Module}}/internal/view/layout"
)

// Page is the full HTML document for the {{.Name}} feature.
templ Page(a *assets.Manager, count int64) {
	@layout.Base(a, "{{.Title}} · App") {
		@component.PageHeader("{{.Title}}", "A scaffolded DB-backed feature — edit internal/feature/{{.Name}}/.")
		@component.Card("") {
			@component.CardBody("") {
				<p class="text-sm text-muted-foreground">
					{ strconv.FormatInt(count, 10) } row(s) in the <code>{{.Name}}</code> table.
				</p>
			}
		}
	}
}
`

const featureTestDBTmpl = `package {{.Name}}

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"{{.Module}}/internal/core/assets"
	"{{.Module}}/internal/feature/{{.Name}}/store"
)

// fakeStore is an in-memory store.Querier for fast, DB-free handler tests.
type fakeStore struct {
	count int64
}

var _ store.Querier = (*fakeStore)(nil)

func (f *fakeStore) Count{{.Title}}(context.Context) (int64, error) {
	return f.count, nil
}

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

func newTestModule(tb testing.TB, q store.Querier) *Module {
	tb.Helper()
	return &Module{
		log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		assets: testAssets(tb),
		q:      q,
	}
}

func TestIndexRenders(t *testing.T) {
	m := newTestModule(t, &fakeStore{count: 3})
	mux := http.NewServeMux()
	m.Routes(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/{{.Name}}", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{"{{.Title}}", "3"} {
		if !strings.Contains(body, want) {
			t.Errorf("body should contain %q", want)
		}
	}
}
`

const queriesTmpl = `-- name: Count{{.Title}} :one
SELECT count(*) FROM {{.Name}};
`

// ── Migrations ───────────────────────────────────────────────────────────────

const migrationStubTmpl = `-- +goose Up
-- TODO: write the forward migration.

-- +goose Down
-- TODO: write the rollback.
`

const migrationCreateTableTmpl = `-- +goose Up
CREATE TABLE {{.Name}} (
	id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
	created_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE {{.Name}};
`

// ── sqlc block (appended to sqlc.yaml on --db) ───────────────────────────────

const sqlcBlockTmpl = `
  - engine: "postgresql"
    schema: "migrations"
    queries: "internal/feature/{{.Name}}/queries.sql"
    gen:
      go:
        package: "store"
        out: "internal/feature/{{.Name}}/store"
        sql_package: "pgx/v5"
        emit_interface: true
        emit_empty_slices: true
        emit_json_tags: false
        emit_prepared_queries: false
        overrides:
          - db_type: "timestamptz"
            go_type: "time.Time"
`

// ── Island & component ───────────────────────────────────────────────────────

const islandTmpl = `// {{.Name}} island. Activate by adding data-island="{{.Name}}" to an element;
// core.mjs lazy-imports it via the auto-generated import map. Pass options with
// data-opt-* attributes (data-opt-url -> opts.url). Keep all logic here — no
// inline handlers (strict CSP).
export function mount(el, opts) {
  // TODO: implement. ` + "`el`" + ` is the mounted element; ` + "`opts`" + ` are the data-opt-* values.
  console.debug("{{.Name}} mounted", el, opts);
}
`

const componentTmpl = `package component

// {{.Title}} is a scaffolded view component. Edit freely. The class arg appends
// extra utility classes; children render inside.
templ {{.Title}}(class string) {
	<div class={ "{{.Name}}", class }>
		{ children... }
	</div>
}
`
