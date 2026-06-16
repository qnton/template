// Command server is the application entry point. It wires configuration, the
// logger, the database pool and the asset manager into the Core app, registers
// the feature slices, and runs the server with graceful shutdown.
//
// The -healthcheck subcommand probes /readyz and exits 0/1; it backs the
// container HEALTHCHECK, since the distroless image ships no shell or curl.
package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	root "github.com/example/app"
	"github.com/example/app/internal/core/app"
	"github.com/example/app/internal/core/assets"
	"github.com/example/app/internal/core/config"
	"github.com/example/app/internal/core/db"
	"github.com/example/app/internal/core/httpx"
	"github.com/example/app/internal/core/jobs"
	"github.com/example/app/internal/feature/registry"
)

// version is stamped at build time via -ldflags "-X main.version=…".
var version = "dev"

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe /readyz and exit 0/1 (for container HEALTHCHECK)")
	flag.Parse()

	if *healthcheck {
		os.Exit(runHealthcheck())
	}

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	// Load config first so the logger can honor LOG_LEVEL. Config errors are
	// aggregated and surfaced to stderr by main() before any logger exists.
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	logger = httpx.WithRequestIDLogging(logger) // every record carries its request_id
	slog.SetDefault(logger)
	logger.Info("starting", slog.String("version", version))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	dbCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pool, err := db.New(dbCtx, cfg)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer pool.Close()

	staticFS, err := fs.Sub(root.StaticFS, "static")
	if err != nil {
		return fmt.Errorf("sub static fs: %w", err)
	}
	assetMgr, err := assets.NewManager(staticFS)
	if err != nil {
		return fmt.Errorf("init assets: %w", err)
	}

	deps := app.Deps{
		Logger: logger,
		Pool:   pool,
		Config: cfg,
		CSRF:   httpx.NewCSRF(cfg.IsProduction()),
		Assets: assetMgr,
	}

	// Background jobs worker (optional; see internal/core/jobs). It shares the
	// signal context, so SIGINT/SIGTERM stops it alongside the server.
	if cfg.JobsEnabled {
		worker := jobs.NewWorker(pool, logger, jobs.Options{PollInterval: cfg.JobsPollInterval})
		registry.RegisterJobs(deps, worker)
		go worker.Run(ctx)
		logger.Info("background jobs worker enabled")
	}

	return app.New(deps, registry.Features(deps)...).Run(ctx)
}

// runHealthcheck performs an HTTP GET against the local /readyz and maps the
// result to a process exit code. Used by the container HEALTHCHECK.
func runHealthcheck() int {
	host, port, err := net.SplitHostPort(envOr("APP_ADDR", ":8080"))
	if err != nil {
		host, port = "127.0.0.1", "8080"
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	url := "http://" + net.JoinHostPort(host, port) + "/readyz"

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
		return 1
	}
	return 0
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
