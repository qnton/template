# Go full-stack starter — secure, minimal, modular

A production-ready starter for full-stack web apps in Go. It optimizes for two
things above all: **maximum security out of the box** and a **minimal
supply-chain surface** — with clean, idiomatic code that is trivial to run locally
and ships as a single distroless container.

**Stack:** Go (stdlib `net/http`, no router) · [templ](https://templ.guide) ·
[HTMX](https://htmx.org) + native JS islands · Tailwind v4 (standalone CLI) ·
PostgreSQL via [pgx](https://github.com/jackc/pgx) + [sqlc](https://sqlc.dev) ·
distroless Docker. **Two runtime dependencies only:** `templ` and `pgx/v5`.

> This is a template. Fork it, rename the module path, build. Pull Core updates
> later with the flow in [`UPGRADING.md`](UPGRADING.md). Working rules and the
> ownership boundary live in [`CLAUDE.md`](CLAUDE.md).

## Quickstart (< 5 minutes)

You only need **Docker** (everything — build, codegen, tests — runs in containers).

```bash
cp .env.example .env

# 1. Start Postgres, run migrations, start the app (migrations run automatically
#    as a one-shot service the app depends on):
docker compose up --build

# App is on http://localhost:8080  (try the example feature + theme toggle).
```

That's it. To regenerate code or run tests without a local Go toolchain:

```bash
docker compose run --rm tools make generate   # sqlc + templ + tailwind
docker compose run --rm tools make test        # go test ./... -race (uses the db service)
docker compose run --rm tools make check        # structure + vet + test + vuln + verify-assets
```

**With a local toolchain** (Go + `make`, plus `templ`, `sqlc`, `goose`, the
Tailwind binary, and optionally `air`): `make build && ./server`, or `make dev`
for live reload. Start the database with `docker compose up -d db && make migrate`.

## Architecture

```
cmd/server/main.go        Entry point: config → logger → pool → app; -healthcheck subcommand
cmd/structurecheck/       Repo-structure checker used by make check
internal/core/            STABLE CORE (template-owned)
  config/                 Env config (stdlib only)
  db/                     pgxpool setup (tuned), the single data-access seam
  httpx/                  Middleware chain: recover, security headers + nonce CSP,
                          real-IP, rate limit, CSRF, gzip, size limit, logging
  assets/                 Embedded-FS server, fingerprint/ETag, SRI, import map
  app/                    Deps, Feature interface, router + server + graceful shutdown
  validate/               Input validation helpers
internal/feature/         FEATURE SLICES (project-owned, swappable)
  registry/               Project-owned list of enabled features
  example/                Reference slice: HTMX list/create/delete (+ store/ = sqlc output)
internal/view/            layout shell + reusable templ components
static/                   css/ (Tailwind in/out) · js/ (core.mjs, htmx, islands, vendor)
migrations/               Numbered goose .sql (also sqlc's schema source)
```

**Core vs features.** Core is a small, stable surface. A feature receives the
`app.Deps` struct and registers routes through `app.Feature`. Core never imports a
feature — the project-owned registry is the one explicit list of enabled slices.
This is what makes Core independently upgradeable. See [`CLAUDE.md`](CLAUDE.md)
for the ownership boundary.

### Strict application structure

This template is intentionally opinionated: every application feature lives in a
feature slice under `internal/feature/<name>/`, and `make structure` enforces the
shape. That gives the repo a Laravel-like working contract without adding a heavy
framework layer.

Each feature slice uses this anatomy:

| Path | Purpose |
|---|---|
| `handler.go` | Defines `type Module`, `func New(deps app.Deps) *Module`, and `func (m *Module) Routes(mux *http.ServeMux)`. |
| `view.templ` | Owns the feature's templ views and HTMX partials. |
| `handler_test.go` | Covers the HTTP behavior with DB-free fakes where possible. |
| `queries.sql` | Optional. Present only for DB-backed features; sqlc generates `store/`. |
| `store/` | Generated sqlc package for that feature only. Do not hand-edit generated files. |

`cmd/server/main.go` imports `internal/feature/registry` only. To enable a
feature, edit `internal/feature/registry/registry.go`; do not wire concrete
features into the server bootstrap.

### Add a feature (the pattern)

1. Create `internal/feature/<name>/` with `handler.go` (`New(deps) *Module` +
   `Routes(mux)`), `view.templ`, and `handler_test.go`.
2. If the feature uses Postgres, add `queries.sql` and a `sql:` entry in
   `sqlc.yaml` pointing at the folder (`package: store`).
3. Register it: one import + one line in `internal/feature/registry/registry.go`.
4. `make generate && make structure && make build`.

Removing a feature is the inverse: delete the folder + its migration + the one
registry line, then `make generate && make structure`.

### Add a JS island (the pattern)

Islands are small ES modules loaded **only** on pages that ask for them — no
bundler, no npm. The example `theme-toggle` island uses native browser APIs only.

1. `static/js/islands/<name>.mjs` exporting `mount(el, opts)`. Keep all logic here
   (strict CSP forbids inline handlers).
2. Activate it in a view by adding `data-island="<name>"` to an element. `core.mjs`
   lazy-imports it through the auto-generated import map; pass options via
   `data-opt-*` attributes.
3. Need a third-party lib (e.g. a chart or PDF viewer)? Vendor it pinned under
   `static/js/vendor/<lib>@<version>/`, add its SHA-256 to
   `static/js/vendor/CHECKSUMS.txt`, and load it with an `integrity=` (SRI)
   attribute. **Never from a CDN.**

## Dependencies & rationale

Runtime modules are kept to the absolute minimum (everything else is the stdlib):

| Module | Why |
|---|---|
| `github.com/a-h/templ` | Type-safe, compiled HTML components that stream into the response. |
| `github.com/jackc/pgx/v5` | Fast, pure-Go PostgreSQL driver + pool (keeps `CGO_ENABLED=0`). |

Everything else is intentionally **not** a runtime dependency:
- Routing: stdlib `net/http` + `http.ServeMux` (Go 1.22+ method/path patterns).
- Middleware, CSRF, rate limiter, gzip: hand-rolled on the stdlib.
- Logging: `log/slog`. Config: `os.Getenv`. Rate limiter: a tiny token bucket
  (swap in `golang.org/x/time/rate` if you prefer).

**Build tools** (not linked into the binary): `templ`, `sqlc`, `goose`,
`govulncheck`, the Tailwind standalone CLI, `syft`, `trivy`.

## Security features

- ✅ Strict, **nonce-based CSP** (no `unsafe-inline` for scripts); `object-src 'none'`,
  `base-uri 'none'`, `frame-ancestors 'none'`.
- ✅ `X-Content-Type-Options`, `X-Frame-Options: DENY`, `Referrer-Policy: no-referrer`,
  minimal `Permissions-Policy`, HSTS in production.
- ✅ **CSRF** on all state-changing requests (masked double-submit cookie;
  `HttpOnly`, `Secure` in prod, `SameSite=Lax`; BREACH-safe via per-response masking).
- ✅ **Proxy-aware real IP**: client-IP headers trusted only from `TRUSTED_PROXIES`
  CIDRs. App rate limit is **defense-in-depth** — the primary layer is the Cloudflare edge.
- ✅ Request-size limit, full set of `http.Server` timeouts, graceful shutdown,
  panic recovery (never leaks stack traces).
- ✅ Parameterized queries by construction (sqlc) — SQL injection effectively impossible.
- ✅ Single distroless **nonroot** image; pinned + checksummed (SRI) vendored JS;
  vendored Go modules; `govulncheck`, Trivy and an SBOM in CI.

## Performance notes

- **Connection pool** (`pgxpool`) is built once and tuned via env:
  `DB_MAX_CONNS`, `DB_MIN_CONNS`, `DB_MAX_CONN_LIFETIME`, `DB_MAX_CONN_IDLE_TIME`,
  `DB_HEALTH_CHECK_PERIOD`. Size `MAX_CONNS` to your Postgres `max_connections`
  budget across all app instances.
- Context deadlines on DB calls keep slow queries from exhausting the pool.
- templ renders straight into the response writer (no buffer). sqlc uses no
  reflection. Static assets are embedded, fingerprinted and cached immutably.
- A benchmark guards the hottest path (`make bench` → `BenchmarkIndex`).
- Optional gzip middleware (defense-in-depth; the edge already compresses).
- `pprof` is available on an internal localhost listener (`PPROF_ENABLED=true`).

## Deployment (Docker on a VPS)

1. Build & push: `docker build -t <registry>/app:<tag> .` then `docker push …`.
2. On the host, provide env (`.env` or your orchestrator's secrets) — at minimum
   `APP_ENV=production`, a real `DATABASE_URL`, and `TRUSTED_PROXIES` set to your
   proxy/Cloudflare ranges.
3. Run migrations as a deploy step: `goose -dir migrations postgres "$DATABASE_URL" up`
   (or the compose `migrate` service).
4. Run the container (read-only rootfs, nonroot, cap-drop — see `docker-compose.yml`).
   The container `HEALTHCHECK` uses `/server -healthcheck`.
5. Terminate TLS at Cloudflare / your reverse proxy and forward to `:8080`; keep
   the app's rate limit on as a second layer behind the edge WAF.

Image signing (cosign) and SLSA provenance are good next steps — see CI comments.

## License

Add your project license here.
