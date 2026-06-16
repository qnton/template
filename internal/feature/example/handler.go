// Package example is the reference feature slice: a minimal DB-backed list with
// create/delete over HTMX. It demonstrates the whole pattern — register pattern,
// CSRF-protected mutations, parameterized sqlc queries, partial re-renders — with
// no JavaScript of its own.
//
// It is intentionally REMOVABLE: delete this package, delete its migration, and
// delete the one line in internal/feature/registry to remove it cleanly.
package example

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/example/app/internal/core/app"
	"github.com/example/app/internal/core/assets"
	"github.com/example/app/internal/core/httpx"
	"github.com/example/app/internal/core/validate"
	"github.com/example/app/internal/feature/example/store"
)

const (
	maxTitleLen = 200
	listLimit   = 100
)

// Module bundles the feature's dependencies and HTTP handlers. It depends on the
// generated store.Querier INTERFACE (not the concrete *Queries), so tests can
// inject a fake and the data layer stays swappable.
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

// Routes registers the feature's endpoints. "GET /{$}" matches only the root path
// (not a catch-all), so removing the feature does not strand other routes.
func (m *Module) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", m.index)
	mux.HandleFunc("POST /items", m.create)
	mux.HandleFunc("DELETE /items/{id}", m.delete)
}

func (m *Module) index(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	items, err := m.list(ctx)
	if err != nil {
		m.serverError(w, r, err)
		return
	}
	if err := httpx.RenderHTML(w, r, http.StatusOK, Page(m.assets, items)); err != nil {
		m.log.ErrorContext(ctx, "render index", slog.Any("error", err))
	}
}

func (m *Module) create(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	title := strings.TrimSpace(r.FormValue("title"))

	v := validate.New()
	v.Required("title", title)
	v.MaxLen("title", title, maxTitleLen)
	if !v.Valid() {
		// Re-render the panel with the rejected value and its message. 422 is
		// configured as swappable in the htmx-config meta tag (see layout).
		m.renderPanel(w, r, ctx, http.StatusUnprocessableEntity, title, v.Errors["title"])
		return
	}

	if _, err := m.q.CreateItem(ctx, title); err != nil {
		m.serverError(w, r, err)
		return
	}
	m.renderPanel(w, r, ctx, http.StatusOK, "", "")
}

func (m *Module) delete(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := m.q.DeleteItem(ctx, id); err != nil {
		m.serverError(w, r, err)
		return
	}
	m.renderPanel(w, r, ctx, http.StatusOK, "", "")
}

// renderPanel re-renders the create-form + list region as an HTMX partial.
func (m *Module) renderPanel(w http.ResponseWriter, r *http.Request, ctx context.Context, status int, formTitle, formErr string) {
	items, err := m.list(ctx)
	if err != nil {
		m.serverError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := Panel(items, formTitle, formErr).Render(ctx, w); err != nil {
		m.log.ErrorContext(ctx, "render panel", slog.Any("error", err))
	}
}

func (m *Module) list(ctx context.Context) ([]store.Item, error) {
	return m.q.ListItems(ctx, int32(listLimit))
}

func (m *Module) serverError(w http.ResponseWriter, r *http.Request, err error) {
	m.log.ErrorContext(r.Context(), "example handler error", slog.Any("error", err))
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}
