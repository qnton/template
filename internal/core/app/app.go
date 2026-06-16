package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/example/app/internal/core/assets"
	"github.com/example/app/internal/core/config"
	"github.com/example/app/internal/core/httpx"
)

// App is the assembled application: a routed, middleware-wrapped handler plus the
// configuration needed to run and gracefully stop the HTTP server.
type App struct {
	cfg     *config.Config
	logger  *slog.Logger
	handler http.Handler
	limiter *httpx.RateLimiter
}

// New wires the router and the middleware chain. Routes: the static asset
// handler, /healthz (liveness) and /readyz (readiness), then every feature's own
// routes. The chain is applied outermost-first (see ordering below).
func New(deps Deps, features ...Feature) *App {
	cfg := deps.Config

	mux := http.NewServeMux()
	mux.Handle("GET "+assets.URLPrefix, deps.Assets.Handler())
	mux.HandleFunc("GET /healthz", liveness())
	mux.HandleFunc("GET /readyz", readiness(deps.Pool, cfg.HealthCheckTimeout))

	for _, f := range features {
		f.Routes(mux)
	}

	// Outermost → innermost. RequestID is outermost so the ID tags the panic and
	// request logs and is echoed even on panic; Recover wraps everything below it;
	// logging observes the final status; real-IP runs before the rate limiter
	// (which keys on it); security headers set the nonce in context before
	// handlers render; CSRF runs after the size limit so form parsing is bounded.
	chain := []httpx.Middleware{
		httpx.RequestID(cfg.TrustedProxies),
		httpx.Recover(deps.Logger),
		httpx.RequestLogger(deps.Logger),
		httpx.RealIP(cfg.TrustedProxies),
	}

	var limiter *httpx.RateLimiter
	if cfg.RateLimitEnabled {
		limiter = httpx.NewRateLimiter(cfg.RateLimitRPS, cfg.RateLimitBurst)
		chain = append(chain, limiter.Middleware)
	}

	chain = append(chain, httpx.SecurityHeaders(cfg.IsProduction()))
	if cfg.GzipEnabled {
		chain = append(chain, httpx.Gzip())
	}
	chain = append(chain,
		httpx.RequestSize(cfg.MaxRequestBytes),
		deps.CSRF.Middleware,
	)

	return &App{
		cfg:     cfg,
		logger:  deps.Logger,
		handler: httpx.Chain(mux, chain...),
		limiter: limiter,
	}
}

// Run starts the server and blocks until ctx is cancelled (signal) or the server
// fails, then shuts down gracefully within ShutdownTimeout.
func (a *App) Run(ctx context.Context) error {
	if a.limiter != nil {
		a.limiter.StartSweeper(ctx)
	}
	if a.cfg.PprofEnabled {
		a.startPprof(ctx)
	}

	srv := &http.Server{
		Addr:              a.cfg.Addr,
		Handler:           a.handler,
		ReadTimeout:       a.cfg.ReadTimeout,
		ReadHeaderTimeout: a.cfg.ReadHeaderTimeout,
		WriteTimeout:      a.cfg.WriteTimeout,
		IdleTimeout:       a.cfg.IdleTimeout,
		ErrorLog:          slog.NewLogLogger(a.logger.Handler(), slog.LevelError),
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}

	errCh := make(chan error, 1)
	go func() {
		a.logger.Info("server listening", slog.String("addr", a.cfg.Addr), slog.String("env", a.cfg.Env))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		a.logger.Info("shutdown signal received")
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	a.logger.Info("server stopped cleanly")
	return nil
}

// liveness reports that the process is up. It deliberately does NOT touch the
// database, so an orchestrator's liveness probe never restarts the app for a
// transient DB outage (that is the readiness probe's job).
func liveness() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
	}
}

// readiness reports whether the app can serve traffic by pinging the database
// within timeout. It backs readiness probes and the container HEALTHCHECK.
func readiness(pool *pgxpool.Pool, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			http.Error(w, "unready", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ready"))
	}
}

// startPprof serves the profiling endpoints on a SEPARATE, internal listener
// (PprofAddr defaults to 127.0.0.1:6060) so profiles are never publicly exposed.
func (a *App) startPprof(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	srv := &http.Server{Addr: a.cfg.PprofAddr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		a.logger.Info("pprof listening (internal only)", slog.String("addr", a.cfg.PprofAddr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.logger.Error("pprof server error", slog.Any("error", err))
		}
	}()
	go func() { //nolint:gosec // G118: shutdown waiter; ctx is already cancelled, so a fresh background ctx is required
		<-ctx.Done()
		// ctx is already cancelled here, so derive a fresh deadline bounded by the
		// same ShutdownTimeout budget the main server uses (not a magic constant).
		sc, cancel := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(sc)
	}()
}
