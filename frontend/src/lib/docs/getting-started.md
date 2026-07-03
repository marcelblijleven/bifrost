# Getting started

Bifrost is a self-hosted release orchestration tool. It watches repositories for push events, runs configurable pipelines (version bump, changelog, tagging, Actions dispatch, releases), and requires human approval at gates you define.

Supported Git hosting providers: **GitHub** (github.com, GitHub Enterprise Server, GitHub EU cloud), **Gitea**, and **Forgejo**.

## Prerequisites

- [Go](https://go.dev/) 1.23+
- [Docker](https://www.docker.com/) and Docker Compose (for Postgres)
- [Node.js](https://nodejs.org/) + [pnpm](https://pnpm.io/) (for the frontend)
- A GitHub personal access token (classic) with `repo` and `workflow` scopes, **or** a GitHub App with those permissions

## Development (hot reload)

The quickest way to start developing is:

```bash
cp .env.example .env   # fill in your values once
make dev
```

This single command:

1. Installs [air](https://github.com/air-verse/air) if not already present
2. Starts the Postgres container via Docker Compose
3. Launches the Go backend with hot reload (rebuilds on every `.go` file change)
4. Launches the Vite dev server for the frontend

Backend listens on `:8080`, frontend on `:5173`. Press **Ctrl+C** to stop both.

To run each separately:

```bash
make dev-backend    # Go + air only
make dev-frontend   # Vite only (cd frontend && pnpm dev)
```

## Production setup

```bash
make docker-up        # start Postgres
cp .env.example .env  # configure environment
make run              # build and run the server
```

Or build a binary:

```bash
make build
./bin/bifrost
```

## Environment variables

| Variable | Required | Description |
|---|---|---|
| `DATABASE_URL` | Yes | PostgreSQL connection string, e.g. `postgres://bifrost:secret@localhost:5432/bifrost?sslmode=disable` |
| `JWT_SECRET` | Yes | Random secret for signing session tokens - `openssl rand -hex 32` |
| `API_KEY` | Yes | Static key for management API calls (creating users, etc.) |
| `HTTP_ADDR` | No | Listen address (default `:8080`) |
| `PUBLIC_URL` | No | Externally reachable URL of this Bifrost instance, e.g. `https://bifrost.example.com`. Required to use the **Install webhook** button in the UI. |
| `GITHUB_TOKEN` | * | Personal access token - required if not using a GitHub App |
| `GITHUB_APP_ID` | * | GitHub App ID - takes priority over `GITHUB_TOKEN` when set |
| `GITHUB_INSTALLATION_ID` | * | GitHub App installation ID |
| `GITHUB_PRIVATE_KEY` | * | GitHub App private key (PEM, newlines as `\n`) |
| `GITHUB_BASE_URL` | No | API base URL for GitHub Enterprise. GHES: `https://github.company.com/api/v3/`. EU cloud: `https://api.eu.github.com/`. Leave empty for github.com. |
| `GITHUB_UPLOAD_URL` | No | Upload URL for GitHub Enterprise. Derived from `GITHUB_BASE_URL` for GHES. Required for EU cloud: `https://uploads.eu.github.com/`. |
| `GITEA_URL` | * | Gitea instance URL, e.g. `https://gitea.example.com` |
| `GITEA_TOKEN` | * | Gitea personal access token (`repo` + `issue` scopes) |
| `FORGEJO_URL` | * | Forgejo instance URL, e.g. `https://codeberg.org` |
| `FORGEJO_TOKEN` | * | Forgejo personal access token |

At least one provider must be configured. Multiple providers can be active simultaneously.

Generate `JWT_SECRET` and `API_KEY`:

```bash
openssl rand -hex 32   # run twice, once for each
```

## First run

On first launch, Bifrost checks whether any users exist. If not, it redirects to `/setup` where you create the initial admin account.

After that, navigate to **Applications** and click **New application** to register your first repository.

## CLI tool

Bifrost ships with a CLI binary for scripting and CI use:

```bash
make build-cli
./bin/bifrost-cli login          # saves credentials to ~/.config/bifrost/config.json
./bin/bifrost-cli apps list
./bin/bifrost-cli runs watch <run-id>
```

Every command respects `BIFROST_URL` and `BIFROST_TOKEN` environment variables, so no interactive login is needed in CI:

```bash
BIFROST_URL=https://bifrost.internal \
BIFROST_TOKEN=$TOKEN \
  ./bin/bifrost-cli runs watch <run-id>
```

Run `./bin/bifrost-cli --help` to see all commands.

## Production deployment

For running Bifrost in production (LXC container, systemd services, nginx with TLS, PostgreSQL, Prometheus), see the [Production deployment](deployment) guide.

## Health check and metrics

| Endpoint | Description |
|---|---|
| `GET /healthz` | Returns `{"status":"ok"}` - useful for load balancer health checks |
| `GET /metrics` | Prometheus metrics (run counts, duration histogram, active runs gauge) |
