<script lang="ts">
  import type { PageData } from './$types'
  import { goto, invalidateAll } from '$app/navigation'
  import { api } from '$lib/api'
  import { parseNotifications, parseSkipConditions, parseTrigger } from '$lib/appForm'
  import PipelineStepBuilder, { validateSteps, newStepId, type Step } from '$lib/components/PipelineStepBuilder.svelte'
  import WebhookSecretInput from '$lib/components/WebhookSecretInput.svelte'
  import SkipConditionsFields from '$lib/components/SkipConditionsFields.svelte'
  import NotificationFields from '$lib/components/NotificationFields.svelte'
  import TriggerFields from '$lib/components/TriggerFields.svelte'

  let { data }: { data: PageData } = $props()

  const app = data.application
  let assignedGroups = $derived(data.assignedGroups ?? [])
  let allGroups = $derived(data.allGroups ?? [])
  let unassignedGroups = $derived(allGroups.filter(g => !assignedGroups.some(a => a.ID === g.ID)))

  let steps = $state<Step[]>(
    (app.PipelineSteps ?? []).map(s => ({
      id: newStepId(),
      type: s.type,
      config: (s.config as Record<string, unknown>) ?? {},
    }))
  )

  let validationError = $state('')
  let triggerType = $state<'push' | 'tag'>(app.TriggerType === 'tag' ? 'tag' : 'push')
  // For tag triggers the pushed tag provides the version.
  const excludeTypes = $derived(triggerType === 'tag' ? ['semver', 'tag'] : [])
  const satisfiedRequires = $derived(triggerType === 'tag' ? ['semver'] : [])

  const configuredProviders = $derived(data.providers ?? [])
  function isConfigured(p: string): boolean {
    // The saved provider stays selectable even if unconfigured, so the form
    // can be submitted without silently switching providers.
    return p === app.Provider || configuredProviders.length === 0 || configuredProviders.includes(p)
  }

  let formEl = $state<HTMLFormElement | null>(null)
  let error = $state('')
  let webhookInstalled = $state(false)
  let webhookURL = $state('')
  let webhookError = $state('')
  let groupError = $state('')

  async function update(e: SubmitEvent) {
    e.preventDefault()
    const stepErr = validateSteps(steps)
    if (stepErr) {
      validationError = stepErr
      return
    }
    validationError = ''
    error = ''

    const data = new FormData(e.currentTarget as HTMLFormElement)
    const name = ((data.get('name') as string) ?? '').trim()
    const provider = ((data.get('provider') as string) ?? '').trim()
    const owner = ((data.get('owner') as string) ?? '').trim()
    const repo = ((data.get('repo') as string) ?? '').trim()
    const branch = ((data.get('branch') as string) ?? '').trim() || 'main'
    const webhookSecret = ((data.get('webhook_secret') as string) ?? '').trim()
    const stepsRaw = ((data.get('steps') as string) ?? '').trim()

    if (!name || !provider || !owner || !repo) {
      error = 'Name, provider, owner, repo, and branch are required'
      return
    }

    let pipelineSteps: { type: string; config?: Record<string, unknown> }[] = []
    if (stepsRaw) {
      try {
        pipelineSteps = JSON.parse(stepsRaw)
      } catch {
        error = 'Pipeline steps must be valid JSON'
        return
      }
    }

    try {
      await api.applications.update(app.ID, {
        Name: name,
        Provider: provider,
        Owner: owner,
        Repo: repo,
        Branch: branch,
        WebhookSecret: webhookSecret,
        PipelineSteps: pipelineSteps,
        Notifications: parseNotifications(data),
        SkipConditions: parseSkipConditions(data),
        ...parseTrigger(data),
      })
    } catch (err: unknown) {
      error = err instanceof Error ? err.message : 'Failed to update application'
      return
    }
    await goto(`/applications/${app.ID}`)
  }

  async function deleteApp() {
    if (!confirm(`Delete "${app.Name}"? This cannot be undone.`)) return
    try {
      await api.applications.delete(app.ID)
    } catch {
      error = 'Failed to delete application'
      return
    }
    await goto('/applications')
  }

  async function grantGroup() {
    groupError = ''
    const groupId = (new FormData(formEl!).get('addGroupId') as string)?.trim()
    if (!groupId) {
      groupError = 'Group is required'
      return
    }
    try {
      await api.applications.grantGroup(app.ID, groupId)
      await invalidateAll()
    } catch (err: unknown) {
      groupError = err instanceof Error ? err.message : 'Failed to grant access'
    }
  }

  async function revokeGroup(groupId: string) {
    groupError = ''
    try {
      await api.applications.revokeGroup(app.ID, groupId)
      await invalidateAll()
    } catch (err: unknown) {
      groupError = err instanceof Error ? err.message : 'Failed to revoke access'
    }
  }

  async function installWebhook() {
    webhookInstalled = false
    webhookError = ''
    try {
      const result = await api.applications.installWebhook(app.ID)
      webhookInstalled = true
      webhookURL = result.webhook_url
    } catch (err: unknown) {
      webhookError = err instanceof Error ? err.message : 'Failed to install webhook'
    }
  }
