# Developer entrypoints. Tool binaries are variables, so every target runs either
# natively (with the tools installed) or inside the tools container for Docker-only
# setups:
#
#   docker compose run --rm tools make <target>
#
# Quick start:  docker compose up -d db && make migrate && docker compose up app

GO       ?= go
TEMPL    ?= templ
SQLC     ?= sqlc
GOOSE    ?= goose
TAILWIND ?= tailwindcss
GOVULN   ?= govulncheck
GOLANGCI ?= golangci-lint

DATABASE_URL ?= postgres://app:app@localhost:5432/app?sslmode=disable
VERSION      := $(shell cat TEMPLATE_VERSION 2>/dev/null || echo dev)
LDFLAGS      := -s -w -X main.version=$(VERSION)

# Template-owned paths pulled on template upgrades (see CLAUDE.md / UPGRADING.md).
TEMPLATE_OWNED := internal/core/ Makefile Dockerfile docker-compose.yml .github/ \
                  static/js/htmx.min.js static/js/core.mjs static/js/vendor/ sqlc.yaml

.DEFAULT_GOAL := help

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
	  | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-16s\033[0m %s\n",$$1,$$2}'

## ── Codegen ──────────────────────────────────────────────────────────────────
.PHONY: generate sqlc templ tailwind
generate: sqlc templ tailwind ## Run all generators (sqlc, templ, tailwind)

sqlc: ## Generate type-safe DB code from SQL
	$(SQLC) generate

templ: ## Generate Go from .templ files
	$(TEMPL) generate

tailwind: ## Build minified CSS (Tailwind standalone)
	$(TAILWIND) -i static/css/input.css -o static/css/app.css --minify

## ── Scaffold (generators) ────────────────────────────────────────────────────
.PHONY: new-feature new-migration new-island new-component
new-feature: ## Scaffold a feature slice: make new-feature NAME=posts [DB=1]
	@test -n "$(NAME)" || { echo "usage: make new-feature NAME=<name> [DB=1]"; exit 2; }
	$(GO) run ./cmd/scaffold feature $(NAME) $(if $(DB),--db,)
	$(MAKE) generate

new-migration: ## Scaffold a migration: make new-migration NAME=create_posts
	@test -n "$(NAME)" || { echo "usage: make new-migration NAME=<name>"; exit 2; }
	$(GO) run ./cmd/scaffold migration $(NAME)

new-island: ## Scaffold a JS island: make new-island NAME=chart
	@test -n "$(NAME)" || { echo "usage: make new-island NAME=<name>"; exit 2; }
	$(GO) run ./cmd/scaffold island $(NAME)

new-component: ## Scaffold a view component: make new-component NAME=avatar
	@test -n "$(NAME)" || { echo "usage: make new-component NAME=<name>"; exit 2; }
	$(GO) run ./cmd/scaffold component $(NAME)
	$(MAKE) templ

## ── Build / run ──────────────────────────────────────────────────────────────
.PHONY: build dev
build: generate ## Generate, then build the hardened static binary
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o ./server ./cmd/server

dev: ## Live reload: templ + tailwind watchers + air (install: go install github.com/air-verse/air@latest)
	$(TEMPL) generate --watch & \
	$(TAILWIND) -i static/css/input.css -o static/css/app.css --watch & \
	air ; \
	kill 0

## ── Quality gates ────────────────────────────────────────────────────────────
.PHONY: check structure vet fmt lint test bench vuln verify-assets checksums
check: structure vet test vuln verify-assets ## Run all pre-commit checks

vet: ## go vet + gofmt check (excludes generated files)
	$(GO) vet ./...
	@unformatted=$$(gofmt -l . | grep -v '_templ.go' || true); \
	 if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi

fmt: ## Format Go code in place (gofmt -w; skips vendored and generated files)
	@files=$$(gofmt -l . | grep -v -e '_templ\.go' -e '^vendor/' || true); \
	 if [ -n "$$files" ]; then echo "$$files" | xargs gofmt -w && echo "formatted:" && echo "$$files"; \
	 else echo "already formatted"; fi

lint: ## Run golangci-lint (security-leaning set; see .golangci.yml). Not in `check`.
	$(GOLANGCI) run

structure: ## Validate the strict app/feature structure
	$(GO) run ./cmd/structurecheck

test: ## Run tests with the race detector
	$(GO) test ./... -race

bench: ## Run benchmarks
	$(GO) test -run='^$$' -bench=. -benchmem ./...

vuln: ## Scan reachable code for known vulnerabilities
	$(GOVULN) ./...

verify-assets: ## Verify vendored asset checksums AND that no vendored file is unlisted
	cd static/js && shasum -a 256 -c vendor/CHECKSUMS.txt
	@cd static/js && status=0; \
	 listed=$$(grep -vE '^[[:space:]]*#' vendor/CHECKSUMS.txt | awk 'NF>=2{print $$NF}'); \
	 for f in $$(find vendor -type f ! -name CHECKSUMS.txt ! -name '.*'); do \
	   echo "$$listed" | grep -qxF "$$f" || { echo "  unlisted: $$f"; status=1; }; \
	 done; \
	 if [ "$$status" -ne 0 ]; then \
	   echo "ERROR: vendored files missing from vendor/CHECKSUMS.txt — run 'make checksums' and add them"; exit 1; \
	 fi

checksums: ## Print sha256 + SRI lines for vendored assets (paste into vendor/CHECKSUMS.txt)
	@cd static/js && for f in htmx.min.js $$(find vendor -type f ! -name CHECKSUMS.txt ! -name '.*' | sort); do \
	   [ -f "$$f" ] || continue; \
	   echo "# SRI: sha384-$$(openssl dgst -sha384 -binary "$$f" | openssl base64 -A)"; \
	   shasum -a 256 "$$f"; \
	 done

## ── Database ─────────────────────────────────────────────────────────────────
.PHONY: migrate migrate-down
migrate: ## Apply all up migrations
	$(GOOSE) -dir migrations postgres "$(DATABASE_URL)" up

migrate-down: ## Roll back the most recent migration
	$(GOOSE) -dir migrations postgres "$(DATABASE_URL)" down

## ── Dependencies / supply chain ──────────────────────────────────────────────
.PHONY: vendor docker sbom
vendor: ## Tidy modules and refresh the vendor tree
	$(GO) mod tidy
	$(GO) mod vendor

docker: ## Build the production image
	docker build -t app:$(VERSION) .

sbom: ## Generate a CycloneDX SBOM (needs syft)
	syft dir:. -o cyclonedx-json=sbom.cdx.json

## ── Template upgrades ────────────────────────────────────────────────────────
.PHONY: upgrade-check
upgrade-check: ## Compare TEMPLATE_VERSION with the newest template tag
	@git remote get-url template >/dev/null 2>&1 || { \
	  echo "No 'template' remote. Add it once: git remote add template <template-repo-url>"; exit 1; }
	@current=$$(cat TEMPLATE_VERSION); \
	 latest=$$(git ls-remote --tags --refs template 'v*' | awk -F/ '{print $$3}' | sort -V | tail -1); \
	 echo "current: v$$current    newest template tag: $$latest"; \
	 if [ "v$$current" = "$$latest" ]; then echo "Up to date."; else \
	   echo "Update available. Fetch and review template-owned paths:"; \
	   echo "  git fetch template --tags"; \
	   echo "  git diff v$$current $$latest -- $(TEMPLATE_OWNED)"; \
	 fi

.PHONY: clean
clean: ## Remove build output and generated CSS
	rm -f ./server sbom.cdx.json static/css/app.css
