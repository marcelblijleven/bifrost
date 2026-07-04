# syntax=docker/dockerfile:1
# Bifrost ships as a single Go binary with the SvelteKit UI embedded:
#
#   docker build --build-arg VERSION=1.2.3 -t bifrost .
#
# The Go server owns port 8080 and serves everything: the static UI, the
# JSON API under /api, webhooks, health, and metrics. Build stages run on
# the build platform and cross-compile, so multi-arch builds do not emulate
# the compilers.

# ── build: SvelteKit frontend (static SPA) ────────────────────────────────────
FROM --platform=$BUILDPLATFORM node:24-alpine AS web-build
WORKDIR /app
RUN npm install -g pnpm@11

COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile

COPY frontend/ .
RUN pnpm build

# ── build: Go binary with the UI embedded ─────────────────────────────────────
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS go-build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY cmd ./cmd
COPY internal ./internal
COPY frontend/embed.go ./frontend/
COPY --from=web-build /app/build ./frontend/build

ARG VERSION=dev
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/bifrost ./cmd/bifrost

# ── runtime ───────────────────────────────────────────────────────────────────
FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 bifrost

COPY --from=go-build /out/bifrost /usr/local/bin/bifrost

USER bifrost
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s \
    CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["bifrost"]
