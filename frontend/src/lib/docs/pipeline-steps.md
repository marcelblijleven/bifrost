# Pipeline steps

A pipeline is an ordered list of steps defined per application. Steps run sequentially; if any step fails the run stops and is marked **failed**.

## Skip conditions

Before any steps run, Bifrost evaluates the **skip conditions** configured on the application. A push matching `commit_patterns` is recorded as a **skipped** run; pushes filtered out by path conditions are only logged, not recorded: with several applications sharing one repository, path filters act as routing and most pushes don't concern a given application.

Configure in the **Edit application → Skip conditions** section, or via the API as `SkipConditions` on the application object.

| Field | Description |
|---|---|
| `commit_patterns` | Skip if the commit message contains any of these strings (e.g. `[skip ci]`) |
| `paths_ignore` | Skip if **all** changed files match at least one glob pattern (e.g. `docs/**`, `*.md`) |
| `paths_include` | Skip if **no** changed files match any pattern - use to run only for changes in specific paths |

Glob patterns support `*` (within a path segment), `**` (across segments), and `?`. Examples:

```
docs/**       matches anything under docs/
**/*.md       matches any .md file at any depth
*.md          matches .md files in the repository root only
src/**        matches anything under src/
```

`paths_ignore` and `paths_include` are evaluated against the union of added and modified files across all commits in the push. If no file information is available, path-based conditions are skipped and only `commit_patterns` applies.

When `paths_include` is set, the `changelog` step also scopes its commit list to those paths, so an application in a monorepo only lists its own changes.

---

Configure steps in the **Edit application** form as a JSON array:

```json
[
  { "type": "semver" },
  { "type": "changelog" },
  { "type": "approval", "config": { "message": "Ready to release?" } },
  { "type": "tag" },
  { "type": "create_release", "config": { "draft": false } },
  { "type": "dispatch_workflow", "config": { "workflow": "deploy.yml", "wait": true } },
  { "type": "notify", "config": { "url": "https://hooks.example.com/bifrost" } }
]
```

Each step exposes its outputs (tag, URL, conclusion, etc.) as key-value pairs on the run detail page. URL values are rendered as clickable links.

---

## semver

