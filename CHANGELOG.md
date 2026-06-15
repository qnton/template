# Changelog

All notable changes to this template are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/), and the template is versioned
with [SemVer](https://semver.org/) via `TEMPLATE_VERSION` and matching git tags.

Breaking changes are marked **BREAKING** and explained in [`UPGRADING.md`](UPGRADING.md).

## [Unreleased]

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

[Unreleased]: https://example.com/compare/v0.1.0...HEAD
[0.1.0]: https://example.com/releases/tag/v0.1.0
