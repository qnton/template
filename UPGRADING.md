# Upgrading from the template

A project created from this template shares **no git history** with the template
repo, so `git merge` doesn't apply. Instead you selectively pull the
**template-owned** paths and regenerate. The canonical ownership list lives in
[`CLAUDE.md`](CLAUDE.md#3-architecture--ownership-boundary) — this document does
not duplicate it.

The rule that makes this work: **extend via feature slices and added files; never
edit `internal/core/` or other template-owned files.** Then pulling updates can
overwrite template-owned paths without touching your code.

## One-time setup

```bash
git remote add template <template-repo-url>
git fetch template --tags
```

## Check what's available

```bash
make upgrade-check
```

This compares your `TEMPLATE_VERSION` with the newest template tag and prints the
`git diff` command scoped to the template-owned paths.

## Perform the upgrade

```bash
git fetch template --tags

# 1. Review what changed in Core since your version (read CHANGELOG.md for
#    breaking changes, which are marked there):
git diff v<your-TEMPLATE_VERSION>..template/v<new> -- \
  internal/core/ \
  Makefile Dockerfile docker-compose.yml .github/ \
  static/js/core.mjs static/js/htmx.min.js static/js/vendor/ \
  sqlc.yaml embed.go .air.toml .dockerignore

# 2. Pull the template-owned paths (no merge base needed). Adjust the list to
#    what actually changed:
git checkout template/v<new> -- \
  internal/core/ \
  Makefile Dockerfile docker-compose.yml .github/ \
  static/js/core.mjs static/js/htmx.min.js static/js/vendor/ \
  sqlc.yaml embed.go .air.toml .dockerignore

# 3. Bump the recorded base version and regenerate + verify:
echo "<new>" > TEMPLATE_VERSION
make generate
make check
```

`git checkout template/<tag> -- <paths>` overwrites the template-owned files but
**leaves files you added** (e.g. your own components or islands) untouched, since
checkout only writes paths present in the template tree.

> **The view layer is yours.** `internal/view/` (layout + components) is
> project-owned scaffolding — the template writes it once at clone time and the
> upgrade lists above deliberately **exclude** it, so your customized components,
> layout, and design tokens are never overwritten. The only view code that stays
> upgradeable is the small head/CSP seam under `internal/core/view/` (covered by
> `internal/core/` above), which `layout.Base` renders via `@coreview.Head`. If a
> CHANGELOG entry notes a change to that seam, it applies automatically when you
> pull `internal/core/`; nothing else in `internal/view/` is touched.

## If you renamed the go.mod module path

Pulled Core files carry the template's module path in their imports. After step 2,
rewrite it to your module path:

```bash
OLD=github.com/example/app
NEW=github.com/your-org/your-app
grep -rl "$OLD" internal/core internal/view embed.go cmd | xargs sed -i '' "s#$OLD#$NEW#g"   # macOS
# Linux: drop the '' after -i
make generate && make check
```

Keeping the original module path avoids this step entirely (the lean default).

## Breaking changes

`CHANGELOG.md` marks breaking changes per version with **BREAKING**. Read the
entries between your version and the target before upgrading, and apply any noted
migrations to your feature code or `.env`.

### v0.1.0 → v0.2.0: feature registry moved to a package

Early projects (scaffolded from the very first tag) wired features directly in
`cmd/server/main.go`. v0.2.0 moved enablement into a **project-owned**
`internal/feature/registry` package and added `cmd/structurecheck`. `cmd/server`
must import *only* the registry, never a concrete feature, so Core stays
independently upgradeable. If your `cmd/server/main.go` still imports features
directly, migrate once:

1. Create `internal/feature/registry/registry.go` returning the enabled slices:

   ```go
   package registry

   import (
       "github.com/your-org/your-app/internal/core/app"
       "github.com/your-org/your-app/internal/feature/example"
       // …your features…
   )

   // Features is the one explicit list of enabled slices.
   func Features(deps app.Deps) []app.Feature {
       return []app.Feature{
           example.New(deps),
           // …your features…
       }
   }
   ```

2. In `cmd/server/main.go`, drop the per-feature imports and pass
   `registry.Features(deps)...` into `app.New(...)`. After this, `main.go`'s only
   feature import is `internal/feature/registry`.

3. Copy `cmd/structurecheck/` from the template tag and wire it into the gate so
   feature anatomy stays enforced:

   ```bash
   git checkout template/v0.2.0 -- cmd/structurecheck/
   ```

   Confirm `make check` runs `structure` (the template's `Makefile` already
   chains it: `structure vet test vuln verify-assets`).

4. `make generate && make structure && make check`.

## Alternative: copier / cruft

Tooling like [copier](https://copier.readthedocs.io/) or
[cruft](https://cruft.github.io/cruft/) can automate template updates with an
answers file and 3-way merge. They're a reasonable option if you outgrow the
git-remote flow, but the approach above needs no extra tooling and is the default.
