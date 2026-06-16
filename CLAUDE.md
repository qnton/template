# CLAUDE.md — working guide

Compact, action-oriented rules for working in this repo (for Claude Code and for
humans). Not a duplicate of the README; this is *how to work here*.

## 1. Project overview

A minimal, security-first full-stack starter. Stack:
**Go (stdlib `net/http`) · templ · HTMX + JS islands · Tailwind v4 (standalone) ·
PostgreSQL (pgx/sqlc) · distroless Docker.** Two runtime dependencies only
(`templ`, `pgx/v5`); everything else is the standard library or a build tool.
Base version is in `TEMPLATE_VERSION`.

## 2. Golden rules

**DO**
- Extend via **feature slices** in `internal/feature/<name>/`.
- All SQL through **sqlc** (parameterized); add queries in `queries.sql`, run `make sqlc`.
- Keep dependencies **minimal** — justify any new runtime module in the README.
- Keep frontend assets **vendored + pinned** (checksum + SRI); update `CHECKSUMS.txt`.
- Run `make check` before every commit.

**DON'T**
- ❌ No npm / node_modules / package.json. No CDN scripts.
- ❌ Don't edit `internal/core/` (template-owned — overwritten on updates). Extend via features.
- ❌ No external HTTP router. No ORM. No string-concatenated SQL.
- ❌ Never trust client-IP headers from untrusted sources (see `TRUSTED_PROXIES`).
- ❌ No `unsafe-inline` in the CSP. No `hx-on`/inline event handlers — put JS in island modules.

## 3. Architecture & ownership boundary

**Core vs features.** `internal/core/` is the stable bootstrap (server, middleware
chain, config, DB pool, asset/island loader, CSRF, the head/CSP seam). Features are
self-contained slices that depend only on the small **`app.Deps`** struct
(`Logger`, `Pool`, `Config`, `CSRF`, `Assets`) and register routes via the
**`app.Feature`** interface. Core never imports concrete features;
`internal/feature/registry` is the project-owned list of enabled slices.

**Ownership (canonical — referenced by `UPGRADING.md` and `make upgrade-check`):**

| Template-owned (DO NOT edit — overwritten on update) | Project-owned (yours — never touched by updates) |
|---|---|
| `internal/core/` (incl. the `internal/core/view/` head/CSP seam) | `internal/feature/` |
| `embed.go`, `sqlc.yaml` | `internal/view/` (layout + components — scaffolded once, then yours) |
| `Makefile`, `Dockerfile`, `docker-compose.yml`, `.air.toml` | `migrations/` · `static/js/islands/` (your islands) |
| `.github/`, `.dockerignore` | `static/css/input.css` (your `@theme`) |
| `static/js/core.mjs`, `static/js/htmx.min.js`, `static/js/vendor/` | `.env`, `TEMPLATE_VERSION` value · the go.mod module path, `README.md` |

Rule: **extend by adding files (features, islands, migrations), never by editing
Core files.** Adding files under template-owned dirs is fine — updates overwrite
base files but leave your additions. The **view layer (`internal/view/`) is
project-owned scaffolding**: the template writes it once and never overwrites it,
so customize components and layout (and design tokens in `static/css/input.css`)
freely. Only the small head/CSP seam under `internal/core/view/` (nonce'd import
map, CSRF meta, vendored-script SRI) stays template-owned and upgradeable —
`layout.Base` renders it with `@coreview.Head(a, title)`.

**Strict feature anatomy (`make structure` enforces this):**
- `handler.go` — package name matches the feature directory; defines
  `type Module`, `func New(deps app.Deps) *Module`, and
  `func (m *Module) Routes(mux *http.ServeMux)`.
- `view.templ` — feature views and HTMX partials.
- `handler_test.go` — HTTP behavior tests, preferably with DB-free fakes.
- `queries.sql` — optional; if present, the feature must have sqlc-generated
  `store/` files and a matching `sqlc.yaml` entry.
- `internal/feature/registry` is not a feature slice; it only enables slices.

## 4. Recipes

**Add a feature** (`internal/feature/<name>/`):

Fast path: **`make new-feature NAME=<name>`** scaffolds the whole slice
(handler + view + test, wired into the registry) so it passes `make structure`
immediately; add `DB=1` to also emit `queries.sql` + a migration + the `sqlc.yaml`
block (then `make sqlc` materialises `store/`). Doing it by hand, the anatomy is:
1. `handler.go` — `type Module struct{…}`, `func New(deps app.Deps) *Module`, `func (m *Module) Routes(mux *http.ServeMux)`.
2. `view.templ` — components rendered by the handlers.
3. `handler_test.go` — handler tests using a fake store where possible.
4. Optional `queries.sql` — sqlc queries; add a matching `sql:` entry in `sqlc.yaml` (`out`/`queries` → this folder, `package: store`).
5. Register: one import + one line in `internal/feature/registry/registry.go`.
6. `make generate && make structure && make build`. (Remove a feature = delete the folder + its migration/query config + registry line, then `make generate && make structure`.)

**Add a JS island** (`static/js/islands/<name>.mjs`):
1. Export `mount(el, opts)`; keep all logic here (no inline handlers — strict CSP).
2. Activate in a view: add `data-island="<name>"` to an element. `core.mjs` lazy-imports it via the import map (auto-generated from the islands dir).
3. Pass options with `data-opt-*` attributes (`data-opt-url` → `opts.url`).
4. If it needs a third-party lib: vendor it pinned under `static/js/vendor/<lib>@<ver>/`, add its SHA to `static/js/vendor/CHECKSUMS.txt`, load with `integrity=` (SRI). **Never** a CDN.

