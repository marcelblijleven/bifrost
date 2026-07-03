.PHONY: build build-cli run dev dev-backend dev-frontend test lint docker-up docker-down docker-build air-install

AIR := $(shell go env GOPATH)/bin/air
VERSION ?= dev
LDFLAGS := -X main.version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/bifrost ./cmd/bifrost

build-cli:
	go build -ldflags "$(LDFLAGS)" -o bin/bifrost-cli ./cmd/bifrost-cli

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

# Build both production images locally (the release workflow builds and
# pushes the multi-arch versions to GHCR).
docker-build:
	docker build --build-arg VERSION=$(VERSION) -t bifrost:$(VERSION) .
	docker build -t bifrost-web:$(VERSION) frontend

# ── quality ───────────────────────────────────────────────────────────────────

test:
	go test ./...

lint:
	golangci-lint run
