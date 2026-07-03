# syntax=docker/dockerfile:1
# Bifrost: one Dockerfile for the Go API server and the SvelteKit frontend.
#
#   docker build --build-arg VERSION=1.2.3 -t bifrost .   # combined, port 8080
#   docker build --target api -t bifrost-api .            # Go backend only
#   docker build --target web -t bifrost-web .            # SSR frontend only
#
# Build stages run on the build platform and cross-compile, so multi-arch
# builds do not emulate the compilers.

# ── build: Go backend ─────────────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS go-build
WORKDIR /src

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY cmd ./cmd
COPY internal ./internal

ARG VERSION=dev
ARG TARGETOS TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/bifrost ./cmd/bifrost

# ── build: SvelteKit frontend (adapter-node) ──────────────────────────────────
# Output and runtime deps are pure JS, safe to build on the build platform.
FROM --platform=$BUILDPLATFORM node:24-alpine AS web-build
WORKDIR /app
RUN npm install -g pnpm@11

COPY frontend/package.json frontend/pnpm-lock.yaml frontend/pnpm-workspace.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile

COPY frontend/ .
RUN pnpm build && pnpm prune --prod

# ── api: Go backend only ──────────────────────────────────────────────────────
FROM alpine:3.22 AS api
RUN apk add --no-cache ca-certificates tzdata \
    && adduser -D -H -u 10001 bifrost

COPY --from=go-build /out/bifrost /usr/local/bin/bifrost

USER bifrost
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s \
    CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["bifrost"]

# ── web: SvelteKit SSR server only ────────────────────────────────────────────
# Runtime env: API_URL, ORIGIN, PORT.
FROM node:24-alpine AS web
WORKDIR /app
ENV NODE_ENV=production PORT=3000

COPY --from=web-build /app/build ./build
COPY --from=web-build /app/node_modules ./node_modules
COPY --from=web-build /app/package.json ./package.json

USER node
EXPOSE 3000
CMD ["node", "build"]

# ── default: combined image, both processes, one port ─────────────────────────
# The Go server owns port 8080 and serves everything: UI (proxied to the
# in-container SvelteKit server, which binds loopback only), API under /api,
# webhooks, health, metrics.
FROM node:24-alpine AS bundle
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
ENV NODE_ENV=production \
    HOST=127.0.0.1 PORT=3000 \
    HTTP_ADDR=:8080 \
    FRONTEND_URL=http://127.0.0.1:3000 \
    API_URL=http://127.0.0.1:8080/api

COPY --from=go-build /out/bifrost /usr/local/bin/bifrost
COPY --from=web-build /app/build ./build
COPY --from=web-build /app/node_modules ./node_modules
COPY --from=web-build /app/package.json ./package.json
COPY docker/entrypoint.sh /usr/local/bin/bifrost-entrypoint

USER node
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s \
    CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1
ENTRYPOINT ["bifrost-entrypoint"]