**Add a query / migration:**
1. New numbered file in `migrations/` (goose: `-- +goose Up` / `-- +goose Down`).
2. `make migrate`.
3. Add the query to the feature's `queries.sql` → `make sqlc` → use via the feature's `store.Querier`.

**Background jobs** (`internal/core/jobs`, optional — set `JOBS_ENABLED=true`):
enqueue from a handler with `jobs.New(deps.Pool).Enqueue(ctx, "kind", payload)`;
register the handler in `internal/feature/registry` `RegisterJobs`. The worker
(Postgres `FOR UPDATE SKIP LOCKED`, retry/backoff) starts in the server bootstrap.

**Scheduled tasks** (`internal/core/schedule`, optional — set `SCHEDULER_ENABLED=true`):
register recurring work in `registry.RegisterSchedule` with
`s.Every("name", interval, fn)`. In-process per replica — guard once-only tasks
with a Postgres advisory lock.

**Mail** (`internal/core/mail`): build a mailer on demand with
`m, _ := mail.FromEnv(deps.Logger)`, then `m.Send(ctx, mail.Message{…})`. Driver +
credentials come from `MAIL_*` env (`MAIL_DRIVER=log` default; `smtp` for real
sending). No central config field, no bootstrap wiring.

**Cache** (`internal/core/cache`): `cache.NewMemory()` (process-local, TTL) or
`cache.NewPostgres(deps.Pool)` (shared, `cache` table). Both satisfy `cache.Cache`
(`Get`/`Set`/`Delete`); `cache.Remember(ctx, c, key, ttl, fn)` for compute-and-store.

**Reusable UI:** `internal/view/component/` ships a token-based starter design
system (`Button`, `Alert`/`AlertBox`, `Badge`, `Card`, `CardLink`, `Stat`,
`EmptyState`, `Icon` + lucide set, and `Label`/`Field`/`PageHeader`/`Section`/
`Kbd`/`Skeleton`). Compose these or add your own templ component here. Two layouts
exist: `layout.Base` (minimal, for public/auth pages) and `layout.AppShell`
(sidebar + topbar for authenticated areas; edit the nav in `layout/nav.go`).
**Re-theme by editing the oklch tokens in `static/css/input.css`** (`:root`/`.dark`)
— components reference semantic tokens (`bg-primary`, `text-muted-foreground`,
`border-border`, …), never raw colours, so one file restyles everything.

## 5. Conventions

- Idiomatic Go: small functions, wrap errors with `%w`, DI via structs, no global state.
- **Context deadlines on every DB/HTTP call** (`context.WithTimeout`).
- **Validate input before** it reaches a store or template (`internal/core/validate`).
- Thin handlers; keep logic testable. Handlers depend on the `store.Querier` interface, not concrete `*Queries`.

## 6. Security (mandatory)

- **Nonce CSP**, no `unsafe-inline`; the only inline script is the nonce'd import map.
- **CSRF** (masked double-submit cookie) on every POST/PUT/PATCH/DELETE; HTMX sends the token via the `csrf-token` meta tag (`core.mjs`).
- **Proxy-aware real IP** via `TRUSTED_PROXIES`; app rate limit is **defense-in-depth** (primary limiting = Cloudflare edge).
- Upload/body size capped (`MAX_REQUEST_BYTES`); validate size/MIME in any upload handler you add.
- Secrets only via env. pprof binds to an internal address and is off by default.

## 7. Commands

| Command | Purpose |
|---|---|
| `make help` | List all targets |
| `make dev` | Live reload (templ + tailwind watch + air) |
| `make generate` | Run sqlc + templ + tailwind |
| `make sqlc` / `make templ` / `make tailwind` | Individual generators |
| `make build` | Generate + build the hardened static binary |
| `make new-feature NAME=x [DB=1]` | Scaffold a feature slice (+ optional DB store) |
| `make new-migration NAME=x` | Scaffold an auto-numbered goose migration |
| `make new-island NAME=x` / `make new-component NAME=x` | Scaffold a JS island / view component |
| `make structure` | Validate strict app/feature structure |
| `make migrate` / `make migrate-down` | Apply / roll back migrations (goose) |
| `make test` | `go test ./... -race` |
| `make bench` | Benchmarks |
| `make vet` | `go vet` + gofmt check |
| `make vuln` | `govulncheck ./...` |
| `make verify-assets` | Check vendored asset checksums |
| `make vendor` | `go mod tidy && go mod vendor` |
| `make check` | structure + vet + test + vuln + verify-assets (pre-commit) |
| `make docker` | Build the production image |
| `make sbom` | Syft SBOM |
| `make upgrade-check` | Compare `TEMPLATE_VERSION` with newest template tag |

Docker-only (no local Go): prefix with `docker compose run --rm tools `, e.g.
`docker compose run --rm tools make generate`.

## 8. Definition of Done (green before every commit)

`make structure` green · `gofmt` clean · `go vet` clean · `sqlc generate` &
`templ generate` produce **no diff** · Tailwind rebuilt · `go test ./... -race`
green · `govulncheck` clean · `make verify-assets` passes. `make check` covers
most of these.

## 9. Updating from the template

See `UPGRADING.md`. In short: `git remote add template <url>` → `git fetch template --tags`
→ review with `git diff <your-version>..template/<tag> -- <template-owned paths>` →
`git checkout template/<tag> -- <template-owned paths>` → bump `TEMPLATE_VERSION` →
`make build && make test`. If you renamed the go.mod module path, re-run the sed in `UPGRADING.md`.