Determines the next semantic version by inspecting the latest Git tag on the repository and applying [Conventional Commits](https://www.conventionalcommits.org/) rules across every commit since that tag (not just the commit that triggered the push) - the highest-priority bump found wins.

| Field | Type | Default | Description |
|---|---|---|---|
| `v_prefix` | boolean | `true` | Whether to prepend `v` to the very first tag. Once a repo has existing tags their prefix style is mirrored automatically. |

**Version bump logic (highest bump across all commits since the latest tag wins):**

| Commit message | Bump |
|---|---|
| Contains `BREAKING CHANGE` or `!:` | major (minor if `major == 0`) |
| Starts with `feat` | minor |
| Anything else | patch |

If no valid semver tags exist yet, the first version is `v0.1.0` (or `0.1.0` when `v_prefix: false`). For subsequent tags the prefix follows the latest tag - repos using bare versions like `1.2.3` will continue to produce bare versions.

If the application has a **tag prefix** (monorepo), only tags carrying that prefix are considered and the computed tag carries it too: with prefix `frontend-`, versions run `frontend-v1.2.0`, `frontend-v1.2.1`, ... independently of other applications' tags in the same repository.

Tag-triggered applications cannot use this step: the pushed tag itself provides the version.

**Outputs:** `tag` (new version, e.g. `v1.3.0`), `previous` (previous tag)

---

## changelog

Generates a changelog entry since the last **released** run on the same branch - the most recent run that completed all of its steps successfully, not simply the latest Git tag. This matters if e.g. a previous run created a tag but a later step (such as `create_release`) failed: that run never counted as released, so its commits are still included in the next changelog. The baseline is that run's tag, or its commit SHA if no tag was created. If no run has ever been released, the changelog covers the full commit history up to the current commit.

- **GitHub** - when a previous tag is available, uses GitHub's native release-notes generation API (`generate-notes`) to produce the entry.
- **Gitea / Forgejo, or GitHub with no previous tag** - manually collects commit messages and groups them by Conventional Commits type (Features, Bug Fixes, Performance, Refactor, Documentation, Other).

The entry is stored in the pipeline context and used in two places: as the annotated **tag message** (by the `tag` step) and as the **release description** (by the `create_release` step). Nothing is committed to the repository, so protected branches are not a concern.

No configuration fields.

---

## approval

Pauses the pipeline and creates an approval request visible on the dashboard and the run detail page. The pipeline resumes when a user approves or rejects the request.

| Field | Type | Default | Description |
|---|---|---|---|
| `message` | string | `Approve release?` | Prompt shown to the approver |
| `timeout_hours` | number | `24` | Hours before the request automatically expires and the run fails |

If a newer run supersedes this one while it waits, the approval is cancelled and the run is marked **superseded**.

Set an **On approval requested webhook URL** in the application's **Edit** page to get a JSON POST as soon as the request is created, instead of relying on the dashboard.

---

## tag

Creates and pushes a Git tag for the version computed by the `semver` step.

No configuration fields.

**Outputs:** none (the tag was already written to the run by `semver`)

---

## create_release

Creates a release (GitHub Release, Gitea Release, or Forgejo Release) for the computed tag. Requires the `semver` and `tag` steps to have run first.

| Field | Type | Default | Description |
|---|---|---|---|
| `draft` | boolean | `false` | Publish as a draft release |
| `prerelease` | boolean | `false` | Mark as a pre-release |

**Outputs:** `tag`, `url` (release URL), `draft`

---

## dispatch_workflow

Triggers a CI workflow via the provider's workflow dispatch API, then optionally polls until it completes.

- **GitHub** - uses the GitHub Actions workflow dispatch API. The run ID is returned directly.
- **Gitea / Forgejo** - requires Actions to be enabled on the instance (Gitea 1.21+ / Forgejo equivalent). After dispatch, Bifrost polls for up to 30 seconds to locate the new run by creation time. Requires the token to have `Actions` read permission.

| Field | Type | Default | Description |
|---|---|---|---|
| `workflow` | string | **required** | Workflow file name, e.g. `deploy.yml` |
| `wait` | boolean | `false` | Block and poll until the workflow run completes |
| `timeout_minutes` | number | `30` | Timeout for completion polling (only applies when `wait: true`) |
| `require_approval` | boolean | `false` | Show an approval gate before dispatching |
| `approval_message` | string | auto | Message shown on the approval gate |
| `approval_timeout_hours` | number | `24` | Hours before the approval gate expires |

When `wait: false` the step completes immediately after dispatch.

**Outputs (always):** `workflow`, `ref`, `run_id`, `url`

**Outputs (when `wait: true`):** additionally `conclusion` (`success`, `failure`, `cancelled`, etc.)

The run fails if the workflow completes with a conclusion other than `success` or `skipped`. The pipeline never continues past a failed dispatch on its own. If a human decides the failure is acceptable (e.g. the deploy succeeded and only a flaky verification step failed), they can use **Override & continue** on the failed step in the run detail page: a reason is mandatory, and both the reason and the overriding user are recorded on the step for the audit trail. The run then resumes from the next step; the original failure message is preserved.

---

## notify

Sends an HTTP POST to a webhook URL with a JSON payload describing the completed pipeline run. Works with any service that accepts a JSON POST (custom services, n8n, Make, Zapier, etc.). A non-2xx response is logged but does **not** fail the pipeline.

> **Note:** Slack and Discord webhooks expect their own payload format and will not accept this payload directly. Use an intermediary (e.g. a cloud function or automation tool) to translate the Bifrost payload into the format they expect.

| Field | Type | Default | Description |
|---|---|---|---|
| `url` | string | **required** | Webhook URL to POST to |
| `headers` | object | `{}` | Additional HTTP headers, e.g. `{"Authorization": "Bearer token"}` |

The payload includes: `run_id`, `application_id`, `status`, `tag`, `branch`, `commit_sha`, `triggered_by`, `started_at`, `completed_at`.
