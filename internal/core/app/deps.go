// Package app is the stable Core seam between the server bootstrap and the
// feature slices. It defines Deps (the dependencies every feature receives) and
// the Feature interface (how a feature registers its routes), then assembles the
// middleware chain and runs the HTTP server.
//
// Features import this package for Deps and Feature only; this package never
// imports a feature (the registry is an explicit list in cmd/server/main.go), so
// there is no import cycle and Core stays independently upgradeable.
package app

import (
	"context"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/app/internal/core/assets"
	"github.com/example/app/internal/core/config"
	"github.com/example/app/internal/core/httpx"
)

// Deps is the deliberately small, stable set of dependencies injected into every
// feature. Keep this lean — adding a field here is a Core change that ripples to
// all features.
type Deps struct {
	Logger *slog.Logger
	Pool   *pgxpool.Pool
	Config *config.Config
	CSRF   *httpx.CSRF
	Assets *assets.Manager
}

// Nonce returns the per-request CSP nonce for the given request context.
func (d Deps) Nonce(ctx context.Context) string { return httpx.Nonce(ctx) }

// CSRFToken returns the masked per-request CSRF token for the given context.
func (d Deps) CSRFToken(ctx context.Context) string { return httpx.CSRFToken(ctx) }

// Feature is a self-contained slice of the app. It registers its own routes on
// the shared mux; patterns should be fully qualified, e.g. "GET /items".
type Feature interface {
	Routes(mux *http.ServeMux)
}
