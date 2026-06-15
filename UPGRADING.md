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
  internal/core/ internal/view/layout/ internal/view/component/ \
  Makefile Dockerfile docker-compose.yml .github/ \
  static/js/core.mjs static/js/htmx.min.js static/js/vendor/ \
  sqlc.yaml embed.go .air.toml .dockerignore

# 2. Pull the template-owned paths (no merge base needed). Adjust the list to
#    what actually changed:
git checkout template/v<new> -- \
  internal/core/ internal/view/layout/ internal/view/component/ \
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

## Alternative: copier / cruft

Tooling like [copier](https://copier.readthedocs.io/) or
[cruft](https://cruft.github.io/cruft/) can automate template updates with an
answers file and 3-way merge. They're a reasonable option if you outgrow the
git-remote flow, but the approach above needs no extra tooling and is the default.
