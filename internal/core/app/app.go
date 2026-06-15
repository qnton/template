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
// handler, /healthz, then every feature's own routes. The chain is applied
// outermost-first (see ordering below).
func New(deps Deps, features ...Feature) *App {
	mux := http.NewServeMux()
	mux.Handle("GET "+assets.URLPrefix, deps.Assets.Handler())
	mux.HandleFunc("GET /healthz", healthz(deps.Pool))

	for _, f := range features {
		f.Routes(mux)
	}

	cfg := deps.Config

	// Outermost → innermost. Recover wraps everything; logging observes the
	// final status; real-IP runs before the rate limiter (which keys on it);
	// security headers set the nonce in context before handlers render; CSRF runs
	// after the size limit so form parsing is bounded.
	chain := []httpx.Middleware{
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

// healthz reports readiness by pinging the database with a short deadline.
func healthz(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := pool.Ping(ctx); err != nil {
			http.Error(w, "unhealthy", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok"))
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
	go func() {
		<-ctx.Done()
		sc, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sc)
	}()
}
