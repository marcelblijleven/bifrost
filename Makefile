.PHONY: build run dev dev-backend dev-frontend test lint docker-up docker-down air-install

AIR := $(shell go env GOPATH)/bin/air

build:
	go build -o bin/bifrost ./cmd/bifrost

build-cli:
	go build -o bin/bifrost-cli ./cmd/bifrost-cli

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

# ── quality ───────────────────────────────────────────────────────────────────

test:
	go test ./...

lint:
	golangci-lint run
