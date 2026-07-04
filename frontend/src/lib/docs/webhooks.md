# Webhooks

Bifrost listens for push events from GitHub, Gitea, and Forgejo.

Several applications may be registered on the same repository (monorepo): each webhook delivery fans out to every application registered for that repository, and each application decides independently whether to start a run based on its trigger type, branch, and skip conditions. Use `paths_include` to route pushes to the right application and a **tag prefix** to keep their release tags apart (e.g. `frontend-v1.2.3`).

## Automatic installation (recommended)

If `PUBLIC_URL` is set in the server environment, Bifrost can install the webhook on the provider for you:

1. Open an application and go to **Edit**.
2. Fill in (or confirm) the owner, repository, and webhook secret fields.
3. Click **Save changes**, then click **Install / Update webhook**.

Bifrost will create the webhook if it does not exist, or update it in-place if one already points at the same URL. The webhook is configured with `push`, `create`, and `workflow_run` events and `application/json` content type (`create` carries tag creation on Gitea/Forgejo; GitHub reports tag pushes through `push`).

If you rotate the webhook secret, save the application first (so the new secret is stored), then click **Install / Update webhook** again to push the new secret to the provider.

## Manual setup

### GitHub

In your repository go to Settings → Webhooks → Add webhook.

- Payload URL: `https://bifrost.example.com/webhooks/github`
- Content type: `application/json`
- Secret: paste the webhook secret from the Bifrost application
- Which events: **Send me everything** (Bifrost only acts on `push` and `workflow_run`)

### Gitea

In your repository go to Settings → Webhooks → Add webhook → Gitea.

- Target URL: `https://bifrost.example.com/webhooks/gitea`
- Content type: `application/json`
- Secret: paste the webhook secret from the Bifrost application
- Trigger on: Push events (add **Create** events for tag-triggered applications)

### Forgejo

In your repository go to Settings → Webhooks → Add webhook → Forgejo.

- Target URL: `https://bifrost.example.com/webhooks/forgejo`
- Content type: `application/json`
- Secret: paste the webhook secret from the Bifrost application
- Trigger on: Push events (add **Create** events for tag-triggered applications)

Forgejo also accepts webhooks at `/webhooks/gitea` but `/webhooks/forgejo` is recommended.

---

All providers validate payloads with HMAC-SHA256 and reject invalid signatures with 401.

## Run queuing

Only one run per application executes at a time. New runs are queued while a run is active. If a superseding run starts while a previous one awaits approval, the pending approval is cancelled and the older run is marked superseded.

## Commit lineage and force pushes

Bifrost tracks the head of each application's release branch. Every push must chain onto the last head Bifrost saw (the webhook's `before` field). This works for all merge styles — direct pushes, merge commits, squash merges, and rebase merges all fast-forward the branch.

- **Missed webhooks** (e.g. Bifrost was down): if the pushed head still fast-forwards the last known head, Bifrost backfills a run for each commit whose webhook was missed, oldest first, so none are skipped. These runs execute in commit order ahead of the pushed head's own run. Set `skip_backfill` in the application's skip conditions to disable this and sync straight to the pushed head instead (the missed commits are then covered by that head's run).
- **Force push / history rewrite**: the application is **blocked**. New runs are paused and queued runs are cancelled, because releases could otherwise reference commits that no longer exist. The application page shows the reason plus recovery steps; after verifying the rewrite was intentional, click **Accept current head** (optionally **Accept & run pipeline**) to re-baseline and resume. Via the API:

```bash
curl -X POST -H "Authorization: Bearer TOKEN" \
  -d '{"trigger_run": true}' \
  https://bifrost.example.com/applications/APP_ID/head/accept
```

Deleting the tracked branch also blocks the application.

## Tag triggers

An application can listen to **tag pushes** instead of branch pushes (never both). Set the trigger type to *Tag push* and configure a tag pattern (e.g. `v*` or `frontend-v*`).

- The pushed tag **is** the release: it provides the version directly, so the pipeline may not contain `semver` or `tag` steps. Steps that need the version (`changelog`, `create_release`, `dispatch_workflow`, ...) receive the pushed tag.
- The tagged commit must be **reachable from the application's branch**. A tag on an unmerged feature branch is recorded as a skipped run and reported via the failure notification URL; once the commit is merged, pushing the tag again (or redelivering the webhook) starts the run.
- Each tag triggers **at most one run**. Duplicate deliveries are ignored. Deleting a tag and recreating it at a different commit is treated like a force push: the application is blocked until a human accepts the current branch head.


```
GET /applications/APP_ID/runs?status=failed&branch=main&limit=20&offset=0
```

## Live run progress

Pipeline runs stream progress via Server-Sent Events. The run detail page connects automatically. Subscribe directly:

```bash
curl -N -H "Authorization: Bearer TOKEN" \
  https://bifrost.example.com/runs/RUN_ID/events
```
