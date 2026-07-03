<script lang="ts">
  import { enhance } from '$app/forms'
  import type { PageData, ActionData } from './$types'
  import PipelineStepBuilder, { validateSteps, type Step } from '$lib/components/PipelineStepBuilder.svelte'
  import WebhookSecretInput from '$lib/components/WebhookSecretInput.svelte'
  import SkipConditionsFields from '$lib/components/SkipConditionsFields.svelte'
  import NotificationFields from '$lib/components/NotificationFields.svelte'
  import TriggerFields from '$lib/components/TriggerFields.svelte'

  let { data, form }: { data: PageData; form: ActionData } = $props()

  let steps = $state<Step[]>([])
  let validationError = $state('')
  let triggerType = $state<'push' | 'tag'>('push')
  // The pushed tag provides the version for tag-triggered apps, so semver and
  // tag steps are excluded and steps requiring semver are satisfied by it.
  const excludeTypes = $derived(triggerType === 'tag' ? ['semver', 'tag'] : [])
  const satisfiedRequires = $derived(triggerType === 'tag' ? ['semver'] : [])

  // When the providers request fails we can't tell what is configured, so
  // leave every option enabled rather than blocking the form.
  const configuredProviders = $derived(data.providers ?? [])
  function isConfigured(p: string): boolean {
    return configuredProviders.length === 0 || configuredProviders.includes(p)
  }
</script>

<svelte:head><title>New application - Bifrost</title></svelte:head>

<div class="p-4 sm:p-8 max-w-2xl">
  <div class="mb-6">
    <div class="mb-4 flex items-center gap-2 text-sm text-zinc-400 dark:text-zinc-500">
      <a href="/applications" class="hover:text-zinc-700 dark:hover:text-zinc-300 transition-colors">Applications</a>
      <span>/</span>
      <span class="text-zinc-700 dark:text-zinc-300">New</span>
    </div>
    <h1 class="text-xl font-semibold text-zinc-900 dark:text-zinc-100">New application</h1>
  </div>

  {#if form?.error}
    <div class="mb-6 rounded-md bg-red-500/10 border border-red-500/20 px-4 py-3 text-sm text-red-600 dark:text-red-400">
      {form.error}
    </div>
  {/if}

  {#if validationError}
    <div class="mb-6 rounded-md border border-amber-500/30 bg-amber-500/10 px-4 py-3 text-sm text-amber-700 dark:text-amber-400">
      {validationError}
    </div>
  {/if}

  <form method="POST" class="space-y-6" use:enhance={({ cancel }) => {
    const err = validateSteps(steps)
    if (err) {
      validationError = err
      cancel()
      return
    }
    validationError = ''
  }}>
    <!-- Basic info -->
    <div class="rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 space-y-4">
      <h2 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Repository</h2>

      <div class="grid gap-4 sm:grid-cols-2">
        <div class="col-span-2">
          <label class="block text-xs font-medium text-zinc-500 dark:text-zinc-400 mb-1" for="name">Name</label>
          <input
            id="name" name="name" type="text" required
            value={form?.values?.name ?? ''}
            class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 focus:outline-none focus:ring-2 focus:ring-brand-500"
            placeholder="my-service"
          />
        </div>

        <div>
          <label class="block text-xs font-medium text-zinc-500 dark:text-zinc-400 mb-1" for="provider">Provider</label>
          <select
            id="provider" name="provider"
            class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-brand-500"
          >
            <option value="github" disabled={!isConfigured('github')}>GitHub{isConfigured('github') ? '' : ' (not configured)'}</option>
            <option value="gitea" disabled={!isConfigured('gitea')}>Gitea{isConfigured('gitea') ? '' : ' (not configured)'}</option>
            <option value="forgejo" disabled={!isConfigured('forgejo')}>Forgejo{isConfigured('forgejo') ? '' : ' (not configured)'}</option>
          </select>
          {#if configuredProviders.length > 0 && configuredProviders.length < 3}
            <p class="mt-1 text-xs text-zinc-400 dark:text-zinc-500">Greyed-out providers are not configured on this server.</p>
          {/if}
        </div>

        <div>
          <label class="block text-xs font-medium text-zinc-500 dark:text-zinc-400 mb-1" for="branch">Branch</label>
          <input
            id="branch" name="branch" type="text"
            value={form?.values?.branch ?? 'main'}
            class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 focus:outline-none focus:ring-2 focus:ring-brand-500"
            placeholder="main"
          />
        </div>

        <div>
          <label class="block text-xs font-medium text-zinc-500 dark:text-zinc-400 mb-1" for="owner">Owner</label>
          <input
            id="owner" name="owner" type="text" required
            value={form?.values?.owner ?? ''}
            class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 focus:outline-none focus:ring-2 focus:ring-brand-500"
            placeholder="my-org"
          />
        </div>

        <div>
          <label class="block text-xs font-medium text-zinc-500 dark:text-zinc-400 mb-1" for="repo">Repository</label>
          <input
            id="repo" name="repo" type="text" required
            value={form?.values?.repo ?? ''}
            class="w-full rounded-md border border-zinc-300 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800 px-3 py-2 text-sm text-zinc-900 dark:text-white placeholder-zinc-400 dark:placeholder-zinc-500 focus:outline-none focus:ring-2 focus:ring-brand-500"
            placeholder="my-service"
          />
        </div>

        <div class="col-span-2">
          <WebhookSecretInput mode="create" />
        </div>

        <div class="col-span-2 border-t border-zinc-200 dark:border-zinc-800 pt-4">
          <div class="flex items-start justify-between gap-4">
            <div>
              <p class="text-xs font-medium text-zinc-500 dark:text-zinc-400">Install webhook on provider</p>
              <p class="mt-0.5 text-xs text-zinc-400 dark:text-zinc-500">
                Save the application first, then use the Install webhook button on the edit page.
              </p>
            </div>
            <button
              type="button"
              disabled
              title="Save the application first to install the webhook"
              class="shrink-0 rounded-md border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900 px-3 py-2 text-xs font-medium text-zinc-400 dark:text-zinc-600 cursor-not-allowed"
            >
              Install / Update webhook
            </button>
          </div>
        </div>
      </div>
    </div>

    <TriggerFields bind:triggerType />

    <!-- Pipeline steps -->
    <div class="rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-5 space-y-4">
      <h2 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Pipeline steps</h2>
      <PipelineStepBuilder bind:steps {excludeTypes} {satisfiedRequires} />
    </div>

    <SkipConditionsFields />

    <NotificationFields />

    <div class="flex gap-3">
      <button
        type="submit"
        class="rounded-md bg-brand-600 px-4 py-2 text-sm font-semibold text-white hover:bg-brand-500 transition-colors"
      >
        Create application
      </button>
      <a
        href="/applications"
        class="rounded-md border border-zinc-300 dark:border-zinc-700 px-4 py-2 text-sm font-medium text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-800 hover:text-zinc-900 dark:hover:text-white transition-colors"
      >
        Cancel
      </a>
    </div>
  </form>
</div>
