# syntax=docker/dockerfile:1
#
# Multi-stage build:
#   - tools : full toolchain (Go + templ + sqlc + goose + Tailwind + govulncheck)
#             for codegen, tests and CI. Also the Docker-only dev environment.
#             NOT part of the production image.
#   - build : compiles the static binary OFFLINE from committed vendor/ and the
#             committed generated artifacts (*_templ.go, sqlc output, app.css).
#             Runs `go build` only — no codegen, no network.
#   - final : distroless, nonroot, no shell or package manager.
#
# Pinned tool versions (bump together with go.mod / CI):
ARG GO_VERSION=1.26.4
ARG TEMPL_VERSION=v0.3.1020
ARG SQLC_VERSION=v1.31.1
ARG GOOSE_VERSION=v3.27.1
ARG TAILWIND_VERSION=v4.3.1

###############################################################################
# tools
###############################################################################
FROM golang:${GO_VERSION}-bookworm AS tools
ARG TEMPL_VERSION
ARG SQLC_VERSION
ARG GOOSE_VERSION
ARG TAILWIND_VERSION

RUN apt-get update \
 && apt-get install -y --no-install-recommends make ca-certificates curl \
 && rm -rf /var/lib/apt/lists/*

RUN go install github.com/a-h/templ/cmd/templ@${TEMPL_VERSION} \
 && go install github.com/sqlc-dev/sqlc/cmd/sqlc@${SQLC_VERSION} \
 && go install github.com/pressly/goose/v3/cmd/goose@${GOOSE_VERSION} \
 && go install golang.org/x/vuln/cmd/govulncheck@latest

# Tailwind CSS standalone CLI (no npm/node_modules).
RUN set -eux; \
    case "$(dpkg --print-architecture)" in \
      amd64) asset=tailwindcss-linux-x64 ;; \
      arm64) asset=tailwindcss-linux-arm64 ;; \
      *) echo "unsupported arch"; exit 1 ;; \
    esac; \
    curl -fsSL -o /usr/local/bin/tailwindcss \
      "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/${asset}"; \
    chmod +x /usr/local/bin/tailwindcss

WORKDIR /src

###############################################################################
# build
###############################################################################
FROM golang:${GO_VERSION}-bookworm AS build
WORKDIR /src

# Module graph + vendored deps first for better layer caching.
COPY go.mod go.sum ./
COPY vendor/ vendor/
COPY . .

ARG VERSION=docker
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOFLAGS=-mod=vendor \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /server ./cmd/server

###############################################################################
# final
###############################################################################
# To debug a running container (busybox shell + coreutils), switch the base to:
#   gcr.io/distroless/static-debian12:debug-nonroot
# and run: docker run --entrypoint=sh -it <image>
FROM gcr.io/distroless/static-debian12:nonroot AS final
COPY --from=build /server /server
EXPOSE 8080
USER nonroot:nonroot
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ["/server", "-healthcheck"]
ENTRYPOINT ["/server"]
