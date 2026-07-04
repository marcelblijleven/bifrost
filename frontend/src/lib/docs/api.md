# API reference

All endpoints are served under the `/api` prefix (e.g. `https://bifrost.example.com/api/auth/login`); the paths below omit it. Webhooks (`/webhooks/<provider>`), health (`/healthz`), and metrics (`/metrics`) stay at the server root. Most endpoints require a JWT bearer token obtained from the login endpoint.

## Authentication

### POST /auth/login

```json
{ "email": "admin@example.com", "password": "secret" }
```

Returns `{ "token": "<jwt>" }` and also sets the same JWT as an httpOnly session cookie (used by the web UI). API clients include the token in subsequent requests:

```
Authorization: Bearer <token>
```

Failed attempts are rate-limited per email and per source IP: 5 failures within 5 minutes locks that email/IP out for 5 minutes. A locked-out request gets `429 Too Many Requests` with a `Retry-After` header (seconds).

### POST /auth/logout

Clears the session cookie. The JWT itself remains valid until it expires. No authentication required.

### PUT /auth/password

Change the caller's own password. Requires a JWT (not available to static-API-key requests).

```json
{ "current_password": "old", "new_password": "new-password-min-8-chars" }
```

### GET /auth/me

Returns the currently authenticated user.

---

## Setup

### GET /setup

Returns `{ "required": true }` if no users exist yet.

### POST /setup

Creates the initial admin user. Only works when no users exist.

```json
{ "email": "admin@example.com", "password": "secret" }
```

---

## Health and metrics

These endpoints are public (no authentication required).

### GET /healthz

Returns `{ "status": "ok" }`. Use for load balancer health checks.

### GET /metrics

Prometheus metrics in text format. Exposed metrics:

| Metric | Type | Description |
|---|---|---|
| `bifrost_pipeline_runs_total` | Counter | Total runs, labelled by `status` |
| `bifrost_pipeline_run_duration_seconds` | Histogram | Run duration, labelled by `status` |
| `bifrost_running_runs` | Gauge | Currently executing runs |

---

## Applications

### GET /applications

Returns an array of all applications the authenticated user has access to.
Each entry includes a `LastRun` field with the most recent pipeline run
(`null` when the application has never run).

### POST /applications

Create a new application.

```json
{
  "Name": "my-service",
  "Provider": "github",
  "Owner": "my-org",
  "Repo": "my-service",
  "Branch": "main",
  "WebhookSecret": "<generated secret>",
  "PipelineSteps": [
    { "type": "semver" },
    { "type": "changelog" },
    { "type": "tag" }
  ]
}
```

### GET /applications/:id

### PUT /applications/:id

### DELETE /applications/:id

### POST /applications/:id/webhook/install

Creates or updates a webhook on the application's repository at the provider. Requires `PUBLIC_URL` to be set in the server environment so the provider knows where to send events.

The webhook is registered with `push` and `workflow_run` events and `application/json` content type. If a webhook already exists for the Bifrost URL it is updated in-place (useful when rotating the webhook secret).

Returns:

```json
{ "webhook_url": "https://bifrost.example.com/webhooks/github" }
```

---

## Pipeline runs

### GET /applications/:id/runs

Query params:

| Param | Default | Description |
|---|---|---|
| `limit` | `20` | Max results |
| `offset` | `0` | Pagination offset |
| `status` | - | Filter by status: `pending`, `running`, `success`, `failed`, `cancelled`, `superseded` |
| `branch` | - | Filter by branch name |

### GET /runs/:id

Returns the run record.

### GET /runs/:id/steps

Returns an array of step results for the run.

### GET /runs/:id/events

Server-Sent Events stream. Emits an event whenever a step result changes. Connect with:

```bash
curl -N -H "Authorization: Bearer <token>" \
  https://<host>/runs/<id>/events
```

### POST /runs/:id/steps/:stepIndex/retry

Retries the pipeline from the given step index. All step results from that index onwards are cleared and the run is re-queued.

### POST /runs/:id/cancel

Cancels a pending or running run.

---

## Approvals

### GET /runs/:id/approvals

Returns an array of approval requests for the run.

### POST /runs/:id/approvals/:stepIndex/approve

Approves the approval gate at the given step index.

### POST /runs/:id/approvals/:stepIndex/reject

Rejects the approval gate, which fails the run.

---

## Users

### GET /users

Returns all users. Requires a valid JWT.

### POST /users

Creates a user. Can use either a JWT **or** the static `API_KEY` in the `Authorization: Bearer` header.

```json
{ "email": "user@example.com", "password": "secret" }
```

### DELETE /users/:id

Admin-only. Refuses to delete the caller's own account or the last remaining admin.

### POST /users/:id/password

Admin-only password reset — sets a user's password without knowing their current one.

```json
{ "password": "new-password-min-8-chars" }
```

### PUT /users/:id/admin

Admin-only. Grants or revokes admin rights. Refuses to demote the last remaining admin.

```json
{ "is_admin": true }
```

---

## Providers

### GET /providers

Returns the git providers configured on this server, so clients can avoid
offering providers that would fail at webhook time.

```json
{ "providers": ["github", "gitea"] }
```

---

## Groups

Groups control which users can see which applications.

### GET /groups

### POST /groups

```json
{ "name": "platform-team" }
```

### PUT /groups/:id

Rename a group.

```json
{ "name": "new-name" }
```

### DELETE /groups/:id

### GET /groups/:id/members

### PUT /groups/:id/members/:userId

Add a user to the group.

### DELETE /groups/:id/members/:userId

Remove a user from the group.

### GET /applications/:id/groups

List groups that have access to the application.

### PUT /applications/:id/groups/:groupId

Grant a group access to the application.

### DELETE /applications/:id/groups/:groupId

Revoke a group's access.

---

## Dashboard

### GET /dashboard

Returns aggregated stats: 30-day run counts, per-day buckets, recent runs, and pending actions (approval requests waiting for a human).

---

## Webhooks

### POST /webhooks/github

Receives GitHub push events. Must include a valid `X-Hub-Signature-256` header matching the webhook secret configured on the application.
