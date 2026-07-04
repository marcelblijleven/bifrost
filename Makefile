.PHONY: build build-server build-cli frontend-build run dev dev-backend dev-frontend test lint docker-up docker-down docker-build air-install

AIR := $(shell go env GOPATH)/bin/air
VERSION ?= dev
LDFLAGS := -X main.version=$(VERSION)

# Full server binary: builds the frontend first so the UI is embedded.
build: frontend-build build-server

# Go compile only; embeds whatever is already in frontend/build.
build-server:
	go build -ldflags "$(LDFLAGS)" -o bin/bifrost ./cmd/bifrost

build-cli:
	go build -ldflags "$(LDFLAGS)" -o bin/bifrost-cli ./cmd/bifrost-cli

frontend-build:
	cd frontend && pnpm install --frozen-lockfile && pnpm build

run:
	@if [ ! -f .env ]; then cp .env.example .env; fi
	@set -a && . ./.env && set +a && go run ./cmd/bifrost

# ── dev: hot-reload backend + frontend ────────────────────────────────────────

dev: air-install docker-up
	@if [ ! -f .env ]; then cp .env.example .env; fi
	@echo "Starting backend (air) and frontend (vite)..."
	@trap 'kill 0' SIGINT SIGTERM; \
	  (set -a && . ./.env && set +a && $(AIR)) & \
	  (cd frontend && pnpm dev) & \
	  wait

dev-backend: air-install docker-up
	@if [ ! -f .env ]; then cp .env.example .env; fi
	@set -a && . ./.env && set +a && $(AIR)

dev-frontend:
	cd frontend && pnpm dev

air-install:
	@if [ ! -f $(AIR) ]; then \
	  echo "Installing air..."; \
	  go install github.com/air-verse/air@latest; \
	fi

# ── infra ─────────────────────────────────────────────────────────────────────

docker-up:
	docker compose up -d --wait db

docker-down:
	docker compose down

# Build the production image locally (the release workflow builds and
# pushes the multi-arch version to GHCR).
docker-build:
	docker build --build-arg VERSION=$(VERSION) -t bifrost:$(VERSION) .

# ── quality ───────────────────────────────────────────────────────────────────

test:
	go test ./...

lint:
	golangci-lint run