</script>

<svelte:head><title>Edit {app.Name} - Bifrost</title></svelte:head>

<div class="p-4 sm:p-8 max-w-2xl">
  <div class="mb-6">
    <div class="mb-4 flex items-center gap-2 text-sm text-zinc-400 dark:text-zinc-500">
      <a href="/applications" class="hover:text-zinc-700 dark:hover:text-zinc-300 transition-colors">Applications</a>
      <span>/</span>
      <a href="/applications/{app.ID}" class="hover:text-zinc-700 dark:hover:text-zinc-300 transition-colors">{app.Name}</a>
      <span>/</span>
      <span class="text-zinc-700 dark:text-zinc-300">Edit</span>
    </div>
    <h1 class="text-xl font-semibold text-zinc-900 dark:text-zinc-100">Edit application</h1>
  </div>

  {#if error}
    <div class="mb-6 rounded-md bg-red-500/10 border border-red-500/20 px-4 py-3 text-sm text-red-600 dark:text-red-400">
      {error}
    </div>
  {/if}

  {#if validationError}
    <div class="mb-6 rounded-md border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-700 dark:text-amber-400">
      {validationError}
    </div>
  {/if}

  <form bind:this={formEl} onsubmit={update} class="space-y-6">
    <div class="rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 space-y-4">
      <h2 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Repository</h2>

      <div class="grid grid-cols-2 gap-4">
        <div class="col-span-2">
          <label class="block text-xs font-medium text-zinc-500 dark:text-zinc-400 mb-1" for="name">Name</label>
          <input
            id="name" name="name" type="text" required
            value={app.Name}
            class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 focus:outline-none focus:ring-2 focus:ring-brand-500"
          />
        </div>

        <div>
          <label class="block text-xs font-medium text-zinc-500 dark:text-zinc-400 mb-1" for="provider">Provider</label>
          <select
            id="provider" name="provider"
            class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-brand-500"
          >
            <option value="github" selected={app.Provider === 'github'} disabled={!isConfigured('github')}>GitHub{isConfigured('github') ? '' : ' (not configured)'}</option>
            <option value="gitea" selected={app.Provider === 'gitea'} disabled={!isConfigured('gitea')}>Gitea{isConfigured('gitea') ? '' : ' (not configured)'}</option>
            <option value="forgejo" selected={app.Provider === 'forgejo'} disabled={!isConfigured('forgejo')}>Forgejo{isConfigured('forgejo') ? '' : ' (not configured)'}</option>
          </select>
        </div>

        <div>
          <label class="block text-xs font-medium text-zinc-500 dark:text-zinc-400 mb-1" for="branch">Branch</label>
          <input
            id="branch" name="branch" type="text"
            value={app.Branch}
            class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 focus:outline-none focus:ring-2 focus:ring-brand-500"
          />
        </div>

        <div>
          <label class="block text-xs font-medium text-zinc-500 dark:text-zinc-400 mb-1" for="owner">Owner</label>
          <input
            id="owner" name="owner" type="text" required
            value={app.Owner}
            class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 focus:outline-none focus:ring-2 focus:ring-brand-500"
          />
        </div>

        <div>
          <label class="block text-xs font-medium text-zinc-500 dark:text-zinc-400 mb-1" for="repo">Repository</label>
          <input
            id="repo" name="repo" type="text" required
            value={app.Repo}
            class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 focus:outline-none focus:ring-2 focus:ring-brand-500"
          />
        </div>

        <div class="col-span-2">
          <WebhookSecretInput mode="edit" />
        </div>

        <div class="col-span-2 border-t border-zinc-200 dark:border-zinc-800 pt-4">
          <div class="flex items-start justify-between gap-4">
            <div>
              <p class="text-xs font-medium text-zinc-500 dark:text-zinc-400">Install webhook on provider</p>
              <p class="mt-0.5 text-xs text-zinc-400 dark:text-zinc-500">
                Creates or updates the webhook on {app.Provider} using the saved secret.
                Save any changes above before installing.
              </p>
              {#if webhookInstalled}
                <p class="mt-1 text-xs text-green-500 dark:text-green-400">Webhook installed — <span class="font-mono">{webhookURL}</span></p>
              {/if}
              {#if webhookError}
                <p class="mt-1 text-xs text-red-500 dark:text-red-400">{webhookError}</p>
              {/if}
            </div>
            <button
              type="button"
              onclick={installWebhook}
              class="shrink-0 rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-800 px-3 py-2 text-xs font-medium text-zinc-700 dark:text-zinc-300 hover:border-brand-500 hover:text-brand-300 transition-colors"
            >
              Install / Update webhook
            </button>
          </div>
        </div>
      </div>
    </div>

    <TriggerFields bind:triggerType initialTagPattern={app.TagPattern} initialTagPrefix={app.TagPrefix} />

    <div class="rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 space-y-4">
      <h2 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Pipeline steps</h2>

      <PipelineStepBuilder bind:steps {excludeTypes} {satisfiedRequires} />
    </div>

    <SkipConditionsFields initial={app.SkipConditions} />

    <div class="rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 space-y-4">
      <h2 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Access groups</h2>
      <p class="text-xs text-zinc-400 dark:text-zinc-500">
        Groups that can view this application and its runs. An application with no groups assigned is visible to all users.
      </p>

      {#if groupError}
        <p class="text-xs text-red-500 dark:text-red-400">{groupError}</p>
      {/if}

      {#if assignedGroups.length === 0}
        <p class="text-xs text-zinc-400 dark:text-zinc-600 italic">No groups assigned — visible to all users.</p>
      {:else}
        <div class="space-y-2">
          {#each assignedGroups as group (group.ID)}
            <div class="flex items-center justify-between rounded-md border border-zinc-200 dark:border-zinc-800 bg-zinc-100/50 dark:bg-zinc-800/50 px-3 py-2">
              <span class="text-sm text-zinc-700 dark:text-zinc-300">{group.Name}</span>
              <button
                type="button"
                onclick={() => revokeGroup(group.ID)}
                class="text-xs text-zinc-400 dark:text-zinc-600 hover:text-red-400 transition-colors"
              >
                Remove
              </button>
            </div>
          {/each}
        </div>
      {/if}

      {#if unassignedGroups.length > 0}
        <div class="flex gap-2">
          <select
            name="addGroupId"
            class="flex-1 rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-brand-500"
          >
            <option value="" disabled selected>Select a group…</option>
            {#each unassignedGroups as group (group.ID)}
              <option value={group.ID}>{group.Name}</option>
            {/each}
          </select>
          <button
            type="button"
            onclick={grantGroup}
            class="shrink-0 rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-800 px-3 py-2 text-xs font-medium text-zinc-700 dark:text-zinc-300 hover:border-brand-500 hover:text-brand-300 transition-colors"
          >
            Add group
          </button>
        </div>
      {/if}
    </div>

    <NotificationFields initial={app.Notifications} />

    <div class="flex items-center justify-between gap-3">
      <div class="flex gap-3">
        <button
          type="submit"
          class="rounded-md bg-brand-600 px-4 py-2 text-sm font-semibold text-white hover:bg-brand-500 transition-colors"
        >
          Save changes
        </button>
        <a
          href="/applications/{app.ID}"
          class="rounded-md border border-zinc-300 dark:border-zinc-700 px-4 py-2 text-sm font-medium text-zinc-700 dark:text-zinc-300 hover:text-zinc-900 dark:hover:text-white transition-colors"
        >
          Cancel
        </a>
      </div>
    </div>
  </form>

  <div class="mt-8 rounded-lg border border-red-900/40 bg-red-50 dark:bg-red-950/20 p-5">
    <h2 class="mb-1 text-sm font-medium text-red-600 dark:text-red-400">Danger zone</h2>
    <p class="mb-4 text-xs text-zinc-400 dark:text-zinc-500">Deleting this application cannot be undone. All associated runs and step results will be lost.</p>
    <button
      type="button"
      onclick={deleteApp}
      class="rounded-md border border-red-700 bg-red-900/30 px-3 py-1.5 text-xs font-medium text-red-500 dark:text-red-400 hover:bg-red-900/60 hover:text-red-600 dark:hover:text-red-300 transition-colors"
    >
      Delete application
    </button>
  </div>
</div>
