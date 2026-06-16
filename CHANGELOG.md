# Changelog

All notable changes to this template are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/), and the template is versioned
with [SemVer](https://semver.org/) via `TEMPLATE_VERSION` and matching git tags.

Breaking changes are marked **BREAKING** and explained in [`UPGRADING.md`](UPGRADING.md).

> **Releasing (maintainers).** Every change set that touches a template-owned
> path must bump `TEMPLATE_VERSION`, add an entry here, and ship a matching
> `git tag vX.Y.Z`. Consumers' `make upgrade-check` compares against the newest
> **tag** — untagged commits on `main` are invisible to it, so unreleased work
> reads as "up to date" downstream. Tag it, or it didn't ship.

## [Unreleased]

Roadmap toward "Laravel's productivity, Go's soul" — optional, stdlib-first
batteries, each removable.

### Added
- **Background jobs** (`internal/core/jobs`): a Postgres-backed queue
  (`SELECT … FOR UPDATE SKIP LOCKED`) + worker with exponential-backoff retries and
  panic recovery. Optional via `JOBS_ENABLED` (started in the server bootstrap on
  the signal context). Enqueue with `jobs.New(deps.Pool).Enqueue(ctx, kind, payload)`;
  register handlers in `registry.RegisterJobs`. Ships `migrations/00003_create_jobs.sql`.

## [0.2.0]

### Added
- **Scaffolding CLI** (`cmd/scaffold`, template-owned, stdlib-only) + Make targets:
  `make new-feature NAME=x [DB=1]`, `new-migration`, `new-island`, `new-component`.
  The inverse of `cmd/structurecheck` — it emits the exact feature anatomy the checker
  enforces (handler + view + test), auto-wires `internal/feature/registry`, and on
  `DB=1` also generates `queries.sql` + an auto-numbered migration + the `sqlc.yaml`
  block. Generated Go is run through `go/format`. The first step toward
  "Laravel's productivity, Go's soul" (DX via codegen, no runtime magic).
- **Starter design system** (project-owned, in `internal/view/component` +
  `static/css/input.css`): a token-based component set — `Button` (variants +
  sizes + leading icon), `Alert`/`AlertBox`, `Badge`, `Card`, `CardLink`, `Stat`,
  `EmptyState`, `Icon` (vendored lucide set in `icons.go`), and form/layout
  primitives (`Label`, `Field`, `PageHeader`, `Section`, `Kbd`, `Skeleton`). All
  classes reference semantic oklch tokens defined in `input.css` (`:root`/`.dark`),
  so re-theming is a one-file edit. Adds a second layout, `AppShell` (sidebar +
  topbar + breadcrumbs + mobile nav, see `nav.go`), alongside the minimal `Base`.
- Email/password **auth** feature slice: server-side sessions with hashed tokens,
  `RequireAuth` wrapper, login/register/logout/account routes, `users`/`sessions`
  migration. Removable like any slice.
- `cmd/structurecheck` — strict feature-anatomy checker, wired into `make check`
  and CI.
- `RequestID` middleware (sanitizes inbound `X-Request-Id`, correlates logs) and
  `internal/core/httpx/errors.go` helpers.
- Config: `LOG_LEVEL` (slog level) and `HEALTHCHECK_TIMEOUT`.
- Expanded core test suite — CSRF/real-IP fuzz tests, gzip/headers/logging/
  recover/size tests, example handler tests + a list-path benchmark.
- `SECURITY.md`, `.golangci.yml` lint config, and a release GitHub Actions
  workflow.

### Changed
- **BREAKING** — Feature enablement moved to a project-owned
  `internal/feature/registry` package; `cmd/server` imports only the registry,
  never a concrete feature, keeping Core independently upgradeable. Migration:
  [`UPGRADING.md`](UPGRADING.md) → "feature registry moved to a package".
- **BREAKING** — The view layer (`internal/view/`: layout + components) is now
  **project-owned scaffolding** — scaffolded once and never overwritten on
  upgrade (previously template-owned). Customize components, layout, and design
  tokens freely. The security-critical head wiring (nonce'd import map, CSRF
  meta, vendored-script SRI) moved to the new template-owned `internal/core/view`
  seam, which `layout.Base` renders via `@coreview.Head(a, title)`, so CSP/asset
  fixes still reach you when you pull `internal/core/`. Migration:
  [`UPGRADING.md`](UPGRADING.md) → "The view layer is yours".

## [0.1.0] — initial release

### Added
- Core HTTP server on stdlib `net/http` + `http.ServeMux` (no external router):
  graceful shutdown, full `http.Server` timeouts, request-size limit, internal
  pprof listener (off by default).
- Hand-rolled middleware chain: panic recovery, structured request logging
  (`log/slog` JSON), proxy-aware real-IP (via `TRUSTED_PROXIES`), token-bucket
  rate limit (defense-in-depth), strict nonce-based CSP + security headers,
  masked double-submit CSRF, optional gzip.
- `pgxpool` data layer (env-tuned) and `sqlc` (pgx/v5) per-feature query
  generation into a `store` subpackage; goose migrations.
- templ layout shell + reusable components; Tailwind v4 via the standalone CLI
  (CSS-first, no npm).
- Frontend: vendored + pinned HTMX (SRI + checksums), `core.mjs` island loader
  with auto-generated import map, and a native-only `theme-toggle` island.
- Reference feature slice `example` (HTMX list/create/delete) with handler,
  store and security tests, plus a list-path benchmark.
- Distroless, nonroot, multi-stage Docker build (offline image build from
  committed vendor + generated artifacts) with a `-healthcheck` subcommand.
- docker-compose (hardened Postgres + app + one-shot migrate + tools profile);
  Makefile; live-reload dev via templ/Tailwind watch + air.
- Supply-chain hardening: vendored Go modules, `govulncheck`, Trivy fs+image
  scans, Syft SBOM, SHA-pinned GitHub Actions, dependabot (no auto-merge).
- Docs: `README.md`, `CLAUDE.md` (rules + ownership boundary), `UPGRADING.md`.

[Unreleased]: https://example.com/compare/v0.2.0...HEAD
[0.2.0]: https://example.com/compare/v0.1.0...v0.2.0
[0.1.0]: https://example.com/releases/tag/v0.1.0
